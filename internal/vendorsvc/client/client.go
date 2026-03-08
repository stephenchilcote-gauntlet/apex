package client

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/apex-checkout/mobile-check-deposit/internal/vendorsvc/model"
	"github.com/google/uuid"
)

type VendorClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

func New(baseURL string) *VendorClient {
	return &VendorClient{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Analyze calls POST /stub/v1/checks/analyze on the vendor stub.
func (c *VendorClient) Analyze(ctx context.Context, req model.AnalyzeRequest, vendorScenario string) (*model.AnalyzeResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal analyze request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/stub/v1/checks/analyze", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if vendorScenario != "" {
		httpReq.Header.Set("X-Vendor-Scenario", vendorScenario)
	}

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("vendor request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vendor returned status %d", resp.StatusCode)
	}

	var result model.AnalyzeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode vendor response: %w", err)
	}

	return &result, nil
}

// SaveVendorResult persists a vendor analysis response to the vendor_results table.
func SaveVendorResult(db *sql.DB, transferID string, resp *model.AnalyzeResponse) error {
	rawJSON, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal vendor response: %w", err)
	}

	var micrRouting, micrAccount, micrCheckNumber *string
	var micrConfidence *float64
	if resp.MICR != nil {
		micrRouting = &resp.MICR.Routing
		micrAccount = &resp.MICR.Account
		micrCheckNumber = &resp.MICR.Serial
		micrConfidence = &resp.MICR.Confidence
	}

	var ocrAmount *int
	if resp.OCRAmountCents != nil {
		ocrAmount = resp.OCRAmountCents
	}

	_, err = db.Exec(`
		INSERT INTO vendor_results (
			id, transfer_id, vendor_transaction_id, decision,
			iqa_status, micr_routing_number, micr_account_number,
			micr_check_number, micr_confidence, ocr_amount_cents,
			amount_matches, duplicate_detected, risk_score,
			manual_review_required, raw_response_json, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), transferID, resp.VendorTransactionID, resp.Decision,
		resp.IQAStatus, micrRouting, micrAccount,
		micrCheckNumber, micrConfidence, ocrAmount,
		resp.AmountMatches, resp.DuplicateDetected, resp.RiskScore,
		resp.ManualReviewRequired, string(rawJSON), time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert vendor result: %w", err)
	}
	return nil
}

// GetVendorResult retrieves the vendor analysis response for a given transfer.
func GetVendorResult(db *sql.DB, transferID string) (*model.AnalyzeResponse, error) {
	var rawJSON string
	err := db.QueryRow("SELECT raw_response_json FROM vendor_results WHERE transfer_id = ?", transferID).Scan(&rawJSON)
	if err != nil {
		return nil, fmt.Errorf("get vendor result: %w", err)
	}

	var resp model.AnalyzeResponse
	if err := json.Unmarshal([]byte(rawJSON), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal vendor result: %w", err)
	}
	return &resp, nil
}
