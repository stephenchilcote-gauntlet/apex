package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	_ "github.com/mattn/go-sqlite3"

	"github.com/apex-checkout/mobile-check-deposit/internal/ledger"
	"github.com/apex-checkout/mobile-check-deposit/internal/returns"
	"github.com/apex-checkout/mobile-check-deposit/internal/settlement"
	"github.com/apex-checkout/mobile-check-deposit/internal/transfers"
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

	for _, name := range []string{"001_init.sql", "002_investor_names.sql"} {
		content, err := os.ReadFile(filepath.Join(migrationsDir(), name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(content)); err != nil {
			t.Fatalf("migration %s: %v", name, err)
		}
	}
	return db
}

func newRouter(t *testing.T, db *sql.DB) *chi.Mux {
	t.Helper()
	transferSvc := &transfers.TransferService{}
	ledgerSvc := &ledger.LedgerService{}
	settlementSvc := &settlement.SettlementService{
		DB:          db,
		OutputPath:  t.TempDir(),
		TransferSvc: transferSvc,
	}
	returnsSvc := &returns.ReturnsService{
		DB:          db,
		TransferSvc: transferSvc,
		LedgerSvc:   ledgerSvc,
	}

	h := &Handlers{
		DB:            db,
		TransferSvc:   transferSvc,
		LedgerSvc:     ledgerSvc,
		SettlementSvc: settlementSvc,
		ReturnsSvc:    returnsSvc,
	}

	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r
}

