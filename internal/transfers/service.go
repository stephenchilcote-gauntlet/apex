package transfers

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/apex-checkout/mobile-check-deposit/internal/audit"
	"github.com/google/uuid"
)

type TransferFilters struct {
	State             *State
	InvestorAccountID *string
	ReviewRequired    *bool
	ReviewStatus      *string
}

type TransferService struct{}

func (s *TransferService) Create(db *sql.DB, t *Transfer) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	t.CreatedAt = now
	t.UpdatedAt = now
	if t.State == "" {
		t.State = StateRequested
	}

	_, err := db.Exec(`
		INSERT INTO transfers (
			id, client_request_id, investor_account_id, correspondent_id,
			omnibus_account_id, state, amount_cents, currency,
			contribution_type, review_required, review_status,
			business_date_ct,
			rejection_code, rejection_message,
			return_reason_code, return_fee_cents,
			duplicate_fingerprint,
			submitted_at, approved_at, posted_at, completed_at, returned_at,
			created_at, updated_at
		) VALUES (
			?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?,
			?,
			?, ?,
			?, ?,
			?,
			?, ?, ?, ?, ?,
			?, ?
		)`,
		t.ID, t.ClientRequestID, t.InvestorAccountID, t.CorrespondentID,
		t.OmnibusAccountID, string(t.State), t.AmountCents, t.Currency,
		t.ContributionType, t.ReviewRequired, t.ReviewStatus,
		t.BusinessDateCT,
		t.RejectionCode, t.RejectionMessage,
		t.ReturnReasonCode, t.ReturnFeeCents,
		t.DuplicateFingerprint,
		t.SubmittedAt, t.ApprovedAt, t.PostedAt, t.CompletedAt, t.ReturnedAt,
		t.CreatedAt, t.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert transfer: %w", err)
	}

	slog.Info("transfer created", "id", t.ID, "state", t.State)
	return nil
}

func (s *TransferService) GetByID(db *sql.DB, id string) (*Transfer, error) {
	row := db.QueryRow(`
		SELECT
			id, client_request_id, investor_account_id, correspondent_id,
			omnibus_account_id, state, amount_cents, currency,
			contribution_type, review_required, review_status,
			business_date_ct,
			rejection_code, rejection_message,
			return_reason_code, return_fee_cents,
			duplicate_fingerprint,
			submitted_at, approved_at, posted_at, completed_at, returned_at,
			created_at, updated_at
		FROM transfers WHERE id = ?`, id)

	return scanTransfer(row)
}

func (s *TransferService) List(db *sql.DB, filters TransferFilters) ([]Transfer, error) {
	query := `
		SELECT
			id, client_request_id, investor_account_id, correspondent_id,
			omnibus_account_id, state, amount_cents, currency,
			contribution_type, review_required, review_status,
			business_date_ct,
			rejection_code, rejection_message,
			return_reason_code, return_fee_cents,
			duplicate_fingerprint,
			submitted_at, approved_at, posted_at, completed_at, returned_at,
			created_at, updated_at
		FROM transfers WHERE 1=1`
	var args []any

	if filters.State != nil {
		query += " AND state = ?"
		args = append(args, string(*filters.State))
	}
	if filters.InvestorAccountID != nil {
		query += " AND investor_account_id = ?"
		args = append(args, *filters.InvestorAccountID)
	}
	if filters.ReviewRequired != nil {
		query += " AND review_required = ?"
		args = append(args, *filters.ReviewRequired)
	}
	if filters.ReviewStatus != nil {
		query += " AND review_status = ?"
		args = append(args, *filters.ReviewStatus)
	}

	query += " ORDER BY created_at DESC"

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list transfers: %w", err)
	}
	defer rows.Close()

	var transfers []Transfer
	for rows.Next() {
		t, err := scanTransferRows(rows)
		if err != nil {
			return nil, fmt.Errorf("scan transfer: %w", err)
		}
		transfers = append(transfers, *t)
	}
	return transfers, rows.Err()
}

func (s *TransferService) Transition(db *sql.DB, transferID string, to State, actorType, actorID string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	var currentState string
	err = tx.QueryRow("SELECT state FROM transfers WHERE id = ?", transferID).Scan(&currentState)
	if err != nil {
		return fmt.Errorf("get current state: %w", err)
	}

	from := State(currentState)
	if !CanTransition(from, to) {
		return fmt.Errorf("invalid transition from %s to %s", from, to)
	}

	now := time.Now().UTC()
	_, err = tx.Exec("UPDATE transfers SET state = ?, updated_at = ? WHERE id = ?",
		string(to), now, transferID)
	if err != nil {
		return fmt.Errorf("update state: %w", err)
	}

	fromStr := string(from)
	toStr := string(to)
	err = audit.LogEventTx(tx, audit.Event{
		ID:         uuid.New().String(),
		EntityType: "transfer",
		EntityID:   transferID,
		ActorType:  actorType,
		ActorID:    actorID,
		EventType:  "STATE_TRANSITION",
		FromState:  &fromStr,
		ToState:    &toStr,
		CreatedAt:  now,
	})
	if err != nil {
		return fmt.Errorf("log audit event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transition: %w", err)
	}

	slog.Info("transfer transitioned", "id", transferID, "from", from, "to", to)
	return nil
}

func (s *TransferService) UpdateReviewStatus(db *sql.DB, transferID string, status string) error {
	_, err := db.Exec("UPDATE transfers SET review_status = ?, updated_at = ? WHERE id = ?",
		status, time.Now().UTC(), transferID)
	if err != nil {
		return fmt.Errorf("update review status: %w", err)
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanTransferFrom(s scanner) (*Transfer, error) {
	var t Transfer
	var state string
	err := s.Scan(
		&t.ID, &t.ClientRequestID, &t.InvestorAccountID, &t.CorrespondentID,
		&t.OmnibusAccountID, &state, &t.AmountCents, &t.Currency,
		&t.ContributionType, &t.ReviewRequired, &t.ReviewStatus,
		&t.BusinessDateCT,
		&t.RejectionCode, &t.RejectionMessage,
		&t.ReturnReasonCode, &t.ReturnFeeCents,
		&t.DuplicateFingerprint,
		&t.SubmittedAt, &t.ApprovedAt, &t.PostedAt, &t.CompletedAt, &t.ReturnedAt,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	t.State = State(state)
	return &t, nil
}

func scanTransfer(row *sql.Row) (*Transfer, error) {
	return scanTransferFrom(row)
}

func scanTransferRows(rows *sql.Rows) (*Transfer, error) {
	return scanTransferFrom(rows)
}
