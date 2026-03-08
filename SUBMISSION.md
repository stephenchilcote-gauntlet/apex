# Submission

- **Project name:** Mobile Check Deposit System

- **Summary:**
  Built a complete mobile check deposit system in Go that handles the full deposit lifecycle: image capture simulation, vendor validation (stubbed with 7 configurable scenarios), business rule enforcement, operator review workflows, double-entry ledger posting, settlement batch generation, and check return/reversal processing. Chose Go for compile-time type safety and single-binary deployment; SQLite for zero-ops data storage; HTMX with server-rendered templates for a functional UI without a JS build step; and a separate vendor stub process for clean isolation and independent configurability. Settlement files are generated as real X9.37 ICL binary format with embedded check images using moov-io/imagecashletter. Key trade-offs: centralized state machine validation (correctness over flexibility), and hardcoded $30 return fee (simplicity for MVP).

- **How to run:**
  ```bash
  cp .env.example .env
  make dev
  # App at http://localhost:8080, Vendor stub at http://localhost:8081
  ```

- **Test/eval results:**
  ```bash
  # Go unit/integration tests — 14 test functions, all passing
  make test

  # Playwright E2E tests — 14 spec files
  make test-e2e
  ```
  Go tests cover: happy path E2E, all 7 vendor stub scenarios, funding rule rejections (over-limit, inactive account), internal duplicate fingerprint detection, state machine transitions (valid + invalid), settlement batch generation and acknowledgment, return processing with $30 fee calculation, business date cutoff with weekend rollforward.

  Test report artifacts in `/reports/`.

- **With one more week, we would:**
  - Add daily/monthly cumulative deposit limit enforcement (currently only per-deposit $5,000 cap)
  - Add operator authentication and role-based access control
  - Build a resubmission flow for IQA failures (endpoint removed pending UI integration)
  - Add deposit amount distribution analytics and reporting dashboards
  - Implement webhook/callback notifications instead of the outbox table
  - Add concurrent deposit stress testing

- **Risks and limitations:**
  - **No authentication:** The operator review and admin endpoints have no auth; any user can approve/reject deposits
  - **Stub-only vendor integration:** The vendor service returns canned responses; a real integration would require network error handling, retries, and credential management
  - **Single-node SQLite:** Not suitable for horizontal scaling; a production system would need PostgreSQL or similar
  - **No real settlement:** Settlement files are real X9.37 ICL binary format (via moov-io/imagecashletter) with embedded check images, but are not submitted to any bank
  - **Simplified business rules:** Only per-deposit limit ($5,000); no daily/monthly/annual cumulative limits or correspondent-specific caps
  - **No image validation:** Check images are stored but not actually analyzed; the stub ignores image content
  - **Hardcoded return fee:** $30 fee is compile-time constant; production would need configurable fee schedules

- **How should ACME evaluate production readiness?**
  1. **State machine correctness:** Submit deposits through all 7 vendor scenarios and verify each terminates in the correct state. Attempt invalid transitions via API and confirm they're rejected.
  2. **Ledger integrity:** After a batch of deposits, verify that the sum of all ledger entries across all accounts nets to zero (double-entry invariant). Process returns and confirm reversal + fee entries balance.
  3. **Settlement accuracy:** Generate a batch, parse the X9.37 ICL file with moov-io/imagecashletter, confirm it contains exactly the FundsPosted deposits for the business date with correct MICR data and embedded images, and that acknowledgment transitions all items to Completed.
  4. **Audit completeness:** Pull the decision trace for any deposit and verify every state transition, rule evaluation, and operator action is logged with actor, timestamp, and details.
  5. **Boundary testing:** Submit a $5,001 deposit (should reject), submit the same check twice (duplicate fingerprint should block), submit after 6:30 PM CT on Friday (should roll to Monday).
