package deposits

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
	"sync"
	"sync/atomic"
	"testing"

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

func newVendorStub(t *testing.T, resp model.AnalyzeResponse) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp.VendorTransactionID = "vtx-test-001"
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newDepositService(t *testing.T, db *sql.DB, vendorURL string) *DepositService {
	t.Helper()
	return &DepositService{
		DB:           db,
		TransferSvc:  &transfers.TransferService{},
		FundingSvc:   &funding.FundingService{DB: db},
		LedgerSvc:    &ledger.LedgerService{},
		VendorClient: vendorclient.New(vendorURL),
		ImageDir:     t.TempDir(),
	}
}

func front() *bytes.Reader { return bytes.NewReader([]byte("front-image-data")) }
func back() *bytes.Reader  { return bytes.NewReader([]byte("back-image-data")) }

func ptr(s string) *string { return &s }

func mustGetTransfer(t *testing.T, db *sql.DB, id string) *transfers.Transfer {
	t.Helper()
	svc := &transfers.TransferService{}
	tr, err := svc.GetByID(db, id)
	if err != nil {
		t.Fatalf("get transfer %s: %v", id, err)
	}
	return tr
}

func cleanPassResp() model.AnalyzeResponse {
	return model.AnalyzeResponse{
		Decision:             "PASS",
		IQAStatus:            "PASS",
		ManualReviewRequired: false,
		DuplicateDetected:    false,
		AmountMatches:        true,
		RiskScore:            10,
		MICR: &model.MICRResult{
			Routing:    "111000025",
			Account:    "123456789",
			Serial:     "1001",
			Confidence: 0.99,
		},
	}
}

func TestDepositService_SubmitDeposit_CleanPass_E2E(t *testing.T) {
	db := newTestDB(t)
	vendor := newVendorStub(t, cleanPassResp())
	svc := newDepositService(t, db, vendor.URL)

	res, err := svc.SubmitDeposit(context.Background(), "INV-1001", 12500, front(), back(), ptr("clean_pass"))
	if err != nil {
		t.Fatalf("SubmitDeposit: %v", err)
	}

	if res.State != transfers.StateFundsPosted {
		t.Errorf("result state = %s, want FundsPosted", res.State)
	}

	tr := mustGetTransfer(t, db, res.TransferID)
	if tr.State != transfers.StateFundsPosted {
		t.Errorf("transfer state = %s, want FundsPosted", tr.State)
	}
	if tr.ContributionType == nil || *tr.ContributionType != "INDIVIDUAL" {
		t.Errorf("contribution_type = %v, want INDIVIDUAL", tr.ContributionType)
	}
	if tr.ApprovedAt == nil {
		t.Error("approved_at should be set")
	}
	if tr.PostedAt == nil {
		t.Error("posted_at should be set")
	}

	// Verify images
	var imgCount int
	db.QueryRow("SELECT COUNT(*) FROM transfer_images WHERE transfer_id = ?", res.TransferID).Scan(&imgCount)
	if imgCount != 2 {
		t.Errorf("images = %d, want 2", imgCount)
	}

	// Verify vendor result
	var decision string
	db.QueryRow("SELECT decision FROM vendor_results WHERE transfer_id = ?", res.TransferID).Scan(&decision)
	if decision != "PASS" {
		t.Errorf("vendor decision = %s, want PASS", decision)
	}

	// Verify rule evaluations all PASS
	rows, _ := db.Query("SELECT rule_name, outcome FROM rule_evaluations WHERE transfer_id = ?", res.TransferID)
	defer rows.Close()
	for rows.Next() {
		var name, outcome string
		rows.Scan(&name, &outcome)
		if outcome != "PASS" {
			t.Errorf("rule %s outcome = %s, want PASS", name, outcome)
		}
	}

	// Verify ledger: one DEPOSIT_POSTING journal
	var journalCount int
	db.QueryRow("SELECT COUNT(*) FROM ledger_journals WHERE transfer_id = ? AND journal_type = 'DEPOSIT_POSTING'", res.TransferID).Scan(&journalCount)
	if journalCount != 1 {
		t.Errorf("DEPOSIT_POSTING journals = %d, want 1", journalCount)
	}

	// Verify double-entry: entries sum to zero
	var journalID string
	db.QueryRow("SELECT id FROM ledger_journals WHERE transfer_id = ?", res.TransferID).Scan(&journalID)
	var entrySum int64
	db.QueryRow("SELECT COALESCE(SUM(signed_amount_cents), 0) FROM ledger_entries WHERE journal_id = ?", journalID).Scan(&entrySum)
	if entrySum != 0 {
		t.Errorf("journal entries sum = %d, want 0 (double-entry)", entrySum)
	}

	// Verify investor credited +12500, omnibus debited -12500
	var investorAmt, omnibusAmt int64
	db.QueryRow("SELECT signed_amount_cents FROM ledger_entries WHERE journal_id = ? AND account_id = '00000000-0000-0000-0000-000000001001'", journalID).Scan(&investorAmt)
	db.QueryRow("SELECT signed_amount_cents FROM ledger_entries WHERE journal_id = ? AND account_id = '00000000-0000-0000-0000-000000000001'", journalID).Scan(&omnibusAmt)
	if investorAmt != 12500 {
		t.Errorf("investor entry = %d, want +12500", investorAmt)
	}
	if omnibusAmt != -12500 {
		t.Errorf("omnibus entry = %d, want -12500", omnibusAmt)
	}

	// Verify audit trail has 4 state transitions
	var auditCount int
	db.QueryRow("SELECT COUNT(*) FROM audit_events WHERE entity_id = ? AND event_type = 'STATE_TRANSITION'", res.TransferID).Scan(&auditCount)
	if auditCount != 4 {
		t.Errorf("state transition audit events = %d, want 4", auditCount)
	}
}

