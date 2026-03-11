package ledger

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

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

	content, err := os.ReadFile(filepath.Join(migrationsDir(), "001_init.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(content)); err != nil {
		t.Fatalf("migration: %v", err)
	}
	return db
}

// Seeded account IDs from 001_init.sql
const (
	investorAccountID = "00000000-0000-0000-0000-000000001001" // INV-1001
	omnibusAccountID  = "00000000-0000-0000-0000-000000000001" // OMNI-ACME
	feeRevenueID      = "00000000-0000-0000-0000-000000000002" // FEE-REVENUE
	correspondentID   = "00000000-0000-0000-0000-000000000010" // ACME
)

// seedTransfer inserts a minimal transfer row so ledger FK constraints are satisfied.
func seedTransfer(t *testing.T, db *sql.DB, transferID string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO transfers (
			id, investor_account_id, correspondent_id, omnibus_account_id,
			amount_cents, currency, state, created_at, updated_at
		) VALUES (?, ?, ?, ?, 25000, 'USD', 'FundsPosted', datetime('now'), datetime('now'))`,
		transferID, investorAccountID, correspondentID, omnibusAccountID)
	if err != nil {
		t.Fatalf("seedTransfer: %v", err)
	}
}

func accountBalance(t *testing.T, db *sql.DB, accountID string) int64 {
	t.Helper()
	var bal int64
	err := db.QueryRow(`
		SELECT COALESCE(SUM(signed_amount_cents), 0)
		FROM ledger_entries WHERE account_id = ?`, accountID).Scan(&bal)
	if err != nil {
		t.Fatalf("accountBalance: %v", err)
	}
	return bal
}

func TestPostDeposit_CreditsInvestorDebitsOmnibus(t *testing.T) {
	db := newTestDB(t)
	svc := &LedgerService{}

	const amount = int64(25000) // $250.00

	// Capture balances before to compute delta (seed data has pre-existing entries)
	invBefore := accountBalance(t, db, investorAccountID)
	omniBefore := accountBalance(t, db, omnibusAccountID)

	seedTransfer(t, db, "transfer-001")
	if err := svc.PostDeposit(db, "transfer-001", investorAccountID, omnibusAccountID, amount); err != nil {
		t.Fatalf("PostDeposit: %v", err)
	}

	invDelta := accountBalance(t, db, investorAccountID) - invBefore
	omniDelta := accountBalance(t, db, omnibusAccountID) - omniBefore

	if invDelta != amount {
		t.Errorf("investor balance delta = %d, want %d", invDelta, amount)
	}
	if omniDelta != -amount {
		t.Errorf("omnibus balance delta = %d, want %d", omniDelta, -amount)
	}
}

func TestPostDeposit_ZeroSumInvariant(t *testing.T) {
	db := newTestDB(t)
	svc := &LedgerService{}

	amounts := []int64{10000, 25000, 50000}
	for i, amt := range amounts {
		transferID := fmt.Sprintf("transfer-%03d", i)
		seedTransfer(t, db, transferID)
		if err := svc.PostDeposit(db, transferID, investorAccountID, omnibusAccountID, amt); err != nil {
			t.Fatalf("PostDeposit: %v", err)
		}
	}

	// Zero-sum: sum of all entries should be 0
	var total int64
	err := db.QueryRow(`SELECT COALESCE(SUM(signed_amount_cents), 0) FROM ledger_entries`).Scan(&total)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Errorf("ledger sum = %d, want 0 (zero-sum invariant)", total)
	}
}

func TestPostDeposit_CreatesJournal(t *testing.T) {
	db := newTestDB(t)
	svc := &LedgerService{}

	const transferID = "transfer-journal-test"
	seedTransfer(t, db, transferID)
	if err := svc.PostDeposit(db, transferID, investorAccountID, omnibusAccountID, 15000); err != nil {
		t.Fatalf("PostDeposit: %v", err)
	}

	journals, err := svc.GetJournalsByTransfer(db, transferID)
	if err != nil {
		t.Fatalf("GetJournalsByTransfer: %v", err)
	}
	if len(journals) != 1 {
		t.Fatalf("expected 1 journal, got %d", len(journals))
	}
	if journals[0].JournalType != "DEPOSIT_POSTING" {
		t.Errorf("JournalType = %q, want DEPOSIT_POSTING", journals[0].JournalType)
	}
	if journals[0].TransferID != transferID {
		t.Errorf("TransferID = %q, want %q", journals[0].TransferID, transferID)
	}
}

func TestPostDeposit_JournalHasTwoEntries(t *testing.T) {
	db := newTestDB(t)
	svc := &LedgerService{}

	const transferID = "transfer-entries-test"
	seedTransfer(t, db, transferID)
	if err := svc.PostDeposit(db, transferID, investorAccountID, omnibusAccountID, 20000); err != nil {
		t.Fatalf("PostDeposit: %v", err)
	}

	journals, _ := svc.GetJournalsByTransfer(db, transferID)
	entries, err := svc.GetEntriesByJournal(db, journals[0].ID)
	if err != nil {
		t.Fatalf("GetEntriesByJournal: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestPostReversal_ZeroSumInvariant(t *testing.T) {
	db := newTestDB(t)
	svc := &LedgerService{}

	const transferID = "transfer-reversal"
	const amount = int64(40000) // $400.00
	const fee = int64(3000)     // $30.00

	seedTransfer(t, db, transferID)
	// First post the deposit
	if err := svc.PostDeposit(db, transferID, investorAccountID, omnibusAccountID, amount); err != nil {
		t.Fatalf("PostDeposit: %v", err)
	}

	// Then post the reversal with fee
	if err := svc.PostReversal(db, transferID, investorAccountID, omnibusAccountID, feeRevenueID, amount, fee); err != nil {
		t.Fatalf("PostReversal: %v", err)
	}

	// Zero-sum invariant must still hold
	var total int64
	err := db.QueryRow(`SELECT COALESCE(SUM(signed_amount_cents), 0) FROM ledger_entries`).Scan(&total)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Errorf("ledger sum = %d after reversal, want 0", total)
	}
}

func TestPostReversal_InvestorDebited(t *testing.T) {
	db := newTestDB(t)
	svc := &LedgerService{}

	const transferID = "transfer-reversal-check"
	const amount = int64(35000)
	const fee = int64(3000)

	invBefore := accountBalance(t, db, investorAccountID)
	feeBefore := accountBalance(t, db, feeRevenueID)

	seedTransfer(t, db, transferID)
	if err := svc.PostDeposit(db, transferID, investorAccountID, omnibusAccountID, amount); err != nil {
		t.Fatalf("PostDeposit: %v", err)
	}
	if err := svc.PostReversal(db, transferID, investorAccountID, omnibusAccountID, feeRevenueID, amount, fee); err != nil {
		t.Fatalf("PostReversal: %v", err)
	}

	// Investor: credited amount then debited amount + fee => net delta = -fee
	invDelta := accountBalance(t, db, investorAccountID) - invBefore
	if invDelta != -fee {
		t.Errorf("investor balance delta after reversal = %d, want %d (just the fee debit)", invDelta, -fee)
	}

	// Fee revenue: delta should be +fee
	feeDelta := accountBalance(t, db, feeRevenueID) - feeBefore
	if feeDelta != fee {
		t.Errorf("fee revenue balance delta = %d, want %d", feeDelta, fee)
	}
}

func TestPostReversal_CreatesTwoJournals(t *testing.T) {
	db := newTestDB(t)
	svc := &LedgerService{}

	const transferID = "transfer-two-journals"
	seedTransfer(t, db, transferID)
	if err := svc.PostDeposit(db, transferID, investorAccountID, omnibusAccountID, 20000); err != nil {
		t.Fatalf("PostDeposit: %v", err)
	}
	if err := svc.PostReversal(db, transferID, investorAccountID, omnibusAccountID, feeRevenueID, 20000, 3000); err != nil {
		t.Fatalf("PostReversal: %v", err)
	}

	// Reversal + fee = 2 new journals (plus the original deposit journal = 3 total)
	journals, err := svc.GetJournalsByTransfer(db, transferID)
	if err != nil {
		t.Fatalf("GetJournalsByTransfer: %v", err)
	}
	if len(journals) != 3 {
		t.Fatalf("expected 3 journals (deposit + reversal + fee), got %d", len(journals))
	}

	types := map[string]bool{}
	for _, j := range journals {
		types[j.JournalType] = true
	}
	for _, want := range []string{"DEPOSIT_POSTING", "RETURN_REVERSAL", "RETURN_FEE"} {
		if !types[want] {
			t.Errorf("missing journal type %q", want)
		}
	}
}
