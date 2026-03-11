package funding

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/apex-checkout/mobile-check-deposit/internal/transfers"
	"github.com/apex-checkout/mobile-check-deposit/internal/vendorsvc/model"
	"github.com/google/uuid"
)

type RuleResult struct {
	RuleName string
	Outcome  string // PASS, FAIL, WARN
	Details  string
}

type FundingService struct {
	DB *sql.DB
}

// ApplyRules runs all business rules and persists rule_evaluations.
// Returns overall pass/fail and details.
func (s *FundingService) ApplyRules(ctx context.Context, t *transfers.Transfer, vendorResult *model.AnalyzeResponse) (bool, []RuleResult, error) {
	rules := []func(context.Context, *transfers.Transfer, *model.AnalyzeResponse) (RuleResult, error){
		s.ruleAccountEligible,
		s.ruleMaxDepositLimit,
		s.ruleDailyDepositLimit,
		s.ruleContributionTypeDefault,
		s.ruleInternalDuplicate,
	}

	var results []RuleResult
	passed := true

	for _, rule := range rules {
		result, err := rule(ctx, t, vendorResult)
		if err != nil {
			return false, nil, err
		}
		results = append(results, result)

		if err := s.persistRuleEvaluation(t.ID, result); err != nil {
			return false, nil, fmt.Errorf("persist rule evaluation: %w", err)
		}

		if result.Outcome == "FAIL" {
			passed = false
		}
	}

	return passed, results, nil
}

func (s *FundingService) ruleAccountEligible(_ context.Context, t *transfers.Transfer, _ *model.AnalyzeResponse) (RuleResult, error) {
	var status string
	err := s.DB.QueryRow("SELECT status FROM accounts WHERE id = ?", t.InvestorAccountID).Scan(&status)
	if err != nil {
		return RuleResult{}, fmt.Errorf("query account status: %w", err)
	}

	if status != "ACTIVE" {
		return RuleResult{
			RuleName: "ACCOUNT_ELIGIBLE",
			Outcome:  "FAIL",
			Details:  fmt.Sprintf("account status is %s, expected ACTIVE", status),
		}, nil
	}

	return RuleResult{
		RuleName: "ACCOUNT_ELIGIBLE",
		Outcome:  "PASS",
		Details:  "account is ACTIVE",
	}, nil
}

func (s *FundingService) ruleMaxDepositLimit(_ context.Context, t *transfers.Transfer, _ *model.AnalyzeResponse) (RuleResult, error) {
	const maxCents int64 = 500000

	if t.AmountCents > maxCents {
		return RuleResult{
			RuleName: "MAX_DEPOSIT_LIMIT",
			Outcome:  "FAIL",
			Details:  fmt.Sprintf("amount %d cents exceeds maximum %d cents", t.AmountCents, maxCents),
		}, nil
	}

	return RuleResult{
		RuleName: "MAX_DEPOSIT_LIMIT",
		Outcome:  "PASS",
		Details:  fmt.Sprintf("amount %d cents within limit", t.AmountCents),
	}, nil
}

