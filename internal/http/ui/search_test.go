package ui

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

func searchMigrationsDir() string {
	_, f, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(f), "..", "..", "..", "db", "migrations")
}

func newSearchTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file::memory:?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	for _, name := range []string{"001_init.sql", "002_investor_names.sql"} {
		b, err := os.ReadFile(filepath.Join(searchMigrationsDir(), name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(b)); err != nil {
			t.Fatalf("migration %s: %v", name, err)
		}
	}
	return db
}

// seedTransfer inserts a transfer linked to the given external account ID.
func seedTransfer(t *testing.T, db *sql.DB, externalAccountID string, amountCents int64) string {
	t.Helper()
	var accountID string
	if err := db.QueryRow(`SELECT id FROM accounts WHERE external_account_id = ?`, externalAccountID).Scan(&accountID); err != nil {
		t.Fatalf("account not found for %s: %v", externalAccountID, err)
	}
	id := uuid.New().String()
	_, err := db.Exec(fmt.Sprintf(`INSERT INTO transfers
		(id, investor_account_id, correspondent_id, omnibus_account_id, state,
		 amount_cents, currency, contribution_type, business_date_ct, created_at, updated_at)
		VALUES ('%s', '%s', '00000000-0000-0000-0000-000000000010',
		        '00000000-0000-0000-0000-000000000001', 'Requested', %d,
		        'USD', 'INDIVIDUAL', '2026-01-01', datetime('now'), datetime('now'))`,
		id, accountID, amountCents))
	if err != nil {
		t.Fatalf("seed transfer: %v", err)
	}
	return id
}

func doSearch(t *testing.T, db *sql.DB, q string) *httptest.ResponseRecorder {
	t.Helper()
	h := &UIHandlers{DB: db}
	req := httptest.NewRequest(http.MethodGet, "/ui/search?q="+q, nil)
	w := httptest.NewRecorder()
	h.searchHandler(w, req)
	return w
}

// ── empty query ──────────────────────────────────────────────────────────────

func TestSearchHandler_EmptyQuery_ReturnsPlaceholder(t *testing.T) {
	db := newSearchTestDB(t)
	w := doSearch(t, db, "")
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Type to search") {
		t.Errorf("expected placeholder text, got: %s", body)
	}
}

// ── match by external account ID ─────────────────────────────────────────────

