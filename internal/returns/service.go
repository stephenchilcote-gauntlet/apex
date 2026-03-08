package returns

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/apex-checkout/mobile-check-deposit/internal/audit"
	"github.com/apex-checkout/mobile-check-deposit/internal/ledger"
	"github.com/apex-checkout/mobile-check-deposit/internal/transfers"
	"github.com/google/uuid"
)

type ReturnNotification struct {
	ID             string
	TransferID     string
	ReasonCode     string
	ReasonText     string
	FeeCents       int64
	RawPayloadJSON *string
	ReceivedAt     time.Time
	ProcessedAt    *time.Time
}

type ReturnsService struct {
	DB          *sql.DB
	TransferSvc *transfers.TransferService
	LedgerSvc   *ledger.LedgerService
}

func (s *ReturnsService) ProcessReturn(ctx interface{}, transferID, reasonCode, reasonText string) error {
	// Get the transfer and verify eligible state
	t, err := s.TransferSvc.GetByID(s.DB, transferID)
	if err != nil {
		return fmt.Errorf("get transfer: %w", err)
	}

	if t.State != transfers.StateFundsPosted && t.State != transfers.StateCompleted {
		return fmt.Errorf("transfer %s in state %s is not eligible for return", transferID, t.State)
	}

	var feeRevenueAccountID string
	err = s.DB.QueryRow(`
		SELECT id FROM accounts
		WHERE correspondent_id = ? AND account_type = 'FEE_REVENUE'
		LIMIT 1`, t.CorrespondentID).Scan(&feeRevenueAccountID)
	if err != nil {
		return fmt.Errorf("resolve fee revenue account: %w", err)
	}

	now := time.Now().UTC()
	notifID := uuid.New().String()
	var feeCents int64 = 3000

	// Create return notification
	_, err = s.DB.Exec(`
		INSERT INTO return_notifications (id, transfer_id, reason_code, reason_text, fee_cents, received_at, processed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		notifID, transferID, reasonCode, reasonText, feeCents, now, now)
	if err != nil {
		return fmt.Errorf("insert return notification: %w", err)
	}

	// Post reversal and fee via ledger
	err = s.LedgerSvc.PostReversal(s.DB, transferID, t.InvestorAccountID, t.OmnibusAccountID, feeRevenueAccountID, t.AmountCents, feeCents)
	if err != nil {
		return fmt.Errorf("post reversal: %w", err)
	}

	// Transition transfer to Returned
	if err := s.TransferSvc.Transition(s.DB, transferID, transfers.StateReturned, "SYSTEM", "returns"); err != nil {
		return fmt.Errorf("transition to Returned: %w", err)
	}

	// Update transfer return fields
	_, err = s.DB.Exec(`UPDATE transfers SET return_reason_code = ?, return_fee_cents = ?, returned_at = ?, updated_at = ? WHERE id = ?`,
		reasonCode, feeCents, now, now, transferID)
	if err != nil {
		return fmt.Errorf("update transfer return fields: %w", err)
	}

	// Create notifications outbox record
	_, err = s.DB.Exec(`
		INSERT INTO notifications_outbox (id, transfer_id, channel, template_code, status, created_at)
		VALUES (?, ?, 'EMAIL', 'RETURNED_CHECK', 'PENDING', ?)`,
		uuid.New().String(), transferID, now)
	if err != nil {
		return fmt.Errorf("insert notification outbox: %w", err)
	}

	// Create audit event
	err = audit.LogEvent(s.DB, audit.Event{
		EntityType: "transfer",
		EntityID:   transferID,
		ActorType:  "SYSTEM",
		ActorID:    "returns",
		EventType:  "RETURN_PROCESSED",
		CreatedAt:  now,
	})
	if err != nil {
		return fmt.Errorf("log audit event: %w", err)
	}

	slog.Info("return processed", "transferID", transferID, "reasonCode", reasonCode, "feeCents", feeCents)
	return nil
}
