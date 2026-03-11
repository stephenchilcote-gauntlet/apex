# Decision Log

Key design decisions and alternatives considered.

---

## 1. Go vs. Java

**Decision:** Go (1.22+) with `chi` router

**Rationale:**
- **Single static binary:** `go build` produces one executable with zero runtime dependencies — no JVM, no classpath, no WAR deployment. `make dev` compiles and runs both services in under 2 seconds.
- **Fast compile times:** Sub-second incremental builds enable tight iteration loops. The entire 10K-line codebase compiles from scratch in ~3 seconds.
- **Built-in HTTP server:** Go's `net/http` is production-quality out of the box. No framework bootstrap, no dependency injection container, no annotation processor. The `chi` router adds idiomatic URL parameter routing and middleware chaining on top of the stdlib — it's a thin library, not a framework.
- **Strong typing without ceremony:** Compile-time type checking catches bugs that Java catches with annotations and Spring validates at startup. Go's explicit error handling makes failure paths visible in the code rather than hidden in exception hierarchies.
- **Concurrency primitives:** Goroutines and channels are built into the language. The concurrent deposit stress test (20 goroutines) required no external libraries.
- **Financial systems affinity:** Go's explicit error handling, lack of exceptions, and simple control flow make it easier to reason about correctness in a system where a missed error could post funds incorrectly.

**Why `chi` specifically:**
- Compatible with `net/http` — handlers are standard `http.HandlerFunc`, middleware is standard `func(http.Handler) http.Handler`. No framework lock-in.
- URL parameter extraction (`chi.URLParam(r, "transferId")`) — the one feature stdlib lacks.
- Composable middleware chain (rate limiting, API key auth, CORS, logging) without reflection or magic.

**Alternatives considered:**
- **Java (Spring Boot):** Richer ecosystem for enterprise patterns (DI, ORM, validation annotations), but heavier operational footprint. Spring's auto-configuration adds cognitive overhead for a system this size. Would require a JVM, a build tool (Maven/Gradle), and ~30s startup times. Would have been a reasonable choice if the system needed complex transaction management, JPA entity relationships, or enterprise integration patterns.
- **Python (Flask/FastAPI):** Rapid prototyping but no compile-time type safety. For a financial system with 14 database tables and a state machine, runtime type errors are an unacceptable risk.

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

This is an **internal operations dashboard**, not a consumer-facing app. The interaction patterns — submit a form, view a table, click approve/reject, refresh a queue — are exactly what server-rendered HTML does best. HTMX bridges the gap to "feels dynamic" without introducing a second language, a second build system, or a second mental model:

- **Zero JS build pipeline:** The entire frontend is 12 Go templates (2,294 lines) + one 14KB vendored `htmx.min.js`. No webpack, no Vite, no npm, no transpilation. `make dev` compiles and runs in seconds.
- **Single-language codebase:** All business logic, validation, and rendering lives in Go. There is no API serialization layer (no JSON ↔ TypeScript DTO mapping), no state management library, and no client-side routing. This eliminates an entire class of bugs (stale client state, API contract drift, hydration mismatches).
- **HTMX delivers the specific dynamism this app needs:** live-polling dashboard cards (every 15s), auto-refreshing review queue (every 20s), real-time transfer status updates (every 3–5s), debounced command-palette search (150ms delay), and health-status indicators (every 30s) — all via declarative HTML attributes (`hx-get`, `hx-trigger`, `hx-swap`), not imperative JavaScript.
- **No custom JavaScript:** The project contains zero `.js` files beyond the vendored HTMX library. All interactivity is expressed as HTML attributes. This is auditable, testable (Playwright sees the same DOM), and has zero client-side state to get out of sync.
- **Operational fit:** Financial operations UIs prioritize correctness and auditability over animation smoothness. Server-rendered HTML means the server is always the source of truth — there's no "optimistic update" that could show a stale approval status. Every render reflects the actual database state.

**Quantitative comparison:**

| Metric | HTMX approach (actual) | React SPA (estimated) |
|--------|------------------------|----------------------|
| Frontend source files | 12 templates, 0 JS files | ~40+ components, hooks, stores |
| Build dependencies | 0 (vendored .js) | ~200+ npm packages |
| Lines of frontend code | 2,294 (templates) + 1,063 (CSS) | ~5,000–8,000 (TSX + CSS + state) |
| API surface to maintain | 0 (templates render directly) | ~20 REST endpoints × request/response types |
| Time to first render | Instant (SSR) | Depends on JS bundle parse time |
| Client-side state bugs | Impossible (no client state) | Ongoing risk |

**Alternatives considered:**
- **React/Next.js:** Would provide richer interactivity (drag-and-drop, complex filtering, offline support) but none of those are needed here. Would double the codebase size, introduce npm/node as a runtime dependency, require a JSON API contract layer, and create a second CI pipeline. For an ops dashboard that 1–3 operators use, this is pure overhead.
- **Vue + Vite:** Lighter than React but same fundamental cost: a second language, a second build system, API serialization. The spec says "minimal UI or CLI" — a full SPA framework is the opposite of minimal.
- **CLI-only:** Would satisfy the spec's "CLI or minimal UI" requirement, but the operator review workflow (view check images, compare MICR data, click approve/reject) is fundamentally visual. A CLI would make the demo harder to evaluate and the operator workflow unusable.
- **Bare HTML (no HTMX):** Would work for static pages but the spec requires real-time operator queues and status tracking. Without HTMX, we'd need either full page reloads (poor UX) or hand-written fetch/DOM code (reimplements what HTMX does).

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

