# Architecture

## System Diagram

```
                         ┌──────────────────────────────────┐
                         │         Web Browser               │
                         │                                    │
                         │  /ui/simulate  /ui/review          │
                         │  /ui/transfers /ui/ledger          │
                         │  /ui/settlement /ui/returns        │
                         └──────────────┬───────────────────┘
                                        │ HTTP
                                        ▼
┌──────────────────────────────────────────────────────────────┐
│                    App Server (port 8080)                     │
│                                                              │
│  ┌─────────────┐  ┌──────────────┐  ┌──────────────────────┐│
│  │  API Layer   │  │   UI Layer   │  │   Static Files       ││
│  │  /api/v1/*   │  │   /ui/*      │  │   /static/*          ││
│  └──────┬──────┘  └──────┬───────┘  └──────────────────────┘│
│         │                │                                   │
│         └────────┬───────┘                                   │
│                  ▼                                           │
│  ┌────────────────────────────────────────────────────────┐  │
│  │                   Service Layer                        │  │
│  │                                                        │  │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐  │  │
│  │  │ Deposit  │ │ Funding  │ │ Transfer │ │  Ledger  │  │  │
│  │  │ Service  │ │ Service  │ │ Service  │ │ Service  │  │  │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘  │  │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐  │  │
│  │  │Settlement│ │ Returns  │ │  Audit   │ │  Clock   │  │  │
│  │  │ Service  │ │ Service  │ │          │ │          │  │  │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────┘  │  │
│  └────────────────────────────────────────────────────────┘  │
│                  │                    │                       │
│                  ▼                    ▼                       │
│         ┌──────────────┐     ┌──────────────┐                │
│         │   SQLite DB  │     │  File System  │               │
│         │  (mcd.db)    │     │  (images,     │               │
│         │              │     │   settlement) │               │
│         └──────────────┘     └──────────────┘                │
└──────────────────────────────┬───────────────────────────────┘
                               │ HTTP (vendor client)
                               ▼
                   ┌──────────────────────┐
                   │  Vendor Stub Service │
                   │     (port 8081)      │
                   │                      │
                   │  POST /stub/v1/      │
                   │    checks/analyze    │
                   │  GET  /stub/v1/      │
                   │    scenarios         │
                   └──────────────────────┘
```

## Service Boundaries

### App Server (`cmd/app`)

The main application server, responsible for:

| Service | Package | Responsibility |
|---------|---------|----------------|
| Deposit Service | `internal/deposits` | Orchestrates the deposit submission flow: image storage, vendor call, rule evaluation, state transitions, ledger posting |
| Funding Service | `internal/funding` | Business rule engine: account eligibility, deposit limits, contribution type defaulting, internal duplicate detection |
| Transfer Service | `internal/transfers` | Transfer CRUD, state machine enforcement, audit logging on every transition |
| Ledger Service | `internal/ledger` | Double-entry bookkeeping: deposit postings, reversal postings, fee postings |
| Settlement Service | `internal/settlement` | Batch generation (X9.37 ICL binary via moov-io/imagecashletter), batch acknowledgment |
| Returns Service | `internal/returns` | Return notification processing, reversal journal creation, $30 fee posting |
| Audit | `internal/audit` | Central audit event logging (entity, actor, event type, details) |
| Clock | `internal/clock` | Business date calculation with 6:30 PM CT cutoff and weekend rollforward |
| Config | `internal/config` | Environment variable loading with defaults |
| Repository | `internal/repository` | Database initialization and migration runner |
| API Handlers | `internal/http/api` | REST API endpoint handlers |
| UI Handlers | `internal/http/ui` | Web UI page handlers (Go templates + HTMX) |
| Vendor Client | `internal/vendorsvc/client` | HTTP client for vendor stub + vendor result persistence |
| Vendor Model | `internal/vendorsvc/model` | Shared request/response types for vendor API |

### Vendor Stub (`cmd/vendorstub`)

Standalone HTTP service that simulates an external check validation vendor. Returns deterministic responses based on:
1. `X-Vendor-Scenario` request header (highest priority)
2. Account ID suffix → scenario mapping from `config/vendor_scenarios.yaml`
3. Default scenario from config

## Data Flow

### Happy Path (Clean Pass)

```
1. Investor submits deposit
   └─→ POST /api/v1/deposits (frontImage, backImage, amount, investorAccountId)

2. Account resolution
   └─→ Look up investor account → correspondent → omnibus account

3. Transfer created (state: Requested)
   └─→ INSERT into transfers table

4. Images saved to disk
   └─→ data/images/{transferId}/FRONT.jpg, BACK.jpg
   └─→ SHA256 hash computed for each

5. Transition → Validating
   └─→ Audit event logged

6. Vendor analysis
   └─→ POST to vendor stub /stub/v1/checks/analyze
   └─→ Response saved to vendor_results table

7. Transition → Analyzing
   └─→ Audit event logged

8. Funding rules evaluated
   └─→ ACCOUNT_ELIGIBLE: check account status = ACTIVE
   └─→ MAX_DEPOSIT_LIMIT: check amount ≤ $5,000
   └─→ CONTRIBUTION_TYPE_DEFAULT: set from account config
   └─→ INTERNAL_DUPLICATE: check SHA256 fingerprint
   └─→ Each rule result saved to rule_evaluations table

9. Vendor decision = PASS, rules passed, no review required
   └─→ Transition → Approved (audit logged)
   └─→ Ledger posting: credit investor account, debit omnibus account
   └─→ Transition → FundsPosted (audit logged)

10. Settlement batch generation (EOD)
    └─→ POST /api/v1/settlement/batches/generate
    └─→ Batch file written to reports/settlement/
    └─→ Transfers remain in FundsPosted state

11. Settlement acknowledgment
    └─→ POST /api/v1/settlement/batches/{id}/ack
    └─→ Transition → Completed (audit logged)
```

