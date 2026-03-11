package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/apex-checkout/mobile-check-deposit/internal/audit"
	"github.com/apex-checkout/mobile-check-deposit/internal/deposits"
	"github.com/apex-checkout/mobile-check-deposit/internal/ledger"
	"github.com/apex-checkout/mobile-check-deposit/internal/returns"
	"github.com/apex-checkout/mobile-check-deposit/internal/settlement"
	"github.com/apex-checkout/mobile-check-deposit/internal/transfers"
	vendorclient "github.com/apex-checkout/mobile-check-deposit/internal/vendorsvc/client"
)

type Handlers struct {
	DB            *sql.DB
	DepositSvc    *deposits.DepositService
	TransferSvc   *transfers.TransferService
	LedgerSvc     *ledger.LedgerService
	SettlementSvc *settlement.SettlementService
	ReturnsSvc    *returns.ReturnsService
}

func (h *Handlers) RegisterRoutes(r chi.Router) {
	// Deposits
	r.Post("/api/v1/deposits", h.submitDeposit)
	r.Get("/api/v1/deposits", h.listDeposits)
	r.Get("/api/v1/deposits/{transferId}", h.getDeposit)
	r.Get("/api/v1/deposits/{transferId}/decision-trace", h.getDecisionTrace)

	// Operator
	r.Get("/api/v1/operator/review-queue", h.getReviewQueue)
	r.Post("/api/v1/operator/transfers/{transferId}/approve", h.approveTransfer)
	r.Post("/api/v1/operator/transfers/{transferId}/reject", h.rejectTransfer)

	// Ledger
	r.Get("/api/v1/ledger/accounts", h.getAccountBalances)
	r.Get("/api/v1/ledger/accounts/{accountId}", h.getAccountDetail)
	r.Get("/api/v1/ledger/journals", h.getJournals)

	// Settlement
	r.Post("/api/v1/settlement/batches/generate", h.generateBatch)
	r.Get("/api/v1/settlement/batches", h.listBatches)
	r.Get("/api/v1/settlement/batches/{batchId}", h.getBatch)
	r.Post("/api/v1/settlement/batches/{batchId}/ack", h.ackBatch)

	// Returns
	r.Post("/api/v1/returns", h.processReturn)

	// Metrics
	r.Get("/api/v1/metrics", h.getMetrics)

	// Audit
	r.Get("/api/v1/audit", h.getAuditLog)

	// Test / Demo
	if os.Getenv("ENABLE_TEST_RESET") == "true" {
		r.Post("/api/v1/test/reset", h.testReset)
		r.Post("/api/v1/test/seed", h.testSeed)
	}
}

// ---------------------------------------------------------------------------
// Deposits
// ---------------------------------------------------------------------------

func (h *Handlers) submitDeposit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		respondError(w, http.StatusBadRequest, "invalid multipart form: "+err.Error())
		return
	}

	investorAccountID := r.FormValue("investorAccountId")
	if investorAccountID == "" {
		respondError(w, http.StatusBadRequest, "investorAccountId is required")
		return
	}

	amountStr := r.FormValue("amount")
	if amountStr == "" {
		respondError(w, http.StatusBadRequest, "amount is required")
		return
	}
	amountCents, err := parseCents(amountStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid amount: "+err.Error())
		return
	}

	frontFile, _, err := r.FormFile("frontImage")
	if err != nil {
		respondError(w, http.StatusBadRequest, "frontImage is required: "+err.Error())
		return
	}
	defer frontFile.Close()

	backFile, _, err := r.FormFile("backImage")
	if err != nil {
		respondError(w, http.StatusBadRequest, "backImage is required: "+err.Error())
		return
	}
	defer backFile.Close()

	result, err := h.DepositSvc.SubmitDeposit(r.Context(), investorAccountID, amountCents, frontFile, backFile)
	if err != nil {
		internalError(w, "submit deposit", err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"transferId":       result.TransferID,
		"state":            string(result.State),
		"reviewRequired":   result.ReviewRequired,
		"businessDateCT":   result.BusinessDateCT,
		"message":          result.Message,
		"rejectionCode":    result.RejectionCode,
		"rejectionMessage": result.RejectionMessage,
	})
}

