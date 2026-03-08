package settlement

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/apex-checkout/mobile-check-deposit/internal/deposits"
	"github.com/apex-checkout/mobile-check-deposit/internal/funding"
	"github.com/apex-checkout/mobile-check-deposit/internal/ledger"
	"github.com/apex-checkout/mobile-check-deposit/internal/transfers"
	vendorclient "github.com/apex-checkout/mobile-check-deposit/internal/vendorsvc/client"
	"github.com/apex-checkout/mobile-check-deposit/internal/vendorsvc/model"
	_ "github.com/mattn/go-sqlite3"
)

func migrationsDir() string {
	_, f, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(f), "..", "..", "db", "migrations")
}

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file::memory:?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	mDir := migrationsDir()
	for _, name := range []string{"001_initial.sql", "002_seed.sql"} {
		content, err := os.ReadFile(filepath.Join(mDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(content)); err != nil {
			t.Fatalf("migration %s: %v", name, err)
		}
	}
	return db
}

func cleanPassStub(t *testing.T) *httptest.Server {
	t.Helper()
	var counter int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		counter++
		resp := model.AnalyzeResponse{
			VendorTransactionID:  fmt.Sprintf("vtx-test-%d", counter),
			Decision:             "PASS",
			IQAStatus:            "PASS",
			ManualReviewRequired: false,
			DuplicateDetected:    false,
			AmountMatches:        true,
			RiskScore:            10,
			MICR: &model.MICRResult{
				Routing: "111000025", Account: "123456789", Serial: fmt.Sprintf("%04d", counter), Confidence: 0.99,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func ptr(s string) *string { return &s }

func mustSubmitDeposit(t *testing.T, db *sql.DB, vendorURL, imageDir, externalAcctID string, amount int64) *deposits.SubmitResult {
	t.Helper()
	depSvc := &deposits.DepositService{
		DB:           db,
		TransferSvc:  &transfers.TransferService{},
		FundingSvc:   &funding.FundingService{DB: db},
		LedgerSvc:    &ledger.LedgerService{},
		VendorClient: vendorclient.New(vendorURL),
		ImageDir:     imageDir,
	}
	res, err := depSvc.SubmitDeposit(context.Background(), externalAcctID, amount,
		bytes.NewReader([]byte("front")), bytes.NewReader([]byte("back")), ptr("clean_pass"))
	if err != nil {
		t.Fatalf("submit deposit: %v", err)
	}
	if res.State != transfers.StateFundsPosted {
		t.Fatalf("deposit state = %s, want FundsPosted", res.State)
	}
	return res
}

func TestSettlementService_GenerateAndAcknowledgeBatch(t *testing.T) {
	db := newTestDB(t)
	vendor := cleanPassStub(t)
	imageDir := t.TempDir()
	outputDir := t.TempDir()

	// Submit two deposits
	r1 := mustSubmitDeposit(t, db, vendor.URL, imageDir, "INV-1001", 10000)
	r2 := mustSubmitDeposit(t, db, vendor.URL, imageDir, "INV-1002", 20000)

	// Compute business date the same way deposit service does
	loc, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatal(err)
	}
	bizDate := time.Now().UTC().In(loc).Format("2006-01-02")

	svc := &SettlementService{
		DB:          db,
		OutputPath:  outputDir,
		TransferSvc: &transfers.TransferService{},
	}

	batch, err := svc.GenerateBatch(context.Background(), bizDate)
	if err != nil {
		t.Fatalf("GenerateBatch: %v", err)
	}

	if batch.TotalItems != 2 {
		t.Errorf("batch total_items = %d, want 2", batch.TotalItems)
	}
	if batch.TotalAmountCents != 30000 {
		t.Errorf("batch total_amount_cents = %d, want 30000", batch.TotalAmountCents)
	}
	if batch.Status != "GENERATED" {
		t.Errorf("batch status = %s, want GENERATED", batch.Status)
	}
	if batch.FilePath == nil {
		t.Fatal("batch file_path is nil")
	}

	// Verify settlement file contents
	data, err := os.ReadFile(*batch.FilePath)
	if err != nil {
		t.Fatalf("read settlement file: %v", err)
	}
	var sf settlementFile
	if err := json.Unmarshal(data, &sf); err != nil {
		t.Fatalf("unmarshal settlement file: %v", err)
	}
	if sf.FileHeader.BatchID != batch.ID {
		t.Errorf("file header batch_id = %s, want %s", sf.FileHeader.BatchID, batch.ID)
	}
	if sf.FileHeader.Format != "X9_JSON_EQUIVALENT" {
		t.Errorf("file header format = %s, want X9_JSON_EQUIVALENT", sf.FileHeader.Format)
	}
	if sf.FileHeader.TotalItems != 2 {
		t.Errorf("file header total_items = %d, want 2", sf.FileHeader.TotalItems)
	}
	if sf.FileHeader.TotalAmountCents != 30000 {
		t.Errorf("file header total_amount_cents = %d, want 30000", sf.FileHeader.TotalAmountCents)
	}
	if len(sf.Items) != 2 {
		t.Errorf("file items count = %d, want 2", len(sf.Items))
	}

	// Collect transfer IDs from settlement items
	itemIDs := map[string]bool{}
	for _, item := range sf.Items {
		itemIDs[item.TransferID] = true
	}
	if !itemIDs[r1.TransferID] || !itemIDs[r2.TransferID] {
		t.Errorf("settlement items should contain both transfer IDs")
	}

	// Second GenerateBatch should fail (no new eligible transfers)
	_, err = svc.GenerateBatch(context.Background(), bizDate)
	if err == nil {
		t.Error("expected error for no eligible transfers on second batch")
	}

	// Acknowledge batch
	err = svc.AcknowledgeBatch(context.Background(), batch.ID, "ACK-001")
	if err != nil {
		t.Fatalf("AcknowledgeBatch: %v", err)
	}

	// Verify batch status
	var batchStatus, ackRef string
	db.QueryRow("SELECT status, ack_reference FROM settlement_batches WHERE id = ?", batch.ID).Scan(&batchStatus, &ackRef)
	if batchStatus != "ACKNOWLEDGED" {
		t.Errorf("batch status = %s, want ACKNOWLEDGED", batchStatus)
	}
	if ackRef != "ACK-001" {
		t.Errorf("ack_reference = %s, want ACK-001", ackRef)
	}

	// Verify transfers transitioned to Completed
	for _, tid := range []string{r1.TransferID, r2.TransferID} {
		var state string
		db.QueryRow("SELECT state FROM transfers WHERE id = ?", tid).Scan(&state)
		if state != "Completed" {
			t.Errorf("transfer %s state = %s, want Completed", tid, state)
		}
	}
}
