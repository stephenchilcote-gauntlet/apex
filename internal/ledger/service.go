package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Journal struct {
	ID          string
	TransferID  string
	JournalType string // DEPOSIT_POSTING, RETURN_REVERSAL, RETURN_FEE
	Memo        string
	EffectiveAt time.Time
	CreatedAt   time.Time
}

type Entry struct {
	ID                  string
	JournalID           string
	AccountID           string
	SignedAmountCents   int64
	Currency            string
	LineType            string // PRINCIPAL, FEE
	SourceApplicationID string
	CreatedAt           time.Time
}

type AccountBalance struct {
	AccountID         string
	ExternalAccountID string
	AccountName       string
	AccountType       string
	BalanceCents      int64
	Currency          string
}

type LedgerService struct {
	DB *sql.DB
}

// PostDeposit creates a DEPOSIT_POSTING journal with two entries:
//   - investor account: +amountCents (PRINCIPAL)
//   - omnibus account: -amountCents (PRINCIPAL)
func (s *LedgerService) PostDeposit(ctx context.Context, transferID, investorAccountID, omnibusAccountID string, amountCents int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	journalID := uuid.New().String()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO ledger_journals (id, transfer_id, journal_type, memo, effective_at, created_at)
		VALUES (?, ?, 'DEPOSIT_POSTING', ?, ?, ?)`,
		journalID, transferID,
		fmt.Sprintf("Deposit posting for transfer %s", transferID),
		now, now,
	)
	if err != nil {
		return fmt.Errorf("insert deposit journal: %w", err)
	}

	// investor: +amount
	_, err = tx.ExecContext(ctx, `
		INSERT INTO ledger_entries (id, journal_id, account_id, signed_amount_cents, currency, line_type, source_application_id, created_at)
		VALUES (?, ?, ?, ?, 'USD', 'PRINCIPAL', 'mobile-check-deposit', ?)`,
		uuid.New().String(), journalID, investorAccountID, amountCents, now,
	)
	if err != nil {
		return fmt.Errorf("insert investor entry: %w", err)
	}

	// omnibus: -amount
	_, err = tx.ExecContext(ctx, `
		INSERT INTO ledger_entries (id, journal_id, account_id, signed_amount_cents, currency, line_type, source_application_id, created_at)
		VALUES (?, ?, ?, ?, 'USD', 'PRINCIPAL', 'mobile-check-deposit', ?)`,
		uuid.New().String(), journalID, omnibusAccountID, -amountCents, now,
	)
	if err != nil {
		return fmt.Errorf("insert omnibus entry: %w", err)
	}

	return tx.Commit()
}

// PostReversal creates a RETURN_REVERSAL journal (reverse the deposit) + RETURN_FEE journal ($30 fee):
//
//	Reversal: investor -amountCents, omnibus +amountCents
//	Fee: investor -feeCents, feeRevenue +feeCents
func (s *LedgerService) PostReversal(ctx context.Context, transferID, investorAccountID, omnibusAccountID, feeRevenueAccountID string, amountCents, feeCents int64) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()

	// RETURN_REVERSAL journal
	reversalJournalID := uuid.New().String()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO ledger_journals (id, transfer_id, journal_type, memo, effective_at, created_at)
		VALUES (?, ?, 'RETURN_REVERSAL', ?, ?, ?)`,
		reversalJournalID, transferID,
		fmt.Sprintf("Return reversal for transfer %s", transferID),
		now, now,
	)
	if err != nil {
		return fmt.Errorf("insert reversal journal: %w", err)
	}

	// investor: -amount
	_, err = tx.ExecContext(ctx, `
		INSERT INTO ledger_entries (id, journal_id, account_id, signed_amount_cents, currency, line_type, source_application_id, created_at)
		VALUES (?, ?, ?, ?, 'USD', 'PRINCIPAL', 'mobile-check-deposit', ?)`,
		uuid.New().String(), reversalJournalID, investorAccountID, -amountCents, now,
	)
	if err != nil {
		return fmt.Errorf("insert reversal investor entry: %w", err)
	}

	// omnibus: +amount
	_, err = tx.ExecContext(ctx, `
		INSERT INTO ledger_entries (id, journal_id, account_id, signed_amount_cents, currency, line_type, source_application_id, created_at)
		VALUES (?, ?, ?, ?, 'USD', 'PRINCIPAL', 'mobile-check-deposit', ?)`,
		uuid.New().String(), reversalJournalID, omnibusAccountID, amountCents, now,
	)
	if err != nil {
		return fmt.Errorf("insert reversal omnibus entry: %w", err)
	}

	// RETURN_FEE journal
	feeJournalID := uuid.New().String()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO ledger_journals (id, transfer_id, journal_type, memo, effective_at, created_at)
		VALUES (?, ?, 'RETURN_FEE', ?, ?, ?)`,
		feeJournalID, transferID,
		fmt.Sprintf("Return fee for transfer %s", transferID),
		now, now,
	)
	if err != nil {
		return fmt.Errorf("insert fee journal: %w", err)
	}

	// investor: -fee
	_, err = tx.ExecContext(ctx, `
		INSERT INTO ledger_entries (id, journal_id, account_id, signed_amount_cents, currency, line_type, source_application_id, created_at)
		VALUES (?, ?, ?, ?, 'USD', 'FEE', 'mobile-check-deposit', ?)`,
		uuid.New().String(), feeJournalID, investorAccountID, -feeCents, now,
	)
	if err != nil {
		return fmt.Errorf("insert fee investor entry: %w", err)
	}

	// feeRevenue: +fee
	_, err = tx.ExecContext(ctx, `
		INSERT INTO ledger_entries (id, journal_id, account_id, signed_amount_cents, currency, line_type, source_application_id, created_at)
		VALUES (?, ?, ?, ?, 'USD', 'FEE', 'mobile-check-deposit', ?)`,
		uuid.New().String(), feeJournalID, feeRevenueAccountID, feeCents, now,
	)
	if err != nil {
		return fmt.Errorf("insert fee revenue entry: %w", err)
	}

	return tx.Commit()
}

// GetAccountBalances returns all accounts with their computed balances.
func (s *LedgerService) GetAccountBalances(ctx context.Context) ([]AccountBalance, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT a.id, a.external_account_id, a.account_name, a.account_type, a.currency,
			COALESCE(SUM(le.signed_amount_cents), 0) AS balance_cents
		FROM accounts a
		LEFT JOIN ledger_entries le ON le.account_id = a.id
		GROUP BY a.id
		ORDER BY a.account_type, a.account_name`)
	if err != nil {
		return nil, fmt.Errorf("query account balances: %w", err)
	}
	defer rows.Close()

	var balances []AccountBalance
	for rows.Next() {
		var b AccountBalance
		if err := rows.Scan(&b.AccountID, &b.ExternalAccountID, &b.AccountName, &b.AccountType, &b.Currency, &b.BalanceCents); err != nil {
			return nil, fmt.Errorf("scan account balance: %w", err)
		}
		balances = append(balances, b)
	}
	return balances, rows.Err()
}

