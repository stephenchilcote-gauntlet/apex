package settlement

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/apex-checkout/mobile-check-deposit/internal/transfers"
	"github.com/google/uuid"
)

type Batch struct {
	ID               string
	BusinessDateCT   string
	CutoffAtCT       *string
	FileFormat       string
	FilePath         *string
	Status           string
	TotalItems       int
	TotalAmountCents int64
	AckReference     *string
	CreatedAt        time.Time
	SubmittedAt      *time.Time
	AcknowledgedAt   *time.Time
}

type BatchItem struct {
	ID               string
	BatchID          string
	TransferID       string
	SequenceNumber   int
	AmountCents      int64
	MICRSnapshotJSON *string
	FrontImagePath   *string
	BackImagePath    *string
	CreatedAt        time.Time
}

type SettlementService struct {
	DB          *sql.DB
	OutputPath  string
	TransferSvc *transfers.TransferService
}

type fileHeader struct {
	BatchID          string `json:"batchId"`
	BusinessDateCT   string `json:"businessDateCT"`
	CreatedAt        string `json:"createdAt"`
	Format           string `json:"format"`
	TotalItems       int    `json:"totalItems"`
	TotalAmountCents int64  `json:"totalAmountCents"`
}

type micrData struct {
	RoutingNumber string `json:"routingNumber"`
	AccountNumber string `json:"accountNumber"`
	CheckNumber   string `json:"checkNumber"`
}

type imageData struct {
	FrontPath *string `json:"frontPath"`
	BackPath  *string `json:"backPath"`
}

type fileItem struct {
	SequenceNumber int      `json:"sequenceNumber"`
	TransferID     string   `json:"transferId"`
	AmountCents    int64    `json:"amountCents"`
	MICR           micrData `json:"micr"`
	Images         imageData `json:"images"`
}

type settlementFile struct {
	FileHeader fileHeader `json:"fileHeader"`
	Items      []fileItem `json:"items"`
}

