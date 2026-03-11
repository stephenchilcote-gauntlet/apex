package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/apex-checkout/mobile-check-deposit/internal/vendorsvc/model"
	"github.com/google/uuid"
)

// VisionEvidence is the narrow JSON schema we ask the LLM to return.
type VisionEvidence struct {
	ImageQuality struct {
		FrontReadable bool     `json:"frontReadable"`
		BackReadable  bool     `json:"backReadable"`
		Issues        []string `json:"issues"`
	} `json:"imageQuality"`
	MICR struct {
		Routing  string `json:"routing"`
		Account  string `json:"account"`
		Serial   string `json:"serial"`
		Readable bool   `json:"readable"`
	} `json:"micr"`
	Amount struct {
		CourtesyAmountCents *int   `json:"courtesyAmountCents"`
		WrittenAmountText   string `json:"writtenAmountText"`
	} `json:"amount"`
	Endorsement struct {
		BackEndorsed bool `json:"backEndorsed"`
	} `json:"endorsement"`
}

// Anthropic API request/response types.

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type   string       `json:"type"`
	Text   string       `json:"text,omitempty"`
	Source *imageSource `json:"source,omitempty"`
}

type imageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

const visionPrompt = `Analyze these check images. Front image is first, back image is second.
Return ONLY a JSON object with this exact schema: {"imageQuality": {"frontReadable": bool, "backReadable": bool, "issues": [string]}, "micr": {"routing": string, "account": string, "serial": string, "readable": bool}, "amount": {"courtesyAmountCents": int|null, "writtenAmountText": string}, "endorsement": {"backEndorsed": bool}}
For MICR: look ONLY at the very bottom edge of the FRONT image for the MICR encoding line — it uses special MICR font characters (⑈ ⑆ symbols or E13B font). This is a specific machine-readable line, NOT any other account numbers printed elsewhere on the check. If there is no MICR line at the bottom of the front image, set readable=false and leave routing/account/serial as empty strings.
For amount: courtesyAmountCents is the numeric dollar amount in the box, converted to cents (e.g. $500.00 = 50000).
For imageQuality: set frontReadable/backReadable to false if significant portions of the text are obscured by blur, glare, or other defects. For issues, list any of ["blur", "glare", "cropped", "dark", "skewed"] that apply — include "glare" if bright white areas wash out text. Empty array if none.`

// detectMediaType sniffs the media type from base64-encoded image data.
func detectMediaType(b64 string) string {
	// PNG starts with \x89PNG (\x89\x50\x4e\x47), base64: "iVBOR"
	if strings.HasPrefix(b64, "iVBOR") {
		return "image/png"
	}
	// JPEG starts with \xff\xd8, base64: "/9j/"
	if strings.HasPrefix(b64, "/9j/") {
		return "image/jpeg"
	}
	// WebP starts with RIFF....WEBP, base64: "UklGR"
	if strings.HasPrefix(b64, "UklGR") {
		return "image/webp"
	}
	return "image/jpeg"
}

