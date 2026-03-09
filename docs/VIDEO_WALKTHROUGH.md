# Video Walkthrough Script — Mobile Check Deposit System

**Target length:** 12–15 minutes  
**Audience:** Apex Fintech Services interview panel  
**Format:** Screen recording with narration (show browser + terminal side-by-side where noted)

---

## Pre-Recording Checklist

```bash
# 1. Clean state
make reset

# 2. Start the system
make dev
# Verify: App at http://localhost:8080, Vendor Stub at http://localhost:8081

# 3. Open browser tabs (pre-loaded, ready to switch):
#    Tab 1: http://localhost:8080/ui/simulate
#    Tab 2: http://localhost:8080/ui/transfers
#    Tab 3: http://localhost:8080/ui/review
#    Tab 4: http://localhost:8080/ui/ledger
#    Tab 5: http://localhost:8080/ui/settlement
#    Tab 6: http://localhost:8080/ui/returns

# 4. Have two small image files ready (any PNG/JPEG) for upload

# 5. Terminal with the project root open (for showing code/structure if needed)
```

---

## Section 1: Introduction & Architecture (0:00–2:30)

### 0:00–0:30 — Opening

**Show:** Title slide or the README header in the browser.

> "This is a mobile check deposit system built for brokerage accounts. It handles the full deposit lifecycle — from image capture through vendor validation, business rule enforcement, operator review, ledger posting, settlement file generation, and return/reversal handling."

### 0:30–1:30 — Architecture Overview

**Show:** `docs/architecture.md` system diagram in the browser, or the ASCII diagram from README.

> "The system is two Go binaries. The main app server on port 8080 serves both the REST API and a server-rendered web UI using HTMX — no JavaScript build step. The vendor stub runs separately on port 8081, simulating an external check validation service like the one Apex would integrate with in production."
>
> "I chose Go for compile-time safety and single-binary deployment. SQLite for zero-ops — the database creates itself on startup, and `make reset` deletes one file. HTMX over a React SPA because the operator review workflow doesn't need rich client-side interactivity, and keeping everything server-rendered means all business logic lives in one place."
>
> "For settlement files, I'm generating real X9.37 ICL binary format using moov-io/imagecashletter — not JSON approximations. These files embed the actual check images and use proper X9 record types."

### 1:30–2:30 — State Machine & Data Model

**Show:** The state machine diagram from `docs/architecture.md`.

> "Every deposit flows through this state machine. The happy path goes Requested → Validating → Analyzing → Approved → FundsPosted → Completed. But the system also handles rejection at multiple points — vendor failures reject from Validating, business rule failures reject from Analyzing. Returns can happen after FundsPosted or even after Completed."
>
> "All transitions are enforced by a centralized validator in `internal/transfers/state.go` — there's one map of valid transitions, and every state change goes through a single function that validates the transition, updates the database, and writes an audit event atomically. You can't get into an invalid state."
>
> "The data model has 14 tables covering transfers, images, vendor results, rule evaluations, operator actions, audit events, a double-entry ledger, settlement batches, return notifications, and a notification outbox."

---

## Section 2: Happy Path — End to End (2:30–5:30)

### 2:30–3:30 — Submit a Deposit

**Show:** Browser → `/ui/simulate`

> "Let me walk through the happy path. I'm on the deposit simulator — this is what would be the mobile app's submission endpoint in production."

**Do:** Select account INV-1001, enter $250.00, upload front and back images, select "Clean Pass" scenario, click Submit.

> "I'm depositing $250 to account INV-1001 with the 'clean pass' vendor scenario. When I submit, the system does several things in sequence:"
>
> "First, it creates the transfer record in Requested state. Then it saves the check images to disk and computes SHA256 hashes. It transitions to Validating and calls the vendor stub, which returns a PASS decision with extracted MICR data. Then it transitions to Analyzing and runs four business rules: account eligibility, deposit limit check, contribution type defaulting, and internal duplicate detection via fingerprint hash."
>
> "Since everything passes, it auto-approves, creates a double-entry ledger posting — crediting the investor account and debiting the omnibus account — and transitions to FundsPosted. All of that happened in one API call."

