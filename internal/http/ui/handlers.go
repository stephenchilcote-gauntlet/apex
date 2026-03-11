package ui

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os"
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
		// dateOnly trims ISO timestamps to just the YYYY-MM-DD date portion
		"dateOnly": func(s string) string {
			if len(s) >= 10 {
				return s[:10]
			}
			return s
		},
		// shortID truncates UUID-style IDs to 8 chars, but shows full IDs for short human-readable IDs
		"shortID": func(id string) string {
			// UUID format: 36 chars with dashes (e.g., "abc12345-...")
			if len(id) == 36 && id[8] == '-' {
				return id[:8] + "…"
			}
			// Short human-readable ID: show as-is (up to 16 chars)
			if len(id) <= 16 {
				return id
			}
			return id[:16] + "…"
		},
		"pct": func(count, max int) string {
			if max == 0 {
				return "0%"
			}
			return fmt.Sprintf("%.0f%%", float64(count)/float64(max)*100)
		},
	}

	h.templates = make(map[string]*template.Template)
	pages := []string{"dashboard", "simulate", "transfers", "transfer_detail", "review", "review_detail", "ledger", "audit", "settlement", "settlement_batch_detail", "returns"}
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
	r.Get("/ui", h.dashboardPage)
	r.Get("/ui/simulate", h.simulatePage)
	r.Post("/ui/simulate", h.simulateSubmit)
	r.Get("/ui/transfers", h.transfersPage)
	r.Get("/ui/transfers/{id}", h.transferDetailPage)
	r.Get("/ui/review", h.reviewPage)
	r.Get("/ui/review-count", h.reviewCountBadge)
	r.Get("/ui/settlement-count", h.settlementCountBadge)
	r.Get("/ui/review/{id}", h.reviewDetailPage)
	r.Post("/ui/review/{id}/approve", h.reviewApprove)
	r.Post("/ui/review/{id}/reject", h.reviewReject)
	r.Get("/ui/ledger", h.ledgerPage)
	r.Get("/ui/audit", h.auditPage)
	r.Get("/ui/settlement", h.settlementPage)
	r.Post("/ui/settlement/generate", h.settlementGenerate)
	r.Get("/ui/settlement/{id}", h.settlementBatchDetailPage)
	r.Post("/ui/settlement/{id}/ack", h.settlementAck)
	r.Get("/ui/settlement/{id}/download", h.settlementDownload)
	r.Get("/ui/returns", h.returnsPage)
	r.Post("/ui/returns", h.returnsSubmit)
	r.Get("/ui/images/{transferId}/{side}", h.serveImage)
	r.Get("/ui/health-status", h.healthStatus)
	r.Get("/ui/search", h.searchHandler)
}

// ---------------------------------------------------------------------------
// Dashboard
// ---------------------------------------------------------------------------

type stateCounts struct {
	State string
	Count int
}

type dashboardBatch struct {
	ID               string
	Status           string
	BusinessDateCT   string
	TotalItems       int
	TotalAmountCents int64
}

type dailyVolume struct {
	Date        string // "2006-01-02"
	Label       string // "Mar 11"
	Count       int
	AmountCents int64
}

