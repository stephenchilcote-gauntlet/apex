package model

// AnalyzeRequest is the JSON body sent to the vendor check-analysis endpoint.
type AnalyzeRequest struct {
	TransferID         string `json:"transferId"`
	InvestorAccountID  string `json:"investorAccountId"`
	AmountCents        int    `json:"amountCents"`
	FrontImageSha256   string `json:"frontImageSha256"`
	BackImageSha256    string `json:"backImageSha256"`
	FrontImageBase64   string `json:"frontImageBase64,omitempty"`
	BackImageBase64    string `json:"backImageBase64,omitempty"`
}

// MICRResult holds the MICR data extracted from a check image.
type MICRResult struct {
	Routing    string  `json:"routing"`
	Account    string  `json:"account"`
	Serial     string  `json:"serial"`
	Confidence float64 `json:"confidence"`
}

// AnalyzeResponse is returned by the vendor check-analysis endpoint.
type AnalyzeResponse struct {
	VendorTransactionID string      `json:"vendorTransactionId"`
	Decision            string      `json:"decision"`
	IQAStatus           string      `json:"iqaStatus"`
	MICR                *MICRResult `json:"micr"`
	OCRAmountCents      *int        `json:"ocrAmountCents"`
	AmountMatches       bool        `json:"amountMatches"`
	DuplicateDetected   bool        `json:"duplicateDetected"`
	RiskScore           int         `json:"riskScore"`
	ManualReviewRequired bool       `json:"manualReviewRequired"`
}