func (h *Handlers) getDeposit(w http.ResponseWriter, r *http.Request) {
	transferID := chi.URLParam(r, "transferId")

	t, err := h.TransferSvc.GetByID(h.DB, transferID)
	if err != nil {
		respondError(w, http.StatusNotFound, "transfer not found: "+err.Error())
		return
	}

	vendorResult, _ := vendorclient.GetVendorResult(h.DB, transferID)

	ruleEvals, _ := getRuleEvaluations(h.DB, transferID)

	auditEvents, _ := audit.GetByEntity(h.DB, "transfer", transferID)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"transfer":        t,
		"vendorResult":    vendorResult,
		"ruleEvaluations": ruleEvals,
		"auditEvents":     auditEvents,
	})
}

func (h *Handlers) listDeposits(w http.ResponseWriter, r *http.Request) {
	var filters transfers.TransferFilters

	if v := r.URL.Query().Get("state"); v != "" {
		s := transfers.State(v)
		filters.State = &s
	}
	if v := r.URL.Query().Get("investorAccountId"); v != "" {
		filters.InvestorAccountID = &v
	}
	if v := r.URL.Query().Get("reviewRequired"); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			filters.ReviewRequired = &b
		}
	}
	if v := r.URL.Query().Get("reviewStatus"); v != "" {
		filters.ReviewStatus = &v
	}

	list, err := h.TransferSvc.List(h.DB, filters)
	if err != nil {
		internalError(w, "list deposits", err)
		return
	}

	// Ensure nil slice serializes as [] not null
	if list == nil {
		list = []transfers.Transfer{}
	}
	respondJSON(w, http.StatusOK, list)
}

func (h *Handlers) getDecisionTrace(w http.ResponseWriter, r *http.Request) {
	transferID := chi.URLParam(r, "transferId")

	events, err := audit.GetByEntity(h.DB, "transfer", transferID)
	if err != nil {
		internalError(w, "get decision trace", err)
		return
	}

	respondJSON(w, http.StatusOK, events)
}

// ---------------------------------------------------------------------------
// Operator
// ---------------------------------------------------------------------------

func (h *Handlers) getReviewQueue(w http.ResponseWriter, r *http.Request) {
	state := transfers.StateAnalyzing
	reviewRequired := true
	reviewStatus := "PENDING"

	list, err := h.TransferSvc.List(h.DB, transfers.TransferFilters{
		State:          &state,
		ReviewRequired: &reviewRequired,
		ReviewStatus:   &reviewStatus,
	})
	if err != nil {
		internalError(w, "get review queue", err)
		return
	}

	if list == nil {
		list = []transfers.Transfer{}
	}
	respondJSON(w, http.StatusOK, list)
}

func (h *Handlers) approveTransfer(w http.ResponseWriter, r *http.Request) {
	transferID := chi.URLParam(r, "transferId")

	var body struct {
		OperatorID             string `json:"operatorId"`
		Notes                  string `json:"notes"`
		OverrideContributionType string `json:"overrideContributionType"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if body.OperatorID == "" {
		respondError(w, http.StatusBadRequest, "operatorId is required")
		return
	}

	t, err := h.TransferSvc.GetByID(h.DB, transferID)
	if err != nil {
		respondError(w, http.StatusNotFound, "transfer not found: "+err.Error())
		return
	}

	// 1. Create operator_action row
	now := time.Now().UTC()
	action := "APPROVE"
	var overridePtr *string
	if body.OverrideContributionType != "" {
		overridePtr = &body.OverrideContributionType
		action = "OVERRIDE_CONTRIBUTION"
		_, err = h.DB.Exec("UPDATE transfers SET contribution_type = ?, updated_at = ? WHERE id = ?",
			body.OverrideContributionType, now, transferID)
		if err != nil {
			internalError(w, "update contribution type", err)
			return
		}
	}

	_, err = h.DB.Exec(`
		INSERT INTO operator_actions (id, transfer_id, operator_id, action, notes, override_contribution_type, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), transferID, body.OperatorID, action, body.Notes, overridePtr, now)
	if err != nil {
		internalError(w, "create operator action", err)
		return
	}

	// 2. Update review_status to APPROVED
	if err := h.TransferSvc.UpdateReviewStatus(h.DB, transferID, "APPROVED"); err != nil {
		internalError(w, "update review status", err)
		return
	}

	// 3. Transition to Approved
	if err := h.TransferSvc.Transition(h.DB, transferID, transfers.StateApproved, "OPERATOR", body.OperatorID); err != nil {
		if strings.Contains(err.Error(), "invalid transition") {
			respondError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		internalError(w, "transition to Approved", err)
		return
	}
	h.DB.Exec("UPDATE transfers SET approved_at = ? WHERE id = ?", now, transferID)

	// 4. Post ledger deposit
	if err := h.LedgerSvc.PostDeposit(h.DB, transferID, t.InvestorAccountID, t.OmnibusAccountID, t.AmountCents); err != nil {
		internalError(w, "post deposit", err)
		return
	}

	// 5. Transition to FundsPosted
	if err := h.TransferSvc.Transition(h.DB, transferID, transfers.StateFundsPosted, "OPERATOR", body.OperatorID); err != nil {
		internalError(w, "transition to FundsPosted", err)
		return
	}
	h.DB.Exec("UPDATE transfers SET posted_at = ? WHERE id = ?", now, transferID)

	// 6. Create audit event for operator action
	detailsJSON := fmt.Sprintf(`{"notes":%q,"overrideContributionType":%q}`, body.Notes, body.OverrideContributionType)
	audit.LogEvent(h.DB, audit.Event{
		EntityType:  "transfer",
		EntityID:    transferID,
		ActorType:   "OPERATOR",
		ActorID:     body.OperatorID,
		EventType:   "OPERATOR_APPROVE",
		DetailsJSON: &detailsJSON,
		CreatedAt:   now,
	})

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"transferId": transferID,
		"state":      string(transfers.StateFundsPosted),
		"message":    "Transfer approved and funds posted",
	})
}