func (s *FundingService) ruleDailyDepositLimit(ctx context.Context, t *transfers.Transfer, _ *model.AnalyzeResponse) (RuleResult, error) {
	const dailyLimitCents int64 = 1_000_000 // $10,000/day

	// Sum all non-rejected deposits for this account on the same business date,
	// excluding the current transfer.
	var totalCents int64
	businessDate := time.Now().UTC().Format("2006-01-02")
	if t.BusinessDateCT != nil && *t.BusinessDateCT != "" {
		businessDate = *t.BusinessDateCT
	}

	err := s.DB.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(amount_cents), 0)
		FROM transfers
		WHERE investor_account_id = ?
		  AND date(business_date_ct) = ?
		  AND id != ?
		  AND state NOT IN ('Rejected', 'Returned')`,
		t.InvestorAccountID, businessDate, t.ID,
	).Scan(&totalCents)
	if err != nil {
		return RuleResult{}, fmt.Errorf("query daily deposit total: %w", err)
	}

	newTotal := totalCents + t.AmountCents
	if newTotal > dailyLimitCents {
		return RuleResult{
			RuleName: "DAILY_DEPOSIT_LIMIT",
			Outcome:  "FAIL",
			Details: fmt.Sprintf("daily total $%.2f would exceed $%.2f limit (existing $%.2f + this $%.2f)",
				float64(newTotal)/100, float64(dailyLimitCents)/100,
				float64(totalCents)/100, float64(t.AmountCents)/100),
		}, nil
	}

	return RuleResult{
		RuleName: "DAILY_DEPOSIT_LIMIT",
		Outcome:  "PASS",
		Details: fmt.Sprintf("daily total $%.2f within $%.2f limit",
			float64(newTotal)/100, float64(dailyLimitCents)/100),
	}, nil
}

func (s *FundingService) ruleContributionTypeDefault(_ context.Context, t *transfers.Transfer, _ *model.AnalyzeResponse) (RuleResult, error) {
	if t.ContributionType != nil && *t.ContributionType != "" {
		return RuleResult{
			RuleName: "CONTRIBUTION_TYPE_DEFAULT",
			Outcome:  "PASS",
			Details:  fmt.Sprintf("contribution_type already set to %s", *t.ContributionType),
		}, nil
	}

	var defaultType *string
	err := s.DB.QueryRow("SELECT contribution_type_default FROM accounts WHERE id = ?", t.InvestorAccountID).Scan(&defaultType)
	if err != nil {
		return RuleResult{}, fmt.Errorf("query contribution_type_default: %w", err)
	}

	if defaultType != nil && *defaultType != "" {
		t.ContributionType = defaultType
		_, err = s.DB.Exec("UPDATE transfers SET contribution_type = ?, updated_at = ? WHERE id = ?",
			*defaultType, time.Now().UTC(), t.ID)
		if err != nil {
			return RuleResult{}, fmt.Errorf("update contribution_type: %w", err)
		}
		return RuleResult{
			RuleName: "CONTRIBUTION_TYPE_DEFAULT",
			Outcome:  "PASS",
			Details:  fmt.Sprintf("set contribution_type to account default %s", *defaultType),
		}, nil
	}

	return RuleResult{
		RuleName: "CONTRIBUTION_TYPE_DEFAULT",
		Outcome:  "PASS",
		Details:  "no default contribution_type configured",
	}, nil
}

func (s *FundingService) ruleInternalDuplicate(_ context.Context, t *transfers.Transfer, vendorResult *model.AnalyzeResponse) (RuleResult, error) {
	if vendorResult.MICR == nil {
		return RuleResult{
			RuleName: "INTERNAL_DUPLICATE",
			Outcome:  "PASS",
			Details:  "no MICR data available, skipping duplicate check",
		}, nil
	}

	raw := fmt.Sprintf("%s|%s|%s|%d|%s",
		vendorResult.MICR.Routing,
		vendorResult.MICR.Account,
		vendorResult.MICR.Serial,
		t.AmountCents,
		t.InvestorAccountID,
	)
	fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))

	// Update the transfer's fingerprint
	_, err := s.DB.Exec("UPDATE transfers SET duplicate_fingerprint = ?, updated_at = ? WHERE id = ?",
		fingerprint, time.Now().UTC(), t.ID)
	if err != nil {
		return RuleResult{}, fmt.Errorf("update fingerprint: %w", err)
	}
	t.DuplicateFingerprint = &fingerprint

	// Check for existing non-rejected transfers with same fingerprint
	var count int
	err = s.DB.QueryRow(`
		SELECT COUNT(*) FROM transfers
		WHERE duplicate_fingerprint = ?
		  AND id != ?
		  AND state != ?`,
		fingerprint, t.ID, string(transfers.StateRejected),
	).Scan(&count)
	if err != nil {
		return RuleResult{}, fmt.Errorf("check duplicate fingerprint: %w", err)
	}

	if count > 0 {
		return RuleResult{
			RuleName: "INTERNAL_DUPLICATE",
			Outcome:  "FAIL",
			Details:  fmt.Sprintf("duplicate fingerprint %s found in %d other transfer(s)", fingerprint, count),
		}, nil
	}

	return RuleResult{
		RuleName: "INTERNAL_DUPLICATE",
		Outcome:  "PASS",
		Details:  "no duplicate found",
	}, nil
}

func (s *FundingService) persistRuleEvaluation(transferID string, r RuleResult) error {
	detailsJSON, err := json.Marshal(map[string]string{"details": r.Details})
	if err != nil {
		return fmt.Errorf("marshal rule details: %w", err)
	}

	_, err = s.DB.Exec(`
		INSERT INTO rule_evaluations (id, transfer_id, rule_name, outcome, details_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), transferID, r.RuleName, r.Outcome, string(detailsJSON), time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("insert rule evaluation: %w", err)
	}
	return nil
}

// ResolveAccounts looks up the investor account by external_account_id,
// finds their correspondent, and the correspondent's omnibus account.
func ResolveAccounts(db *sql.DB, investorExternalID string) (investorAccountID, correspondentID, omnibusAccountID string, err error) {
	err = db.QueryRow(`
		SELECT a.id, a.correspondent_id, c.omnibus_account_id
		FROM accounts a
		JOIN correspondents c ON c.id = a.correspondent_id
		WHERE a.external_account_id = ?`,
		investorExternalID,
	).Scan(&investorAccountID, &correspondentID, &omnibusAccountID)
	if err != nil {
		return "", "", "", fmt.Errorf("resolve accounts for external_id %s: %w", investorExternalID, err)
	}
	return investorAccountID, correspondentID, omnibusAccountID, nil
}