// GetAccountDetail returns one account's balance + journal entries.
func (s *LedgerService) GetAccountDetail(ctx context.Context, accountID string) (*AccountBalance, []Entry, error) {
	var b AccountBalance
	err := s.DB.QueryRowContext(ctx, `
		SELECT a.id, a.external_account_id, a.account_name, a.account_type, a.currency,
			COALESCE(SUM(le.signed_amount_cents), 0) AS balance_cents
		FROM accounts a
		LEFT JOIN ledger_entries le ON le.account_id = a.id
		WHERE a.id = ?
		GROUP BY a.id`, accountID).
		Scan(&b.AccountID, &b.ExternalAccountID, &b.AccountName, &b.AccountType, &b.Currency, &b.BalanceCents)
	if err != nil {
		return nil, nil, fmt.Errorf("query account detail: %w", err)
	}

	rows, err := s.DB.QueryContext(ctx, `
		SELECT le.id, le.journal_id, le.account_id, le.signed_amount_cents,
			le.currency, le.line_type, COALESCE(le.source_application_id, ''), le.created_at
		FROM ledger_entries le
		WHERE le.account_id = ?
		ORDER BY le.created_at ASC`, accountID)
	if err != nil {
		return nil, nil, fmt.Errorf("query account entries: %w", err)
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.JournalID, &e.AccountID, &e.SignedAmountCents,
			&e.Currency, &e.LineType, &e.SourceApplicationID, &e.CreatedAt); err != nil {
			return nil, nil, fmt.Errorf("scan entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	return &b, entries, nil
}

// GetJournalsByTransfer returns all journals and entries for a transfer.
func (s *LedgerService) GetJournalsByTransfer(ctx context.Context, transferID string) ([]Journal, []Entry, error) {
	jRows, err := s.DB.QueryContext(ctx, `
		SELECT id, transfer_id, journal_type, COALESCE(memo, ''), effective_at, created_at
		FROM ledger_journals
		WHERE transfer_id = ?
		ORDER BY created_at ASC`, transferID)
	if err != nil {
		return nil, nil, fmt.Errorf("query journals: %w", err)
	}
	defer jRows.Close()

	var journals []Journal
	for jRows.Next() {
		var j Journal
		if err := jRows.Scan(&j.ID, &j.TransferID, &j.JournalType, &j.Memo, &j.EffectiveAt, &j.CreatedAt); err != nil {
			return nil, nil, fmt.Errorf("scan journal: %w", err)
		}
		journals = append(journals, j)
	}
	if err := jRows.Err(); err != nil {
		return nil, nil, err
	}

	eRows, err := s.DB.QueryContext(ctx, `
		SELECT le.id, le.journal_id, le.account_id, le.signed_amount_cents,
			le.currency, le.line_type, COALESCE(le.source_application_id, ''), le.created_at
		FROM ledger_entries le
		JOIN ledger_journals lj ON lj.id = le.journal_id
		WHERE lj.transfer_id = ?
		ORDER BY le.created_at ASC`, transferID)
	if err != nil {
		return nil, nil, fmt.Errorf("query entries: %w", err)
	}
	defer eRows.Close()

	var entries []Entry
	for eRows.Next() {
		var e Entry
		if err := eRows.Scan(&e.ID, &e.JournalID, &e.AccountID, &e.SignedAmountCents,
			&e.Currency, &e.LineType, &e.SourceApplicationID, &e.CreatedAt); err != nil {
			return nil, nil, fmt.Errorf("scan entry: %w", err)
		}
		entries = append(entries, e)
	}
	if err := eRows.Err(); err != nil {
		return nil, nil, err
	}

	return journals, entries, nil
}