func (s *SettlementService) GenerateBatch(ctx interface{}, businessDateCT string) (*Batch, error) {
	rows, err := s.DB.Query(`
		SELECT t.id, t.amount_cents
		FROM transfers t
		WHERE t.state = 'FundsPosted'
		  AND t.business_date_ct = ?
		  AND t.id NOT IN (SELECT transfer_id FROM settlement_batch_items)
		ORDER BY t.created_at ASC`, businessDateCT)
	if err != nil {
		return nil, fmt.Errorf("query eligible transfers: %w", err)
	}
	defer rows.Close()

	type eligible struct {
		ID          string
		AmountCents int64
	}
	var eligibleTransfers []eligible
	for rows.Next() {
		var e eligible
		if err := rows.Scan(&e.ID, &e.AmountCents); err != nil {
			return nil, fmt.Errorf("scan eligible transfer: %w", err)
		}
		eligibleTransfers = append(eligibleTransfers, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate eligible transfers: %w", err)
	}

	if len(eligibleTransfers) == 0 {
		return nil, fmt.Errorf("no eligible transfers")
	}

	now := time.Now().UTC()
	batchID := uuid.New().String()
	var totalAmountCents int64
	for _, e := range eligibleTransfers {
		totalAmountCents += e.AmountCents
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO settlement_batches (id, business_date_ct, file_format, status, total_items, total_amount_cents, created_at)
		VALUES (?, ?, 'X9_JSON_EQUIVALENT', 'GENERATED', ?, ?, ?)`,
		batchID, businessDateCT, len(eligibleTransfers), totalAmountCents, now)
	if err != nil {
		return nil, fmt.Errorf("insert batch: %w", err)
	}

	var fileItems []fileItem
	for i, e := range eligibleTransfers {
		itemID := uuid.New().String()
		seq := i + 1

		// Snapshot MICR data from vendor_results
		var routingNumber, accountNumber, checkNumber sql.NullString
		err := tx.QueryRow(`
			SELECT micr_routing_number, micr_account_number, micr_check_number
			FROM vendor_results WHERE transfer_id = ?`, e.ID).
			Scan(&routingNumber, &accountNumber, &checkNumber)
		if err != nil && err != sql.ErrNoRows {
			return nil, fmt.Errorf("query vendor results for %s: %w", e.ID, err)
		}

		micr := micrData{
			RoutingNumber: routingNumber.String,
			AccountNumber: accountNumber.String,
			CheckNumber:   checkNumber.String,
		}
		micrJSON, _ := json.Marshal(micr)
		micrJSONStr := string(micrJSON)

		// Snapshot image paths from transfer_images
		var frontPath, backPath sql.NullString
		_ = tx.QueryRow(`SELECT file_path FROM transfer_images WHERE transfer_id = ? AND side = 'FRONT'`, e.ID).Scan(&frontPath)
		_ = tx.QueryRow(`SELECT file_path FROM transfer_images WHERE transfer_id = ? AND side = 'BACK'`, e.ID).Scan(&backPath)

		var frontPathPtr, backPathPtr *string
		if frontPath.Valid {
			frontPathPtr = &frontPath.String
		}
		if backPath.Valid {
			backPathPtr = &backPath.String
		}

		_, err = tx.Exec(`
			INSERT INTO settlement_batch_items (id, batch_id, transfer_id, sequence_number, amount_cents, micr_snapshot_json, front_image_path, back_image_path, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			itemID, batchID, e.ID, seq, e.AmountCents, micrJSONStr, frontPathPtr, backPathPtr, now)
		if err != nil {
			return nil, fmt.Errorf("insert batch item: %w", err)
		}

		fileItems = append(fileItems, fileItem{
			SequenceNumber: seq,
			TransferID:     e.ID,
			AmountCents:    e.AmountCents,
			MICR:           micr,
			Images:         imageData{FrontPath: frontPathPtr, BackPath: backPathPtr},
		})
	}

	// Generate X9-equivalent JSON file
	sf := settlementFile{
		FileHeader: fileHeader{
			BatchID:          batchID,
			BusinessDateCT:   businessDateCT,
			CreatedAt:        now.Format(time.RFC3339),
			Format:           "X9_JSON_EQUIVALENT",
			TotalItems:       len(eligibleTransfers),
			TotalAmountCents: totalAmountCents,
		},
		Items: fileItems,
	}

	if err := os.MkdirAll(s.OutputPath, 0755); err != nil {
		return nil, fmt.Errorf("create output directory: %w", err)
	}

	filename := fmt.Sprintf("%s_%s.json", businessDateCT, batchID)
	filePath := filepath.Join(s.OutputPath, filename)
	f, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("create settlement file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(sf); err != nil {
		return nil, fmt.Errorf("encode settlement file: %w", err)
	}

	// Update batch with file path
	_, err = tx.Exec(`UPDATE settlement_batches SET file_path = ? WHERE id = ?`, filePath, batchID)
	if err != nil {
		return nil, fmt.Errorf("update batch file_path: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit batch: %w", err)
	}

	slog.Info("settlement batch generated", "batchID", batchID, "items", len(eligibleTransfers), "totalCents", totalAmountCents)

	batch := &Batch{
		ID:               batchID,
		BusinessDateCT:   businessDateCT,
		FileFormat:       "X9_JSON_EQUIVALENT",
		FilePath:         &filePath,
		Status:           "GENERATED",
		TotalItems:       len(eligibleTransfers),
		TotalAmountCents: totalAmountCents,
		CreatedAt:        now,
	}
	return batch, nil
}

func (s *SettlementService) AcknowledgeBatch(ctx interface{}, batchID, ackReference string) error {
	now := time.Now().UTC()

	// Update batch status
	res, err := s.DB.Exec(`
		UPDATE settlement_batches
		SET status = 'ACKNOWLEDGED', ack_reference = ?, acknowledged_at = ?
		WHERE id = ? AND status = 'GENERATED'`,
		ackReference, now, batchID)
	if err != nil {
		return fmt.Errorf("update batch status: %w", err)
	}
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("batch %s not found or not in GENERATED status", batchID)
	}

	// Get all batch items and transition their transfers to Completed
	rows, err := s.DB.Query(`SELECT transfer_id FROM settlement_batch_items WHERE batch_id = ?`, batchID)
	if err != nil {
		return fmt.Errorf("query batch items: %w", err)
	}
	defer rows.Close()

	var transferIDs []string
	for rows.Next() {
		var tid string
		if err := rows.Scan(&tid); err != nil {
			return fmt.Errorf("scan batch item: %w", err)
		}
		transferIDs = append(transferIDs, tid)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate batch items: %w", err)
	}

	for _, tid := range transferIDs {
		if err := s.TransferSvc.Transition(s.DB, tid, transfers.StateCompleted, "SYSTEM", "settlement"); err != nil {
			return fmt.Errorf("transition transfer %s to Completed: %w", tid, err)
		}
		_, err := s.DB.Exec(`UPDATE transfers SET completed_at = ? WHERE id = ?`, now, tid)
		if err != nil {
			return fmt.Errorf("update completed_at for %s: %w", tid, err)
		}
	}

	slog.Info("settlement batch acknowledged", "batchID", batchID, "ackReference", ackReference, "transfers", len(transferIDs))
	return nil
}

