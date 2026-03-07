package ledger

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Journal struct {
	ID          string
	TransferID  string
	JournalType string
	Memo        *string
	EffectiveAt time.Time
	CreatedAt   time.Time
}

type Entry struct {
	ID                  string
	JournalID           string
	AccountID           string
	SignedAmountCents   int64
	Currency            string
	LineType            string
	SourceApplicationID *string
	CreatedAt           time.Time
}

type LedgerService struct{}

func (s *LedgerService) PostDeposit(db *sql.DB, transferID, investorAccountID, omnibusAccountID string, amountCents int64) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	journalID := uuid.New().String()

	_, err = tx.Exec(`
		INSERT INTO ledger_journals (id, transfer_id, journal_type, memo, effective_at, created_at)
		VALUES (?, ?, 'DEPOSIT_POSTING', 'Check deposit posting', ?, ?)`,
		journalID, transferID, now, now)
	if err != nil {
		return fmt.Errorf("insert journal: %w", err)
	}

	// Credit investor account (positive = credit)
	_, err = tx.Exec(`
		INSERT INTO ledger_entries (id, journal_id, account_id, signed_amount_cents, currency, line_type, created_at)
		VALUES (?, ?, ?, ?, 'USD', 'PRINCIPAL', ?)`,
		uuid.New().String(), journalID, investorAccountID, amountCents, now)
	if err != nil {
		return fmt.Errorf("insert investor entry: %w", err)
	}

	// Debit omnibus account (negative = debit)
	_, err = tx.Exec(`
		INSERT INTO ledger_entries (id, journal_id, account_id, signed_amount_cents, currency, line_type, created_at)
		VALUES (?, ?, ?, ?, 'USD', 'PRINCIPAL', ?)`,
		uuid.New().String(), journalID, omnibusAccountID, -amountCents, now)
	if err != nil {
		return fmt.Errorf("insert omnibus entry: %w", err)
	}

	return tx.Commit()
}

func (s *LedgerService) PostReversal(db *sql.DB, transferID, investorAccountID, omnibusAccountID, feeRevenueAccountID string, amountCents, feeCents int64) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()

	// Reversal journal: undo the original deposit
	reversalJournalID := uuid.New().String()
	_, err = tx.Exec(`
		INSERT INTO ledger_journals (id, transfer_id, journal_type, memo, effective_at, created_at)
		VALUES (?, ?, 'RETURN_REVERSAL', 'Return reversal', ?, ?)`,
		reversalJournalID, transferID, now, now)
	if err != nil {
		return fmt.Errorf("insert reversal journal: %w", err)
	}

	// Debit investor (reverse the original credit)
	_, err = tx.Exec(`
		INSERT INTO ledger_entries (id, journal_id, account_id, signed_amount_cents, currency, line_type, created_at)
		VALUES (?, ?, ?, ?, 'USD', 'PRINCIPAL', ?)`,
		uuid.New().String(), reversalJournalID, investorAccountID, -amountCents, now)
	if err != nil {
		return fmt.Errorf("insert reversal investor entry: %w", err)
	}

	// Credit omnibus (reverse the original debit)
	_, err = tx.Exec(`
		INSERT INTO ledger_entries (id, journal_id, account_id, signed_amount_cents, currency, line_type, created_at)
		VALUES (?, ?, ?, ?, 'USD', 'PRINCIPAL', ?)`,
		uuid.New().String(), reversalJournalID, omnibusAccountID, amountCents, now)
	if err != nil {
		return fmt.Errorf("insert reversal omnibus entry: %w", err)
	}

	// Fee journal: charge the return fee
	feeJournalID := uuid.New().String()
	_, err = tx.Exec(`
		INSERT INTO ledger_journals (id, transfer_id, journal_type, memo, effective_at, created_at)
		VALUES (?, ?, 'RETURN_FEE', 'Return fee $30', ?, ?)`,
		feeJournalID, transferID, now, now)
	if err != nil {
		return fmt.Errorf("insert fee journal: %w", err)
	}

	// Debit investor for the fee
	_, err = tx.Exec(`
		INSERT INTO ledger_entries (id, journal_id, account_id, signed_amount_cents, currency, line_type, created_at)
		VALUES (?, ?, ?, ?, 'USD', 'FEE', ?)`,
		uuid.New().String(), feeJournalID, investorAccountID, -feeCents, now)
	if err != nil {
		return fmt.Errorf("insert fee investor entry: %w", err)
	}

	// Credit fee revenue account
	_, err = tx.Exec(`
		INSERT INTO ledger_entries (id, journal_id, account_id, signed_amount_cents, currency, line_type, created_at)
		VALUES (?, ?, ?, ?, 'USD', 'FEE', ?)`,
		uuid.New().String(), feeJournalID, feeRevenueAccountID, feeCents, now)
	if err != nil {
		return fmt.Errorf("insert fee revenue entry: %w", err)
	}

	return tx.Commit()
}

func (s *LedgerService) GetJournalsByTransfer(db *sql.DB, transferID string) ([]Journal, error) {
	rows, err := db.Query(`
		SELECT id, transfer_id, journal_type, memo, effective_at, created_at
		FROM ledger_journals WHERE transfer_id = ? ORDER BY created_at ASC`, transferID)
	if err != nil {
		return nil, fmt.Errorf("query journals: %w", err)
	}
	defer rows.Close()

	var journals []Journal
	for rows.Next() {
		var j Journal
		if err := rows.Scan(&j.ID, &j.TransferID, &j.JournalType, &j.Memo, &j.EffectiveAt, &j.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan journal: %w", err)
		}
		journals = append(journals, j)
	}
	return journals, rows.Err()
}

func (s *LedgerService) GetEntriesByJournal(db *sql.DB, journalID string) ([]Entry, error) {
	rows, err := db.Query(`
		SELECT id, journal_id, account_id, signed_amount_cents, currency, line_type, source_application_id, created_at
		FROM ledger_entries WHERE journal_id = ? ORDER BY created_at ASC`, journalID)
	if err != nil {
		return nil, fmt.Errorf("query entries: %w", err)
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.JournalID, &e.AccountID, &e.SignedAmountCents, &e.Currency, &e.LineType, &e.SourceApplicationID, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