## 9. Raw SQL vs. ORM (GORM, sqlx, ent)

**Decision:** Raw `database/sql` with hand-written queries

**Rationale:**
- **Full control over queries:** Every SQL statement is visible and reviewable. In a financial system, it's critical to know exactly what query hits the database — no magic `SELECT *`, no lazy loading, no N+1 surprises.
- **No abstraction leakage:** ORMs eventually force you to understand the generated SQL anyway. For 14 tables with straightforward CRUD + a few joins, the raw SQL is simpler than learning an ORM's query builder DSL.
- **Transaction control:** The ledger's double-entry posting requires multi-statement transactions with explicit `BEGIN`/`COMMIT`/`ROLLBACK`. Raw `sql.Tx` gives precise control; ORMs often fight you on transaction boundaries.
- **Zero additional dependencies:** `database/sql` is stdlib. No code generation step, no schema DSL, no migration framework beyond our own 50-line runner.
- **SQLite compatibility:** Some Go ORMs have SQLite quirks (type mapping, `RETURNING` clause support). Raw SQL avoids all of them.

**Alternatives considered:**
- **GORM:** Most popular Go ORM. Would reduce boilerplate for simple CRUD but adds ~15K lines of dependency, struct tags, and implicit behavior. Transaction hooks and association loading add cognitive overhead.
- **sqlx:** Lightweight extension that adds struct scanning. Reasonable choice — we'd use it if queries were more complex. For this project, `sql.Scan()` into explicit fields is clear enough.
- **ent:** Facebook's code-generated ORM. Excellent for complex schemas but requires a code generation step and a schema DSL — overkill for 14 tables.

---

## 10. Business Date Cutoff: 6:30 PM CT with Weekend Rollforward

**Decision:** Deposits submitted after 6:30 PM Central Time are assigned to the next business day. Saturday and Sunday submissions roll forward to Monday. Implemented in `internal/clock/` with an injectable `Clock` interface for testability.

**Rationale:**
- The spec requires "EOD processing cutoff (6:30 PM CT)" with late submissions rolling to next business day
- `America/Chicago` timezone used for all business date calculations (handles CST/CDT automatically)
- Weekend rollforward ensures Saturday/Sunday deposits don't create batches for non-business days
- Injectable clock allows tests to verify cutoff behavior without time-dependent flakiness

**Alternatives considered:**
- **Holiday calendar:** Production systems need a holiday calendar (no settlement on bank holidays), but the spec doesn't mention holidays. Would add configuration/data maintenance burden.
- **UTC-based cutoff:** Simpler but doesn't match the spec's "CT" requirement. Financial systems need to use the business timezone.

---

## 11. `make dev` (Native) vs. Docker-First Development

**Decision:** `make dev` runs both services natively with `go run`. Docker is available (`docker-compose.yml`) but optional.

**Rationale:**
- **Fastest possible iteration cycle:** `make dev` starts both services in ~2 seconds. Docker builds add 15–30 seconds per change.
- **Zero prerequisites beyond Go:** No Docker daemon, no container runtime, no volume mount quirks. The evaluator needs only `go` installed (and `npm` for Playwright e2e tests).
- **SQLite makes this possible:** With PostgreSQL, Docker would be semi-required to provide the database server. SQLite's zero-server architecture means the app creates its own database file on first run.
- **Docker still available:** `docker-compose.yml` and both Dockerfiles exist for production-like deployment or evaluators who prefer containers.

**Alternatives considered:**
- **Docker-first (`docker compose up` only):** Consistent environment but slower iteration and requires Docker installed. Some evaluators may not have Docker readily available.
- **Devcontainers:** Great for onboarding but adds `.devcontainer/` configuration complexity for a project that already runs with one command.

---

## 12. Dual Test Strategy: Go Unit/Integration + Playwright E2E

**Decision:** 144 Go test functions for unit/integration testing + 15 Playwright spec files (~105 test cases) for end-to-end browser testing.

**Rationale:**
- **Go tests cover correctness:** State machine transitions, ledger invariants (zero-sum), duplicate detection, rule evaluation, settlement generation, vendor client behavior — all tested in-process with sub-second execution.
- **Playwright tests cover the user experience:** Form submission, operator review workflow, navigation, keyboard shortcuts, visual regression — all tested through a real browser against running servers.
- **Each layer tests what it's best at:** Go tests don't need a browser. Playwright tests don't need to re-test internal logic. No redundant coverage between layers.
- **Playwright provides demo evidence:** Test recordings and screenshots serve as proof the system works, satisfying the spec's requirement for visual evidence.

**Alternatives considered:**
- **Go tests only:** Would miss UI bugs (broken templates, incorrect form actions, missing HTMX attributes). The operator workflow is fundamentally a UI flow.
- **Playwright only:** Would be slow and fragile for testing internal invariants like "ledger entries always sum to zero." Go tests verify this in milliseconds.
- **Cypress:** Similar capability to Playwright but Playwright has better multi-browser support and the `codegen` tool is useful for writing specs quickly.