func (s *SettlementService) ListBatches(ctx interface{}) ([]Batch, error) {
	rows, err := s.DB.Query(`
		SELECT id, business_date_ct, cutoff_at_ct, file_format, file_path,
		       status, total_items, total_amount_cents, ack_reference,
		       created_at, submitted_at, acknowledged_at
		FROM settlement_batches
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("query batches: %w", err)
	}
	defer rows.Close()

	var batches []Batch
	for rows.Next() {
		var b Batch
		if err := rows.Scan(
			&b.ID, &b.BusinessDateCT, &b.CutoffAtCT, &b.FileFormat, &b.FilePath,
			&b.Status, &b.TotalItems, &b.TotalAmountCents, &b.AckReference,
			&b.CreatedAt, &b.SubmittedAt, &b.AcknowledgedAt,
		); err != nil {
			return nil, fmt.Errorf("scan batch: %w", err)
		}
		batches = append(batches, b)
	}
	return batches, rows.Err()
}

func (s *SettlementService) GetBatch(ctx interface{}, batchID string) (*Batch, []BatchItem, error) {
	var b Batch
	err := s.DB.QueryRow(`
		SELECT id, business_date_ct, cutoff_at_ct, file_format, file_path,
		       status, total_items, total_amount_cents, ack_reference,
		       created_at, submitted_at, acknowledged_at
		FROM settlement_batches WHERE id = ?`, batchID).
		Scan(
			&b.ID, &b.BusinessDateCT, &b.CutoffAtCT, &b.FileFormat, &b.FilePath,
			&b.Status, &b.TotalItems, &b.TotalAmountCents, &b.AckReference,
			&b.CreatedAt, &b.SubmittedAt, &b.AcknowledgedAt,
		)
	if err != nil {
		return nil, nil, fmt.Errorf("query batch: %w", err)
	}

	rows, err := s.DB.Query(`
		SELECT id, batch_id, transfer_id, sequence_number, amount_cents,
		       micr_snapshot_json, front_image_path, back_image_path, created_at
		FROM settlement_batch_items
		WHERE batch_id = ?
		ORDER BY sequence_number ASC`, batchID)
	if err != nil {
		return nil, nil, fmt.Errorf("query batch items: %w", err)
	}
	defer rows.Close()

	var items []BatchItem
	for rows.Next() {
		var item BatchItem
		if err := rows.Scan(
			&item.ID, &item.BatchID, &item.TransferID, &item.SequenceNumber, &item.AmountCents,
			&item.MICRSnapshotJSON, &item.FrontImagePath, &item.BackImagePath, &item.CreatedAt,
		); err != nil {
			return nil, nil, fmt.Errorf("scan batch item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate batch items: %w", err)
	}

	return &b, items, nil
}
