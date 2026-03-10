package ui

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"math"
	"net/http"
	"path/filepath"
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

type UIHandlers struct {
	DB            *sql.DB
	TemplateDir   string
	ImageDir      string
	DepositSvc    *deposits.DepositService
	TransferSvc   *transfers.TransferService
	LedgerSvc     *ledger.LedgerService
	SettlementSvc *settlement.SettlementService
	ReturnsSvc    *returns.ReturnsService
	templates     map[string]*template.Template
}

type ruleEval struct {
	RuleName    string
	Outcome     string
	DetailsJSON string
}

type accountBalance struct {
	ID                string
	ExternalAccountID string
	AccountName       string
	AccountType       string
	BalanceCents      int64
}

func (h *UIHandlers) Init() error {
	funcMap := template.FuncMap{
		"formatCents": func(cents int64) string {
			negative := cents < 0
			if negative {
				cents = -cents
			}
			whole := cents / 100
			frac := cents % 100
			s := fmt.Sprintf("%d.%02d", whole, frac)
			// add commas to whole part
			parts := strings.SplitN(s, ".", 2)
			wholeStr := parts[0]
			if len(wholeStr) > 3 {
				var buf strings.Builder
				rem := len(wholeStr) % 3
				if rem > 0 {
					buf.WriteString(wholeStr[:rem])
				}
				for i := rem; i < len(wholeStr); i += 3 {
					if buf.Len() > 0 {
						buf.WriteByte(',')
					}
					buf.WriteString(wholeStr[i : i+3])
				}
				s = buf.String() + "." + parts[1]
			}
			if negative {
				s = "-" + s
			}
			return s
		},
		"formatCentsInt": func(cents int) string {
			c := int64(cents)
			negative := c < 0
			if negative {
				c = -c
			}
			whole := c / 100
			frac := c % 100
			s := fmt.Sprintf("%d.%02d", whole, frac)
			parts := strings.SplitN(s, ".", 2)
			wholeStr := parts[0]
			if len(wholeStr) > 3 {
				var buf strings.Builder
				rem := len(wholeStr) % 3
				if rem > 0 {
					buf.WriteString(wholeStr[:rem])
				}
				for i := rem; i < len(wholeStr); i += 3 {
					if buf.Len() > 0 {
						buf.WriteByte(',')
					}
					buf.WriteString(wholeStr[i : i+3])
				}
				s = buf.String() + "." + parts[1]
			}
			if negative {
				s = "-" + s
			}
			return s
		},
		"deref": func(s *string) string {
			if s == nil {
				return ""
			}
			return *s
		},
		"derefInt": func(i *int) int {
			if i == nil {
				return 0
			}
			return *i
		},
		"abs": func(i int64) int64 {
			if i < 0 {
				return -i
			}
			return i
		},
		"lower":    strings.ToLower,
		"basename": filepath.Base,
	}

	h.templates = make(map[string]*template.Template)
	pages := []string{"simulate", "transfers", "transfer_detail", "review", "review_detail", "ledger", "settlement", "returns"}
	for _, page := range pages {
		t, err := template.New("").Funcs(funcMap).ParseFiles(
			filepath.Join(h.TemplateDir, "layout.html"),
			filepath.Join(h.TemplateDir, page+".html"),
		)
		if err != nil {
			return fmt.Errorf("parse template %s: %w", page, err)
		}
		h.templates[page] = t
	}
	return nil
}

func (h *UIHandlers) render(w http.ResponseWriter, page string, data map[string]interface{}) {
	t := h.templates[page]
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		slog.Error("template render error", "page", page, "err", err)
		http.Error(w, "render error: "+err.Error(), 500)
	}
}

func (h *UIHandlers) RegisterRoutes(r chi.Router) {
	r.Get("/ui/simulate", h.simulatePage)
	r.Post("/ui/simulate", h.simulateSubmit)
	r.Get("/ui/transfers", h.transfersPage)
	r.Get("/ui/transfers/{id}", h.transferDetailPage)
	r.Get("/ui/review", h.reviewPage)
	r.Get("/ui/review-count", h.reviewCountBadge)
	r.Get("/ui/review/{id}", h.reviewDetailPage)
	r.Post("/ui/review/{id}/approve", h.reviewApprove)
	r.Post("/ui/review/{id}/reject", h.reviewReject)
	r.Get("/ui/ledger", h.ledgerPage)
	r.Get("/ui/settlement", h.settlementPage)
	r.Post("/ui/settlement/generate", h.settlementGenerate)
	r.Post("/ui/settlement/{id}/ack", h.settlementAck)
	r.Get("/ui/settlement/{id}/download", h.settlementDownload)
	r.Get("/ui/returns", h.returnsPage)
	r.Post("/ui/returns", h.returnsSubmit)
	r.Get("/ui/images/{transferId}/{side}", h.serveImage)
}

