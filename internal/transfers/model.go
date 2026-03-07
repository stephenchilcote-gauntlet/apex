package transfers

import "time"

type Transfer struct {
	ID                   string
	ClientRequestID      *string
	InvestorAccountID    string
	CorrespondentID      string
	OmnibusAccountID     string
	State                State
	AmountCents          int64
	Currency             string
	ContributionType     *string
	ReviewRequired       bool
	ReviewStatus         *string
	VendorScenario       *string
	BusinessDateCT       *string
	RejectionCode        *string
	RejectionMessage     *string
	ReturnReasonCode     *string
	ReturnFeeCents       int64
	DuplicateFingerprint *string
	SubmittedAt          *time.Time
	ApprovedAt           *time.Time
	PostedAt             *time.Time
	CompletedAt          *time.Time
	ReturnedAt           *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}