func (h *UIHandlers) dashboardPage(w http.ResponseWriter, r *http.Request) {
	// Transfer counts by state
	rows, err := h.DB.Query(`SELECT state, COUNT(*) FROM transfers GROUP BY state ORDER BY state`)
	var counts []stateCounts
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var sc stateCounts
			if err2 := rows.Scan(&sc.State, &sc.Count); err2 == nil {
				counts = append(counts, sc)
			}
		}
	}

	// Pending review: Analyzing + review_required + review_status='PENDING'
	var pendingReview int
	h.DB.QueryRow(`SELECT COUNT(*) FROM transfers WHERE state='Analyzing' AND review_required=1 AND review_status='PENDING'`).Scan(&pendingReview)

	// Total, exceptions, and max per-state count for bar chart
	var totalTransfers, maxStateCount, exceptionsCount int
	for _, sc := range counts {
		totalTransfers += sc.Count
		if sc.State == "Rejected" || sc.State == "Returned" {
			exceptionsCount += sc.Count
		}
		if sc.Count > maxStateCount {
			maxStateCount = sc.Count
		}
	}
	if maxStateCount == 0 {
		maxStateCount = 1
	}

	// Latest settlement batch
	var latestBatch *dashboardBatch
	var b dashboardBatch
	err2 := h.DB.QueryRow(`SELECT id, status, business_date_ct, total_items, total_amount_cents FROM settlement_batches ORDER BY created_at DESC LIMIT 1`).
		Scan(&b.ID, &b.Status, &b.BusinessDateCT, &b.TotalItems, &b.TotalAmountCents)
	if err2 == nil {
		if len(b.BusinessDateCT) > 10 {
			b.BusinessDateCT = b.BusinessDateCT[:10]
		}
		latestBatch = &b
	}

	// FundsPosted amount + count (available for settlement)
	var fundsPostedCents int64
	var fundsPostedCount int
	h.DB.QueryRow(`SELECT COUNT(*), COALESCE(SUM(amount_cents),0) FROM transfers WHERE state='FundsPosted'`).Scan(&fundsPostedCount, &fundsPostedCents)

	// Total volume across all non-rejected, non-returned deposits
	var totalVolumeCents int64
	h.DB.QueryRow(`SELECT COALESCE(SUM(amount_cents),0) FROM transfers WHERE state NOT IN ('Rejected','Returned')`).Scan(&totalVolumeCents)

	// Daily deposit volume — last 7 calendar days with at least one transfer
	var daily []dailyVolume
	var maxDailyCount int
	dvRows, dvErr := h.DB.Query(`
		SELECT date(business_date_ct) as d, COUNT(*), COALESCE(SUM(amount_cents),0)
		FROM transfers
		WHERE business_date_ct IS NOT NULL AND business_date_ct != ''
		GROUP BY d
		ORDER BY d DESC
		LIMIT 7`)
	if dvErr == nil {
		defer dvRows.Close()
		for dvRows.Next() {
			var dv dailyVolume
			if err3 := dvRows.Scan(&dv.Date, &dv.Count, &dv.AmountCents); err3 == nil {
				// Format label: "2026-03-11" → "Mar 11"
				if t, err4 := time.Parse("2006-01-02", dv.Date); err4 == nil {
					dv.Label = t.Format("Jan 2")
				} else {
					dv.Label = dv.Date
				}
				if dv.Count > maxDailyCount {
					maxDailyCount = dv.Count
				}
				daily = append(daily, dv)
			}
		}
	}
	if maxDailyCount == 0 {
		maxDailyCount = 1
	}
	// Reverse so oldest is first (left to right on the chart)
	for i, j := 0, len(daily)-1; i < j; i, j = i+1, j-1 {
		daily[i], daily[j] = daily[j], daily[i]
	}

	// Recent transfers (last 8, newest first)
	recentList := h.recentTransfers(8)

	h.render(w, "dashboard", map[string]interface{}{
		"ActivePage":       "dashboard",
		"StateCounts":      counts,
		"TotalTransfers":   totalTransfers,
		"PendingReview":    pendingReview,
		"ExceptionsCount":  exceptionsCount,
		"MaxStateCount":    maxStateCount,
		"LatestBatch":      latestBatch,
		"FundsPostedCents": fundsPostedCents,
		"FundsPostedCount": fundsPostedCount,
		"DailyVolume":      daily,
		"MaxDailyCount":    maxDailyCount,
		"TotalVolumeCents": totalVolumeCents,
		"Recent":           recentList,
		"AccountNames":     h.loadAccountNames(),
	})
}

// ---------------------------------------------------------------------------
// Simulate
// ---------------------------------------------------------------------------

func (h *UIHandlers) simulatePage(w http.ResponseWriter, r *http.Request) {
	data := map[string]interface{}{
		"ActivePage":   "simulate",
		"Recent":       h.recentTransfers(10),
		"AccountNames": h.loadAccountNames(),
	}
	if msg := r.URL.Query().Get("result"); msg != "" {
		data["Flash"] = map[string]string{"Type": "success", "Text": msg}
	}
	h.render(w, "simulate", data)
}