### 3:30–4:15 — Transfer Detail & Decision Trace

**Show:** Click through to `/ui/transfers`, then click into the deposit detail.

> "Here's the transfer list — I can filter by state. Let me click into this deposit."

**Show:** Transfer detail page with decision trace, vendor results, rule evaluations.

> "The transfer detail page shows everything: the current state, amounts, business date, the vendor's MICR extraction results, and the complete decision trace — every state transition with timestamps and the actor that triggered it. This is the audit trail the spec requires. Every rule evaluation is also persisted — you can see each rule passed with its details."

### 4:15–4:45 — Ledger

**Show:** Switch to `/ui/ledger`

> "The ledger page shows account balances. You can see INV-1001 was credited $250 and the omnibus account OMNI-ACME was debited $250. If I drill into INV-1001, I see the individual journal entries. This is real double-entry bookkeeping — every journal has two entries that sum to zero."

### 4:45–5:30 — Settlement & Completion

**Show:** Switch to `/ui/settlement`

> "Now let's settle. I click 'Generate Batch' — this collects all FundsPosted deposits for the current business date and generates a real X9.37 ICL binary file."

**Do:** Click Generate Batch.

> "The batch shows the item count, total amount, and the file path. This ICL file contains proper X9 record types — file header, cash letter header, bundle header, check detail records with the MICR data, image view records with the actual check images embedded, and all the corresponding trailer records. My Go tests parse these files back and verify the structure."

**Do:** Click Acknowledge.

> "When I acknowledge the batch — simulating the settlement bank's confirmation — all deposits in the batch transition to Completed. That's the full happy path: submit, validate, approve, post funds, settle, complete."

---

## Section 3: Vendor Scenarios & Rejections (5:30–7:30)

### 5:30–6:15 — IQA Failures

**Show:** Browser → `/ui/simulate`

> "The vendor stub supports seven deterministic scenarios, selectable by account suffix, request header, or the scenario picker in the UI. Let me show a few."

**Do:** Submit with INV-1002, $100, "IQA Blur" scenario.

> "This deposit was rejected — the vendor returned an IQA failure for blur. The transfer went Requested → Validating → Rejected. In production, this would prompt the investor to retake the photo. The rejection code and message are stored on the transfer record."

### 6:15–7:00 — Review Queue Scenarios

**Do:** Submit with INV-1004, $300, "MICR Failure" scenario.

> "MICR failure is different — the vendor couldn't read the magnetic ink line, so instead of rejecting outright, it flags the deposit for manual review. The transfer stops at Analyzing with review_required=true."

**Show:** Switch to `/ui/review`

> "It appears in the operator review queue. The operator can see the check images, the vendor's MICR data, confidence scores, risk score, and the rule evaluation results. This gives the operator enough context to make an informed decision."

### 7:00–7:30 — Operator Approve & Reject

**Do:** Click into the MICR failure deposit, click Approve.

> "I'll approve this one — the operator has verified the MICR data manually. The transfer moves from Analyzing → Approved → FundsPosted, and a ledger posting is created. The operator action — who approved, when, and their notes — is recorded in the audit trail."

> "If I had rejected it, the transfer would go to Rejected and no ledger posting would be created. Both actions are logged."

---

## Section 4: Business Rule Enforcement (7:30–8:30)

### 7:30–8:00 — Over-Limit Rejection

**Show:** Browser → `/ui/simulate`

**Do:** Submit with INV-1001, $6,000, "Clean Pass" scenario.

> "Even though the vendor says the check is valid, the funding service enforces a $5,000 per-deposit limit. This deposit is rejected during the Analyzing phase by the MAX_DEPOSIT_LIMIT rule. The vendor scenario doesn't matter — business rules are evaluated independently."

### 8:00–8:30 — Duplicate Detection

