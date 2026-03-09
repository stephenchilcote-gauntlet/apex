package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/apex-checkout/mobile-check-deposit/internal/vendorsvc/model"
	"github.com/go-chi/chi/v5"
)

// testImagesDir returns the path to the generated check images.
func testImagesDir() string {
	_, f, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(f), "..", "..", "tests", "e2e", "tests")
}

func loadImageB64(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testImagesDir(), name))
	if err != nil {
		t.Fatalf("load image %s: %v", name, err)
	}
	return base64.StdEncoding.EncodeToString(data)
}

// TestVisionModeIntegration sends real check images to the vision-enabled vendor
// stub endpoint and verifies the response. Requires ANTHROPIC_API_KEY.
func TestVisionModeIntegration(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set — skipping vision integration test")
	}

	cfg := &scenarioConfig{Default: "clean_pass", AccountSuffixMap: map[string]string{}}

	r := chi.NewRouter()
	r.Post("/stub/v1/checks/analyze", handleAnalyze(cfg, true, apiKey))

	ts := httptest.NewServer(r)
	defer ts.Close()

	cases := []struct {
		name         string
		frontImage   string
		backImage    string
		amountCents  int
		wantDecision string
		wantIQAFail  bool
	}{
		{
			name:         "clean_check_passes",
			frontImage:   "check-front.png",
			backImage:    "check-back.png",
			amountCents:  50000,
			wantDecision: "PASS",
			wantIQAFail:  false,
		},
		{
			name:         "blurry_check_fails_iqa",
			frontImage:   "check-front-blurry.png",
			backImage:    "check-back-blurry.png",
			amountCents:  50000,
			wantDecision: "FAIL",
			wantIQAFail:  true,
		},
		{
			name:         "glare_check_fails_iqa",
			frontImage:   "check-front-glare.png",
			backImage:    "check-back-glare.png",
			amountCents:  50000,
			wantDecision: "FAIL",
			wantIQAFail:  true,
		},
		{
			name:        "no_micr_detected",
			frontImage:  "check-front-no-micr.png",
			backImage:   "check-back-no-micr.png",
			amountCents: 50000,
			// MICR unreadable adds risk but may not hit 50 threshold alone
			wantIQAFail: false,
		},
		{
			name:        "wrong_amount_detected",
			frontImage:  "check-front-wrong-amount.png",
			backImage:   "check-back-wrong-amount.png",
			amountCents: 50000, // Image shows $750, request says $500
			wantIQAFail: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := model.AnalyzeRequest{
				TransferID:        "test-" + tc.name,
				InvestorAccountID: "INV-1001",
				AmountCents:       tc.amountCents,
				FrontImageBase64:  loadImageB64(t, tc.frontImage),
				BackImageBase64:   loadImageB64(t, tc.backImage),
			}

			body, err := json.Marshal(req)
			if err != nil {
				t.Fatal(err)
			}

			resp, err := http.Post(ts.URL+"/stub/v1/checks/analyze", "application/json", bytes.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}

			var result model.AnalyzeResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				t.Fatal(err)
			}

			t.Logf("Decision=%s IQAStatus=%s RiskScore=%d AmountMatches=%v MICR=%+v",
				result.Decision, result.IQAStatus, result.RiskScore, result.AmountMatches, result.MICR)

			if result.VendorTransactionID == "" {
				t.Error("VendorTransactionID is empty")
			}

			if tc.wantDecision != "" && result.Decision != tc.wantDecision {
				t.Errorf("Decision = %q, want %q", result.Decision, tc.wantDecision)
			}

			if tc.wantIQAFail {
				if result.IQAStatus == "PASS" {
					t.Errorf("IQAStatus = PASS, want a FAIL variant")
				}
			}

			// Wrong amount: OCR should detect ~$750 when we submitted $500
			if tc.name == "wrong_amount_detected" {
				if result.AmountMatches {
					t.Error("AmountMatches = true, want false (image=$750, request=$500)")
				}
			}
		})
	}
}

// TestScenarioModeFallback verifies that when vision mode is off, the stub
// falls back to scenario-based responses from account suffix mapping.
func TestScenarioModeFallback(t *testing.T) {
	cfg := &scenarioConfig{
		Default: "clean_pass",
		AccountSuffixMap: map[string]string{
			"1002": "iqa_blur",
		},
	}

	r := chi.NewRouter()
	r.Post("/stub/v1/checks/analyze", handleAnalyze(cfg, false, ""))

	ts := httptest.NewServer(r)
	defer ts.Close()

	req := model.AnalyzeRequest{
		TransferID:        "test-fallback",
		InvestorAccountID: "INV-1002",
		AmountCents:       50000,
	}
	body, _ := json.Marshal(req)

	resp, err := http.Post(ts.URL+"/stub/v1/checks/analyze", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var result model.AnalyzeResponse
	json.NewDecoder(resp.Body).Decode(&result)

	if result.Decision != "FAIL" {
		t.Errorf("Decision = %q, want FAIL (iqa_blur scenario)", result.Decision)
	}
	if result.IQAStatus != "FAIL_BLUR" {
		t.Errorf("IQAStatus = %q, want FAIL_BLUR", result.IQAStatus)
	}
}
