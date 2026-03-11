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
	"strings"
	"testing"
	"time"

	"github.com/apex-checkout/mobile-check-deposit/internal/deposits"
	"github.com/apex-checkout/mobile-check-deposit/internal/funding"
	"github.com/apex-checkout/mobile-check-deposit/internal/ledger"
	"github.com/apex-checkout/mobile-check-deposit/internal/transfers"
	vendorclient "github.com/apex-checkout/mobile-check-deposit/internal/vendorsvc/client"
	"github.com/apex-checkout/mobile-check-deposit/internal/vendorsvc/model"
	_ "github.com/mattn/go-sqlite3"
	"github.com/moov-io/imagecashletter"
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
	for _, name := range []string{"001_init.sql"} {
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
		bytes.NewReader([]byte("front")), bytes.NewReader([]byte("back")))
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

	// Verify file has .x9 extension
	if !strings.HasSuffix(*batch.FilePath, ".x9") {
		t.Errorf("file path = %s, want .x9 extension", *batch.FilePath)
	}

	// Parse the X9 ICL file back using the library's reader
	x9f, err := os.Open(*batch.FilePath)
	if err != nil {
		t.Fatalf("open settlement file: %v", err)
	}
	defer x9f.Close()

	reader := imagecashletter.NewReader(x9f, imagecashletter.ReadVariableLineLengthOption())
	iclFile, err := reader.Read()
	if err != nil {
		t.Fatalf("parse X9 ICL file: %v", err)
	}

	// Validate file structure
	if iclFile.Header.ImmediateOrigin != originRoutingNumber {
		t.Errorf("file header origin = %s, want %s", iclFile.Header.ImmediateOrigin, originRoutingNumber)
	}
	if len(iclFile.CashLetters) != 1 {
		t.Fatalf("cash letter count = %d, want 1", len(iclFile.CashLetters))
	}
	cl := iclFile.CashLetters[0]
	bundles := cl.GetBundles()
	if len(bundles) != 1 {
		t.Fatalf("bundle count = %d, want 1", len(bundles))
	}
	checks := bundles[0].GetChecks()
	if len(checks) != 2 {
		t.Fatalf("check detail count = %d, want 2", len(checks))
	}

	// Verify total amount across checks
	var totalAmount int
	for _, cd := range checks {
		totalAmount += cd.ItemAmount
		if cd.AddendumCount != 1 {
			t.Errorf("check addendum count = %d, want 1", cd.AddendumCount)
		}
		if len(cd.ImageViewDetail) != 2 {
			t.Errorf("image view detail count = %d, want 2 (front+back)", len(cd.ImageViewDetail))
		}
		if len(cd.ImageViewData) != 2 {
			t.Errorf("image view data count = %d, want 2 (front+back)", len(cd.ImageViewData))
		}
	}
	if totalAmount != 30000 {
		t.Errorf("total amount across checks = %d, want 30000", totalAmount)
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
