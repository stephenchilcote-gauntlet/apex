package client

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/apex-checkout/mobile-check-deposit/internal/vendorsvc/model"
	_ "github.com/mattn/go-sqlite3"
)

func migrationsDir() string {
	_, f, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(f), "..", "..", "..", "db", "migrations")
}

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file::memory:?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	content, err := os.ReadFile(filepath.Join(migrationsDir(), "001_init.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(content)); err != nil {
		t.Fatalf("migration: %v", err)
	}
	return db
}

// seedTransfer inserts a minimal transfer row so vendor_results FK is satisfied.
func seedTransfer(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO transfers (
			id, investor_account_id, correspondent_id, omnibus_account_id,
			amount_cents, currency, state, created_at, updated_at
		) VALUES (?, '00000000-0000-0000-0000-000000001001', '00000000-0000-0000-0000-000000000010',
		          '00000000-0000-0000-0000-000000000001', 25000, 'USD', 'Analyzing', datetime('now'), datetime('now'))`, id)
	if err != nil {
		t.Fatalf("seedTransfer: %v", err)
	}
}

func sampleResponse() *model.AnalyzeResponse {
	confidence := 0.97
	ocrAmount := 25000
	return &model.AnalyzeResponse{
		VendorTransactionID:  "vtx-test-001",
		Decision:             "PASS",
		IQAStatus:            "PASS",
		MICR:                 &model.MICRResult{Routing: "021000021", Account: "123456789", Serial: "1001", Confidence: confidence},
		OCRAmountCents:       &ocrAmount,
		AmountMatches:        true,
		DuplicateDetected:    false,
		RiskScore:            10,
		ManualReviewRequired: false,
	}
}

func TestSaveVendorResult_PersistsAndRetrievable(t *testing.T) {
	db := newTestDB(t)
	const transferID = "transfer-vendor-001"
	seedTransfer(t, db, transferID)

	resp := sampleResponse()
	if err := SaveVendorResult(db, transferID, resp); err != nil {
		t.Fatalf("SaveVendorResult: %v", err)
	}

	got, err := GetVendorResult(db, transferID)
	if err != nil {
		t.Fatalf("GetVendorResult: %v", err)
	}

	if got.Decision != resp.Decision {
		t.Errorf("Decision = %q, want %q", got.Decision, resp.Decision)
	}
	if got.VendorTransactionID != resp.VendorTransactionID {
		t.Errorf("VendorTransactionID = %q, want %q", got.VendorTransactionID, resp.VendorTransactionID)
	}
}

func TestSaveVendorResult_PreservesMICR(t *testing.T) {
	db := newTestDB(t)
	const transferID = "transfer-vendor-micr"
	seedTransfer(t, db, transferID)

	resp := sampleResponse()
	if err := SaveVendorResult(db, transferID, resp); err != nil {
		t.Fatalf("SaveVendorResult: %v", err)
	}

	got, err := GetVendorResult(db, transferID)
	if err != nil {
		t.Fatalf("GetVendorResult: %v", err)
	}

	if got.MICR == nil {
		t.Fatal("MICR is nil after save/retrieve")
	}
	if got.MICR.Routing != "021000021" {
		t.Errorf("MICR.Routing = %q, want 021000021", got.MICR.Routing)
	}
	if got.MICR.Confidence != 0.97 {
		t.Errorf("MICR.Confidence = %v, want 0.97", got.MICR.Confidence)
	}
}

func TestSaveVendorResult_NilMICRHandled(t *testing.T) {
	db := newTestDB(t)
	const transferID = "transfer-vendor-nomicr"
	seedTransfer(t, db, transferID)

	resp := &model.AnalyzeResponse{
		VendorTransactionID: "vtx-no-micr",
		Decision:            "FAIL",
		IQAStatus:           "FAIL",
		MICR:                nil,
		AmountMatches:       false,
	}
	if err := SaveVendorResult(db, transferID, resp); err != nil {
		t.Fatalf("SaveVendorResult with nil MICR: %v", err)
	}

	got, err := GetVendorResult(db, transferID)
	if err != nil {
		t.Fatalf("GetVendorResult: %v", err)
	}
	if got.MICR != nil {
		t.Errorf("expected nil MICR, got %+v", got.MICR)
	}
	if got.Decision != "FAIL" {
		t.Errorf("Decision = %q, want FAIL", got.Decision)
	}
}

func TestGetVendorResult_NotFound(t *testing.T) {
	db := newTestDB(t)

	_, err := GetVendorResult(db, "nonexistent-transfer-id")
	if err == nil {
		t.Fatal("expected error for nonexistent transfer, got nil")
	}
}

func TestAnalyze_SuccessfulRequest(t *testing.T) {
	// Mock vendor stub server
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/stub/v1/checks/analyze" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		resp := model.AnalyzeResponse{
			VendorTransactionID:  "vtx-mock-001",
			Decision:             "PASS",
			IQAStatus:            "PASS",
			AmountMatches:        true,
			ManualReviewRequired: false,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := New(srv.URL)
	req := model.AnalyzeRequest{
		TransferID:       "transfer-test",
		InvestorAccountID: "INV-1001",
		AmountCents:      25000,
	}

	got, err := c.Analyze(context.Background(), req)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if got.Decision != "PASS" {
		t.Errorf("Decision = %q, want PASS", got.Decision)
	}
	if got.VendorTransactionID != "vtx-mock-001" {
		t.Errorf("VendorTransactionID = %q, want vtx-mock-001", got.VendorTransactionID)
	}
}

func TestAnalyze_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(srv.URL)
	_, err := c.Analyze(context.Background(), model.AnalyzeRequest{})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

func TestAnalyze_ConnectionRefused(t *testing.T) {
	c := New("http://localhost:19999") // no server listening
	_, err := c.Analyze(context.Background(), model.AnalyzeRequest{})
	if err == nil {
		t.Fatal("expected error for connection refused, got nil")
	}
}