> "There's also internal duplicate detection. The system computes a SHA256 fingerprint from the MICR routing number, account number, serial number, amount, and investor account. If a non-rejected transfer with the same fingerprint already exists, the new deposit is blocked. This is a second layer beyond whatever the vendor checks."

---

## Section 5: Return/Reversal Handling (8:30–10:00)

### 8:30–9:15 — Process a Return

**Show:** Switch to `/ui/returns`

> "After a check has been deposited and funds posted, the check can bounce. Let me process a return."

**Do:** Enter the transfer ID of the completed clean_pass deposit, reason code R01 "Insufficient Funds", submit.

> "The return processing does three things atomically: First, it creates a reversal journal — debiting the investor account and crediting the omnibus account, which undoes the original deposit posting. Second, it creates a fee journal — debiting the investor $30 and crediting the fee revenue account. Third, it transitions the transfer to the Returned state."

### 9:15–10:00 — Verify Reversal in Ledger

**Show:** Switch to `/ui/ledger`, drill into INV-1001.

> "Look at the ledger now. INV-1001 has the original $250 credit, then a $250 debit for the reversal, and a $30 debit for the return fee. The net balance is -$30 — the investor owes the return fee. The omnibus account is balanced. The fee revenue account has $30. All journals sum to zero — the double-entry invariant holds."
>
> "This is a key correctness property I test for: the sum of all ledger entries across all accounts must always be zero."

---

## Section 6: Testing & Quality (10:00–11:30)

### 10:00–10:45 — Go Tests

**Show:** Terminal

> "I have 14 Go test functions across 5 packages."

**Do:** Run `make test` (or show pre-recorded output if time is tight).

> "The Go tests cover the happy path end-to-end, all seven vendor scenarios, funding rule rejections — over-limit and inactive account — internal duplicate fingerprint detection, state machine transitions including invalid ones that should be rejected, settlement batch generation with X9.37 file parsing to verify the output, return processing with the $30 fee calculation, and business date cutoff logic with weekend rollforward."
>
> "These aren't just unit tests — the integration tests stand up the real SQLite database, run the full deposit flow, and verify the final state. The settlement test generates an ICL file, parses it back with the same library, and verifies record counts and amounts."

### 10:45–11:30 — Playwright E2E Tests

> "I also have 14 Playwright end-to-end tests that exercise the web UI."

**Do:** Run `make test-e2e` (or show pre-recorded output).

> "These test the full user journey through the browser — deposit submission, state transitions, operator review approve and reject flows, ledger balances, settlement generation and acknowledgment, returns and reversals, business rule rejections, navigation, and transfer detail views. They verify the UI reflects the correct system state at every step."

---

## Section 7: Design Decisions & Trade-offs (11:30–13:00)

### 11:30–12:15 — Key Decisions

**Show:** `docs/decision_log.md` in browser or editor.

> "I documented nine key design decisions with alternatives considered. Let me highlight the most important ones."
>
> "**Go over Java:** Single static binary, fast compile times, built-in HTTP server. Spring Boot would have worked but adds operational overhead for a system this size."
>
> "**SQLite over PostgreSQL:** Zero-ops, single-file database. For a demo system, there's no need for a separate database server. But if this were production, you'd swap in PostgreSQL — the repository layer abstracts the database."
>
> "**Separate vendor stub process:** The stub runs as its own binary on its own port, mirroring how production would talk to a real external service over HTTP. I could have used a Go interface for in-process stubbing, but that wouldn't exercise the real HTTP serialization path."
>
> "**Real X9.37 ICL files:** I started with structured JSON but replaced it with real X9 binary output using moov-io/imagecashletter. The settlement files have proper record types, embedded images, and can be parsed by any X9-compliant system."

### 12:15–13:00 — What I'd Add with More Time

> "With more time, I'd add: daily and monthly cumulative deposit limits — currently it's only per-deposit. Operator authentication and RBAC. A resubmission flow for IQA failures. Distribution analytics dashboards. Webhook notifications instead of the outbox table. And concurrent deposit stress testing."
>
> "For production readiness, you'd also need PostgreSQL for horizontal scaling, real TLS and authentication, a holiday calendar for business date calculations, and configurable fee schedules per correspondent."

