package main

import (
	"testing"
)

func intPtr(v int) *int { return &v }

func TestMapEvidenceToResponse(t *testing.T) {
	tests := []struct {
		name               string
		ev                 VisionEvidence
		requestAmountCents int
		wantDecision       string
		wantIQAStatus      string
		wantMICRConf       float64
		wantMICRRouting    string
		wantMICRAccount    string
		wantMICRSerial     string
		wantAmountMatches  bool
		wantManualReview   bool
		wantRiskScore      int
		wantOCRAmount      *int
	}{
		{
			name: "clean_pass",
			ev: func() VisionEvidence {
				var ev VisionEvidence
				ev.ImageQuality.FrontReadable = true
				ev.ImageQuality.BackReadable = true
				ev.ImageQuality.Issues = []string{}
				ev.MICR.Readable = true
				ev.MICR.Routing = "021000021"
				ev.MICR.Account = "123456789"
				ev.MICR.Serial = "1001"
				ev.Amount.CourtesyAmountCents = intPtr(50000)
				return ev
			}(),
			requestAmountCents: 50000,
			wantDecision:       "PASS",
			wantIQAStatus:      "PASS",
			wantMICRConf:       0.95,
			wantMICRRouting:    "021000021",
			wantMICRAccount:    "123456789",
			wantMICRSerial:     "1001",
			wantAmountMatches:  true,
			wantManualReview:   false,
			wantRiskScore:      10,
			wantOCRAmount:      intPtr(50000),
		},
		{
			name: "iqa_blur",
			ev: func() VisionEvidence {
				var ev VisionEvidence
				ev.ImageQuality.FrontReadable = false
				ev.ImageQuality.Issues = []string{"blur"}
				return ev
			}(),
			requestAmountCents: 50000,
			wantDecision:       "FAIL",
			wantIQAStatus:      "FAIL_BLUR",
			wantMICRConf:       0.0,
			wantAmountMatches:  false,
			wantManualReview:   false,
			wantRiskScore:      40,
		},
		{
			name: "iqa_glare",
			ev: func() VisionEvidence {
				var ev VisionEvidence
				ev.ImageQuality.FrontReadable = false
				ev.ImageQuality.Issues = []string{"glare"}
				return ev
			}(),
			requestAmountCents: 50000,
			wantDecision:       "FAIL",
			wantIQAStatus:      "FAIL_GLARE",
			wantMICRConf:       0.0,
			wantAmountMatches:  false,
			wantManualReview:   false,
			wantRiskScore:      40,
		},
		{
			name: "micr_failure",
			ev: func() VisionEvidence {
				var ev VisionEvidence
				ev.ImageQuality.FrontReadable = true
				ev.ImageQuality.BackReadable = true
				ev.ImageQuality.Issues = []string{}
				ev.MICR.Readable = false
				ev.Amount.CourtesyAmountCents = intPtr(50000)
				return ev
			}(),
			requestAmountCents: 50000,
			wantDecision:       "PASS",
			wantIQAStatus:      "PASS",
			wantMICRConf:       0.0,
			wantAmountMatches:  true,
			wantManualReview:   false,
			wantRiskScore:      40,
		},
		{
			name: "amount_mismatch",
			ev: func() VisionEvidence {
				var ev VisionEvidence
				ev.ImageQuality.FrontReadable = true
				ev.ImageQuality.BackReadable = true
				ev.ImageQuality.Issues = []string{}
				ev.MICR.Readable = true
				ev.MICR.Routing = "021000021"
				ev.MICR.Account = "123456789"
				ev.MICR.Serial = "1001"
				ev.Amount.CourtesyAmountCents = intPtr(75000)
				return ev
			}(),
			requestAmountCents: 50000,
			wantDecision:       "PASS",
			wantIQAStatus:      "PASS",
			wantMICRConf:       0.95,
			wantMICRRouting:    "021000021",
			wantMICRAccount:    "123456789",
			wantMICRSerial:     "1001",
			wantAmountMatches:  false,
			wantManualReview:   false,
			wantRiskScore:      30,
			wantOCRAmount:      intPtr(75000),
		},
		{
			name: "micr_failure_plus_amount_mismatch",
			ev: func() VisionEvidence {
				var ev VisionEvidence
				ev.ImageQuality.FrontReadable = true
				ev.ImageQuality.BackReadable = true
				ev.ImageQuality.Issues = []string{}
				ev.MICR.Readable = false
				ev.Amount.CourtesyAmountCents = intPtr(75000)
				return ev
			}(),
			requestAmountCents: 50000,
			wantDecision:       "REVIEW",
			wantIQAStatus:      "PASS",
			wantMICRConf:       0.0,
			wantAmountMatches:  false,
			wantManualReview:   true,
			wantRiskScore:      60,
			wantOCRAmount:      intPtr(75000),
		},
		{
			name: "iqa_issues_without_failure",
			ev: func() VisionEvidence {
				var ev VisionEvidence
				ev.ImageQuality.FrontReadable = true
				ev.ImageQuality.BackReadable = true
				ev.ImageQuality.Issues = []string{"skewed"}
				ev.MICR.Readable = true
				ev.MICR.Routing = "021000021"
				ev.MICR.Account = "123456789"
				ev.MICR.Serial = "1001"
				ev.Amount.CourtesyAmountCents = intPtr(50000)
				return ev
			}(),
			requestAmountCents: 50000,
			wantDecision:       "PASS",
			wantIQAStatus:      "PASS",
			wantMICRConf:       0.95,
			wantMICRRouting:    "021000021",
			wantMICRAccount:    "123456789",
			wantMICRSerial:     "1001",
			wantAmountMatches:  true,
			wantManualReview:   false,
			wantRiskScore:      25,
			wantOCRAmount:      intPtr(50000),
		},
		{
			name: "front_not_readable",
			ev: func() VisionEvidence {
				var ev VisionEvidence
				ev.ImageQuality.FrontReadable = false
				ev.ImageQuality.BackReadable = true
				ev.ImageQuality.Issues = []string{}
				ev.MICR.Readable = true
				ev.MICR.Routing = "021000021"
				ev.MICR.Account = "123456789"
				ev.MICR.Serial = "1001"
				ev.Amount.CourtesyAmountCents = intPtr(50000)
				return ev
			}(),
			requestAmountCents: 50000,
			wantDecision:       "FAIL",
			wantIQAStatus:      "FAIL",
			wantMICRConf:       0.95,
			wantMICRRouting:    "021000021",
			wantMICRAccount:    "123456789",
			wantMICRSerial:     "1001",
			wantAmountMatches:  true,
			wantManualReview:   false,
			wantRiskScore:      10,
			wantOCRAmount:      intPtr(50000),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapEvidenceToResponse(&tt.ev, tt.requestAmountCents)

			if got.Decision != tt.wantDecision {
				t.Errorf("Decision = %q, want %q", got.Decision, tt.wantDecision)
			}
			if got.IQAStatus != tt.wantIQAStatus {
				t.Errorf("IQAStatus = %q, want %q", got.IQAStatus, tt.wantIQAStatus)
			}
			if got.MICR == nil {
				t.Fatal("MICR is nil")
			}
			if got.MICR.Confidence != tt.wantMICRConf {
				t.Errorf("MICR.Confidence = %f, want %f", got.MICR.Confidence, tt.wantMICRConf)
			}
			if got.MICR.Routing != tt.wantMICRRouting {
				t.Errorf("MICR.Routing = %q, want %q", got.MICR.Routing, tt.wantMICRRouting)
			}
			if got.MICR.Account != tt.wantMICRAccount {
				t.Errorf("MICR.Account = %q, want %q", got.MICR.Account, tt.wantMICRAccount)
			}
			if got.MICR.Serial != tt.wantMICRSerial {
				t.Errorf("MICR.Serial = %q, want %q", got.MICR.Serial, tt.wantMICRSerial)
			}
			if got.AmountMatches != tt.wantAmountMatches {
				t.Errorf("AmountMatches = %v, want %v", got.AmountMatches, tt.wantAmountMatches)
			}
			if got.ManualReviewRequired != tt.wantManualReview {
				t.Errorf("ManualReviewRequired = %v, want %v", got.ManualReviewRequired, tt.wantManualReview)
			}
			if got.RiskScore != tt.wantRiskScore {
				t.Errorf("RiskScore = %d, want %d", got.RiskScore, tt.wantRiskScore)
			}
			if tt.wantOCRAmount != nil {
				if got.OCRAmountCents == nil {
					t.Errorf("OCRAmountCents = nil, want %d", *tt.wantOCRAmount)
				} else if *got.OCRAmountCents != *tt.wantOCRAmount {
					t.Errorf("OCRAmountCents = %d, want %d", *got.OCRAmountCents, *tt.wantOCRAmount)
				}
			}
			if got.DuplicateDetected {
				t.Error("DuplicateDetected = true, want false")
			}
			if got.VendorTransactionID == "" {
				t.Error("VendorTransactionID is empty")
			}

		})
	}
}