### Rejection Path (Vendor Fail)

```
1–7. Same as happy path through Analyzing

8. Vendor decision = FAIL (e.g., IQA blur, duplicate detected)
   └─→ rejection_code and rejection_message set on transfer
   └─→ Transition → Rejected (audit logged)
   └─→ No ledger posting, no settlement
```

### Manual Review Path

```
1–8. Same as happy path through rule evaluation

9. Vendor decision = REVIEW (e.g., MICR failure, amount mismatch)
   └─→ Transfer stays in Analyzing with review_required=true, review_status=PENDING
   └─→ Appears in operator review queue

10. Operator reviews
    └─→ Views check images, MICR data, risk score, rule evaluations
    └─→ Approves or rejects

11a. Approve
     └─→ operator_actions row created
     └─→ review_status → APPROVED
     └─→ Transition → Approved → FundsPosted (with ledger posting)

11b. Reject
     └─→ operator_actions row created
     └─→ review_status → REJECTED
     └─→ Transition → Rejected
```

### Return/Reversal Path

```
1. Return notification received
   └─→ POST /api/v1/returns (transferId, reasonCode)

2. Validation
   └─→ Transfer must be in FundsPosted or Completed state

3. Return notification recorded
   └─→ INSERT into return_notifications table

4. Reversal journal created
   └─→ Debit investor account (remove the deposit credit)
   └─→ Credit omnibus account (reverse the debit)

5. Fee journal created
   └─→ Debit investor account $30
   └─→ Credit fee revenue account $30

6. Transition → Returned (audit logged)

7. Notification outbox
   └─→ INSERT into notifications_outbox (template: CHECK_RETURNED)
```

## Database Tables (14)

| # | Table | Description |
|---|-------|-------------|
| 1 | `correspondents` | Broker-dealer/correspondent firms; maps to omnibus account |
| 2 | `accounts` | Investor, omnibus, and fee revenue accounts with type, status, currency |
| 3 | `transfers` | Main deposit record: state machine, amounts, dates, review flags, rejection info |
| 4 | `transfer_images` | File references for front/back check images with SHA256 hashes |
| 5 | `vendor_results` | Persisted vendor stub responses: decision, IQA status, MICR data, risk score |
| 6 | `rule_evaluations` | Per-rule pass/fail results with JSON details |
| 7 | `operator_actions` | Approve/reject/override actions with operator ID and notes |
| 8 | `audit_events` | Central decision trace: entity, actor, event type, from/to state, details |
| 9 | `ledger_journals` | Groups of ledger entries by purpose (DEPOSIT_POSTING, RETURN_REVERSAL, RETURN_FEE) |
| 10 | `ledger_entries` | Signed amount per account per journal (double-entry bookkeeping) |
| 11 | `settlement_batches` | Generated batch metadata: business date, file path, status, totals |
| 12 | `settlement_batch_items` | Individual transfers included in each batch with MICR snapshot |
| 13 | `return_notifications` | Simulated return/bounce inputs with reason codes and fees |
| 14 | `notifications_outbox` | Stubbed investor notification queue (email templates, status tracking) |

### Key Indices

- `idx_transfers_state` — Fast state-based filtering
- `idx_transfers_review` — Operator review queue queries (state + review_required + review_status)
- `idx_transfers_business_date` — Settlement batch generation by date
- `idx_transfers_investor` — Per-account deposit lookup
- `idx_transfers_fingerprint` — Duplicate fingerprint checking
- `idx_audit_entity` — Decision trace queries by entity
- `idx_ledger_entries_account` — Account balance computation

## Transfer State Machine

```
                    ┌─────────────┐
                    │  Requested  │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
               ┌────│ Validating  │
               │    └──────┬──────┘
               │           │
               │    ┌──────▼──────┐
               │ ┌──│  Analyzing  │
               │ │  └──────┬──────┘
               │ │         │
               │ │  ┌──────▼──────┐
               │ │  │  Approved   │
               │ │  └──────┬──────┘
               │ │         │
               │ │  ┌──────▼──────┐
               │ │  │ FundsPosted │────────┐
               │ │  └──────┬──────┘        │
               │ │         │               │
               │ │  ┌──────▼──────┐ ┌──────▼──────┐
               │ │  │  Completed  │ │  Returned   │
               │ │  └──────┬──────┘ └─────────────┘
               │ │         │               ▲
               │ │         └───────────────┘
               │ │
               │ │  ┌─────────────┐
               └─┴─▶│  Rejected   │
                    └─────────────┘
```

**Valid transitions:**

| From | To |
|------|----|
| Requested | Validating |
| Validating | Analyzing, Rejected |
| Analyzing | Approved, Rejected |
| Approved | FundsPosted |
| FundsPosted | Completed, Returned |
| Completed | Returned |
| Rejected | _(terminal)_ |
| Returned | _(terminal)_ |

**Terminal states:** Rejected, Returned — no further transitions allowed.

## Ledger Model

Double-entry bookkeeping with journals and entries:

- **DEPOSIT_POSTING journal:** Credit investor account, debit omnibus account
- **RETURN_REVERSAL journal:** Debit investor account, credit omnibus account (reverses the deposit)
- **RETURN_FEE journal:** Debit investor account $30, credit fee revenue account $30

Every journal has two entries that sum to zero, maintaining the accounting equation.

## Seed Data

The system seeds the following on first startup:

- **1 correspondent:** ACME Brokerage (code: ACME)
- **1 omnibus account:** OMNI-ACME
- **1 fee revenue account:** FEE-REVENUE
- **7 investor accounts:** INV-1001 through INV-1007 (one per vendor scenario suffix)
