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

func TestRuleAccountEligible_ActivePasses(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	_, _ = db.Exec(`INSERT INTO accounts (id, status) VALUES ('acct-active', 'ACTIVE')`)

	svc := &FundingService{DB: db}
	tx := &transfers.Transfer{ID: "tx-acct", InvestorAccountID: "acct-active"}
	result, err := svc.ruleAccountEligible(context.Background(), tx, &model.AnalyzeResponse{})
	if err != nil {
		t.Fatalf("ruleAccountEligible: %v", err)
	}
	if result.Outcome != "PASS" {
		t.Errorf("outcome = %q, want PASS", result.Outcome)
	}
}

func TestRuleAccountEligible_InactiveFails(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	_, _ = db.Exec(`INSERT INTO accounts (id, status) VALUES ('acct-inactive', 'INACTIVE')`)

	svc := &FundingService{DB: db}
	tx := &transfers.Transfer{ID: "tx-inactive", InvestorAccountID: "acct-inactive"}
	result, err := svc.ruleAccountEligible(context.Background(), tx, &model.AnalyzeResponse{})
	if err != nil {
		t.Fatalf("ruleAccountEligible: %v", err)
	}
	if result.Outcome != "FAIL" {
		t.Errorf("outcome = %q, want FAIL for inactive account", result.Outcome)
	}
}

func TestRuleMaxDepositLimit_WithinLimitPasses(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := &FundingService{DB: db}

	tx := &transfers.Transfer{ID: "tx-ok", AmountCents: 499_999}
	result, err := svc.ruleMaxDepositLimit(context.Background(), tx, &model.AnalyzeResponse{})
	if err != nil {
		t.Fatalf("ruleMaxDepositLimit: %v", err)
	}
	if result.Outcome != "PASS" {
		t.Errorf("outcome = %q, want PASS for $4,999.99", result.Outcome)
	}
}

func TestRuleMaxDepositLimit_ExceedsLimitFails(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := &FundingService{DB: db}

	tx := &transfers.Transfer{ID: "tx-over", AmountCents: 500_001}
	result, err := svc.ruleMaxDepositLimit(context.Background(), tx, &model.AnalyzeResponse{})
	if err != nil {
		t.Fatalf("ruleMaxDepositLimit: %v", err)
	}
	if result.Outcome != "FAIL" {
		t.Errorf("outcome = %q, want FAIL for $5,000.01", result.Outcome)
	}
}

func TestRuleMaxDepositLimit_AtExactLimitPasses(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	svc := &FundingService{DB: db}

	tx := &transfers.Transfer{ID: "tx-exact", AmountCents: 500_000} // exactly $5,000
	result, err := svc.ruleMaxDepositLimit(context.Background(), tx, &model.AnalyzeResponse{})
	if err != nil {
		t.Fatalf("ruleMaxDepositLimit: %v", err)
	}
	if result.Outcome != "PASS" {
		t.Errorf("outcome = %q, want PASS for exactly $5,000", result.Outcome)
	}
}

func TestRuleInternalDuplicate_NoMICRSkips(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	_, _ = db.Exec(`INSERT INTO transfers (id, investor_account_id, amount_cents, state) VALUES ('tx-nomicr', 'acct-1', 10000, 'Analyzing')`)

	svc := &FundingService{DB: db}
	tx := &transfers.Transfer{ID: "tx-nomicr", InvestorAccountID: "acct-1", AmountCents: 10000}
	result, err := svc.ruleInternalDuplicate(context.Background(), tx, &model.AnalyzeResponse{MICR: nil})
	if err != nil {
		t.Fatalf("ruleInternalDuplicate: %v", err)
	}
	if result.Outcome != "PASS" {
		t.Errorf("outcome = %q, want PASS when no MICR data", result.Outcome)
	}
}

func TestRuleInternalDuplicate_NoDuplicatePasses(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	_, _ = db.Exec(`INSERT INTO transfers (id, investor_account_id, amount_cents, state) VALUES ('tx-uniq', 'acct-1', 10000, 'Analyzing')`)

	svc := &FundingService{DB: db}
	tx := &transfers.Transfer{ID: "tx-uniq", InvestorAccountID: "acct-1", AmountCents: 10000}
	vendorResp := &model.AnalyzeResponse{
		MICR: &model.MICRResult{Routing: "021000021", Account: "123456789", Serial: "1001"},
	}
	result, err := svc.ruleInternalDuplicate(context.Background(), tx, vendorResp)
	if err != nil {
		t.Fatalf("ruleInternalDuplicate: %v", err)
	}
	if result.Outcome != "PASS" {
		t.Errorf("outcome = %q, want PASS for unique transfer", result.Outcome)
	}
}

