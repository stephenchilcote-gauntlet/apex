# Mobile Check Deposit System — Implementation Plan

## Architecture

- **Language:** Go 1.22+
- **Router:** chi
- **DB:** SQLite (modernc.org/sqlite)
- **UI:** html/template + HTMX
- **Vendor Stub:** Separate Go HTTP service
- **Testing:** Playwright (e2e/visual) + Go unit/integration tests
- **Setup:** `make dev` → `docker compose up --build`

## Build Order & Checklist

### Phase 0 — Scaffold & DX
- [ ] Go workspace (`go.mod`, `cmd/app/main.go`, `cmd/vendorstub/main.go`)
- [ ] `Makefile` with targets: `dev`, `test`, `demo`, `reset`
- [ ] `docker-compose.yml` (core app + vendor stub)
- [ ] `Dockerfile.app`, `Dockerfile.vendorstub`
- [ ] `.env.example`
- [ ] SQLite bootstrap + migration runner
- [ ] Seed data (synthetic accounts, correspondents, operators)
- [ ] `README.md` skeleton

### Phase 1 — Playwright E2E Test Shells
Write all Playwright tests FIRST as failing specs, then implement until green.

- [ ] Install Playwright test infrastructure (`tests/e2e/`)
- [ ] **Test: Deposit Submission UI** — navigate to `/ui/simulate`, fill form, submit, see confirmation
- [ ] **Test: Happy Path E2E** — submit deposit → see state transitions → funds posted → settlement → completed
- [ ] **Test: IQA Blur Rejection** — submit with blur scenario → see rejected state + retake message
- [ ] **Test: IQA Glare Rejection** — submit with glare scenario → see rejected state
- [ ] **Test: MICR Failure → Review Queue** — submit → appears in review queue with MICR data
- [ ] **Test: Duplicate Detected** — submit duplicate → rejected
- [ ] **Test: Amount Mismatch → Review Queue** — submit → appears in review queue with amount comparison
- [ ] **Test: Operator Approve** — navigate review queue → approve → state becomes FundsPosted
- [ ] **Test: Operator Reject** — navigate review queue → reject → state becomes Rejected
- [ ] **Test: Ledger View** — after deposit posts, balances reflect correctly
- [ ] **Test: Settlement Generation** — generate batch → file appears → transfers complete on ack
- [ ] **Test: Return/Reversal** — trigger return → reversal posting + $30 fee → state Returned
- [ ] **Test: Over-Limit Rejection** — submit >$5000 → rejected by business rules
- [ ] **Test: Transfer Detail / Decision Trace** — view full audit trail on transfer detail page

### Phase 2 — Schema & State Machine (make tests compilable)
- [ ] Create all 14 DB tables via migrations
- [ ] Transfer aggregate + state transition validator
- [ ] Audit event writer
- [ ] Repository layer (CRUD for all entities)
- [ ] Business date / cutoff calculator (`America/Chicago`, 6:30 PM CT)
- [ ] Clock interface for testability

### Phase 3 — Vendor Stub Service
- [ ] `POST /stub/v1/checks/analyze` endpoint
- [ ] 7 scenarios: `clean_pass`, `iqa_blur`, `iqa_glare`, `micr_failure`, `duplicate_detected`, `amount_mismatch`, `iqa_pass_review`
- [ ] Trigger by: explicit `scenario` field, `X-Vendor-Scenario` header, account suffix, or YAML config
- [ ] `GET /stub/v1/scenarios` — list available scenarios
- [ ] `config/vendor_scenarios.yaml`

### Phase 4 — Core API + Deposit Flow
- [ ] `POST /api/v1/deposits` — multipart form, save images, create transfer
- [ ] Vendor client — call stub, persist `vendor_results`
- [ ] Funding Service — rules engine:
  - [ ] Amount limit ($5,000 max)
  - [ ] Account eligibility
  - [ ] Contribution type defaulting
  - [ ] Internal duplicate detection (fingerprint)
- [ ] Rule evaluation persistence (`rule_evaluations` table)
- [ ] Auto-approve clean pass path: `Analyzing → Approved → FundsPosted`
- [ ] Flag for review path: set `review_required=true`, `review_status=PENDING`
- [ ] `GET /api/v1/deposits/{id}` — full transfer detail
- [ ] `GET /api/v1/deposits` — list with filters
- [ ] `GET /api/v1/deposits/{id}/decision-trace`

### Phase 5 — Operator Review
- [ ] `GET /api/v1/operator/review-queue` — flagged items
- [ ] `POST /api/v1/operator/transfers/{id}/approve`
- [ ] `POST /api/v1/operator/transfers/{id}/reject`
- [ ] Operator action logging + audit trail
- [ ] Contribution type override support

### Phase 6 — Ledger
- [ ] Ledger journal + entry creation on deposit posting
- [ ] Reversal journal creation (for returns)
- [ ] Fee journal creation ($30 return fee)
- [ ] `GET /api/v1/ledger/accounts` — list + balances
- [ ] `GET /api/v1/ledger/accounts/{id}` — detail + journal lines
- [ ] `GET /api/v1/ledger/journals?transferId=...`

