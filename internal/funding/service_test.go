package funding

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/apex-checkout/mobile-check-deposit/internal/transfers"
	"github.com/apex-checkout/mobile-check-deposit/internal/vendorsvc/model"
	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:?_journal_mode=WAL")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE transfers (
			id TEXT PRIMARY KEY,
			investor_account_id TEXT NOT NULL,
			amount_cents INTEGER NOT NULL,
			business_date_ct TEXT,
			state TEXT NOT NULL,
			contribution_type TEXT,
			duplicate_fingerprint TEXT,
			updated_at DATETIME
		);
		CREATE TABLE accounts (
			id TEXT PRIMARY KEY,
			status TEXT NOT NULL DEFAULT 'ACTIVE',
			contribution_type_default TEXT,
			correspondent_id TEXT
		);
		CREATE TABLE rule_evaluations (
			id TEXT PRIMARY KEY,
			transfer_id TEXT NOT NULL,
			rule_name TEXT NOT NULL,
			outcome TEXT NOT NULL,
			details_json TEXT,
			created_at DATETIME
		);
	`)
	if err != nil {
		t.Fatalf("create tables: %v", err)
	}
	return db
}

func ptr(s string) *string { return &s }

func TestRuleDailyDepositLimit(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Seed an account
	_, err := db.Exec(`INSERT INTO accounts (id, status) VALUES ('acct-1', 'ACTIVE')`)
	if err != nil {
		t.Fatalf("seed account: %v", err)
	}

	svc := &FundingService{DB: db}
	today := time.Now().UTC().Format("2006-01-02")
	ctx := context.Background()

	// No prior deposits — should PASS
	t.Run("first deposit passes", func(t *testing.T) {
		tx := &transfers.Transfer{
			ID:                "tx-1",
			InvestorAccountID: "acct-1",
			AmountCents:       250_000, // $2,500
			BusinessDateCT:    &today,
		}
		result, err := svc.ruleDailyDepositLimit(ctx, tx, &model.AnalyzeResponse{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Outcome != "PASS" {
			t.Errorf("expected PASS, got %s: %s", result.Outcome, result.Details)
		}
	})

	// Seed an existing FundsPosted deposit for $8,000 today
	_, err = db.Exec(`INSERT INTO transfers (id, investor_account_id, amount_cents, business_date_ct, state)
		VALUES ('existing-1', 'acct-1', 800000, ?, 'FundsPosted')`, today)
	if err != nil {
		t.Fatalf("seed transfer: %v", err)
	}

	// $8,000 existing + $3,000 new = $11,000 > $10,000 limit — should FAIL
	t.Run("exceeds daily limit fails", func(t *testing.T) {
		tx := &transfers.Transfer{
			ID:                "tx-over",
			InvestorAccountID: "acct-1",
			AmountCents:       300_000, // $3,000
			BusinessDateCT:    &today,
		}
		result, err := svc.ruleDailyDepositLimit(ctx, tx, &model.AnalyzeResponse{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Outcome != "FAIL" {
			t.Errorf("expected FAIL, got %s: %s", result.Outcome, result.Details)
		}
	})

	// $8,000 existing + $2,000 new = $10,000 exactly — should PASS (boundary)
	t.Run("at exact limit passes", func(t *testing.T) {
		tx := &transfers.Transfer{
			ID:                "tx-exact",
			InvestorAccountID: "acct-1",
			AmountCents:       200_000, // $2,000
			BusinessDateCT:    &today,
		}
		result, err := svc.ruleDailyDepositLimit(ctx, tx, &model.AnalyzeResponse{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Outcome != "PASS" {
			t.Errorf("expected PASS at limit, got %s: %s", result.Outcome, result.Details)
		}
	})

	// Rejected deposits should not count toward daily total
	t.Run("rejected deposits excluded from daily total", func(t *testing.T) {
		_, _ = db.Exec(`INSERT INTO transfers (id, investor_account_id, amount_cents, business_date_ct, state)
			VALUES ('rejected-1', 'acct-1', 500000, ?, 'Rejected')`, today)
		tx := &transfers.Transfer{
			ID:                "tx-after-rejected",
			InvestorAccountID: "acct-1",
			AmountCents:       200_000, // $2,000
			BusinessDateCT:    &today,
		}
		result, err := svc.ruleDailyDepositLimit(ctx, tx, &model.AnalyzeResponse{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Outcome != "PASS" {
			t.Errorf("rejected deposits should not count, got %s: %s", result.Outcome, result.Details)
		}
	})
}