func (h *Handlers) rejectTransfer(w http.ResponseWriter, r *http.Request) {
	transferID := chi.URLParam(r, "transferId")

	var body struct {
		OperatorID string `json:"operatorId"`
		Notes      string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if body.OperatorID == "" {
		respondError(w, http.StatusBadRequest, "operatorId is required")
		return
	}

	// Verify transfer exists before taking any action
	if _, err := h.TransferSvc.GetByID(h.DB, transferID); err != nil {
		respondError(w, http.StatusNotFound, "transfer not found: "+err.Error())
		return
	}

	// 1. Create operator_action row
	now := time.Now().UTC()
	_, err := h.DB.Exec(`
		INSERT INTO operator_actions (id, transfer_id, operator_id, action, notes, created_at)
		VALUES (?, ?, ?, 'REJECT', ?, ?)`,
		uuid.New().String(), transferID, body.OperatorID, body.Notes, now)
	if err != nil {
		internalError(w, "create operator action", err)
		return
	}

	// 2. Update review_status to REJECTED
	if err := h.TransferSvc.UpdateReviewStatus(h.DB, transferID, "REJECTED"); err != nil {
		internalError(w, "update review status", err)
		return
	}

	// 3. Transition to Rejected
	if err := h.TransferSvc.Transition(h.DB, transferID, transfers.StateRejected, "OPERATOR", body.OperatorID); err != nil {
		if strings.Contains(err.Error(), "invalid transition") {
			respondError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		internalError(w, "transition to Rejected", err)
		return
	}

	// 4. Create audit event
	detailsJSON := fmt.Sprintf(`{"notes":%q}`, body.Notes)
	audit.LogEvent(h.DB, audit.Event{
		EntityType:  "transfer",
		EntityID:    transferID,
		ActorType:   "OPERATOR",
		ActorID:     body.OperatorID,
		EventType:   "OPERATOR_REJECT",
		DetailsJSON: &detailsJSON,
		CreatedAt:   now,
	})

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"transferId": transferID,
		"state":      string(transfers.StateRejected),
		"message":    "Transfer rejected by operator",
	})
}

// ---------------------------------------------------------------------------
// Ledger
// ---------------------------------------------------------------------------

func (h *Handlers) getAccountBalances(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(`
		SELECT a.id, a.external_account_id, a.account_name, a.account_type, a.status,
		       COALESCE(SUM(e.signed_amount_cents), 0) AS balance_cents
		FROM accounts a
		LEFT JOIN ledger_entries e ON e.account_id = a.id
		GROUP BY a.id
		ORDER BY a.account_type, a.account_name`)
	if err != nil {
		internalError(w, "get account balances", err)
		return
	}
	defer rows.Close()

	type accountBalance struct {
		ID                string `json:"id"`
		ExternalAccountID string `json:"externalAccountId"`
		AccountName       string `json:"accountName"`
		AccountType       string `json:"accountType"`
		Status            string `json:"status"`
		BalanceCents      int64  `json:"balanceCents"`
	}
	var balances []accountBalance
	for rows.Next() {
		var ab accountBalance
		if err := rows.Scan(&ab.ID, &ab.ExternalAccountID, &ab.AccountName, &ab.AccountType, &ab.Status, &ab.BalanceCents); err != nil {
			internalError(w, "scan account balance", err)
			return
		}
		balances = append(balances, ab)
	}
	if err := rows.Err(); err != nil {
		internalError(w, "iterate account balances", err)
		return
	}

	respondJSON(w, http.StatusOK, balances)
}