### Phase 7 — Settlement
- [ ] Business date assignment at submission (cutoff logic)
- [ ] `POST /api/v1/settlement/batches/generate` — batch eligible FundsPosted transfers
- [ ] Generate structured JSON X9-equivalent file under `/reports/settlement/`
- [ ] `GET /api/v1/settlement/batches` — list
- [ ] `GET /api/v1/settlement/batches/{id}` — detail + items
- [ ] `POST /api/v1/settlement/batches/{id}/ack` — acknowledge → transfers to Completed

### Phase 8 — Returns / Reversals
- [ ] `POST /api/v1/returns` — simulate bounced check
- [ ] Reversal posting (debit investor, credit omnibus)
- [ ] Fee posting ($30 to fee revenue account)
- [ ] Transition to `Returned` state
- [ ] Notification outbox record
- [ ] `POST /api/v1/deposits/{id}/resubmit` — for IQA failures

### Phase 9 — Web UI Pages
- [ ] `/ui/simulate` — deposit submission form (front/back image, amount, account, scenario picker)
- [ ] `/ui/transfers` — transfer list with filters
- [ ] `/ui/transfers/{id}` — transfer detail + decision trace + images
- [ ] `/ui/review` — operator review queue (images, MICR, risk, approve/reject buttons)
- [ ] `/ui/ledger` — account list + balances, drill into journal entries
- [ ] `/ui/settlement` — batch list, generate button, ack button
- [ ] `/ui/returns` — trigger return form

### Phase 10 — GREEN ALL PLAYWRIGHT TESTS
- [ ] All 14 Playwright e2e tests pass
- [ ] Screenshot evidence captured

### Phase 11 — Go Unit/Integration Tests (minimum 12)
- [ ] Happy path end-to-end
- [ ] Vendor scenario: `iqa_blur`
- [ ] Vendor scenario: `iqa_glare`
- [ ] Vendor scenario: `micr_failure`
- [ ] Vendor scenario: `duplicate_detected`
- [ ] Vendor scenario: `amount_mismatch`
- [ ] Vendor scenario: `iqa_pass_review`
- [ ] Amount limit >$5,000 rejected
- [ ] Operator approve from review queue
- [ ] Operator reject from review queue
- [ ] Settlement batch excludes rejected, honors cutoff
- [ ] Return creates reversal + $30 fee
- [ ] Invalid state transition rejected
- [ ] Internal duplicate fingerprint blocks redeposit

### Phase 12 — Docs & Polish
- [ ] `README.md` — full setup, architecture, flows, demo instructions
- [ ] `SUBMISSION.md` — common submission format
- [ ] `docs/decision_log.md` — key decisions + alternatives
- [ ] `docs/architecture.md` — system diagram, service boundaries, data flow
- [ ] `scripts/demo_happy_path.sh`
- [ ] `scripts/demo_all_scenarios.sh`
- [ ] `/reports/test-results/` — test report artifact
- [ ] Risks and limitations note

## Database Tables (14)

1. `correspondents` — client/correspondent → omnibus mapping
2. `accounts` — investor, omnibus, fee revenue accounts
3. `transfers` — main deposit record + state machine
4. `transfer_images` — file references (front/back)
5. `vendor_results` — persisted stub responses
6. `rule_evaluations` — per-rule pass/fail records
7. `operator_actions` — approve/reject/override log
8. `audit_events` — central decision trace
9. `ledger_journals` — groups posting events
10. `ledger_entries` — signed amount per account
11. `settlement_batches` — generated file metadata
12. `settlement_batch_items` — transfers in each batch
13. `return_notifications` — simulated return inputs
14. `notifications_outbox` — stubbed investor notifications

## Vendor Stub Scenarios (7)

| Scenario | Account Suffix | Decision | Result |
|---|---|---|---|
| `clean_pass` | 1001 | PASS | Auto-approve |
| `iqa_blur` | 1002 | FAIL | Reject, prompt retake |
| `iqa_glare` | 1003 | FAIL | Reject, prompt retake |
| `micr_failure` | 1004 | REVIEW | Manual review |
| `duplicate_detected` | 1005 | FAIL | Reject |
| `amount_mismatch` | 1006 | REVIEW | Manual review |
| `iqa_pass_review` | 1007 | REVIEW | Manual review (high risk) |

## Transfer State Machine

```
Requested → Validating → Analyzing → Approved → FundsPosted → Completed
                ↓              ↓                      ↓            ↓
             Rejected       Rejected               Returned     Returned
```

## Key Risks

1. **Timezone/cutoff bugs** — use `America/Chicago`, injectable Clock
2. **State machine drift** — centralized transition function, reject invalid
3. **Duplicate detection** — vendor-level + internal fingerprint hash
4. **File/DB consistency** — write files first, then DB
5. **Overbuilding** — no SPA, no Postgres, no message bus
