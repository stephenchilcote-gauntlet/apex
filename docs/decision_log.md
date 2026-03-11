# Decision Log

## 1. Go + chi

Single static binary, sub-second incremental builds, stdlib HTTP server. chi adds URL parameter routing and middleware chaining on top of `net/http` without framework lock-in.

## 2. SQLite

Zero-ops — no server process, single file, auto-created on startup. The spec lists "SQLite, JSON, or equivalent." Foreign keys and transactions come free.

## 3. HTMX + Go Templates

This is an ops dashboard: forms, tables, approve/reject buttons. HTMX adds the dynamic parts we need — polling the review queue (20s), live transfer status (3–5s), debounced search (150ms) — all as HTML attributes. 12 templates, 0 custom JS files, no build step. Server is always the source of truth, so stale-state bugs are structurally impossible.

## 4. Separate Vendor Stub Process

Mirrors production topology — the vendor is an external HTTP service. Exercises real serialization, lets us swap in a real vendor client without changing the app. Configured via YAML, no recompilation needed.

## 5. Centralized State Machine

All valid transitions in one map (`internal/transfers/state.go`). Every transition goes through `TransferService.Transition()`, which validates, updates DB, and logs an audit event atomically. Can't forget to audit.

## 6. Real X9.37 ICL via moov-io/imagecashletter

The spec says "X9 ICL or structured equivalent." We generate real X9.37 binary files with embedded check images. Tests parse them back to verify round-trip correctness.

## 7. SHA256 Duplicate Fingerprint

`SHA256(routing|account|serial|amount|investorAccountID)` — deterministic, stored on the transfer for auditability. Excludes rejected transfers so retries aren't blocked. Supplements vendor-level dedup.

## 8. Hardcoded $30 Return Fee

The spec says "$30 return fee for MVP." Posted as a separate `RETURN_FEE` ledger journal so the accounting is correct.

## 9. Raw SQL (no ORM)

14 tables, straightforward CRUD. The ledger's double-entry posting needs explicit transaction control. Every query is visible in the code.

## 10. Business Date Cutoff

6:30 PM CT with weekend rollforward, per spec. `America/Chicago` handles DST. Injectable `Clock` interface for deterministic tests.

## 11. Native `make dev` (Docker optional)

SQLite means no DB server to start. `make dev` runs both services in ~2 seconds. Docker Compose is available but not required.

## 12. Go Tests + Playwright E2E

144 Go test functions cover correctness (state machine, ledger zero-sum, dedup). 15 Playwright specs (~105 cases) cover the UI (operator workflow, forms, navigation). Each layer tests what it's best at.

---

## Schema Notes

**UUID text primary keys:** Application-generated UUIDs let us create IDs before inserting (needed for image file paths that include the transfer ID). No auto-increment coordination needed between the app and vendor stub.

**`amount_cents INTEGER`:** All money stored as integer cents. Avoids floating-point rounding errors entirely — critical for a ledger system. Formatting to `$X.XX` happens only at the UI layer.

**`signed_amount_cents` (single column, not debit/credit):** Each ledger entry is one signed integer: positive = credit, negative = debit. A deposit posting creates two entries that sum to zero. This makes the zero-sum invariant a trivial `SELECT SUM(signed_amount_cents) FROM ledger_entries` — tested in both Go unit tests and Playwright.

**`duplicate_fingerprint` on transfers:** Denormalized SHA256 hash stored directly on the transfer. Enables a single indexed lookup (`idx_transfers_fingerprint`) instead of joining through vendor_results to reconstruct MICR fields on every submission.

**`return_fee_cents DEFAULT 3000` on transfers:** Snapshot of the fee at time of return. If fee schedules ever changed, historical transfers would still show what was actually charged.

**`raw_response_json` on vendor_results:** Full vendor response stored alongside parsed fields. Parsed fields (`micr_routing_number`, `risk_score`, etc.) are used for queries and display; the raw JSON preserves the original for audit/debugging without needing to reconstruct it.

**`micr_snapshot_json` on settlement_batch_items:** MICR data is snapshotted at batch generation time. If vendor_results were ever corrected after the fact, the settlement file record reflects what was actually sent to the bank.

**`notifications_outbox` table:** Transactional outbox pattern. The notification row is inserted in the same transaction as the return processing, guaranteeing the investor gets notified even if the notification delivery system is down. Status tracks PENDING → SENT.

**`business_date_ct` on transfers:** Explicit column for the CT business date assigned at submission time (after cutoff logic). Settlement queries filter on this column directly rather than recomputing cutoff logic at query time.

**`CHECK` constraints on enums:** State, status, and type fields are enforced at the DB level (`CHECK(state IN (...))`). A bug in application code can't write an invalid state — the INSERT/UPDATE will fail.