func TestSearchHandler_MatchByExternalAccountID(t *testing.T) {
	db := newSearchTestDB(t)
	seedTransfer(t, db, "INV-1001", 125000) // $1,250.00

	w := doSearch(t, db, "INV-1001")
	if w.Code != 200 {
		t.Fatalf("status %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "cmd-result") {
		t.Errorf("expected result rows, got: %s", body)
	}
	if !strings.Contains(body, "1250.00") {
		t.Errorf("expected amount in result, got: %s", body)
	}
}

// ── case-insensitive match ────────────────────────────────────────────────────

func TestSearchHandler_CaseInsensitiveAccountID(t *testing.T) {
	db := newSearchTestDB(t)
	seedTransfer(t, db, "INV-1002", 50000)

	// Lowercase search should still match uppercase external_account_id.
	w := doSearch(t, db, "inv-1002")
	body := w.Body.String()
	if !strings.Contains(body, "cmd-result") {
		t.Errorf("case-insensitive match failed, got: %s", body)
	}
}

// ── match by account name ────────────────────────────────────────────────────

func TestSearchHandler_MatchByAccountName(t *testing.T) {
	db := newSearchTestDB(t)
	// 002_investor_names.sql seeds realistic names; INV-1001 gets a specific name.
	var name string
	db.QueryRow(`SELECT account_name FROM accounts WHERE external_account_id='INV-1001'`).Scan(&name)
	if name == "" {
		t.Skip("no named account seeded")
	}
	seedTransfer(t, db, "INV-1001", 75000)

	// Search by a fragment of the account name.
	fragment := name[:4]
	w := doSearch(t, db, fragment)
	body := w.Body.String()
	if !strings.Contains(body, "cmd-result") {
		t.Errorf("name match failed for %q, got: %s", fragment, body)
	}
}

// ── match by transfer ID prefix ───────────────────────────────────────────────

func TestSearchHandler_MatchByTransferIDPrefix(t *testing.T) {
	db := newSearchTestDB(t)
	id := seedTransfer(t, db, "INV-1003", 30000)

	prefix := id[:6] // first 6 chars of UUID
	w := doSearch(t, db, prefix)
	body := w.Body.String()
	if !strings.Contains(body, id[:8]) {
		t.Errorf("ID prefix match failed, got: %s", body)
	}
}

// ── no match ─────────────────────────────────────────────────────────────────

func TestSearchHandler_NoMatch_ReturnsEmptyMessage(t *testing.T) {
	db := newSearchTestDB(t)

	w := doSearch(t, db, "XYZZY_NO_MATCH_99999")
	body := w.Body.String()
	if !strings.Contains(body, "No transfers matching") {
		t.Errorf("expected no-match message, got: %s", body)
	}
	// Query term must appear in the message.
	if !strings.Contains(body, "XYZZY_NO_MATCH_99999") {
		t.Errorf("expected query echoed in no-match message, got: %s", body)
	}
}

// ── HTML escaping ─────────────────────────────────────────────────────────────

func TestSearchHandler_HTMLInjectionEscaped(t *testing.T) {
	db := newSearchTestDB(t)

	w := doSearch(t, db, "<script>alert(1)</script>")
	body := w.Body.String()
	if strings.Contains(body, "<script>") {
		t.Errorf("unescaped HTML in response: %s", body)
	}
}

// ── result limit ─────────────────────────────────────────────────────────────

func TestSearchHandler_ResultsLimitedToEight(t *testing.T) {
	db := newSearchTestDB(t)
	for i := 0; i < 12; i++ {
		seedTransfer(t, db, "INV-1001", int64(10000+i*100))
	}

	w := doSearch(t, db, "INV-1001")
	body := w.Body.String()
	// cmd-result-id appears exactly once per result row.
	count := strings.Count(body, "cmd-result-id")
	if count > 8 {
		t.Errorf("expected at most 8 results, got %d", count)
	}
	if count == 0 {
		t.Errorf("expected some results, got none")
	}
}

// ── result HTML structure ─────────────────────────────────────────────────────

func TestSearchHandler_ResultContainsLinkAndBadge(t *testing.T) {
	db := newSearchTestDB(t)
	id := seedTransfer(t, db, "INV-1004", 99900)

	w := doSearch(t, db, id[:6])
	body := w.Body.String()
	if !strings.Contains(body, `href="/ui/transfers/`+id) {
		t.Errorf("result missing transfer link, got: %s", body)
	}
	if !strings.Contains(body, "badge--") {
		t.Errorf("result missing state badge, got: %s", body)
	}
	if !strings.Contains(body, "999.00") {
		t.Errorf("result missing formatted amount, got: %s", body)
	}
}

// ── whitespace trimming ───────────────────────────────────────────────────────

func TestSearchHandler_WhitespaceTrimmedQuery(t *testing.T) {
	db := newSearchTestDB(t)
	// A query that is only whitespace should behave like empty.
	req := httptest.NewRequest(http.MethodGet, "/ui/search?q=+++", nil)
	w := httptest.NewRecorder()
	h := &UIHandlers{DB: db}
	h.searchHandler(w, req)
	body := w.Body.String()
	if !strings.Contains(body, "Type to search") {
		t.Errorf("expected placeholder for whitespace-only query, got: %s", body)
	}
}

// ── content type ─────────────────────────────────────────────────────────────

func TestSearchHandler_ContentTypeHTML(t *testing.T) {
	db := newSearchTestDB(t)
	w := doSearch(t, db, "INV")
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected text/html content-type, got %q", ct)
	}
}