// ---------------------------------------------------------------------------
// Simulate
// ---------------------------------------------------------------------------

func (h *UIHandlers) simulatePage(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"ActivePage": "simulate",
	}
	if msg := r.URL.Query().Get("result"); msg != "" {
		data["Flash"] = map[string]string{"Type": "success", "Text": msg}
	}
	h.render(w, "simulate", data)
}

func (h *UIHandlers) simulateSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		h.render(w, "simulate", map[string]interface{}{
			"ActivePage": "simulate",
			"Flash":      map[string]string{"Type": "error", "Text": "Invalid form: " + err.Error()},
		})
		return
	}

	investorAccountID := r.FormValue("investorAccountId")
	amountStr := r.FormValue("amount")
	amountCents, err := parseCents(amountStr)
	if err != nil {
		h.render(w, "simulate", map[string]interface{}{
			"ActivePage": "simulate",
			"Flash":      map[string]string{"Type": "error", "Text": "Invalid amount: " + err.Error()},
		})
		return
	}

	frontFile, _, err := r.FormFile("frontImage")
	if err != nil {
		h.render(w, "simulate", map[string]interface{}{
			"ActivePage": "simulate",
			"Flash":      map[string]string{"Type": "error", "Text": "Front image required: " + err.Error()},
		})
		return
	}
	defer frontFile.Close()

	backFile, _, err := r.FormFile("backImage")
	if err != nil {
		h.render(w, "simulate", map[string]interface{}{
			"ActivePage": "simulate",
			"Flash":      map[string]string{"Type": "error", "Text": "Back image required: " + err.Error()},
		})
		return
	}
	defer backFile.Close()

	result, err := h.DepositSvc.SubmitDeposit(r.Context(), investorAccountID, amountCents, frontFile, backFile)
	if err != nil {
		h.render(w, "simulate", map[string]interface{}{
			"ActivePage": "simulate",
			"Flash":      map[string]string{"Type": "error", "Text": "Deposit failed: " + err.Error()},
		})
		return
	}

	h.render(w, "simulate", map[string]interface{}{
		"ActivePage": "simulate",
		"Result": map[string]interface{}{
			"TransferID":       result.TransferID,
			"State":            string(result.State),
			"Message":          result.Message,
			"RejectionCode":    result.RejectionCode,
			"RejectionMessage": result.RejectionMessage,
			"ReviewRequired":   result.ReviewRequired,
		},
	})
}

// ---------------------------------------------------------------------------
// Transfers
// ---------------------------------------------------------------------------

func (h *UIHandlers) transfersPage(w http.ResponseWriter, r *http.Request) {
	var filters transfers.TransferFilters
	if v := r.URL.Query().Get("state"); v != "" {
		s := transfers.State(v)
		filters.State = &s
	}
	if v := r.URL.Query().Get("investorAccountId"); v != "" {
		filters.InvestorAccountID = &v
	}

	list, err := h.TransferSvc.List(h.DB, filters)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	h.render(w, "transfers", map[string]interface{}{
		"ActivePage": "transfers",
		"Transfers":  list,
	})
}

func (h *UIHandlers) transferDetailPage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	data, err := h.buildTransferDetailData(id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	data["ActivePage"] = "transfers"
	h.render(w, "transfer_detail", data)
}

func (h *UIHandlers) buildTransferDetailData(id string) (map[string]interface{}, error) {
	t, err := h.TransferSvc.GetByID(h.DB, id)
	if err != nil {
		return nil, fmt.Errorf("transfer not found: %w", err)
	}

	data := map[string]interface{}{
		"Transfer": t,
	}

	vendorResult, err := vendorclient.GetVendorResult(h.DB, id)
	if err == nil {
		data["VendorResult"] = vendorResult
	}

	evals, err := h.getRuleEvaluations(id)
	if err == nil && len(evals) > 0 {
		data["RuleEvaluations"] = evals
	}

	events, err := audit.GetByEntity(h.DB, "transfer", id)
	if err == nil && len(events) > 0 {
		data["AuditEvents"] = events
	}

	return data, nil
}

