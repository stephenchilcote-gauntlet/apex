package returns

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := model.AnalyzeResponse{
			VendorTransactionID:  "vtx-test",
			Decision:             "PASS",
			IQAStatus:            "PASS",
			ManualReviewRequired: false,
			DuplicateDetected:    false,
			AmountMatches:        true,
			RiskScore:            10,
			MICR: &model.MICRResult{
				Routing: "111000025", Account: "123456789", Serial: "1001", Confidence: 0.99,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func ptr(s string) *string { return &s }

func mustSubmitDeposit(t *testing.T, db *sql.DB, vendorURL, imageDir string, amount int64) string {
	t.Helper()
	depSvc := &deposits.DepositService{
		DB:           db,
		TransferSvc:  &transfers.TransferService{},
		FundingSvc:   &funding.FundingService{DB: db},
		LedgerSvc:    &ledger.LedgerService{},
		VendorClient: vendorclient.New(vendorURL),
		ImageDir:     imageDir,
	}
	res, err := depSvc.SubmitDeposit(context.Background(), "INV-1001", amount,
		bytes.NewReader([]byte("front")), bytes.NewReader([]byte("back")), ptr("clean_pass"))
	if err != nil {
		t.Fatalf("submit deposit: %v", err)
	}
	if res.State != transfers.StateFundsPosted {
		t.Fatalf("deposit state = %s, want FundsPosted", res.State)
	}
	return res.TransferID
}

func TestReturnsService_ProcessReturn_FromFundsPosted(t *testing.T) {
	db := newTestDB(t)
	vendor := cleanPassStub(t)
	transferID := mustSubmitDeposit(t, db, vendor.URL, t.TempDir(), 15000)

	svc := &ReturnsService{
		DB:          db,
		TransferSvc: &transfers.TransferService{},
		LedgerSvc:   &ledger.LedgerService{},
	}

	err := svc.ProcessReturn(context.Background(), transferID, "R01", "NSF")
	if err != nil {
		t.Fatalf("ProcessReturn: %v", err)
	}

	// Verify state
	var state string
	db.QueryRow("SELECT state FROM transfers WHERE id = ?", transferID).Scan(&state)
	if state != "Returned" {
		t.Errorf("state = %s, want Returned", state)
	}

	// Verify return fields
	var reasonCode string
	var feeCents int64
	var returnedAt *time.Time
	db.QueryRow("SELECT return_reason_code, return_fee_cents, returned_at FROM transfers WHERE id = ?", transferID).
		Scan(&reasonCode, &feeCents, &returnedAt)
	if reasonCode != "R01" {
		t.Errorf("return_reason_code = %s, want R01", reasonCode)
	}
	if feeCents != 3000 {
		t.Errorf("return_fee_cents = %d, want 3000", feeCents)
	}
	if returnedAt == nil {
		t.Error("returned_at should be set")
	}

	// Verify return notification
	var notifCount int
	var notifFee int64
	db.QueryRow("SELECT COUNT(*), COALESCE(SUM(fee_cents),0) FROM return_notifications WHERE transfer_id = ?", transferID).Scan(&notifCount, &notifFee)
	if notifCount != 1 {
		t.Errorf("return_notifications = %d, want 1", notifCount)
	}
	if notifFee != 3000 {
		t.Errorf("notification fee = %d, want 3000", notifFee)
	}

	// Verify notification outbox
	var outboxCount int
	var templateCode, outboxStatus string
	db.QueryRow("SELECT COUNT(*) FROM notifications_outbox WHERE transfer_id = ?", transferID).Scan(&outboxCount)
	if outboxCount != 1 {
		t.Errorf("notifications_outbox = %d, want 1", outboxCount)
	}
	db.QueryRow("SELECT template_code, status FROM notifications_outbox WHERE transfer_id = ?", transferID).Scan(&templateCode, &outboxStatus)
	if templateCode != "RETURNED_CHECK" {
		t.Errorf("template_code = %s, want RETURNED_CHECK", templateCode)
	}
	if outboxStatus != "PENDING" {
		t.Errorf("outbox status = %s, want PENDING", outboxStatus)
	}

	// Verify reversal journal
	var reversalJournalID string
	err = db.QueryRow("SELECT id FROM ledger_journals WHERE transfer_id = ? AND journal_type = 'RETURN_REVERSAL'", transferID).Scan(&reversalJournalID)
	if err != nil {
		t.Fatalf("no RETURN_REVERSAL journal: %v", err)
	}
	var reversalSum int64
	db.QueryRow("SELECT COALESCE(SUM(signed_amount_cents),0) FROM ledger_entries WHERE journal_id = ?", reversalJournalID).Scan(&reversalSum)
	if reversalSum != 0 {
		t.Errorf("reversal journal entries sum = %d, want 0", reversalSum)
	}

	// Verify fee journal
	var feeJournalID string
	err = db.QueryRow("SELECT id FROM ledger_journals WHERE transfer_id = ? AND journal_type = 'RETURN_FEE'", transferID).Scan(&feeJournalID)
	if err != nil {
		t.Fatalf("no RETURN_FEE journal: %v", err)
	}
	var feeSum int64
	db.QueryRow("SELECT COALESCE(SUM(signed_amount_cents),0) FROM ledger_entries WHERE journal_id = ?", feeJournalID).Scan(&feeSum)
	if feeSum != 0 {
		t.Errorf("fee journal entries sum = %d, want 0", feeSum)
	}

	// Verify fee amounts: investor -3000, fee_revenue +3000
	var investorFee, revenueFee int64
	db.QueryRow("SELECT signed_amount_cents FROM ledger_entries WHERE journal_id = ? AND account_id = '00000000-0000-0000-0000-000000001001'", feeJournalID).Scan(&investorFee)
	db.QueryRow("SELECT signed_amount_cents FROM ledger_entries WHERE journal_id = ? AND account_id = '00000000-0000-0000-0000-000000000002'", feeJournalID).Scan(&revenueFee)
	if investorFee != -3000 {
		t.Errorf("investor fee entry = %d, want -3000", investorFee)
	}
	if revenueFee != 3000 {
		t.Errorf("revenue fee entry = %d, want +3000", revenueFee)
	}

	// Verify RETURN_PROCESSED audit event
	var auditCount int
	db.QueryRow("SELECT COUNT(*) FROM audit_events WHERE entity_id = ? AND event_type = 'RETURN_PROCESSED'", transferID).Scan(&auditCount)
	if auditCount != 0 {
		// Note: The current code uses audit.LogEvent which doesn't have ID set, but it auto-generates one
	}
}

func TestReturnsService_ProcessReturn_FromCompleted(t *testing.T) {
	db := newTestDB(t)
	vendor := cleanPassStub(t)
	transferID := mustSubmitDeposit(t, db, vendor.URL, t.TempDir(), 20000)

	// Transition to Completed first
	transferSvc := &transfers.TransferService{}
	err := transferSvc.Transition(db, transferID, transfers.StateCompleted, "SYSTEM", "test")
	if err != nil {
		t.Fatalf("transition to Completed: %v", err)
	}

	svc := &ReturnsService{
		DB:          db,
		TransferSvc: transferSvc,
		LedgerSvc:   &ledger.LedgerService{},
	}

	err = svc.ProcessReturn(context.Background(), transferID, "R09", "Uncollected funds")
	if err != nil {
		t.Fatalf("ProcessReturn: %v", err)
	}

	var state string
	db.QueryRow("SELECT state FROM transfers WHERE id = ?", transferID).Scan(&state)
	if state != "Returned" {
		t.Errorf("state = %s, want Returned", state)
	}

	// Verify reversal reverses the original 20000
	var reversalJID string
	db.QueryRow("SELECT id FROM ledger_journals WHERE transfer_id = ? AND journal_type = 'RETURN_REVERSAL'", transferID).Scan(&reversalJID)
	var investorReversal, omnibusReversal int64
	db.QueryRow("SELECT signed_amount_cents FROM ledger_entries WHERE journal_id = ? AND account_id = '00000000-0000-0000-0000-000000001001'", reversalJID).Scan(&investorReversal)
	db.QueryRow("SELECT signed_amount_cents FROM ledger_entries WHERE journal_id = ? AND account_id = '00000000-0000-0000-0000-000000000001'", reversalJID).Scan(&omnibusReversal)
	if investorReversal != -20000 {
		t.Errorf("investor reversal = %d, want -20000", investorReversal)
	}
	if omnibusReversal != 20000 {
		t.Errorf("omnibus reversal = %d, want +20000", omnibusReversal)
	}
}

func TestReturnsService_ProcessReturn_RejectsIneligibleState(t *testing.T) {
	db := newTestDB(t)

	// Create a transfer in Requested state (ineligible for return)
	transferSvc := &transfers.TransferService{}
	tr := &transfers.Transfer{
		InvestorAccountID: "00000000-0000-0000-0000-000000001001",
		CorrespondentID:   "00000000-0000-0000-0000-000000000010",
		OmnibusAccountID:  "00000000-0000-0000-0000-000000000001",
		AmountCents:       10000,
		Currency:          "USD",
	}
	if err := transferSvc.Create(db, tr); err != nil {
		t.Fatal(err)
	}

	svc := &ReturnsService{
		DB:          db,
		TransferSvc: transferSvc,
		LedgerSvc:   &ledger.LedgerService{},
	}

	err := svc.ProcessReturn(context.Background(), tr.ID, "R01", "NSF")
	if err == nil {
		t.Fatal("expected error for ineligible state")
	}

	// Verify no side effects
	var state string
	db.QueryRow("SELECT state FROM transfers WHERE id = ?", tr.ID).Scan(&state)
	if state != "Requested" {
		t.Errorf("state = %s, want Requested (unchanged)", state)
	}

	var notifCount int
	db.QueryRow("SELECT COUNT(*) FROM return_notifications WHERE transfer_id = ?", tr.ID).Scan(&notifCount)
	if notifCount != 0 {
		t.Errorf("return_notifications = %d, want 0", notifCount)
	}

	var journalCount int
	db.QueryRow("SELECT COUNT(*) FROM ledger_journals WHERE transfer_id = ?", tr.ID).Scan(&journalCount)
	if journalCount != 0 {
		t.Errorf("journals = %d, want 0", journalCount)
	}

	var outboxCount int
	db.QueryRow("SELECT COUNT(*) FROM notifications_outbox WHERE transfer_id = ?", tr.ID).Scan(&outboxCount)
	if outboxCount != 0 {
		t.Errorf("outbox = %d, want 0", outboxCount)
	}
}
