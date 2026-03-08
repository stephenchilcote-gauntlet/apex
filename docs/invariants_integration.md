# Integration-Testable Invariants

## Transfer Service (internal/transfers/service.go)
- Create() generates UUID if ID empty, sets state to Requested, sets timestamps
- Transition() validates against CanTransition, updates state in DB, logs audit event in same transaction
- Invalid Transition() returns error and does not modify DB state or create audit events

## Deposit Service (internal/deposits/service.go)
- Happy path (clean_pass): Requested→Validating→Analyzing→Approved→FundsPosted
- Vendor FAIL scenarios (iqa_blur, iqa_glare, micr_failure, duplicate_detected, amount_mismatch): end in Rejected with rejection_code=VENDOR_REJECT
- Review scenario (iqa_pass_review): stays in Analyzing, sets review_required=true, review_status=PENDING
- No ledger entries created on rejection or review hold

## Funding Rules (internal/funding/service.go)
- MAX_DEPOSIT_LIMIT: rejects amounts > 500000 cents ($5,000)
- ACCOUNT_ELIGIBLE: rejects if account status ≠ ACTIVE
- CONTRIBUTION_TYPE_DEFAULT: copies contribution_type_default from account if not set on transfer
- INTERNAL_DUPLICATE: SHA256(routing|account|serial|amount|investorAccountID) fingerprint; second non-rejected transfer with same fingerprint → FAIL
- Rule evaluations persisted to rule_evaluations table

## Ledger (internal/ledger/service.go)
- PostDeposit: creates DEPOSIT_POSTING journal, credits investor +amount, debits omnibus -amount
- PostReversal: creates RETURN_REVERSAL journal reversing original, plus RETURN_FEE journal debiting investor -feeCents and crediting fee revenue +feeCents
- Every journal's entries sum to zero (double-entry invariant)

## Settlement (internal/settlement/service.go)
- GenerateBatch: selects FundsPosted transfers for given business date not already in settlement_batch_items
- Creates JSON file with header (batchId, businessDateCT, format, totalItems, totalAmountCents) and items
- AcknowledgeBatch: marks batch ACKNOWLEDGED, transitions included transfers to Completed

## Returns (internal/returns/service.go)
- ProcessReturn: only from FundsPosted or Completed states
- Creates return_notification with fee_cents=3000
- Posts reversal via ledger (reversal + $30 fee)
- Transitions to Returned
- Updates transfer return_reason_code, return_fee_cents, returned_at
- Creates notifications_outbox record with template RETURNED_CHECK
- Creates RETURN_PROCESSED audit event
- Returns from ineligible states are rejected with no side effects