---

## Section 8: Closing (13:00–14:00)

### 13:00–14:00 — Summary & Evaluation Guidance

**Show:** Transfer list or architecture diagram.

> "To summarize: this is a complete mobile check deposit system covering the full lifecycle. The key things I'd want evaluators to look at are:"
>
> "**State machine correctness** — submit deposits through all seven vendor scenarios and verify each terminates in the correct state. Try invalid transitions via the API and confirm they're rejected."
>
> "**Ledger integrity** — after a batch of deposits, verify that all ledger entries across all accounts net to zero. Process a return and confirm the reversal plus fee entries balance."
>
> "**Settlement accuracy** — generate a batch, parse the X9.37 ICL file, and confirm it contains exactly the FundsPosted deposits for the business date with correct MICR data and embedded images."
>
> "**Audit completeness** — pull the decision trace for any deposit and verify every state transition, rule evaluation, and operator action is logged with actor, timestamp, and details."
>
> "Thank you — I'm happy to go deeper on any part of the system."

---

## Quick-Reference: What to Show When

| Timestamp | Screen | What's Happening |
|-----------|--------|-----------------|
| 0:00 | README / Title | Introduction |
| 0:30 | architecture.md diagram | Two-binary architecture, tech choices |
| 1:30 | State machine diagram | Transfer lifecycle, centralized validation |
| 2:30 | /ui/simulate | Submit happy path deposit |
| 3:30 | /ui/transfers/{id} | Transfer detail + decision trace |
| 4:15 | /ui/ledger | Double-entry postings |
| 4:45 | /ui/settlement | Generate batch, show X9.37 output |
| 5:15 | /ui/settlement | Acknowledge → Completed |
| 5:30 | /ui/simulate | IQA blur rejection |
| 6:15 | /ui/simulate → /ui/review | MICR failure → review queue |
| 7:00 | /ui/review | Operator approve |
| 7:30 | /ui/simulate | Over-limit rejection ($6,000) |
| 8:30 | /ui/returns | Process a return |
| 9:15 | /ui/ledger | Reversal + fee in ledger |
| 10:00 | Terminal | `make test` — Go tests |
| 10:45 | Terminal | `make test-e2e` — Playwright |
| 11:30 | docs/decision_log.md | Design decisions |
| 12:15 | (talking) | What I'd add with more time |
| 13:00 | Transfer list | Summary & evaluation guidance |

---

## Talking Points Cheat Sheet (Keep Nearby)

**If asked "Why Go?"** → Single binary, fast compile, strong typing, built-in HTTP/concurrency. Chi router is idiomatic, no framework overhead.

**If asked "Why not PostgreSQL?"** → Zero-ops for demo. Repository layer abstracts DB — swap is straightforward. SQLite with WAL mode handles single-user fine.

**If asked "Why HTMX?"** → No JS build step, no API serialization layer, all logic in Go. Templates type-checked at startup. HTMX gives dynamic behavior via HTML attributes.

**If asked "How do you ensure ledger correctness?"** → Double-entry: every journal has entries summing to zero. Tests verify the invariant after operations. Reversals create new journals rather than modifying existing ones.

**If asked "How does settlement work?"** → Batch all FundsPosted deposits for a business date. Generate X9.37 ICL binary with moov-io/imagecashletter. Embedded check images. Ack transitions to Completed.

**If asked "How do you handle duplicates?"** → Two layers: vendor-level detection (stub scenario) + internal SHA256 fingerprint of MICR+amount+account. Non-rejected transfers with same fingerprint are blocked.

**If asked "What about the cutoff?"** → 6:30 PM CT (America/Chicago, handles DST). After cutoff → next business day. Weekends roll to Monday. Injectable Clock interface for testable time.

**If asked "How is the audit trail structured?"** → `audit_events` table: entity type/ID, actor, event type, from/to state, JSON details, timestamp. Every state transition auto-creates an event. Operator actions separately logged in `operator_actions`.