func (h *UIHandlers) recentTransfers(n int) []transfers.Transfer {
	var filters transfers.TransferFilters
	list, err := h.TransferSvc.List(h.DB, filters)
	if err != nil || len(list) == 0 {
		return nil
	}
	if len(list) > n {
		list = list[len(list)-n:]
	}
	// Reverse so newest is first
	for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
		list[i], list[j] = list[j], list[i]
	}
	return list
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

	var frontReader io.ReadCloser
	var backReader io.ReadCloser

	frontFile, _, fErr := r.FormFile("frontImage")
	backFile, _, bErr := r.FormFile("backImage")

	useSample := r.FormValue("useSampleImages") == "true" || (fErr != nil && bErr != nil)

	if useSample {
		// Fall back to bundled sample images
		staticDir := filepath.Join(h.TemplateDir, "..", "static")
		f, err2 := os.Open(filepath.Join(staticDir, "sample-check-front.png"))
		if err2 != nil {
			h.render(w, "simulate", map[string]interface{}{
				"ActivePage": "simulate",
				"Flash":      map[string]string{"Type": "error", "Text": "Sample images not found — please upload images manually"},
			})
			return
		}
		b, err3 := os.Open(filepath.Join(staticDir, "sample-check-back.png"))
		if err3 != nil {
			f.Close()
			h.render(w, "simulate", map[string]interface{}{
				"ActivePage": "simulate",
				"Flash":      map[string]string{"Type": "error", "Text": "Sample images not found — please upload images manually"},
			})
			return
		}
		frontReader = f
		backReader = b
		if frontFile != nil {
			frontFile.Close()
		}
		if backFile != nil {
			backFile.Close()
		}
	} else {
		if fErr != nil {
			h.render(w, "simulate", map[string]interface{}{
				"ActivePage": "simulate",
				"Flash":      map[string]string{"Type": "error", "Text": "Front image required: " + fErr.Error()},
			})
			return
		}
		if bErr != nil {
			frontFile.Close()
			h.render(w, "simulate", map[string]interface{}{
				"ActivePage": "simulate",
				"Flash":      map[string]string{"Type": "error", "Text": "Back image required: " + bErr.Error()},
			})
			return
		}
		frontReader = frontFile
		backReader = backFile
	}
	defer frontReader.Close()
	defer backReader.Close()

	result, err := h.DepositSvc.SubmitDeposit(r.Context(), investorAccountID, amountCents, frontReader, backReader)
	if err != nil {
		h.render(w, "simulate", map[string]interface{}{
			"ActivePage": "simulate",
			"Flash":      map[string]string{"Type": "error", "Text": "Deposit failed: " + err.Error()},
		})
		return
	}

	h.render(w, "simulate", map[string]interface{}{
		"ActivePage":   "simulate",
		"AccountNames": h.loadAccountNames(),
		"Recent":       h.recentTransfers(10),
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
	const pageSize = 50
	q := r.URL.Query()

	var filters transfers.TransferFilters
	if v := q.Get("state"); v != "" {
		s := transfers.State(v)
		filters.State = &s
	}
	if v := q.Get("investorAccountId"); v != "" {
		filters.InvestorAccountID = &v
	}
	if v := q.Get("dateFrom"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			filters.DateFrom = &t
		}
	}
	if v := q.Get("dateTo"); v != "" {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			filters.DateTo = &t
		}
	}

	// CSV export: return all matching transfers as a CSV file
	if q.Get("format") == "csv" {
		h.transfersCSV(w, r, filters)
		return
	}

	page := 1
	if v := q.Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	filters.Limit = pageSize
	filters.Offset = (page - 1) * pageSize

	total, _ := h.TransferSvc.Count(h.DB, filters)
	list, err := h.TransferSvc.List(h.DB, filters)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	totalPages := (total + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}

	var pageAmountCents int64
	for _, t := range list {
		pageAmountCents += t.AmountCents
	}

	h.render(w, "transfers", map[string]interface{}{
		"ActivePage":       "transfers",
		"Transfers":        list,
		"StateFilter":      q.Get("state"),
		"AccountFilter":    q.Get("investorAccountId"),
		"DateFromFilter":   q.Get("dateFrom"),
		"DateToFilter":     q.Get("dateTo"),
		"Page":             page,
		"TotalPages":       totalPages,
		"Total":            total,
		"PageAmountCents":  pageAmountCents,
		"HasPrev":          page > 1,
		"HasNext":          page < totalPages,
		"PrevPage":         page - 1,
		"NextPage":         page + 1,
		"AccountNames":     h.loadAccountNames(),
	})
}

