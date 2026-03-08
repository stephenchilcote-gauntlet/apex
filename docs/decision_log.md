# Decision Log

Key design decisions and alternatives considered.

---

## 1. Go vs. Java

**Decision:** Go

**Rationale:**
- Single static binary — no JVM, no classpath, no WAR deployment
- Fast compile times enable tight iteration loops
- Built-in HTTP server and concurrency primitives (no framework bootstrap)
- Strong typing catches bugs at compile time without annotation overhead
- `chi` router provides idiomatic HTTP routing without a full framework

**Alternatives considered:**
- **Java (Spring Boot):** Richer ecosystem for enterprise patterns (DI, ORM, validation annotations), but heavier operational footprint. Spring's auto-configuration adds cognitive overhead for a system this size. Would have been a reasonable choice if the system needed complex transaction management or enterprise integration patterns.

---

## 2. SQLite vs. PostgreSQL

**Decision:** SQLite via `github.com/mattn/go-sqlite3`

**Rationale:**
- Zero-ops: no database server to install, configure, or manage
- Single-file database simplifies backup, reset, and demo workflows (`make reset` deletes one file)
- `PRAGMA foreign_keys = ON` provides referential integrity
- Performance is more than adequate for a single-user demonstration system
- Database is created and migrated automatically on startup

**Alternatives considered:**
- **PostgreSQL:** Better for production (concurrent writes, MVCC, rich query planner), but requires a running server and connection management. Would add Docker Compose complexity for a system that runs with `make dev`.
- **In-memory/JSON files:** Simpler but no query capability, no foreign keys, no transactional guarantees.

---

## 3. HTMX + Server Templates vs. SPA (React/Vue)

**Decision:** Go `html/template` + HTMX

**Rationale:**
- No JavaScript build step, no node_modules, no bundler configuration
- Server-side rendering means all business logic stays in Go
- HTMX provides dynamic interactions (form submission, partial page updates) with HTML attributes
- Templates are type-checked at startup, not at runtime in the browser
- Total frontend complexity: ~8 HTML templates + one HTMX script tag

**Alternatives considered:**
- **React/Next.js:** Richer interactivity, component reuse, but requires a separate build pipeline, API serialization layer, and doubles the surface area of the codebase for a demo system.
- **CLI-only:** Would satisfy the spec's "CLI or minimal UI" requirement, but a web UI better demonstrates the operator review workflow and provides visual evidence of the system working.

---

## 4. Separate Vendor Stub Process

**Decision:** Vendor stub runs as a separate Go binary (`cmd/vendorstub`) on port 8081

**Rationale:**
- Clean separation mirrors production architecture where the vendor is an external service
- The stub can be started, stopped, and configured independently
- The main app communicates with the stub via HTTP, exercising the real integration path
- Scenario configuration lives in `config/vendor_scenarios.yaml`, changeable without recompilation
- The stub could be replaced with a real vendor SDK without changing the main app

**Alternatives considered:**
- **In-process stub (Go interface):** Simpler to test but doesn't exercise HTTP serialization/deserialization or network error paths. Would make the integration feel artificial.
- **Docker-only stub:** Adds Docker as a hard requirement for development.

---

## 5. Centralized State Machine with Transition Validator

**Decision:** All valid state transitions defined in a single map (`internal/transfers/state.go`). Every transition goes through `TransferService.Transition()`, which validates the transition, updates the database, and logs an audit event atomically.

**Rationale:**
- Single source of truth for allowed transitions prevents state corruption
- Invalid transitions fail loudly with an error rather than silently proceeding
- Audit events are created automatically on every transition — impossible to forget
- Easy to reason about: look at the map to understand all possible flows

**Alternatives considered:**
- **State pattern (per-state objects with methods):** More extensible for complex per-state behavior, but overkill for 8 states with simple transition rules.
- **Event sourcing:** Would provide a complete history by design, but adds significant complexity (event store, projections, snapshots) beyond what the spec requires.

---

## 6. X9.37 ICL Binary via moov-io/imagecashletter

**Decision:** Settlement files are generated as real X9.37 ICL binary files using `github.com/moov-io/imagecashletter` (v0.13.5), with embedded check images.

**Rationale:**
- Production-fidelity output: real X9.37 record types (01/10/20/25/26/50/52/70/90/99) with proper field encoding
- Mature, well-tested open-source library handles all X9 encoding complexity
- Check images are embedded directly in the ICL file via ImageViewData (record 52)
- Files can be parsed back by any X9-compliant system or the same library for round-trip validation
- Tests verify the generated file by parsing it back and checking record structure, item counts, and amounts

**Alternatives considered:**
- **Structured JSON equivalent:** Simpler and human-readable, but doesn't demonstrate real settlement file capability. Used initially, then replaced.
- **Custom X9 writer:** Would avoid the library dependency, but reimplements well-tested functionality unnecessarily.

---

## 7. SHA256 Fingerprint for Internal Duplicate Detection

**Decision:** Compute `SHA256(routing|account|serial|amountCents|investorAccountID)` as a duplicate fingerprint. Store on the transfer record and check for existing non-rejected transfers with the same fingerprint.

**Rationale:**
- Deterministic: same check deposited twice always produces the same fingerprint
- Excludes rejected transfers from duplicate checks (a rejected deposit shouldn't block a retry)
- Supplements the vendor-level duplicate detection with an internal safety net
- Fingerprint is stored on the transfer for auditability

**Alternatives considered:**
- **Image perceptual hash:** Would catch re-photographs of the same check, but adds ML/image-processing complexity beyond the spec's requirements.
- **MICR-only matching:** Would miss cases where the same check is deposited to different accounts (the spec implies per-account dedup is desired).

---

## 8. Hardcoded $30 Return Fee

**Decision:** Return fee is hardcoded at $30 (3000 cents) in the `return_notifications` table default and the returns service logic.

**Rationale:**
- The spec explicitly states "$30 return fee for MVP"
- A configurable fee schedule would add unnecessary complexity for a single fee amount
- The fee is applied as a separate ledger journal entry (`RETURN_FEE` type), so the accounting is correct even with a hardcoded amount

**Alternatives considered:**
- **Configurable fee per correspondent:** Proper for production (different correspondents may have different fee schedules), but beyond MVP scope.
- **Fee waiver logic:** Some returns in production might waive fees (bank error, first-time, etc.), but the spec doesn't mention this.

---

## 9. Business Date Cutoff: 6:30 PM CT with Weekend Rollforward

**Decision:** Deposits submitted after 6:30 PM Central Time are assigned to the next business day. Saturday and Sunday submissions roll forward to Monday. Implemented in `internal/clock/` with an injectable `Clock` interface for testability.

**Rationale:**
- The spec requires "EOD processing cutoff (6:30 PM CT)" with late submissions rolling to next business day
- `America/Chicago` timezone used for all business date calculations (handles CST/CDT automatically)
- Weekend rollforward ensures Saturday/Sunday deposits don't create batches for non-business days
- Injectable clock allows tests to verify cutoff behavior without time-dependent flakiness

**Alternatives considered:**
- **Holiday calendar:** Production systems need a holiday calendar (no settlement on bank holidays), but the spec doesn't mention holidays. Would add configuration/data maintenance burden.
- **UTC-based cutoff:** Simpler but doesn't match the spec's "CT" requirement. Financial systems need to use the business timezone.
