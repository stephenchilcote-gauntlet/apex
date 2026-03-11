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