// transfersCSV writes all transfers matching filters as CSV attachment.
func (h *UIHandlers) transfersCSV(w http.ResponseWriter, _ *http.Request, filters transfers.TransferFilters) {
	// No pagination — export everything
	filters.Limit = 0
	filters.Offset = 0
	list, err := h.TransferSvc.List(h.DB, filters)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	accountNames := h.loadAccountNames()

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=transfers.csv")
	w.Header().Set("Cache-Control", "no-store")

	fmt.Fprint(w, "ID,Account,Amount,State,BusinessDate,ReturnReason,ReturnFee,CreatedAt\n")
	for _, t := range list {
		acct := t.InvestorAccountID
		if n, ok := accountNames[t.InvestorAccountID]; ok {
			acct = n
		}
		bizDate := ""
		if t.BusinessDateCT != nil {
			bizDate = *t.BusinessDateCT
			if len(bizDate) > 10 {
				bizDate = bizDate[:10]
			}
		}
		returnReason := ""
		if t.ReturnReasonCode != nil {
			returnReason = *t.ReturnReasonCode
		}
		fmt.Fprintf(w, "%s,%s,%s,%s,%s,%s,%s,%s\n",
			t.ID,
			csvEscape(acct),
			fmt.Sprintf("%.2f", float64(t.AmountCents)/100),
			string(t.State),
			bizDate,
			returnReason,
			fmt.Sprintf("%.2f", float64(t.ReturnFeeCents)/100),
			t.CreatedAt.Format(time.RFC3339),
		)
	}
}