func (h *Handlers) getAccountDetail(w http.ResponseWriter, r *http.Request) {
	accountID := chi.URLParam(r, "accountId")

	var acct struct {
		ID                string `json:"id"`
		ExternalAccountID string `json:"externalAccountId"`
		AccountName       string `json:"accountName"`
		AccountType       string `json:"accountType"`
		Status            string `json:"status"`
	}
	err := h.DB.QueryRow(`
		SELECT id, external_account_id, account_name, account_type, status
		FROM accounts WHERE id = ?`, accountID).
		Scan(&acct.ID, &acct.ExternalAccountID, &acct.AccountName, &acct.AccountType, &acct.Status)
	if err != nil {
		respondError(w, http.StatusNotFound, "account not found: "+err.Error())
		return
	}

	rows, err := h.DB.Query(`
		SELECT e.id, e.journal_id, e.account_id, e.signed_amount_cents, e.currency, e.line_type, e.source_application_id, e.created_at
		FROM ledger_entries e
		WHERE e.account_id = ?
		ORDER BY e.created_at ASC`, accountID)
	if err != nil {
		internalError(w, "get account entries", err)
		return
	}
	defer rows.Close()

	var entries []ledger.Entry
	for rows.Next() {
		var e ledger.Entry
		if err := rows.Scan(&e.ID, &e.JournalID, &e.AccountID, &e.SignedAmountCents, &e.Currency, &e.LineType, &e.SourceApplicationID, &e.CreatedAt); err != nil {
			internalError(w, "scan ledger entry", err)
			return
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		internalError(w, "iterate ledger entries", err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"account": acct,
		"entries": entries,
	})
}

func (h *Handlers) getJournals(w http.ResponseWriter, r *http.Request) {
	transferID := r.URL.Query().Get("transferId")
	if transferID == "" {
		respondError(w, http.StatusBadRequest, "transferId query param is required")
		return
	}

	journals, err := h.LedgerSvc.GetJournalsByTransfer(h.DB, transferID)
	if err != nil {
		internalError(w, "get journals", err)
		return
	}

	respondJSON(w, http.StatusOK, journals)
}

// ---------------------------------------------------------------------------
// Settlement
// ---------------------------------------------------------------------------

func (h *Handlers) generateBatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BusinessDateCT string `json:"businessDateCT"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if body.BusinessDateCT == "" {
		loc, err := time.LoadLocation("America/Chicago")
		if err != nil {
			internalError(w, "load timezone", err)
			return
		}
		body.BusinessDateCT = time.Now().In(loc).Format("2006-01-02")
	}

	batch, err := h.SettlementSvc.GenerateBatch(r.Context(), body.BusinessDateCT)
	if err != nil {
		if strings.Contains(err.Error(), "no eligible transfers") {
			respondError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		internalError(w, "generate batch", err)
		return
	}

	respondJSON(w, http.StatusOK, batch)
}

func (h *Handlers) listBatches(w http.ResponseWriter, r *http.Request) {
	batches, err := h.SettlementSvc.ListBatches(r.Context())
	if err != nil {
		internalError(w, "list batches", err)
		return
	}

	// Ensure nil slice serializes as [] not null
	if batches == nil {
		batches = []settlement.Batch{}
	}
	respondJSON(w, http.StatusOK, batches)
}

func (h *Handlers) getBatch(w http.ResponseWriter, r *http.Request) {
	batchID := chi.URLParam(r, "batchId")

	batch, items, err := h.SettlementSvc.GetBatch(r.Context(), batchID)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"batch": batch,
		"items": items,
	})
}

func (h *Handlers) ackBatch(w http.ResponseWriter, r *http.Request) {
	batchID := chi.URLParam(r, "batchId")

	var body struct {
		AckReference string `json:"ackReference"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if body.AckReference == "" {
		respondError(w, http.StatusBadRequest, "ackReference is required")
		return
	}

	if err := h.SettlementSvc.AcknowledgeBatch(r.Context(), batchID, body.AckReference); err != nil {
		// AcknowledgeBatch returns "not found" when batch doesn't exist or isn't GENERATED
		if strings.Contains(err.Error(), "not found") {
			respondError(w, http.StatusNotFound, err.Error())
			return
		}
		internalError(w, "ack batch", err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"batchId": batchID,
		"status":  "ACKNOWLEDGED",
		"message": "Batch acknowledged successfully",
	})
}

