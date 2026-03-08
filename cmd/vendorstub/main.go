package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/apex-checkout/mobile-check-deposit/internal/vendorsvc/model"
)

// scenarioConfig maps to config/vendor_scenarios.yaml.
type scenarioConfig struct {
	Default          string            `yaml:"default"`
	AccountSuffixMap map[string]string `yaml:"account_suffix_map"`
}

var scenarioDescriptions = map[string]string{
	"clean_pass":        "All checks pass — MICR valid, IQA good, amounts match, no duplicate.",
	"iqa_blur":          "Image quality fails due to blur.",
	"iqa_glare":         "Image quality fails due to glare.",
	"micr_failure":      "MICR extraction fails (confidence 0); manual review required.",
	"duplicate_detected": "Duplicate check detected; automatic rejection.",
	"amount_mismatch":   "OCR amount does not match submitted amount; manual review required.",
	"iqa_pass_review":   "IQA passes but low MICR confidence (0.45); manual review required.",
}

func main() {
	port := os.Getenv("VENDOR_STUB_PORT")
	if port == "" {
		port = "8081"
	}

	cfgPath := "config/vendor_scenarios.yaml"
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		slog.Error("failed to load scenario config", "path", cfgPath, "error", err)
		os.Exit(1)
	}
	slog.Info("loaded scenario config", "default", cfg.Default, "suffixes", len(cfg.AccountSuffixMap))

	r := chi.NewRouter()
	r.Post("/stub/v1/checks/analyze", handleAnalyze(cfg))
	r.Get("/stub/v1/scenarios", handleScenarios())

	addr := ":" + port
	slog.Info("vendor stub listening", "addr", addr)
	if err := http.ListenAndServe(addr, r); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

func loadConfig(path string) (*scenarioConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	var cfg scenarioConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	return &cfg, nil
}

func resolveScenario(req *model.AnalyzeRequest, header string, cfg *scenarioConfig) string {
	if header != "" {
		return header
	}
	acct := req.InvestorAccountID
	if len(acct) >= 4 {
		suffix := acct[len(acct)-4:]
		if s, ok := cfg.AccountSuffixMap[suffix]; ok {
			return s
		}
	}
	return cfg.Default
}

func handleAnalyze(cfg *scenarioConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req model.AnalyzeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
			return
		}

		scenario := resolveScenario(&req, r.Header.Get("X-Vendor-Scenario"), cfg)
		slog.Info("analyze request", "transferId", req.TransferID, "scenario", scenario, "amountCents", req.AmountCents)

		resp, err := buildResponse(scenario, req.AmountCents)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func buildResponse(scenario string, amountCents int) (*model.AnalyzeResponse, error) {
	resp := &model.AnalyzeResponse{
		VendorTransactionID: uuid.New().String(),
	}

	validMICR := &model.MICRResult{
		Routing:    "021000021",
		Account:    "123456789",
		Serial:     "0001",
		Confidence: 0.99,
	}

	switch scenario {
	case "clean_pass":
		resp.Decision = "PASS"
		resp.IQAStatus = "PASS"
		resp.MICR = validMICR
		resp.OCRAmountCents = &amountCents
		resp.AmountMatches = true
		resp.DuplicateDetected = false
		resp.RiskScore = 12
		resp.ManualReviewRequired = false

	case "iqa_blur":
		resp.Decision = "FAIL"
		resp.IQAStatus = "FAIL_BLUR"
		resp.RiskScore = 0

	case "iqa_glare":
		resp.Decision = "FAIL"
		resp.IQAStatus = "FAIL_GLARE"
		resp.RiskScore = 0

	case "micr_failure":
		resp.Decision = "REVIEW"
		resp.IQAStatus = "PASS"
		resp.MICR = &model.MICRResult{
			Confidence: 0,
		}
		resp.OCRAmountCents = &amountCents
		resp.AmountMatches = true
		resp.RiskScore = 65
		resp.ManualReviewRequired = true

	case "duplicate_detected":
		resp.Decision = "FAIL"
		resp.IQAStatus = "PASS"
		resp.MICR = validMICR
		resp.OCRAmountCents = &amountCents
		resp.AmountMatches = true
		resp.DuplicateDetected = true
		resp.RiskScore = 90

	case "amount_mismatch":
		mismatch := amountCents + 500
		resp.Decision = "REVIEW"
		resp.IQAStatus = "PASS"
		resp.MICR = validMICR
		resp.OCRAmountCents = &mismatch
		resp.AmountMatches = false
		resp.RiskScore = 55
		resp.ManualReviewRequired = true

	case "iqa_pass_review":
		resp.Decision = "REVIEW"
		resp.IQAStatus = "PASS"
		resp.MICR = &model.MICRResult{
			Routing:    "021000021",
			Account:    "123456789",
			Serial:     "0001",
			Confidence: 0.45,
		}
		resp.OCRAmountCents = &amountCents
		resp.AmountMatches = true
		resp.RiskScore = 78
		resp.ManualReviewRequired = true

	default:
		return nil, fmt.Errorf("unknown scenario: %s", scenario)
	}

	return resp, nil
}

type scenarioInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func handleScenarios() http.HandlerFunc {
	scenarios := make([]scenarioInfo, 0, len(scenarioDescriptions))
	for name, desc := range scenarioDescriptions {
		scenarios = append(scenarios, scenarioInfo{Name: name, Description: desc})
	}
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, scenarios)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to write response", "error", err)
	}
}