func (h *UIHandlers) transferDetailPage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	data, err := h.buildTransferDetailData(id)
	if err != nil {
		if strings.Contains(err.Error(), "transfer not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
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

	// Resolve account display name
	var accountDisplay string
	var extID, name string
	if h.DB.QueryRow(`SELECT external_account_id, account_name FROM accounts WHERE id = ?`, t.InvestorAccountID).Scan(&extID, &name) == nil {
		accountDisplay = name + " (" + extID + ")"
	} else {
		accountDisplay = t.InvestorAccountID
	}

	data := map[string]interface{}{
		"Transfer":        t,
		"AccountDisplay":  accountDisplay,
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

	// Lookup settlement batch for this transfer
	var batchID string
	if h.DB.QueryRow(`SELECT batch_id FROM settlement_batch_items WHERE transfer_id = ? LIMIT 1`, id).Scan(&batchID) == nil {
		data["SettlementBatchID"] = batchID
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

func (h *UIHandlers) settlementCountBadge(w http.ResponseWriter, r *http.Request) {
	var count int
	h.DB.QueryRow(`SELECT COUNT(*) FROM transfers WHERE state='FundsPosted'`).Scan(&count)
	if count == 0 {
		w.Write([]byte(""))
		return
	}
	fmt.Fprintf(w, `<span class="review-badge">%d</span>`, count)
}

type reviewQueueItem struct {
	*transfers.Transfer
	Reason string
}

func deriveReason(micrConf *float64, amountMatches bool, riskScore int) string {
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
			Reason: deriveReason(micrConf, amountMatches, riskScore),
		}
	}

	h.render(w, "review", map[string]interface{}{
		"ActivePage":   "review",
		"Transfers":    items,
		"AccountNames": h.loadAccountNames(),
	})
}

func (h *UIHandlers) reviewDetailPage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	data, err := h.buildTransferDetailData(id)
	if err != nil {
		if strings.Contains(err.Error(), "transfer not found") {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	data["ActivePage"] = "review"

	// Build prev/next navigation within the review queue
	state := transfers.StateAnalyzing
	reviewRequired := true
	reviewStatus := "PENDING"
	queue, _ := h.TransferSvc.List(h.DB, transfers.TransferFilters{
		State:          &state,
		ReviewRequired: &reviewRequired,
		ReviewStatus:   &reviewStatus,
	})
	for i, t := range queue {
		if t.ID == id {
			if i > 0 {
				data["PrevReviewID"] = queue[i-1].ID
			}
			if i < len(queue)-1 {
				data["NextReviewID"] = queue[i+1].ID
			}
			data["QueuePos"] = i + 1
			data["QueueLen"] = len(queue)
			break
		}
	}

	// Account history: last 5 deposits from the same investor (for pattern context)
	if t, ok := data["Transfer"].(*transfers.Transfer); ok {
		acctID := t.InvestorAccountID
		history, _ := h.TransferSvc.List(h.DB, transfers.TransferFilters{
			InvestorAccountID: &acctID,
			Limit:             6, // fetch 6, will skip the current one
		})
		var filtered []transfers.Transfer
		for _, hist := range history {
			if hist.ID != id {
				filtered = append(filtered, hist)
				if len(filtered) == 5 {
					break
				}
			}
		}
		if len(filtered) > 0 {
			data["AccountHistory"] = filtered
			data["AccountNames"] = h.loadAccountNames()
		}
	}

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

	// Recent journal entries (last 30) with account and transfer context
	type journalEntry struct {
		AccountExtID  string
		AccountName   string
		AmountCents   int64
		Purpose       string
		TransferID    string
		CreatedAt     string
	}
	eRows, eErr := h.DB.Query(`
		SELECT a.external_account_id, a.account_name, e.signed_amount_cents,
		       j.journal_type, j.transfer_id, e.created_at
		FROM ledger_entries e
		JOIN ledger_journals j ON j.id = e.journal_id
		JOIN accounts a ON a.id = e.account_id
		ORDER BY e.created_at DESC
		LIMIT 30`)
	var entries []journalEntry
	if eErr == nil {
		defer eRows.Close()
		for eRows.Next() {
			var ent journalEntry
			if err2 := eRows.Scan(&ent.AccountExtID, &ent.AccountName, &ent.AmountCents, &ent.Purpose, &ent.TransferID, &ent.CreatedAt); err2 == nil {
				entries = append(entries, ent)
			}
		}
	}

	// Global zero-sum invariant check
	var totalLedger int64
	h.DB.QueryRow(`SELECT COALESCE(SUM(signed_amount_cents), 0) FROM ledger_entries`).Scan(&totalLedger)

	h.render(w, "ledger", map[string]interface{}{
		"ActivePage":      "ledger",
		"Accounts":        accounts,
		"RecentEntries":   entries,
		"LedgerSum":       totalLedger,
		"LedgerBalanced":  totalLedger == 0,
	})
}

func (h *UIHandlers) auditPage(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	transferID := r.URL.Query().Get("transferId")
	eventType := r.URL.Query().Get("eventType")

	type auditRow struct {
		ID          string
		EntityType  string
		EntityID    string
		ActorID     string
		EventType   string
		FromState   string
		ToState     string
		Details     string
		CreatedAt   string
	}

	query := `SELECT id, entity_type, entity_id, COALESCE(actor_id,''), event_type,
	               COALESCE(from_state,''), COALESCE(to_state,''),
	               COALESCE(details_json,''), created_at
	          FROM audit_events WHERE 1=1`
	args := []interface{}{}
	if transferID != "" {
		query += ` AND entity_id = ?`
		args = append(args, transferID)
	}
	if eventType != "" {
		query += ` AND event_type = ?`
		args = append(args, eventType)
	}
	query += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)

	rows, err := h.DB.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var events []auditRow
	for rows.Next() {
		var e auditRow
		if rows.Scan(&e.ID, &e.EntityType, &e.EntityID, &e.ActorID, &e.EventType,
			&e.FromState, &e.ToState, &e.Details, &e.CreatedAt) == nil {
			events = append(events, e)
		}
	}

	// Get distinct event types for the filter dropdown
	etRows, _ := h.DB.Query(`SELECT DISTINCT event_type FROM audit_events ORDER BY event_type`)
	var eventTypes []string
	if etRows != nil {
		defer etRows.Close()
		for etRows.Next() {
			var et string
			if etRows.Scan(&et) == nil {
				eventTypes = append(eventTypes, et)
			}
		}
	}

	h.render(w, "audit", map[string]interface{}{
		"ActivePage":  "audit",
		"Events":      events,
		"TransferID":  transferID,
		"EventType":   eventType,
		"EventTypes":  eventTypes,
		"Limit":       limit,
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

	var eligibleCount int
	var eligibleCents int64
	_ = h.DB.QueryRow(`SELECT COUNT(*), COALESCE(SUM(amount_cents),0) FROM transfers WHERE state='FundsPosted'`).Scan(&eligibleCount, &eligibleCents)

	data := map[string]interface{}{
		"ActivePage":          "settlement",
		"Batches":             batches,
		"EligibleCount":       eligibleCount,
		"EligibleAmountCents": eligibleCents,
	}

	if msg := r.URL.Query().Get("msg"); msg != "" {
		data["Message"] = map[string]string{"Type": "success", "Text": msg}
	}

	h.render(w, "settlement", data)
}

func (h *UIHandlers) settlementBatchDetailPage(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	batch, items, err := h.SettlementSvc.GetBatch(nil, id)
	if err != nil {
		http.Error(w, "batch not found: "+err.Error(), 404)
		return
	}

	// Enrich items with account display names
	type enrichedItem struct {
		Item           settlement.BatchItem
		AccountDisplay string
		TransferState  string
		MICR           map[string]string
	}
	var enriched []enrichedItem
	for _, item := range items {
		ei := enrichedItem{Item: item}
		var extID, name, state string
		h.DB.QueryRow(`SELECT a.external_account_id, a.account_name, t.state
			FROM transfers t LEFT JOIN accounts a ON a.id = t.investor_account_id
			WHERE t.id = ?`, item.TransferID).Scan(&extID, &name, &state)
		if extID != "" {
			ei.AccountDisplay = name + " (" + extID + ")"
		} else {
			ei.AccountDisplay = item.TransferID
		}
		ei.TransferState = state
		if item.MICRSnapshotJSON != nil && *item.MICRSnapshotJSON != "" {
			var m map[string]string
			if json.Unmarshal([]byte(*item.MICRSnapshotJSON), &m) == nil {
				ei.MICR = m
			}
		}
		enriched = append(enriched, ei)
	}

	h.render(w, "settlement_batch_detail", map[string]interface{}{
		"ActivePage": "settlement",
		"Batch":      batch,
		"Items":      enriched,
	})
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
		http.Redirect(w, r, "/ui/settlement?msg="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/ui/settlement?msg="+url.QueryEscape("Batch generated: "+batch.ID[:8]), http.StatusSeeOther)
}

func (h *UIHandlers) settlementAck(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	now := time.Now().UTC().Format("20060102")
	ackRef := fmt.Sprintf("ACK-%s-%s", now, id[:8])

	if err := h.SettlementSvc.AcknowledgeBatch(r.Context(), id, ackRef); err != nil {
		http.Redirect(w, r, "/ui/settlement?msg="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/ui/settlement?msg="+url.QueryEscape("Batch acknowledged"), http.StatusSeeOther)
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
	if id := r.URL.Query().Get("id"); id != "" {
		data["PrefillID"] = id
	}

	// Eligible transfers: FundsPosted or Completed, last 10
	type eligibleTransfer struct {
		ID             string
		Amount         string
		State          string
		AccountDisplay string
		BusinessDate   string
	}
	rows, err := h.DB.Query(`
		SELECT t.id, t.amount_cents, t.state,
		       COALESCE(a.account_name, '') || ' (' || COALESCE(a.external_account_id, '') || ')' as acct,
		       COALESCE(t.business_date_ct, '')
		FROM transfers t
		LEFT JOIN accounts a ON a.id = t.investor_account_id
		WHERE t.state IN ('FundsPosted', 'Completed')
		ORDER BY t.created_at DESC LIMIT 10`)
	if err == nil {
		defer rows.Close()
		var eligible []eligibleTransfer
		for rows.Next() {
			var e eligibleTransfer
			var amtCents int64
			if rows.Scan(&e.ID, &amtCents, &e.State, &e.AccountDisplay, &e.BusinessDate) == nil {
				e.Amount = fmt.Sprintf("$%d.%02d", amtCents/100, amtCents%100)
				if len(e.BusinessDate) > 10 {
					e.BusinessDate = e.BusinessDate[:10]
				}
				eligible = append(eligible, e)
			}
		}
		if len(eligible) > 0 {
			data["EligibleTransfers"] = eligible
		}
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
		var extID, name string
		if h.DB.QueryRow(`SELECT external_account_id, account_name FROM accounts WHERE id = ?`, t.InvestorAccountID).Scan(&extID, &name) == nil {
			data["AccountDisplay"] = name + " (" + extID + ")"
		}
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
// Health Status (HTMX fragment — nav status strip)
// ---------------------------------------------------------------------------

func (h *UIHandlers) healthStatus(w http.ResponseWriter, r *http.Request) {
	ok := true
	if err := h.DB.PingContext(r.Context()); err != nil {
		ok = false
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if ok {
		fmt.Fprint(w, `<span class="status-dot" data-active></span><span>Connected</span>`)
	} else {
		fmt.Fprint(w, `<span class="status-dot" style="background:var(--red);"></span><span style="color:var(--red);">Degraded</span>`)
	}
}

// ---------------------------------------------------------------------------
// Search (command palette HTMX fragment)
// ---------------------------------------------------------------------------

func (h *UIHandlers) searchHandler(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if q == "" {
		fmt.Fprint(w, `<div class="cmd-empty">Type to search transfers by ID or account…</div>`)
		return
	}

	// Search transfers: ID prefix, account name, or external account ID (case-insensitive)
	rows, err := h.DB.QueryContext(r.Context(), `
		SELECT t.id, COALESCE(a.account_name || ' (' || a.external_account_id || ')', t.investor_account_id), t.amount_cents, t.state
		FROM transfers t
		LEFT JOIN accounts a ON a.id = t.investor_account_id
		WHERE (t.id LIKE ?
		   OR LOWER(t.investor_account_id) LIKE ?
		   OR LOWER(a.account_name) LIKE ?
		   OR LOWER(a.external_account_id) LIKE ?)
		ORDER BY t.created_at DESC
		LIMIT 8
	`, q+"%", "%"+strings.ToLower(q)+"%", "%"+strings.ToLower(q)+"%", "%"+strings.ToLower(q)+"%")
	if err != nil {
		fmt.Fprint(w, `<div class="cmd-empty">Search error</div>`)
		return
	}
	defer rows.Close()

	type result struct {
		ID                string
		InvestorAccountID string
		AmountCents       int64
		State             string
	}
	var results []result
	for rows.Next() {
		var res result
		if err := rows.Scan(&res.ID, &res.InvestorAccountID, &res.AmountCents, &res.State); err == nil {
			results = append(results, res)
		}
	}

	if len(results) == 0 {
		fmt.Fprintf(w, `<div class="cmd-empty">No transfers matching <em>%s</em></div>`, html.EscapeString(q))
		return
	}

	var sb strings.Builder
	for i, res := range results {
		short := res.ID
		if len(short) > 8 {
			short = short[:8] + "…"
		}
		dollars := fmt.Sprintf("$%d.%02d", res.AmountCents/100, res.AmountCents%100)
		sb.WriteString(fmt.Sprintf(
			`<a class="cmd-result%s" href="/ui/transfers/%s" data-cmd-idx="%d">
			  <span class="cmd-result-id">%s</span>
			  <span class="cmd-result-account">%s</span>
			  <span class="cmd-result-amount">%s</span>
			  <span class="badge badge--%s">%s</span>
			</a>`,
			func() string {
				if i == 0 {
					return " cmd-result--active"
				}
				return ""
			}(),
			html.EscapeString(res.ID),
			i,
			html.EscapeString(short),
			html.EscapeString(res.InvestorAccountID),
			dollars,
			html.EscapeString(res.State),
			html.EscapeString(res.State),
		))
	}
	fmt.Fprint(w, sb.String())
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// loadAccountNames returns a map of account UUID → "Name (EXT-ID)" for display.
func (h *UIHandlers) loadAccountNames() map[string]string {
	rows, err := h.DB.Query(`SELECT id, external_account_id, account_name FROM accounts`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	m := make(map[string]string)
	for rows.Next() {
		var id, extID, name string
		if rows.Scan(&id, &extID, &name) == nil {
			m[id] = name + " (" + extID + ")"
		}
	}
	return m
}

// csvEscape wraps a field in double-quotes if it contains commas, quotes, or newlines.
func csvEscape(s string) string {
	if strings.ContainsAny(s, `",`+"\n\r") {
		s = strings.ReplaceAll(s, `"`, `""`)
		return `"` + s + `"`
	}
	return s
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