// ---------------------------------------------------------------------------
// Returns
// ---------------------------------------------------------------------------

func (h *Handlers) processReturn(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TransferID string `json:"transferId"`
		ReasonCode string `json:"reasonCode"`
		ReasonText string `json:"reasonText"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if body.TransferID == "" {
		respondError(w, http.StatusBadRequest, "transferId is required")
		return
	}
	if body.ReasonCode == "" {
		respondError(w, http.StatusBadRequest, "reasonCode is required")
		return
	}

	if err := h.ReturnsSvc.ProcessReturn(r.Context(), body.TransferID, body.ReasonCode, body.ReasonText); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "get transfer") {
			respondError(w, http.StatusNotFound, "transfer not found")
			return
		}
		if strings.Contains(errMsg, "not eligible for return") {
			respondError(w, http.StatusUnprocessableEntity, errMsg)
			return
		}
		internalError(w, "process return", err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"transferId": body.TransferID,
		"status":     "RETURNED",
		"message":    "Return processed successfully",
	})
}

// ---------------------------------------------------------------------------
// Test
// ---------------------------------------------------------------------------

func (h *Handlers) testReset(w http.ResponseWriter, r *http.Request) {
	tables := []string{
		"settlement_batch_items",
		"settlement_batches",
		"return_notifications",
		"notifications_outbox",
		"ledger_entries",
		"ledger_journals",
		"operator_actions",
		"rule_evaluations",
		"vendor_results",
		"transfer_images",
		"audit_events",
		"transfers",
	}
	for _, t := range tables {
		if _, err := h.DB.Exec("DELETE FROM " + t); err != nil {
			internalError(w, "reset "+t, err)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// SeedDemoData inserts demo transfers if they don't already exist.
// Returns the number of rows inserted.
func SeedDemoData(db *sql.DB) (int, error) {
	now := time.Now().UTC()
	day := func(d int) time.Time { return now.AddDate(0, 0, d) }
	ptr := func(s string) *string { return &s }

	acctID := func(extID string) string {
		var id string
		db.QueryRow("SELECT id FROM accounts WHERE external_account_id = ?", extID).Scan(&id)
		return id
	}

	seeds := []struct {
		id, acct, state, bizDate string
		cents                    int64
		rejCode, rejMsg          *string
		reviewRequired           bool
		reviewStatus             *string
		submittedAt, approvedAt, postedAt, completedAt *time.Time
	}{
		{id: "demo-seed-0001", acct: acctID("INV-1001"), state: "Completed", bizDate: day(-7).Format("2006-01-02"), cents: 125000,
			submittedAt: timePtr(day(-7)), approvedAt: timePtr(day(-7)), postedAt: timePtr(day(-7)), completedAt: timePtr(day(-7))},
		{id: "demo-seed-0002", acct: acctID("INV-1002"), state: "Completed", bizDate: day(-5).Format("2006-01-02"), cents: 75000,
			submittedAt: timePtr(day(-5)), approvedAt: timePtr(day(-5)), postedAt: timePtr(day(-5)), completedAt: timePtr(day(-5))},
		{id: "demo-seed-0003", acct: acctID("INV-1003"), state: "Returned", bizDate: day(-3).Format("2006-01-02"), cents: 200000,
			submittedAt: timePtr(day(-3)), approvedAt: timePtr(day(-3)), postedAt: timePtr(day(-3)), completedAt: timePtr(day(-3))},
		{id: "demo-seed-0004", acct: acctID("INV-1001"), state: "FundsPosted", bizDate: now.Format("2006-01-02"), cents: 350000,
			submittedAt: timePtr(now.Add(-2 * time.Hour)), approvedAt: timePtr(now.Add(-2 * time.Hour)), postedAt: timePtr(now.Add(-2 * time.Hour))},
		{id: "demo-seed-0005", acct: acctID("INV-1004"), state: "FundsPosted", bizDate: now.Format("2006-01-02"), cents: 50000,
			submittedAt: timePtr(now.Add(-1 * time.Hour)), approvedAt: timePtr(now.Add(-1 * time.Hour)), postedAt: timePtr(now.Add(-1 * time.Hour))},
		{id: "demo-seed-0006", acct: acctID("INV-1006"), state: "Analyzing", bizDate: now.Format("2006-01-02"), cents: 480000,
			reviewRequired: true, reviewStatus: ptr("PENDING"),
			submittedAt: timePtr(now.Add(-90 * time.Minute))},
		{id: "demo-seed-0007", acct: acctID("INV-1007"), state: "Analyzing", bizDate: now.Format("2006-01-02"), cents: 95000,
			reviewRequired: true, reviewStatus: ptr("PENDING"),
			submittedAt: timePtr(now.Add(-20 * time.Minute))},
		{id: "demo-seed-0008", acct: acctID("INV-1005"), state: "Rejected", bizDate: now.Format("2006-01-02"), cents: 20000,
			rejCode: ptr("DUPLICATE_DETECTED"), rejMsg: ptr("Fingerprint matches transfer demo-seed-0001"),
			submittedAt: timePtr(now.Add(-3 * time.Hour))},
		{id: "demo-seed-0009", acct: acctID("INV-1002"), state: "Rejected", bizDate: day(-1).Format("2006-01-02"), cents: 15000,
			rejCode: ptr("IQA_BLUR"), rejMsg: ptr("Image quality score 0.23 below threshold 0.60"),
			submittedAt: timePtr(day(-1))},
	}

	const correspondentID = "00000000-0000-0000-0000-000000000010"
	const omnibusID = "00000000-0000-0000-0000-000000000001"

	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	inserted := 0
	for _, s := range seeds {
		if s.acct == "" {
			continue
		}
		var exists int
		tx.QueryRow("SELECT COUNT(*) FROM transfers WHERE id = ?", s.id).Scan(&exists)
		if exists > 0 {
			continue
		}
		_, err := tx.Exec(`INSERT INTO transfers
			(id, investor_account_id, correspondent_id, omnibus_account_id, state,
			 amount_cents, currency, contribution_type, business_date_ct,
			 review_required, review_status,
			 rejection_code, rejection_message,
			 duplicate_fingerprint,
			 submitted_at, approved_at, posted_at, completed_at,
			 created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, 'USD', 'INDIVIDUAL', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			s.id, s.acct, correspondentID, omnibusID, s.state,
			s.cents, s.bizDate,
			s.reviewRequired, s.reviewStatus,
			s.rejCode, s.rejMsg,
			"demo-fp-"+s.id,
			s.submittedAt, s.approvedAt, s.postedAt, s.completedAt,
			now, now,
		)
		if err != nil {
			return 0, err
		}
		inserted++
	}
	return inserted, tx.Commit()
}