func analyzeWithVision(apiKey string, frontB64, backB64 string) (*VisionEvidence, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("anthropic API key is empty")
	}

	reqBody := anthropicRequest{
		Model:     "claude-haiku-4-5-20251001",
		MaxTokens: 1024,
		Messages: []anthropicMessage{
			{
				Role: "user",
				Content: []contentBlock{
					{
						Type: "image",
						Source: &imageSource{
							Type:      "base64",
							MediaType: detectMediaType(frontB64),
							Data:      frontB64,
						},
					},
					{
						Type: "image",
						Source: &imageSource{
							Type:      "base64",
							MediaType: detectMediaType(backB64),
							Data:      backB64,
						},
					},
					{
						Type: "text",
						Text: visionPrompt,
					},
				},
			},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("content-type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API call failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("parse API response: %w", err)
	}

	var rawText string
	for _, block := range apiResp.Content {
		if block.Type == "text" {
			rawText = block.Text
			break
		}
	}
	if rawText == "" {
		return nil, fmt.Errorf("no text block in API response")
	}

	slog.Debug("raw LLM response", "text", rawText)

	// Strip markdown fences if present
	cleaned := rawText
	if idx := strings.Index(cleaned, "```json"); idx != -1 {
		cleaned = cleaned[idx+7:]
	} else if idx := strings.Index(cleaned, "```"); idx != -1 {
		cleaned = cleaned[idx+3:]
	}
	if idx := strings.LastIndex(cleaned, "```"); idx != -1 {
		cleaned = cleaned[:idx]
	}
	cleaned = strings.TrimSpace(cleaned)

	var evidence VisionEvidence
	if err := json.Unmarshal([]byte(cleaned), &evidence); err != nil {
		return nil, fmt.Errorf("parse evidence JSON: %w (raw: %s)", err, rawText)
	}

	return &evidence, nil
}

// analyzeWithVisionAndMap calls the Anthropic API and deterministically maps
// the extracted evidence to an AnalyzeResponse.
func analyzeWithVisionAndMap(apiKey string, req *model.AnalyzeRequest) (*model.AnalyzeResponse, error) {
	evidence, err := analyzeWithVision(apiKey, req.FrontImageBase64, req.BackImageBase64)
	if err != nil {
		return nil, err
	}

	return mapEvidenceToResponse(evidence, req.AmountCents), nil
}

func mapEvidenceToResponse(ev *VisionEvidence, requestAmountCents int) *model.AnalyzeResponse {
	resp := &model.AnalyzeResponse{
		VendorTransactionID: uuid.New().String(),
	}

	// IQA Status
	iqaFailed := false
	for _, issue := range ev.ImageQuality.Issues {
		switch strings.ToLower(issue) {
		case "blur":
			resp.IQAStatus = "FAIL_BLUR"
			iqaFailed = true
		case "glare":
			resp.IQAStatus = "FAIL_GLARE"
			iqaFailed = true
		}
		if iqaFailed {
			break
		}
	}
	if !iqaFailed {
		if ev.ImageQuality.FrontReadable && ev.ImageQuality.BackReadable {
			resp.IQAStatus = "PASS"
		} else {
			resp.IQAStatus = "FAIL"
			iqaFailed = true
		}
	}

	// MICR
	if ev.MICR.Readable {
		resp.MICR = &model.MICRResult{
			Routing:    ev.MICR.Routing,
			Account:    ev.MICR.Account,
			Serial:     ev.MICR.Serial,
			Confidence: 0.95,
		}
	} else {
		resp.MICR = &model.MICRResult{
			Confidence: 0.0,
		}
	}

	// OCR Amount
	resp.OCRAmountCents = ev.Amount.CourtesyAmountCents

	// Amount match
	if resp.OCRAmountCents != nil {
		resp.AmountMatches = *resp.OCRAmountCents == requestAmountCents
	}

	// Duplicate detection is stateful — always false here
	resp.DuplicateDetected = false

	// Risk score
	riskScore := 10
	if !ev.MICR.Readable {
		riskScore += 30
	}
	if resp.OCRAmountCents != nil && !resp.AmountMatches {
		riskScore += 20
	}
	if len(ev.ImageQuality.Issues) > 0 && !iqaFailed {
		riskScore += 15
	}
	resp.RiskScore = riskScore

	// Manual review
	resp.ManualReviewRequired = riskScore >= 50

	// Amount mismatch is a hard REVIEW trigger regardless of risk score
	amountMismatch := resp.OCRAmountCents != nil && !resp.AmountMatches

	// Decision
	if iqaFailed {
		resp.Decision = "FAIL"
	} else if resp.ManualReviewRequired || amountMismatch {
		resp.Decision = "REVIEW"
	} else {
		resp.Decision = "PASS"
	}

	return resp
}
