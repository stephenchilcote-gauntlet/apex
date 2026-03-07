package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
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

func (h *Handlers) Routes() chi.Router {
	r := chi.NewRouter()

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

	return r
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

	var vendorScenario *string
	if v := r.FormValue("vendorScenario"); v != "" {
		vendorScenario = &v
	}

	result, err := h.DepositSvc.SubmitDeposit(r.Context(), investorAccountID, amountCents, frontFile, backFile, vendorScenario)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
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
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, list)
}

func (h *Handlers) getDecisionTrace(w http.ResponseWriter, r *http.Request) {
	transferID := chi.URLParam(r, "transferId")

	events, err := audit.GetByEntity(h.DB, "transfer", transferID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
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
		respondError(w, http.StatusInternalServerError, err.Error())
		return
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
			respondError(w, http.StatusInternalServerError, "update contribution type: "+err.Error())
			return
		}
	}

	_, err = h.DB.Exec(`
		INSERT INTO operator_actions (id, transfer_id, operator_id, action, notes, override_contribution_type, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), transferID, body.OperatorID, action, body.Notes, overridePtr, now)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "create operator action: "+err.Error())
		return
	}

	// 2. Update review_status to APPROVED
	if err := h.TransferSvc.UpdateReviewStatus(h.DB, transferID, "APPROVED"); err != nil {
		respondError(w, http.StatusInternalServerError, "update review status: "+err.Error())
		return
	}

	// 3. Transition to Approved
	if err := h.TransferSvc.Transition(h.DB, transferID, transfers.StateApproved, "OPERATOR", body.OperatorID); err != nil {
		respondError(w, http.StatusInternalServerError, "transition to Approved: "+err.Error())
		return
	}
	h.DB.Exec("UPDATE transfers SET approved_at = ? WHERE id = ?", now, transferID)

	// 4. Post ledger deposit
	if err := h.LedgerSvc.PostDeposit(h.DB, transferID, t.InvestorAccountID, t.OmnibusAccountID, t.AmountCents); err != nil {
		respondError(w, http.StatusInternalServerError, "post deposit: "+err.Error())
		return
	}

	// 5. Transition to FundsPosted
	if err := h.TransferSvc.Transition(h.DB, transferID, transfers.StateFundsPosted, "OPERATOR", body.OperatorID); err != nil {
		respondError(w, http.StatusInternalServerError, "transition to FundsPosted: "+err.Error())
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

	// 1. Create operator_action row
	now := time.Now().UTC()
	_, err := h.DB.Exec(`
		INSERT INTO operator_actions (id, transfer_id, operator_id, action, notes, created_at)
		VALUES (?, ?, ?, 'REJECT', ?, ?)`,
		uuid.New().String(), transferID, body.OperatorID, body.Notes, now)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "create operator action: "+err.Error())
		return
	}

	// 2. Update review_status to REJECTED
	if err := h.TransferSvc.UpdateReviewStatus(h.DB, transferID, "REJECTED"); err != nil {
		respondError(w, http.StatusInternalServerError, "update review status: "+err.Error())
		return
	}

	// 3. Transition to Rejected
	if err := h.TransferSvc.Transition(h.DB, transferID, transfers.StateRejected, "OPERATOR", body.OperatorID); err != nil {
		respondError(w, http.StatusInternalServerError, "transition to Rejected: "+err.Error())
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
		respondError(w, http.StatusInternalServerError, err.Error())
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
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		balances = append(balances, ab)
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
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
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var entries []ledger.Entry
	for rows.Next() {
		var e ledger.Entry
		if err := rows.Scan(&e.ID, &e.JournalID, &e.AccountID, &e.SignedAmountCents, &e.Currency, &e.LineType, &e.SourceApplicationID, &e.CreatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
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
		respondError(w, http.StatusInternalServerError, err.Error())
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
			respondError(w, http.StatusInternalServerError, "load timezone: "+err.Error())
			return
		}
		body.BusinessDateCT = time.Now().In(loc).Format("2006-01-02")
	}

	batch, err := h.SettlementSvc.GenerateBatch(r.Context(), body.BusinessDateCT)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, batch)
}

func (h *Handlers) listBatches(w http.ResponseWriter, r *http.Request) {
	batches, err := h.SettlementSvc.ListBatches(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
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
		respondError(w, http.StatusInternalServerError, err.Error())
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
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"transferId": body.TransferID,
		"status":     "RETURNED",
		"message":    "Return processed successfully",
	})
}

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

func parseCents(s string) (int64, error) {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("parse amount %q: %w", s, err)
	}
	return int64(math.Round(f * 100)), nil
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