// testSeed inserts demo transfers in various states for demonstration purposes.
func (h *Handlers) testSeed(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	day := func(d int) time.Time { return now.AddDate(0, 0, d) }
	ptr := func(s string) *string { return &s }

	// Look up account UUIDs by external_account_id.
	acctID := func(extID string) string {
		var id string
		h.DB.QueryRow("SELECT id FROM accounts WHERE external_account_id = ?", extID).Scan(&id)
		return id
	}

	seeds := []struct {
		id, acct, state, bizDate string
		cents                    int64
		rejCode, rejMsg          *string
		reviewRequired           bool
		reviewStatus             *string
		submittedAt, approvedAt, postedAt, completedAt *time.Time
	}{
		// Completed — settled last week
		{id: "demo-seed-0001", acct: acctID("INV-1001"), state: "Completed", bizDate: day(-7).Format("2006-01-02"), cents: 125000,
			submittedAt: timePtr(day(-7)), approvedAt: timePtr(day(-7)), postedAt: timePtr(day(-7)), completedAt: timePtr(day(-7))},
		{id: "demo-seed-0002", acct: acctID("INV-1002"), state: "Completed", bizDate: day(-5).Format("2006-01-02"), cents: 75000,
			submittedAt: timePtr(day(-5)), approvedAt: timePtr(day(-5)), postedAt: timePtr(day(-5)), completedAt: timePtr(day(-5))},
		// Returned
		{id: "demo-seed-0003", acct: acctID("INV-1003"), state: "Returned", bizDate: day(-3).Format("2006-01-02"), cents: 200000,
			submittedAt: timePtr(day(-3)), approvedAt: timePtr(day(-3)), postedAt: timePtr(day(-3)), completedAt: timePtr(day(-3))},
		// FundsPosted — today, awaiting settlement
		{id: "demo-seed-0004", acct: acctID("INV-1001"), state: "FundsPosted", bizDate: now.Format("2006-01-02"), cents: 350000,
			submittedAt: timePtr(now.Add(-2 * time.Hour)), approvedAt: timePtr(now.Add(-2 * time.Hour)), postedAt: timePtr(now.Add(-2 * time.Hour))},
		{id: "demo-seed-0005", acct: acctID("INV-1004"), state: "FundsPosted", bizDate: now.Format("2006-01-02"), cents: 50000,
			submittedAt: timePtr(now.Add(-1 * time.Hour)), approvedAt: timePtr(now.Add(-1 * time.Hour)), postedAt: timePtr(now.Add(-1 * time.Hour))},
		// Pending review — Analyzing state with review_required=true, review_status=PENDING
		{id: "demo-seed-0006", acct: acctID("INV-1006"), state: "Analyzing", bizDate: now.Format("2006-01-02"), cents: 480000,
			reviewRequired: true, reviewStatus: ptr("PENDING"),
			submittedAt: timePtr(now.Add(-90 * time.Minute))},
		{id: "demo-seed-0007", acct: acctID("INV-1007"), state: "Analyzing", bizDate: now.Format("2006-01-02"), cents: 95000,
			reviewRequired: true, reviewStatus: ptr("PENDING"),
			submittedAt: timePtr(now.Add(-20 * time.Minute))},
		// Rejected
		{id: "demo-seed-0008", acct: acctID("INV-1005"), state: "Rejected", bizDate: now.Format("2006-01-02"), cents: 20000,
			rejCode: ptr("DUPLICATE_DETECTED"), rejMsg: ptr("Fingerprint matches transfer demo-seed-0001"),
			submittedAt: timePtr(now.Add(-3 * time.Hour))},
		{id: "demo-seed-0009", acct: acctID("INV-1002"), state: "Rejected", bizDate: day(-1).Format("2006-01-02"), cents: 15000,
			rejCode: ptr("IQA_BLUR"), rejMsg: ptr("Image quality score 0.23 below threshold 0.60"),
			submittedAt: timePtr(day(-1))},
	}

	tx, err := h.DB.Begin()
	if err != nil {
		internalError(w, "begin tx", err)
		return
	}
	defer tx.Rollback()

	const correspondentID = "00000000-0000-0000-0000-000000000010"
	const omnibusID = "00000000-0000-0000-0000-000000000001"

	inserted := 0
	for _, s := range seeds {
		if s.acct == "" {
			continue // account not found, skip
		}
		var exists int
		tx.QueryRow("SELECT COUNT(*) FROM transfers WHERE id = ?", s.id).Scan(&exists)
		if exists > 0 {
			continue
		}
		_, err := tx.Exec(`INSERT INTO transfers
			(id, investor_account_id, correspondent_id, omnibus_account_id, state,
			 amount_cents, currency, contribution_type, business_date_ct,
			 review_required, review_status,
			 rejection_code, rejection_message,
			 duplicate_fingerprint,
			 submitted_at, approved_at, posted_at, completed_at,
			 created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, 'USD', 'INDIVIDUAL', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			s.id, s.acct, correspondentID, omnibusID, s.state,
			s.cents, s.bizDate,
			s.reviewRequired, s.reviewStatus,
			s.rejCode, s.rejMsg,
			"demo-fp-"+s.id,
			s.submittedAt, s.approvedAt, s.postedAt, s.completedAt,
			now, now,
		)
		if err != nil {
			internalError(w, "insert seed "+s.id, err)
			return
		}
		inserted++
	}

	if err := tx.Commit(); err != nil {
		internalError(w, "commit seed", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok", "inserted": inserted})
}

func timePtr(t time.Time) *time.Time { return &t }

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

// internalError logs the full error server-side and returns a generic message to the client.
func internalError(w http.ResponseWriter, op string, err error) {
	slog.Error("internal error", "op", op, "err", err)
	respondError(w, http.StatusInternalServerError, "an internal error occurred")
}

func parseCents(s string) (int64, error) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("parse amount %q: %w", s, err)
	}
	cents := int64(math.Round(f * 100))
	if cents <= 0 {
		return 0, fmt.Errorf("amount must be positive")
	}
	const maxCents = 10_000_000 // $100,000.00
	if cents > maxCents {
		return 0, fmt.Errorf("amount exceeds maximum of $100,000.00")
	}
	return cents, nil
}

type ruleEvaluation struct {
	ID          string `json:"id"`
	TransferID  string `json:"transferId"`
	RuleName    string `json:"ruleName"`
	Outcome     string `json:"outcome"`
	DetailsJSON string `json:"detailsJson"`
	CreatedAt   string `json:"createdAt"`
}

func getRuleEvaluations(db *sql.DB, transferID string) ([]ruleEvaluation, error) {
	rows, err := db.Query(`
		SELECT id, transfer_id, rule_name, outcome, COALESCE(details_json, ''), created_at
		FROM rule_evaluations
		WHERE transfer_id = ?
		ORDER BY created_at ASC`, transferID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var evals []ruleEvaluation
	for rows.Next() {
		var e ruleEvaluation
		if err := rows.Scan(&e.ID, &e.TransferID, &e.RuleName, &e.Outcome, &e.DetailsJSON, &e.CreatedAt); err != nil {
			return nil, err
		}
		evals = append(evals, e)
	}
	return evals, rows.Err()
}

// ---------------------------------------------------------------------------
// Metrics
// ---------------------------------------------------------------------------

func (h *Handlers) getMetrics(w http.ResponseWriter, r *http.Request) {
	// Counts by state
	stateRows, err := h.DB.QueryContext(r.Context(), `SELECT state, COUNT(*) FROM transfers GROUP BY state`)
	if err != nil {
		internalError(w, "getMetrics/states", err)
		return
	}
	defer stateRows.Close()

	byState := make(map[string]int)
	total := 0
	for stateRows.Next() {
		var s string
		var c int
		if scanErr := stateRows.Scan(&s, &c); scanErr == nil {
			byState[s] = c
			total += c
		}
	}

	var fundsPostedCents, totalVolumeCents int64
	h.DB.QueryRowContext(r.Context(), `SELECT COALESCE(SUM(amount_cents),0) FROM transfers WHERE state='FundsPosted'`).Scan(&fundsPostedCents)
	h.DB.QueryRowContext(r.Context(), `SELECT COALESCE(SUM(amount_cents),0) FROM transfers WHERE state NOT IN ('Rejected','Returned')`).Scan(&totalVolumeCents)

	var pendingReview int
	h.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM transfers WHERE state='Analyzing' AND review_required=1 AND review_status='PENDING'`).Scan(&pendingReview)

	exceptions := byState["Rejected"] + byState["Returned"]

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"transfers": map[string]interface{}{
			"total":           total,
			"by_state":        byState,
			"pending_review":  pendingReview,
			"exceptions":      exceptions,
			"funds_posted":    byState["FundsPosted"],
			"completed":       byState["Completed"],
		},
		"volume": map[string]interface{}{
			"total_cents":        totalVolumeCents,
			"funds_posted_cents": fundsPostedCents,
		},
		"generated_at": time.Now().UTC().Format(time.RFC3339),
	})
}