func doRequest(r *chi.Mux, method, path string, body []byte) *httptest.ResponseRecorder {
	var bodyReader *bytes.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	} else {
		bodyReader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

func TestHandlers_ListDeposits_ReturnsArray(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	rr := doRequest(r, "GET", "/api/v1/deposits", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/deposits: status %d, body: %s", rr.Code, rr.Body.String())
	}

	var body []map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Seed data includes several transfers; verify shape
	if len(body) == 0 {
		t.Fatal("expected seeded deposits, got empty array")
	}
	// Verify PascalCase field names (Go struct without json tags)
	first := body[0]
	if _, ok := first["ID"]; !ok {
		t.Error("missing ID field")
	}
	if _, ok := first["State"]; !ok {
		t.Error("missing State field")
	}
	if _, ok := first["AmountCents"]; !ok {
		t.Error("missing AmountCents field")
	}
}

func TestHandlers_ListDeposits_StateFilter(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	rr := doRequest(r, "GET", "/api/v1/deposits?state=FundsPosted", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	var body []map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &body)
	// All results must be FundsPosted
	for _, d := range body {
		if d["State"] != "FundsPosted" {
			t.Errorf("expected State=FundsPosted, got %v", d["State"])
		}
	}
}

func TestHandlers_GetDeposit_NotFound(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	rr := doRequest(r, "GET", "/api/v1/deposits/nonexistent-id", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestHandlers_GetDecisionTrace_NotFound(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	// decision-trace returns 200 with empty array for unknown transfer
	rr := doRequest(r, "GET", "/api/v1/deposits/nonexistent-id/decision-trace", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var body []interface{}
	json.Unmarshal(rr.Body.Bytes(), &body)
	if len(body) != 0 {
		t.Errorf("expected 0 trace events for unknown transfer, got %d", len(body))
	}
}

func TestHandlers_GetReviewQueue_ReturnsSeededItems(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	rr := doRequest(r, "GET", "/api/v1/operator/review-queue", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	var body []map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &body)
	// Seed data includes review-pending items
	if len(body) == 0 {
		t.Fatal("expected seeded review items, got empty queue")
	}
	// All items must have ReviewRequired=true
	for _, item := range body {
		if item["ReviewRequired"] != true {
			t.Errorf("expected ReviewRequired=true, got %v", item["ReviewRequired"])
		}
	}
}

func TestHandlers_GetAccountBalances_ReturnsSeededAccounts(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	rr := doRequest(r, "GET", "/api/v1/ledger/accounts", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}

	var body []map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("expected seeded accounts, got empty list")
	}
	// Verify shape — accounts have externalAccountId and balanceCents
	first := body[0]
	if _, ok := first["externalAccountId"]; !ok {
		t.Error("missing externalAccountId field")
	}
	if _, ok := first["balanceCents"]; !ok {
		t.Error("missing balanceCents field")
	}
	// Verify INV-1001 is present
	found := false
	for _, a := range body {
		if a["externalAccountId"] == "INV-1001" {
			found = true
			break
		}
	}
	if !found {
		t.Error("INV-1001 not found in account list")
	}
}

func TestHandlers_GetAccountDetail_NotFound(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	rr := doRequest(r, "GET", "/api/v1/ledger/accounts/nonexistent-id", nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestHandlers_GetJournals_MissingTransferID(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	rr := doRequest(r, "GET", "/api/v1/ledger/journals", nil)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing transferId, got %d", rr.Code)
	}
}

func TestHandlers_GetJournals_EmptyForUnknownTransfer(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	rr := doRequest(r, "GET", "/api/v1/ledger/journals?transferId=nonexistent", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var body []interface{}
	json.Unmarshal(rr.Body.Bytes(), &body)
	if len(body) != 0 {
		t.Errorf("expected 0 journals for unknown transfer, got %d", len(body))
	}
}

func TestHandlers_ListBatches_ReturnsArray(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	rr := doRequest(r, "GET", "/api/v1/settlement/batches", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	// Response must be an array (not null)
	var raw json.RawMessage
	json.Unmarshal(rr.Body.Bytes(), &raw)
	if len(raw) < 2 || raw[0] != '[' {
		t.Errorf("expected JSON array, got: %s", string(raw))
	}
}

func TestHandlers_AckBatch_MissingAckReference(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	body := `{"ackReference": ""}`
	rr := doRequest(r, "POST", "/api/v1/settlement/batches/fake-batch-id/ack", []byte(body))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing ackReference, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandlers_GetAuditLog_Empty(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	rr := doRequest(r, "GET", "/api/v1/audit", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	var body []interface{}
	json.Unmarshal(rr.Body.Bytes(), &body)
	// Should be an array (possibly empty)
	if body == nil {
		t.Error("expected array, got null")
	}
}

func TestHandlers_GetMetrics_Shape(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	rr := doRequest(r, "GET", "/api/v1/metrics", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := body["transfers"]; !ok {
		t.Error("missing transfers key")
	}
	if _, ok := body["volume"]; !ok {
		t.Error("missing volume key")
	}
	transfers, ok := body["transfers"].(map[string]interface{})
	if !ok {
		t.Fatal("transfers is not an object")
	}
	if _, ok := transfers["total"]; !ok {
		t.Error("missing transfers.total")
	}
}

func TestHandlers_ProcessReturn_MissingTransferID(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	body := `{"reasonCode": "NSF"}`
	rr := doRequest(r, "POST", "/api/v1/returns", []byte(body))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing transferId, got %d", rr.Code)
	}
}

func TestHandlers_ProcessReturn_MissingReasonCode(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	body := `{"transferId": "some-id"}`
	rr := doRequest(r, "POST", "/api/v1/returns", []byte(body))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing reasonCode, got %d", rr.Code)
	}
}

func TestHandlers_ApproveTransfer_MissingOperatorID(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	body := `{"notes": "approved"}`
	rr := doRequest(r, "POST", "/api/v1/operator/transfers/some-id/approve", []byte(body))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing operatorId, got %d", rr.Code)
	}
}

func TestHandlers_ApproveTransfer_NotFound(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	body := `{"operatorId": "op-1", "notes": "approved"}`
	rr := doRequest(r, "POST", "/api/v1/operator/transfers/nonexistent-id/approve", []byte(body))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown transfer, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandlers_RejectTransfer_MissingOperatorID(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	body := `{"notes": "rejected"}`
	rr := doRequest(r, "POST", "/api/v1/operator/transfers/some-id/reject", []byte(body))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing operatorId, got %d", rr.Code)
	}
}

func TestHandlers_RejectTransfer_NotFound(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	body := `{"operatorId": "op-1", "notes": "rejected", "rejectionCode": "MANUAL_REVIEW_FAILED"}`
	rr := doRequest(r, "POST", "/api/v1/operator/transfers/nonexistent-id/reject", []byte(body))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown transfer, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandlers_GetAccountDetail_ValidAccount(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	// Look up the UUID for INV-1001 from seeded data
	var accountID string
	err := db.QueryRow("SELECT id FROM accounts WHERE external_account_id = 'INV-1001'").Scan(&accountID)
	if err != nil {
		t.Fatalf("get INV-1001 account ID: %v", err)
	}

	rr := doRequest(r, "GET", "/api/v1/ledger/accounts/"+accountID, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var body map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &body)
	if _, ok := body["account"]; !ok {
		t.Error("missing account field")
	}
	if _, ok := body["entries"]; !ok {
		t.Error("missing entries field")
	}
	acct := body["account"].(map[string]interface{})
	if acct["externalAccountId"] != "INV-1001" {
		t.Errorf("expected externalAccountId=INV-1001, got %v", acct["externalAccountId"])
	}
}

func TestHandlers_GetDeposit_SeededTransfer(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	// Get a seeded transfer ID from the DB
	var transferID string
	err := db.QueryRow("SELECT id FROM transfers LIMIT 1").Scan(&transferID)
	if err != nil {
		t.Fatalf("get seeded transfer: %v", err)
	}

	rr := doRequest(r, "GET", "/api/v1/deposits/"+transferID, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var body map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &body)
	if _, ok := body["transfer"]; !ok {
		t.Error("missing transfer field")
	}
	if _, ok := body["vendorResult"]; !ok {
		t.Error("missing vendorResult field")
	}
	if _, ok := body["ruleEvaluations"]; !ok {
		t.Error("missing ruleEvaluations field")
	}
}

func TestHandlers_GetMetrics_SeededDataCounts(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	rr := doRequest(r, "GET", "/api/v1/metrics", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}

	var body map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &body)
	txData := body["transfers"].(map[string]interface{})
	total := int(txData["total"].(float64))
	if total == 0 {
		t.Error("expected seeded transfers in metrics total, got 0")
	}
	pendingReview := int(txData["pending_review"].(float64))
	if pendingReview == 0 {
		t.Error("expected pending_review > 0 from seeded data")
	}
}

func TestHandlers_ApproveTransfer_SeededAnalyzingTransfer(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	// Use a known seeded Analyzing+review_required transfer (T5)
	const transferID = "00000000-seed-0000-0000-000000000005"
	body := `{"operatorId": "test-op-1", "notes": "Approved in unit test"}`
	rr := doRequest(r, "POST", "/api/v1/operator/transfers/"+transferID+"/approve", []byte(body))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["state"] != "FundsPosted" {
		t.Errorf("expected state=FundsPosted, got %v", resp["state"])
	}
	if resp["transferId"] != transferID {
		t.Errorf("expected transferId=%s, got %v", transferID, resp["transferId"])
	}
}

func TestHandlers_RejectTransfer_SeededAnalyzingTransfer(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	// Use the other seeded Analyzing+review_required transfer (T6)
	const transferID = "00000000-seed-0000-0000-000000000006"
	body := `{"operatorId": "test-op-1", "notes": "MICR unreadable", "rejectionCode": "MICR_FAILURE"}`
	rr := doRequest(r, "POST", "/api/v1/operator/transfers/"+transferID+"/reject", []byte(body))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["state"] != "Rejected" {
		t.Errorf("expected state=Rejected, got %v", resp["state"])
	}
}

func TestHandlers_GenerateBatch_WithFundsPosted(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	// Insert a FundsPosted transfer for today with no images (avoids filesystem access)
	const bizDate = "2099-01-15"
	_, err := db.Exec(`INSERT INTO transfers
		(id, investor_account_id, correspondent_id, omnibus_account_id, state,
		 amount_cents, currency, contribution_type, business_date_ct, created_at, updated_at)
		VALUES ('test-batch-transfer', '00000000-0000-0000-0000-000000001001', '00000000-0000-0000-0000-000000000010',
		        '00000000-0000-0000-0000-000000000001', 'FundsPosted', 75000, 'USD', 'INDIVIDUAL', ?, datetime('now'), datetime('now'))`,
		bizDate)
	if err != nil {
		t.Fatalf("seed FundsPosted transfer: %v", err)
	}

	body := `{"businessDateCT": "` + bizDate + `"}`
	rr := doRequest(r, "POST", "/api/v1/settlement/batches/generate", []byte(body))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var batch map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &batch)
	if _, ok := batch["ID"]; !ok {
		t.Error("missing ID in batch response")
	}
	if batch["Status"] != "GENERATED" {
		t.Errorf("expected Status=GENERATED, got %v", batch["Status"])
	}
	if int(batch["TotalItems"].(float64)) != 1 {
		t.Errorf("expected TotalItems=1, got %v", batch["TotalItems"])
	}
}

func TestHandlers_SubmitDeposit_MissingFields(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	// Missing investorAccountId — send a proper multipart form
	body := &bytes.Buffer{}
	// Use a minimal multipart body with just amount
	body.WriteString("--boundary\r\n")
	body.WriteString("Content-Disposition: form-data; name=\"amount\"\r\n\r\n")
	body.WriteString("100.00\r\n")
	body.WriteString("--boundary--\r\n")

	req := httptest.NewRequest("POST", "/api/v1/deposits", body)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing investorAccountId, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandlers_SubmitDeposit_InvalidAmount(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	body := &bytes.Buffer{}
	body.WriteString("--boundary\r\n")
	body.WriteString("Content-Disposition: form-data; name=\"investorAccountId\"\r\n\r\n")
	body.WriteString("INV-1001\r\n")
	body.WriteString("--boundary\r\n")
	body.WriteString("Content-Disposition: form-data; name=\"amount\"\r\n\r\n")
	body.WriteString("not-a-number\r\n")
	body.WriteString("--boundary--\r\n")

	req := httptest.NewRequest("POST", "/api/v1/deposits", body)
	req.Header.Set("Content-Type", "multipart/form-data; boundary=boundary")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid amount, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestHandlers_ResponseContentType_IsJSON(t *testing.T) {
	db := newTestDB(t)
	r := newRouter(t, db)

	endpoints := []string{
		"/api/v1/deposits",
		"/api/v1/operator/review-queue",
		"/api/v1/ledger/accounts",
		"/api/v1/settlement/batches",
		"/api/v1/audit",
		"/api/v1/metrics",
	}
	for _, ep := range endpoints {
		rr := doRequest(r, "GET", ep, nil)
		ct := rr.Header().Get("Content-Type")
		if !strings.Contains(ct, "application/json") {
			t.Errorf("GET %s: expected application/json content-type, got %q", ep, ct)
		}
	}
}