func (h *UIHandlers) getRuleEvaluations(transferID string) ([]ruleEval, error) {
	rows, err := h.DB.Query(`
		SELECT rule_name, outcome, COALESCE(details_json, '')
		FROM rule_evaluations
		WHERE transfer_id = ?
		ORDER BY created_at ASC`, transferID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var evals []ruleEval
	for rows.Next() {
		var e ruleEval
		var raw string
		if err := rows.Scan(&e.RuleName, &e.Outcome, &raw); err != nil {
			return nil, err
		}
		// Extract just the "details" text from the JSON wrapper
		var parsed map[string]string
		if raw != "" && json.Unmarshal([]byte(raw), &parsed) == nil {
			if d, ok := parsed["details"]; ok {
				e.DetailsJSON = d
			} else {
				e.DetailsJSON = raw
			}
		}
		evals = append(evals, e)
	}
	return evals, rows.Err()
}

// ---------------------------------------------------------------------------
// Review
// ---------------------------------------------------------------------------

func (h *UIHandlers) reviewCountBadge(w http.ResponseWriter, r *http.Request) {
	state := transfers.StateAnalyzing
	reviewRequired := true
	reviewStatus := "PENDING"

	list, err := h.TransferSvc.List(h.DB, transfers.TransferFilters{
		State:          &state,
		ReviewRequired: &reviewRequired,
		ReviewStatus:   &reviewStatus,
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	count := len(list)
	if count == 0 {
		w.Write([]byte(""))
		return
	}
	fmt.Fprintf(w, `<span class="review-badge">%d</span>`, count)
}

type reviewQueueItem struct {
	*transfers.Transfer
	Scenario string
}

func deriveScenario(micrConf *float64, amountMatches bool, riskScore int) string {
	if micrConf == nil || *micrConf < 0.5 {
		return "micr_failure"
	}
	if !amountMatches {
		return "amount_mismatch"
	}
	if riskScore >= 80 {
		return "iqa_pass_review"
	}
	return "manual_review"
}

func (h *UIHandlers) reviewPage(w http.ResponseWriter, r *http.Request) {
	state := transfers.StateAnalyzing
	reviewRequired := true
	reviewStatus := "PENDING"

	list, err := h.TransferSvc.List(h.DB, transfers.TransferFilters{
		State:          &state,
		ReviewRequired: &reviewRequired,
		ReviewStatus:   &reviewStatus,
	})
	if err != nil {
		http.Error(w, "an internal error occurred", http.StatusInternalServerError)
		slog.Error("internal error", "op", "list review queue", "err", err)
		return
	}

	items := make([]reviewQueueItem, len(list))
	for i, t := range list {
		tc := t // copy to take address of loop var
		var micrConf *float64
		var amountMatches bool
		var riskScore int
		h.DB.QueryRow(`SELECT micr_confidence, amount_matches, risk_score
			FROM vendor_results WHERE transfer_id = ?`, t.ID).
			Scan(&micrConf, &amountMatches, &riskScore)
		items[i] = reviewQueueItem{
			Transfer: &tc,
			Scenario: deriveScenario(micrConf, amountMatches, riskScore),
		}
	}

	h.render(w, "review", map[string]interface{}{
		"ActivePage": "review",
		"Transfers":  items,
	})
}

func (h *UIHandlers) reviewDetailPage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	data, err := h.buildTransferDetailData(id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	data["ActivePage"] = "review"
	h.render(w, "review_detail", data)
}

func (h *UIHandlers) reviewApprove(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	operatorID := r.FormValue("operatorId")
	notes := r.FormValue("notes")

	t, err := h.TransferSvc.GetByID(h.DB, id)
	if err != nil {
		http.Error(w, "transfer not found: "+err.Error(), 404)
		return
	}

	now := time.Now().UTC()

	// Insert operator action
	_, err = h.DB.Exec(`
		INSERT INTO operator_actions (id, transfer_id, operator_id, action, notes, created_at)
		VALUES (?, ?, ?, 'APPROVE', ?, ?)`,
		uuid.New().String(), id, operatorID, notes, now)
	if err != nil {
		http.Error(w, "create operator action: "+err.Error(), 500)
		return
	}

	if err := h.TransferSvc.UpdateReviewStatus(h.DB, id, "APPROVED"); err != nil {
		http.Error(w, "update review status: "+err.Error(), 500)
		return
	}

	if err := h.TransferSvc.Transition(h.DB, id, transfers.StateApproved, "OPERATOR", operatorID); err != nil {
		http.Error(w, "transition to Approved: "+err.Error(), 500)
		return
	}
	h.DB.Exec("UPDATE transfers SET approved_at = ? WHERE id = ?", now, id)

	if err := h.LedgerSvc.PostDeposit(h.DB, id, t.InvestorAccountID, t.OmnibusAccountID, t.AmountCents); err != nil {
		http.Error(w, "post deposit: "+err.Error(), 500)
		return
	}

	if err := h.TransferSvc.Transition(h.DB, id, transfers.StateFundsPosted, "OPERATOR", operatorID); err != nil {
		http.Error(w, "transition to FundsPosted: "+err.Error(), 500)
		return
	}
	h.DB.Exec("UPDATE transfers SET posted_at = ? WHERE id = ?", now, id)

	detailsJSON := fmt.Sprintf(`{"notes":%q}`, notes)
	audit.LogEvent(h.DB, audit.Event{
		EntityType:  "transfer",
		EntityID:    id,
		ActorType:   "OPERATOR",
		ActorID:     operatorID,
		EventType:   "OPERATOR_APPROVE",
		DetailsJSON: &detailsJSON,
		CreatedAt:   now,
	})

	http.Redirect(w, r, "/ui/transfers/"+id, http.StatusSeeOther)
}

func (h *UIHandlers) reviewReject(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	operatorID := r.FormValue("operatorId")
	notes := r.FormValue("notes")

	now := time.Now().UTC()

	// Insert operator action
	_, err := h.DB.Exec(`
		INSERT INTO operator_actions (id, transfer_id, operator_id, action, notes, created_at)
		VALUES (?, ?, ?, 'REJECT', ?, ?)`,
		uuid.New().String(), id, operatorID, notes, now)
	if err != nil {
		http.Error(w, "create operator action: "+err.Error(), 500)
		return
	}

	if err := h.TransferSvc.UpdateReviewStatus(h.DB, id, "REJECTED"); err != nil {
		http.Error(w, "update review status: "+err.Error(), 500)
		return
	}

	rejCode := "OPERATOR_REJECT"
	rejMsg := notes
	h.DB.Exec("UPDATE transfers SET rejection_code = ?, rejection_message = ?, updated_at = ? WHERE id = ?",
		rejCode, rejMsg, now, id)

	if err := h.TransferSvc.Transition(h.DB, id, transfers.StateRejected, "OPERATOR", operatorID); err != nil {
		http.Error(w, "transition to Rejected: "+err.Error(), 500)
		return
	}

	detailsJSON := fmt.Sprintf(`{"notes":%q}`, notes)
	audit.LogEvent(h.DB, audit.Event{
		EntityType:  "transfer",
		EntityID:    id,
		ActorType:   "OPERATOR",
		ActorID:     operatorID,
		EventType:   "OPERATOR_REJECT",
		DetailsJSON: &detailsJSON,
		CreatedAt:   now,
	})

	http.Redirect(w, r, "/ui/transfers/"+id, http.StatusSeeOther)
}

// ---------------------------------------------------------------------------
// Ledger
// ---------------------------------------------------------------------------

func (h *UIHandlers) ledgerPage(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Query(`
		SELECT a.id, a.external_account_id, a.account_name, a.account_type,
		       COALESCE(SUM(e.signed_amount_cents), 0) AS balance_cents
		FROM accounts a
		LEFT JOIN ledger_entries e ON e.account_id = a.id
		GROUP BY a.id
		ORDER BY a.account_type, a.external_account_id`)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var accounts []accountBalance
	for rows.Next() {
		var ab accountBalance
		if err := rows.Scan(&ab.ID, &ab.ExternalAccountID, &ab.AccountName, &ab.AccountType, &ab.BalanceCents); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		accounts = append(accounts, ab)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	h.render(w, "ledger", map[string]interface{}{
		"ActivePage": "ledger",
		"Accounts":   accounts,
	})
}

// ---------------------------------------------------------------------------
// Settlement
// ---------------------------------------------------------------------------

func (h *UIHandlers) settlementPage(w http.ResponseWriter, r *http.Request) {
	batches, err := h.SettlementSvc.ListBatches(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	data := map[string]interface{}{
		"ActivePage": "settlement",
		"Batches":    batches,
	}

	if msg := r.URL.Query().Get("msg"); msg != "" {
		data["Message"] = map[string]string{"Type": "success", "Text": msg}
	}

	h.render(w, "settlement", data)
}

func (h *UIHandlers) settlementGenerate(w http.ResponseWriter, r *http.Request) {
	loc, err := time.LoadLocation("America/Chicago")
	if err != nil {
		http.Error(w, "load timezone: "+err.Error(), 500)
		return
	}
	businessDate := time.Now().In(loc).Format("2006-01-02")

	batch, err := h.SettlementSvc.GenerateBatch(r.Context(), businessDate)
	if err != nil {
		http.Redirect(w, r, "/ui/settlement?msg="+err.Error(), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/ui/settlement?msg=Batch+generated:+"+batch.ID[:8], http.StatusSeeOther)
}

func (h *UIHandlers) settlementAck(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	now := time.Now().UTC().Format("20060102")
	ackRef := fmt.Sprintf("ACK-%s-%s", now, id[:8])

	if err := h.SettlementSvc.AcknowledgeBatch(r.Context(), id, ackRef); err != nil {
		http.Redirect(w, r, "/ui/settlement?msg="+err.Error(), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/ui/settlement?msg=Batch+acknowledged", http.StatusSeeOther)
}

func (h *UIHandlers) settlementDownload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	batch, _, err := h.SettlementSvc.GetBatch(r.Context(), id)
	if err != nil || batch == nil || batch.FilePath == nil {
		http.Error(w, "batch not found", 404)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(*batch.FilePath)))
	http.ServeFile(w, r, *batch.FilePath)
}

// ---------------------------------------------------------------------------
// Returns
// ---------------------------------------------------------------------------

func (h *UIHandlers) returnsPage(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"ActivePage": "returns",
	}
	if msg := r.URL.Query().Get("msg"); msg != "" {
		data["Message"] = map[string]string{"Type": "success", "Text": msg}
	}
	h.render(w, "returns", data)
}

func (h *UIHandlers) returnsSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	transferID := r.FormValue("transferId")
	reasonCode := r.FormValue("reasonCode")

	if err := h.ReturnsSvc.ProcessReturn(r.Context(), transferID, reasonCode, reasonCode); err != nil {
		h.render(w, "returns", map[string]interface{}{
			"ActivePage": "returns",
			"Message":    map[string]string{"Type": "error", "Text": err.Error()},
		})
		return
	}

	t, _ := h.TransferSvc.GetByID(h.DB, transferID)
	data := map[string]interface{}{
		"ActivePage": "returns",
		"Message":    map[string]string{"Type": "success", "Text": "Return processed successfully"},
	}
	if t != nil {
		data["Transfer"] = t
	}
	h.render(w, "returns", data)
}

// ---------------------------------------------------------------------------
// Images
// ---------------------------------------------------------------------------

func (h *UIHandlers) serveImage(w http.ResponseWriter, r *http.Request) {
	transferID := chi.URLParam(r, "transferId")
	side := chi.URLParam(r, "side")

	var filePath string
	err := h.DB.QueryRow("SELECT file_path FROM transfer_images WHERE transfer_id = ? AND side = ?",
		transferID, side).Scan(&filePath)
	if err != nil {
		http.Error(w, "image not found", 404)
		return
	}

	// Resolve symlinks and ensure the file is within the expected image directory.
	absImage, err := filepath.EvalSymlinks(filepath.Clean(filePath))
	if err != nil {
		http.Error(w, "image not found", 404)
		return
	}
	absDir, err := filepath.EvalSymlinks(filepath.Clean(h.ImageDir))
	if err != nil {
		http.Error(w, "image not found", 404)
		return
	}
	if !strings.HasPrefix(absImage, absDir+string(filepath.Separator)) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	http.ServeFile(w, r, absImage)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

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