// ---------------------------------------------------------------------------
// Audit Log
// ---------------------------------------------------------------------------

// getAuditLog returns recent audit events.
// Query params:
//   - transferId: filter by transfer entity ID
//   - limit:      max events to return (default 100, max 500)
func (h *Handlers) getAuditLog(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	limit := 100
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	var rows *sql.Rows
	var err error
	if tid := q.Get("transferId"); tid != "" {
		rows, err = h.DB.QueryContext(r.Context(), `
			SELECT id, entity_type, entity_id, actor_type, actor_id,
			       event_type, from_state, to_state, details_json, created_at
			FROM audit_events
			WHERE entity_type = 'transfer' AND entity_id = ?
			ORDER BY created_at ASC
			LIMIT ?`, tid, limit)
	} else {
		rows, err = h.DB.QueryContext(r.Context(), `
			SELECT id, entity_type, entity_id, actor_type, actor_id,
			       event_type, from_state, to_state, details_json, created_at
			FROM audit_events
			ORDER BY created_at DESC
			LIMIT ?`, limit)
	}
	if err != nil {
		internalError(w, "getAuditLog", err)
		return
	}
	defer rows.Close()

	type auditEntry struct {
		ID          string  `json:"id"`
		EntityType  string  `json:"entityType"`
		EntityID    string  `json:"entityId"`
		ActorType   string  `json:"actorType"`
		ActorID     string  `json:"actorId"`
		EventType   string  `json:"eventType"`
		FromState   *string `json:"fromState"`
		ToState     *string `json:"toState"`
		DetailsJSON *string `json:"detailsJson"`
		CreatedAt   string  `json:"createdAt"`
	}

	var events []auditEntry
	for rows.Next() {
		var e auditEntry
		var createdAt time.Time
		if scanErr := rows.Scan(&e.ID, &e.EntityType, &e.EntityID, &e.ActorType, &e.ActorID,
			&e.EventType, &e.FromState, &e.ToState, &e.DetailsJSON, &createdAt); scanErr == nil {
			e.CreatedAt = createdAt.UTC().Format(time.RFC3339)
			events = append(events, e)
		}
	}
	if events == nil {
		events = []auditEntry{}
	}
	respondJSON(w, http.StatusOK, events)
}