func TestDepositService_SubmitDeposit_VendorScenarios(t *testing.T) {
	cases := []struct {
		scenario   string
		resp       model.AnalyzeResponse
		wantState  transfers.State
		wantReject bool
		wantReview bool
	}{
		{
			"iqa_blur",
			model.AnalyzeResponse{Decision: "FAIL", IQAStatus: "BLUR", DuplicateDetected: false},
			transfers.StateRejected, true, false,
		},
		{
			"iqa_glare",
			model.AnalyzeResponse{Decision: "FAIL", IQAStatus: "GLARE", DuplicateDetected: false},
			transfers.StateRejected, true, false,
		},
		{
			"micr_failure",
			model.AnalyzeResponse{Decision: "FAIL", IQAStatus: "PASS", MICR: nil, DuplicateDetected: false},
			transfers.StateRejected, true, false,
		},
		{
			"duplicate_detected",
			model.AnalyzeResponse{Decision: "FAIL", IQAStatus: "PASS", DuplicateDetected: true},
			transfers.StateRejected, true, false,
		},
		{
			"amount_mismatch",
			model.AnalyzeResponse{Decision: "FAIL", IQAStatus: "PASS", AmountMatches: false, DuplicateDetected: false},
			transfers.StateRejected, true, false,
		},
		{
			"iqa_pass_review",
			model.AnalyzeResponse{
				Decision: "PASS", IQAStatus: "PASS", ManualReviewRequired: true,
				AmountMatches: true, DuplicateDetected: false,
				MICR: &model.MICRResult{Routing: "111000025", Account: "123456789", Serial: "9999", Confidence: 0.95},
			},
			transfers.StateAnalyzing, false, true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.scenario, func(t *testing.T) {
			db := newTestDB(t)
			vendor := newVendorStub(t, tc.resp)
			svc := newDepositService(t, db, vendor.URL)

			res, err := svc.SubmitDeposit(context.Background(), "INV-1001", 12500, front(), back(), ptr(tc.scenario))
			if err != nil {
				t.Fatalf("SubmitDeposit: %v", err)
			}

			tr := mustGetTransfer(t, db, res.TransferID)
			if tr.State != tc.wantState {
				t.Errorf("state = %s, want %s", tr.State, tc.wantState)
			}

			if tc.wantReject {
				if tr.RejectionCode == nil || *tr.RejectionCode != "VENDOR_REJECT" {
					t.Errorf("rejection_code = %v, want VENDOR_REJECT", tr.RejectionCode)
				}
			}

			if tc.wantReview {
				if !tr.ReviewRequired {
					t.Error("review_required should be true")
				}
				if tr.ReviewStatus == nil || *tr.ReviewStatus != "PENDING" {
					t.Errorf("review_status = %v, want PENDING", tr.ReviewStatus)
				}
			}

			// No ledger entries for rejections or reviews
			var journalCount int
			db.QueryRow("SELECT COUNT(*) FROM ledger_journals WHERE transfer_id = ?", res.TransferID).Scan(&journalCount)
			if journalCount != 0 {
				t.Errorf("journals = %d, want 0 (no posting)", journalCount)
			}
		})
	}
}

func TestDepositService_SubmitDeposit_FundingRuleRejections(t *testing.T) {
	cases := []struct {
		name           string
		setup          func(*sql.DB)
		externalAcctID string
		amount         int64
		wantFailRule   string
	}{
		{
			name:           "over_max_deposit_limit",
			setup:          func(db *sql.DB) {},
			externalAcctID: "INV-1001",
			amount:         500001,
			wantFailRule:   "MAX_DEPOSIT_LIMIT",
		},
		{
			name: "inactive_account",
			setup: func(db *sql.DB) {
				db.Exec("UPDATE accounts SET status = 'BLOCKED' WHERE external_account_id = 'INV-1002'")
			},
			externalAcctID: "INV-1002",
			amount:         10000,
			wantFailRule:   "ACCOUNT_ELIGIBLE",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t)
			tc.setup(db)
			vendor := newVendorStub(t, cleanPassResp())
			svc := newDepositService(t, db, vendor.URL)

			res, err := svc.SubmitDeposit(context.Background(), tc.externalAcctID, tc.amount, front(), back(), ptr("clean_pass"))
			if err != nil {
				t.Fatalf("SubmitDeposit: %v", err)
			}

			tr := mustGetTransfer(t, db, res.TransferID)
			if tr.State != transfers.StateRejected {
				t.Errorf("state = %s, want Rejected", tr.State)
			}
			if tr.RejectionCode == nil || *tr.RejectionCode != "RULES_REJECT" {
				t.Errorf("rejection_code = %v, want RULES_REJECT", tr.RejectionCode)
			}

			var outcome string
			err = db.QueryRow("SELECT outcome FROM rule_evaluations WHERE transfer_id = ? AND rule_name = ?", res.TransferID, tc.wantFailRule).Scan(&outcome)
			if err != nil {
				t.Fatalf("query rule %s: %v", tc.wantFailRule, err)
			}
			if outcome != "FAIL" {
				t.Errorf("rule %s outcome = %s, want FAIL", tc.wantFailRule, outcome)
			}

			var journalCount int
			db.QueryRow("SELECT COUNT(*) FROM ledger_journals WHERE transfer_id = ?", res.TransferID).Scan(&journalCount)
			if journalCount != 0 {
				t.Errorf("journals = %d, want 0", journalCount)
			}
		})
	}
}

