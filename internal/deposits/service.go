package deposits

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/apex-checkout/mobile-check-deposit/internal/funding"
	"github.com/apex-checkout/mobile-check-deposit/internal/ledger"
	"github.com/apex-checkout/mobile-check-deposit/internal/transfers"
	vendorclient "github.com/apex-checkout/mobile-check-deposit/internal/vendorsvc/client"
	"github.com/apex-checkout/mobile-check-deposit/internal/vendorsvc/model"
	"github.com/google/uuid"
)

type SubmitResult struct {
	TransferID       string
	State            transfers.State
	ReviewRequired   bool
	BusinessDateCT   string
	Message          string
	RejectionCode    string
	RejectionMessage string
}

type DepositService struct {
	DB          *sql.DB
	TransferSvc *transfers.TransferService
	FundingSvc  *funding.FundingService
	LedgerSvc   *ledger.LedgerService
	VendorClient *vendorclient.VendorClient
	ImageDir    string
}

func (s *DepositService) SubmitDeposit(ctx context.Context, investorAccountID string, amountCents int64, frontImage, backImage io.Reader) (*SubmitResult, error) {
	accountID, correspondentID, omnibusAccountID, err := funding.ResolveAccounts(s.DB, investorAccountID)
	if err != nil {
		return nil, fmt.Errorf("resolve accounts: %w", err)
	}

	now := time.Now().UTC()
	loc, err := time.LoadLocation("America/Chicago")
	if err != nil {
		return nil, fmt.Errorf("load timezone: %w", err)
	}
	businessDate := now.In(loc).Format("2006-01-02")

	t := &transfers.Transfer{
		InvestorAccountID: accountID,
		CorrespondentID:   correspondentID,
		OmnibusAccountID:  omnibusAccountID,
		AmountCents:       amountCents,
		Currency:          "USD",
		BusinessDateCT:    &businessDate,
		SubmittedAt:       &now,
	}
	if err := s.TransferSvc.Create(s.DB, t); err != nil {
		return nil, fmt.Errorf("create transfer: %w", err)
	}

	frontHash, err := s.saveImage(t.ID, "FRONT", frontImage)
	if err != nil {
		return nil, fmt.Errorf("save front image: %w", err)
	}
	backHash, err := s.saveImage(t.ID, "BACK", backImage)
	if err != nil {
		return nil, fmt.Errorf("save back image: %w", err)
	}

	if err := s.TransferSvc.Transition(s.DB, t.ID, transfers.StateValidating, "SYSTEM", "deposit-service"); err != nil {
		return nil, fmt.Errorf("transition to Validating: %w", err)
	}

	// Read saved images for vision analysis
	var frontB64, backB64 string
	frontPath := filepath.Join(s.ImageDir, t.ID, "FRONT.jpg")
	backPath := filepath.Join(s.ImageDir, t.ID, "BACK.jpg")
	if frontData, readErr := os.ReadFile(frontPath); readErr != nil {
		slog.Warn("could not read front image for vision", "error", readErr)
	} else {
		frontB64 = base64.StdEncoding.EncodeToString(frontData)
	}
	if backData, readErr := os.ReadFile(backPath); readErr != nil {
		slog.Warn("could not read back image for vision", "error", readErr)
	} else {
		backB64 = base64.StdEncoding.EncodeToString(backData)
	}

	analyzeReq := model.AnalyzeRequest{
		TransferID:        t.ID,
		InvestorAccountID: accountID,
		AmountCents:       int(amountCents),
		FrontImageSha256:  frontHash,
		BackImageSha256:   backHash,
		FrontImageBase64:  frontB64,
		BackImageBase64:   backB64,
	}
	vendorResp, err := s.VendorClient.Analyze(ctx, analyzeReq)
	if err != nil {
		return nil, fmt.Errorf("vendor analyze: %w", err)
	}

	if err := vendorclient.SaveVendorResult(s.DB, t.ID, vendorResp); err != nil {
		return nil, fmt.Errorf("save vendor result: %w", err)
	}

	if err := s.TransferSvc.Transition(s.DB, t.ID, transfers.StateAnalyzing, "SYSTEM", "deposit-service"); err != nil {
		return nil, fmt.Errorf("transition to Analyzing: %w", err)
	}

	if vendorResp.Decision == "FAIL" {
		rejCode := "VENDOR_REJECT"
		rejMsg := fmt.Sprintf("Vendor decision FAIL: IQA=%s duplicate=%v", vendorResp.IQAStatus, vendorResp.DuplicateDetected)
		s.DB.Exec("UPDATE transfers SET rejection_code = ?, rejection_message = ?, updated_at = ? WHERE id = ?",
			rejCode, rejMsg, time.Now().UTC(), t.ID)

		if err := s.TransferSvc.Transition(s.DB, t.ID, transfers.StateRejected, "SYSTEM", "deposit-service"); err != nil {
			return nil, fmt.Errorf("transition to Rejected: %w", err)
		}

		return &SubmitResult{
			TransferID:       t.ID,
			State:            transfers.StateRejected,
			BusinessDateCT:   businessDate,
			Message:          "Deposit rejected by vendor analysis",
			RejectionCode:    rejCode,
			RejectionMessage: rejMsg,
		}, nil
	}

	rulesPassed, _, err := s.FundingSvc.ApplyRules(ctx, t, vendorResp)
	if err != nil {
		return nil, fmt.Errorf("apply rules: %w", err)
	}

	if !rulesPassed {
		rejCode := "RULES_REJECT"
		rejMsg := "One or more funding rules failed"
		s.DB.Exec("UPDATE transfers SET rejection_code = ?, rejection_message = ?, updated_at = ? WHERE id = ?",
			rejCode, rejMsg, time.Now().UTC(), t.ID)

		if err := s.TransferSvc.Transition(s.DB, t.ID, transfers.StateRejected, "SYSTEM", "deposit-service"); err != nil {
			return nil, fmt.Errorf("transition to Rejected: %w", err)
		}

		return &SubmitResult{
			TransferID:       t.ID,
			State:            transfers.StateRejected,
			BusinessDateCT:   businessDate,
			Message:          "Deposit rejected by funding rules",
			RejectionCode:    rejCode,
			RejectionMessage: rejMsg,
		}, nil
	}

	if vendorResp.ManualReviewRequired {
		reviewStatus := "PENDING"
		s.DB.Exec("UPDATE transfers SET review_required = 1, review_status = ?, updated_at = ? WHERE id = ?",
			reviewStatus, time.Now().UTC(), t.ID)

		return &SubmitResult{
			TransferID:     t.ID,
			State:          transfers.StateAnalyzing,
			ReviewRequired: true,
			BusinessDateCT: businessDate,
			Message:        "Deposit requires manual review",
		}, nil
	}

	if err := s.TransferSvc.Transition(s.DB, t.ID, transfers.StateApproved, "SYSTEM", "deposit-service"); err != nil {
		return nil, fmt.Errorf("transition to Approved: %w", err)
	}
	s.DB.Exec("UPDATE transfers SET approved_at = ? WHERE id = ?", time.Now().UTC(), t.ID)

	if err := s.LedgerSvc.PostDeposit(s.DB, t.ID, accountID, omnibusAccountID, amountCents); err != nil {
		return nil, fmt.Errorf("post deposit: %w", err)
	}

	if err := s.TransferSvc.Transition(s.DB, t.ID, transfers.StateFundsPosted, "SYSTEM", "deposit-service"); err != nil {
		return nil, fmt.Errorf("transition to FundsPosted: %w", err)
	}
	s.DB.Exec("UPDATE transfers SET posted_at = ? WHERE id = ?", time.Now().UTC(), t.ID)

	return &SubmitResult{
		TransferID:     t.ID,
		State:          transfers.StateFundsPosted,
		BusinessDateCT: businessDate,
		Message:        "Deposit accepted and funds posted",
	}, nil
}

func (s *DepositService) saveImage(transferID, side string, r io.Reader) (string, error) {
	dir := filepath.Join(s.ImageDir, transferID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create image dir: %w", err)
	}

	filePath := filepath.Join(dir, side+".jpg")
	f, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	w := io.MultiWriter(f, h)
	if _, err := io.Copy(w, r); err != nil {
		return "", fmt.Errorf("write image: %w", err)
	}

	hash := fmt.Sprintf("%x", h.Sum(nil))

	_, err = s.DB.Exec(`
		INSERT INTO transfer_images (id, transfer_id, side, file_path, sha256, mime_type, created_at)
		VALUES (?, ?, ?, ?, ?, 'image/jpeg', ?)`,
		uuid.New().String(), transferID, side, filePath, hash, time.Now().UTC())
	if err != nil {
		return "", fmt.Errorf("insert image record: %w", err)
	}

	slog.Info("image saved", "transferID", transferID, "side", side, "path", filePath)
	return hash, nil
}