func TestRuleInternalDuplicate_DuplicateFails(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Run ruleInternalDuplicate for tx-dup-1 first to store its fingerprint;
	// then run it for tx-dup-2 (same MICR+amount+account) and expect FAIL.
	_, _ = db.Exec(`INSERT INTO transfers (id, investor_account_id, amount_cents, state) VALUES ('tx-dup-1', 'acct-1', 15000, 'FundsPosted')`)
	_, _ = db.Exec(`INSERT INTO transfers (id, investor_account_id, amount_cents, state) VALUES ('tx-dup-2', 'acct-1', 15000, 'Analyzing')`)

	svc := &FundingService{DB: db}
	vendorResp := &model.AnalyzeResponse{
		MICR: &model.MICRResult{Routing: "021000021", Account: "987654321", Serial: "2001"},
	}

	// Run the rule on tx-dup-1 first to set its fingerprint
	tx1 := &transfers.Transfer{ID: "tx-dup-1", InvestorAccountID: "acct-1", AmountCents: 15000}
	if _, err := svc.ruleInternalDuplicate(context.Background(), tx1, vendorResp); err != nil {
		t.Fatalf("ruleInternalDuplicate tx1: %v", err)
	}

	// Run the rule on tx-dup-2 with the same MICR+amount+account — should detect duplicate
	tx2 := &transfers.Transfer{ID: "tx-dup-2", InvestorAccountID: "acct-1", AmountCents: 15000}
	result, err := svc.ruleInternalDuplicate(context.Background(), tx2, vendorResp)
	if err != nil {
		t.Fatalf("ruleInternalDuplicate tx2: %v", err)
	}
	if result.Outcome != "FAIL" {
		t.Errorf("outcome = %q, want FAIL for duplicate MICR+amount+account", result.Outcome)
	}
}

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

func TestRuleContributionTypeDefault_SetsFromAccount(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ct := "INDIVIDUAL"
	_, _ = db.Exec(`INSERT INTO accounts (id, status, contribution_type_default) VALUES ('acct-ct', 'ACTIVE', ?)`, ct)
	_, _ = db.Exec(`INSERT INTO transfers (id, investor_account_id, amount_cents, state) VALUES ('tx-ct', 'acct-ct', 10000, 'Analyzing')`)

	svc := &FundingService{DB: db}
	tx := &transfers.Transfer{ID: "tx-ct", InvestorAccountID: "acct-ct"}
	result, err := svc.ruleContributionTypeDefault(context.Background(), tx, &model.AnalyzeResponse{})
	if err != nil {
		t.Fatalf("ruleContributionTypeDefault: %v", err)
	}
	if result.Outcome != "PASS" {
		t.Errorf("outcome = %q, want PASS", result.Outcome)
	}
	if tx.ContributionType == nil || *tx.ContributionType != ct {
		t.Errorf("ContributionType = %v, want %q", tx.ContributionType, ct)
	}
}

func TestRuleContributionTypeDefault_AlreadySet(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, _ = db.Exec(`INSERT INTO accounts (id, status, contribution_type_default) VALUES ('acct-ct2', 'ACTIVE', 'INDIVIDUAL')`)
	_, _ = db.Exec(`INSERT INTO transfers (id, investor_account_id, amount_cents, state) VALUES ('tx-ct2', 'acct-ct2', 10000, 'Analyzing')`)

	svc := &FundingService{DB: db}
	existing := "ROLLOVER"
	tx := &transfers.Transfer{ID: "tx-ct2", InvestorAccountID: "acct-ct2", ContributionType: &existing}
	result, err := svc.ruleContributionTypeDefault(context.Background(), tx, &model.AnalyzeResponse{})
	if err != nil {
		t.Fatalf("ruleContributionTypeDefault: %v", err)
	}
	if result.Outcome != "PASS" {
		t.Errorf("outcome = %q, want PASS", result.Outcome)
	}
	// Should NOT overwrite the already-set contribution type
	if tx.ContributionType == nil || *tx.ContributionType != "ROLLOVER" {
		t.Errorf("ContributionType = %v, want ROLLOVER (should not be overwritten)", tx.ContributionType)
	}
}

func TestRuleContributionTypeDefault_NoDefaultConfigured(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	_, _ = db.Exec(`INSERT INTO accounts (id, status) VALUES ('acct-noct', 'ACTIVE')`)
	_, _ = db.Exec(`INSERT INTO transfers (id, investor_account_id, amount_cents, state) VALUES ('tx-noct', 'acct-noct', 10000, 'Analyzing')`)

	svc := &FundingService{DB: db}
	tx := &transfers.Transfer{ID: "tx-noct", InvestorAccountID: "acct-noct"}
	result, err := svc.ruleContributionTypeDefault(context.Background(), tx, &model.AnalyzeResponse{})
	if err != nil {
		t.Fatalf("ruleContributionTypeDefault: %v", err)
	}
	if result.Outcome != "PASS" {
		t.Errorf("outcome = %q, want PASS when no default configured", result.Outcome)
	}
	if tx.ContributionType != nil {
		t.Errorf("ContributionType should be nil when no default, got %v", *tx.ContributionType)
	}
}