func TestDepositService_SubmitDeposit_InternalDuplicateFingerprint(t *testing.T) {
	db := newTestDB(t)
	vendor := newVendorStub(t, cleanPassResp())
	svc := newDepositService(t, db, vendor.URL)

	// First deposit succeeds
	first, err := svc.SubmitDeposit(context.Background(), "INV-1003", 25000, front(), back(), ptr("clean_pass"))
	if err != nil {
		t.Fatalf("first deposit: %v", err)
	}
	if first.State != transfers.StateFundsPosted {
		t.Errorf("first state = %s, want FundsPosted", first.State)
	}

	tr1 := mustGetTransfer(t, db, first.TransferID)
	if tr1.DuplicateFingerprint == nil {
		t.Fatal("first transfer should have fingerprint")
	}

	// Second deposit with same MICR+amount+account is rejected as duplicate
	second, err := svc.SubmitDeposit(context.Background(), "INV-1003", 25000, front(), back(), ptr("clean_pass"))
	if err != nil {
		t.Fatalf("second deposit: %v", err)
	}
	if second.State != transfers.StateRejected {
		t.Errorf("second state = %s, want Rejected", second.State)
	}

	tr2 := mustGetTransfer(t, db, second.TransferID)
	if tr2.DuplicateFingerprint == nil {
		t.Fatal("second transfer should have fingerprint")
	}
	if *tr1.DuplicateFingerprint != *tr2.DuplicateFingerprint {
		t.Errorf("fingerprints differ: %s vs %s", *tr1.DuplicateFingerprint, *tr2.DuplicateFingerprint)
	}

	var outcome string
	err = db.QueryRow("SELECT outcome FROM rule_evaluations WHERE transfer_id = ? AND rule_name = 'INTERNAL_DUPLICATE'", second.TransferID).Scan(&outcome)
	if err != nil {
		t.Fatalf("query INTERNAL_DUPLICATE rule: %v", err)
	}
	if outcome != "FAIL" {
		t.Errorf("INTERNAL_DUPLICATE outcome = %s, want FAIL", outcome)
	}
}

func TestDepositService_ConcurrentDeposits_LedgerInvariant(t *testing.T) {
	db := newTestDB(t)

	var counter int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&counter, 1)
		resp := cleanPassResp()
		resp.VendorTransactionID = fmt.Sprintf("vtx-concurrent-%d", n)
		resp.MICR.Serial = fmt.Sprintf("%04d", n)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	svc := newDepositService(t, db, srv.URL)

	const numDeposits = 20

	type result struct {
		idx int
		err error
	}
	results := make(chan result, numDeposits)

	var wg sync.WaitGroup
	for i := 0; i < numDeposits; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			amount := int64(1000 + idx*100)
			_, err := svc.SubmitDeposit(context.Background(), "INV-1001", amount, front(), back(), ptr("clean_pass"))
			results <- result{idx: idx, err: err}
		}(i)
	}
	wg.Wait()
	close(results)

	for r := range results {
		if r.err != nil {
			t.Errorf("deposit %d failed: %v", r.idx, r.err)
		}
	}

	// Global ledger invariant: all entries must sum to zero (double-entry)
	var globalSum int64
	if err := db.QueryRow("SELECT COALESCE(SUM(signed_amount_cents), 0) FROM ledger_entries").Scan(&globalSum); err != nil {
		t.Fatalf("query global ledger sum: %v", err)
	}
	if globalSum != 0 {
		t.Errorf("global ledger sum = %d, want 0 (double-entry invariant violated)", globalSum)
	}

	// Exactly 20 DEPOSIT_POSTING journals
	var journalCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM ledger_journals WHERE journal_type = 'DEPOSIT_POSTING'").Scan(&journalCount); err != nil {
		t.Fatalf("query journal count: %v", err)
	}
	if journalCount != numDeposits {
		t.Errorf("DEPOSIT_POSTING journals = %d, want %d", journalCount, numDeposits)
	}
}
