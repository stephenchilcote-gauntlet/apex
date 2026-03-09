import { test as base, expect, Page } from '@playwright/test';
import { CHECK_FRONT, CHECK_BACK } from './fixtures';

/**
 * Video Tour — interview-ready walkthrough of the APEX Mobile Check Deposit system.
 *
 * Produces a self-explanatory narrated video with on-screen captions, section
 * headers, and per-panel highlights so a viewer (Apex interviewer) can
 * understand the full system without a live presenter.
 *
 * Run:  npx playwright test video-tour.spec.ts
 * Output: tests/e2e/test-results/video-tour/
 */

const test = base.extend({});

// ---------------------------------------------------------------------------
// Annotation helpers
// ---------------------------------------------------------------------------

/** Full-width section banner at bottom of screen (title + subtitle). */
async function announce(page: Page, title: string, subtitle?: string) {
  await page.waitForLoadState('domcontentloaded');
  await page.evaluate(
    ({ title, subtitle }) => {
      let overlay = document.getElementById('tour-overlay');
      if (!overlay) {
        overlay = document.createElement('div');
        overlay.id = 'tour-overlay';
        overlay.style.cssText = `
          position: fixed; bottom: 0; left: 0; right: 0; z-index: 99999;
          background: linear-gradient(135deg, rgba(0,20,40,0.95), rgba(0,40,80,0.92));
          border-top: 3px solid #00d9ff;
          padding: 14px 32px;
          font-family: 'Share Tech Mono', 'Courier New', monospace;
          color: #00d9ff;
          text-align: center;
          box-shadow: 0 -4px 30px rgba(0,217,255,0.3);
          pointer-events: none;
          transition: opacity 0.3s ease;
        `;
        document.body.appendChild(overlay);
      }
      overlay.innerHTML = `
        <div style="font-size:22px;font-weight:bold;letter-spacing:2px;text-transform:uppercase;margin-bottom:${subtitle ? '6' : '0'}px;">
          ${title}
        </div>
        ${subtitle ? `<div style="font-size:14px;color:#88ccee;letter-spacing:1px;">${subtitle}</div>` : ''}
      `;
      overlay.style.opacity = '1';
    },
    { title, subtitle },
  );
  await page.waitForTimeout(3000);
}

/** Smaller explanatory caption below the main banner — stays until cleared. */
async function caption(page: Page, text: string, durationMs = 4000) {
  await page.waitForLoadState('domcontentloaded');
  await page.evaluate(
    ({ text }) => {
      let cap = document.getElementById('tour-caption');
      if (!cap) {
        cap = document.createElement('div');
        cap.id = 'tour-caption';
        cap.style.cssText = `
          position: fixed; bottom: 72px; left: 0; right: 0; z-index: 99998;
          background: rgba(0,10,20,0.88);
          border-top: 1px solid rgba(0,217,255,0.3);
          padding: 10px 40px;
          font-family: 'Inter', 'Segoe UI', system-ui, sans-serif;
          color: #cce8ff;
          font-size: 14px; line-height: 1.4;
          text-align: center;
          pointer-events: none;
          transition: opacity 0.3s ease;
        `;
        document.body.appendChild(cap);
      }
      cap.textContent = text;
      cap.style.opacity = '1';
    },
    { text },
  );
  await page.waitForTimeout(durationMs);
}

async function clearOverlay(page: Page) {
  await page.waitForLoadState('domcontentloaded');
  await page.evaluate(() => {
    const el = document.getElementById('tour-overlay');
    if (el) el.style.opacity = '0';
  });
  await page.waitForTimeout(300);
}

async function clearCaption(page: Page) {
  await page.waitForLoadState('domcontentloaded');
  await page.evaluate(() => {
    const el = document.getElementById('tour-caption');
    if (el) el.style.opacity = '0';
  });
}

async function clearAll(page: Page) {
  await clearOverlay(page);
  await clearCaption(page);
}

async function highlight(page: Page, selector: string) {
  await page.evaluate((sel) => {
    document.querySelectorAll('.tour-highlight').forEach((e) => e.remove());
    const el = document.querySelector(sel);
    if (!el) return;
    const rect = el.getBoundingClientRect();
    const ring = document.createElement('div');
    ring.className = 'tour-highlight';
    ring.style.cssText = `
      position: fixed; z-index: 99998;
      left: ${rect.left - 4}px; top: ${rect.top - 4}px;
      width: ${rect.width + 8}px; height: ${rect.height + 8}px;
      border: 2px solid #00d9ff;
      border-radius: 6px;
      box-shadow: 0 0 12px rgba(0,217,255,0.5);
      pointer-events: none;
      transition: all 0.3s ease;
    `;
    document.body.appendChild(ring);
  }, selector);
  await page.waitForTimeout(800);
}

async function clearHighlights(page: Page) {
  await page.evaluate(() => {
    document.querySelectorAll('.tour-highlight').forEach((e) => e.remove());
  });
}

async function pause(page: Page, ms = 1500) {
  await page.waitForTimeout(ms);
}

// Reusable submit helper
async function submitDeposit(
  page: Page,
  opts: { accountId?: string; amount?: string; scenario?: string } = {},
) {
  const accountId = opts.accountId ?? 'INV-1001';
  const amount = opts.amount ?? '500.00';
  const scenario = opts.scenario ?? 'clean_pass';

  await page.goto('/ui/simulate');
  await pause(page, 500);
  await page.locator('select[name="investorAccountId"]').selectOption({ value: accountId });
  await page.locator('input[name="amount"]').fill(amount);
  await page.locator('input[name="frontImage"]').setInputFiles(CHECK_FRONT);
  await page.locator('input[name="backImage"]').setInputFiles(CHECK_BACK);
  await page.locator('select[name="vendorScenario"]').selectOption(scenario);
  await page.locator('button[type="submit"]').click();
  await page.locator('[data-transfer-id]').waitFor();
  const transferId = await page.locator('[data-transfer-id]').getAttribute('data-transfer-id');
  return transferId!;
}

// ===========================================================================
// THE TOUR — single continuous test → one video file
// ===========================================================================

test.use({
  video: { mode: 'on', size: { width: 1280, height: 900 } },
  viewport: { width: 1280, height: 900 },
});

test.describe('Video Tour', () => {
  test.setTimeout(600_000); // 10 minutes — full narrated walkthrough

  test('Full Application Walkthrough', async ({ page, request }) => {

    // =======================================================================
    // SECTION 0 — RESET
    // =======================================================================
    const resp = await request.post('/api/v1/test/reset');
    expect(resp.ok()).toBeTruthy();

    // =======================================================================
    // SECTION 1 — TITLE CARD  (~0:00)
    // =======================================================================
    await page.goto('/ui/simulate');
    await announce(page, 'Mobile Check Deposit System', 'A complete deposit lifecycle for brokerage accounts');
    await caption(page,
      'Built in Go • SQLite • HTMX • X9.37 ICL settlement • 14 Go tests + 14 Playwright E2E tests',
      4000);
    await clearAll(page);

    // =======================================================================
    // SECTION 2 — ARCHITECTURE OVERVIEW VIA NAV  (~0:10)
    // =======================================================================
    await announce(page, '① System Overview', 'Two Go binaries: app server (port 8080) + vendor stub (port 8081)');
    await caption(page,
      'The app serves both a REST API (/api/v1/…) and a server-rendered UI (/ui/…) — no JS build step.',
      5000);
    await clearAll(page);

    // Flash through every tab so the viewer sees the full UI surface
    await announce(page, 'Application Pages', '7 pages covering the full deposit lifecycle');
    await clearOverlay(page);

    const tabs: [string, string][] = [
      ['Simulate', 'Deposit submission — simulates the mobile app capture'],
      ['Transfers', 'Transfer list — every deposit with state, amount, date'],
      ['Review Queue', 'Operator review — flagged deposits needing human decision'],
      ['Ledger', 'Double-entry ledger — account balances and journal entries'],
      ['Settlement', 'Batch settlement — X9.37 ICL file generation and acknowledgment'],
      ['Returns', 'Return/reversal — bounced check processing with $30 fee'],
    ];
    for (const [tab, desc] of tabs) {
      await page.locator('a.nav-level-tab', { hasText: tab }).click();
      await caption(page, `${tab}: ${desc}`, 2500);
    }
    await clearCaption(page);

    // =======================================================================
    // SECTION 3 — STATE MACHINE EXPLANATION  (~0:40)
    // =======================================================================
    // Navigate to Transfers page (neutral background for conceptual section)
    await page.locator('a.nav-level-tab', { hasText: 'Transfers' }).click();
    await page.waitForLoadState('domcontentloaded');

    await announce(page, '② Transfer State Machine',
      'Requested → Validating → Analyzing → Approved → FundsPosted → Completed');
    await caption(page,
      'All transitions enforced by a centralized validator. Invalid transitions are rejected. Audit event written on every change.',
      5000);
    await clearAll(page);

    await announce(page, 'Non-Happy Paths',
      'Rejected (from Validating or Analyzing) • Returned (from FundsPosted or Completed)');
    await caption(page,
      'Vendor failures → Rejected. Business rule failures → Rejected. Bounced checks → Returned with reversal + fee.',
      5000);
    await clearAll(page);

    // =======================================================================
    // SECTION 4 — HAPPY PATH: DEPOSIT SUBMISSION  (~1:00)
    // =======================================================================
    await announce(page, '③ Happy Path — Clean Pass Deposit', 'End-to-end: submit → auto-approve → post funds');
    await clearOverlay(page);

    await page.goto('/ui/simulate');
    await pause(page, 800);

    // Fill form with step-by-step highlights
    await caption(page, 'Select investor account INV-1001 — mapped to "Clean Pass" vendor scenario.', 3000);
    await highlight(page, 'select[name="investorAccountId"]');
    await page.locator('select[name="investorAccountId"]').selectOption({ value: 'INV-1001' });
    await pause(page, 500);
    await clearHighlights(page);

    await caption(page, 'Enter deposit amount: $500.00 (under the $5,000 per-deposit limit).', 3000);
    await highlight(page, 'input[name="amount"]');
    await page.locator('input[name="amount"]').fill('500.00');
    await pause(page, 500);
    await clearHighlights(page);

    await page.locator('input[name="frontImage"]').setInputFiles(CHECK_FRONT);
    await page.locator('input[name="backImage"]').setInputFiles(CHECK_BACK);
    await caption(page, 'Front and back check images uploaded. SHA256 hashes computed for duplicate detection.', 2500);
    await clearCaption(page);

    await caption(page, 'Vendor scenario: Clean Pass — vendor validates images and extracts MICR data.', 3000);
    await highlight(page, 'select[name="vendorScenario"]');
    await page.locator('select[name="vendorScenario"]').selectOption('clean_pass');
    await pause(page, 500);
    await clearHighlights(page);

    await caption(page, 'Submitting deposit — triggers: image save → vendor call → 4 business rules → ledger posting.', 3500);
    await highlight(page, 'button[type="submit"]');
    await pause(page, 500);
    await page.locator('button[type="submit"]').click();
    await clearHighlights(page);

    await page.locator('[data-transfer-id]').waitFor();
    await highlight(page, '[data-state]');
    await announce(page, 'Result: FundsPosted',
      'Vendor PASS + all rules pass → auto-approved → ledger posted → funds available');
    await caption(page,
      'One API call traversed 5 state transitions: Requested → Validating → Analyzing → Approved → FundsPosted.',
      5000);
    await clearAll(page);
    await clearHighlights(page);

    const happyTransferId = await page.locator('[data-transfer-id]').getAttribute('data-transfer-id');

    // =======================================================================
    // SECTION 5 — TRANSFER DETAIL + DECISION TRACE  (~1:45)
    // =======================================================================
    await announce(page, '④ Transfer Detail & Audit Trail', 'Every state transition, rule evaluation, and vendor result is persisted');
    await clearOverlay(page);

    await page.locator('a.nav-level-tab', { hasText: 'Transfers' }).click();
    await page.locator('[data-transfer]').first().waitFor();
    await caption(page, 'Transfer list shows all deposits with state badges, amounts, and business dates.', 2500);
    await clearCaption(page);

    await page.locator('[data-transfer] a').first().click();
    await page.locator('[data-state]').waitFor();
    await pause(page, 1000);

    // Transfer Info panel
    await highlight(page, '.panel');
    await caption(page,
      'Transfer Info: ID, current state, amount, account, correspondent, business date, and all timestamps.',
      4000);
    await clearHighlights(page);
    await clearCaption(page);

    // Check Images panel
    await page.evaluate(() => window.scrollBy(0, 350));
    await pause(page, 500);
    await caption(page, 'Check images: front and back stored on disk, served for operator review.', 3000);
    await clearCaption(page);

    // Vendor Result panel
    await page.evaluate(() => window.scrollBy(0, 350));
    await pause(page, 500);
    await caption(page,
      'Vendor Result: decision (PASS/FAIL/REVIEW), IQA status, MICR routing/account/serial, confidence score, risk score, amount match.',
      5000);
    await clearCaption(page);

    // Rule Evaluations panel
    await page.evaluate(() => window.scrollBy(0, 300));
    await pause(page, 500);
    await highlight(page, 'table');
    await caption(page,
      'Rule Evaluations: 4 business rules — account eligibility, $5K limit, contribution type, duplicate fingerprint. Each logged with pass/fail + details.',
      5000);
    await clearHighlights(page);
    await clearCaption(page);

    // Audit Trail panel
    await page.evaluate(() => window.scrollBy(0, 300));
    await pause(page, 500);
    await highlight(page, 'table');
    await caption(page,
      'Audit Trail: every state transition with timestamp, from/to state, actor, and event details. This is the complete decision trace.',
      5000);
    await clearHighlights(page);
    await clearCaption(page);
    await page.evaluate(() => window.scrollTo(0, 0));
    await pause(page, 500);

    // =======================================================================
    // SECTION 6 — LEDGER: DOUBLE-ENTRY BOOKKEEPING  (~2:45)
    // =======================================================================
    await announce(page, '⑤ Double-Entry Ledger', 'Every deposit creates balanced journal entries — credits and debits sum to zero');
    await clearOverlay(page);

    await page.locator('a.nav-level-tab', { hasText: 'Ledger' }).click();
    await pause(page, 1000);

    await highlight(page, 'table');
    await caption(page,
      'INV-1001 credited $500 (investor gets funds). OMNI-ACME debited $500 (omnibus account). Net across all accounts: $0.',
      5500);
    await clearHighlights(page);
    await clearCaption(page);

    // =======================================================================
    // SECTION 7 — SETTLEMENT: X9.37 ICL  (~3:15)
    // =======================================================================
    await announce(page, '⑥ Settlement — X9.37 ICL Binary', 'Real X9 file format via moov-io/imagecashletter with embedded check images');
    await clearOverlay(page);

    await page.locator('a.nav-level-tab', { hasText: 'Settlement' }).click();
    await pause(page, 1000);

    await caption(page,
      'Generate Batch: collects all FundsPosted deposits for the current business date and writes an X9.37 ICL binary file.',
      4000);
    await highlight(page, '[data-action="generate"]');
    await pause(page, 800);
    await page.locator('[data-action="generate"]').click();
    await clearHighlights(page);

    await page.locator('[data-state]').first().waitFor();
    await highlight(page, 'table');
    await caption(page,
      'Batch created: shows item count, total amount, file path, and status. The ICL file has proper X9 record types (01/10/20/25/26/50/52/70/90/99).',
      5500);
    await clearHighlights(page);
    await clearCaption(page);

    // Acknowledge
    await caption(page,
      'Acknowledge: simulates the settlement bank confirming receipt. All deposits in the batch transition to Completed.',
      4000);
    await highlight(page, '[data-action="ack"]');
    await pause(page, 800);
    await page.locator('[data-action="ack"]').first().click();
    await clearHighlights(page);
    await pause(page, 1500);
    await clearCaption(page);

    // Verify Completed state
    await page.locator('a.nav-level-tab', { hasText: 'Transfers' }).click();
    await page.locator('[data-transfer] a').first().click();
    await highlight(page, '[data-state]');
    await caption(page,
      'Transfer is now Completed. Full lifecycle: Requested → Validating → Analyzing → Approved → FundsPosted → Completed.',
      5000);
    await clearHighlights(page);
    await clearAll(page);

    // =======================================================================
    // SECTION 8 — VENDOR REJECTION SCENARIOS  (~4:15)
    // =======================================================================
    await announce(page, '⑦ Vendor Scenarios — Rejection Paths',
      '7 deterministic scenarios selectable by account suffix, header, or picker');
    await caption(page,
      'The vendor stub is a separate Go binary (port 8081) configured via vendor_scenarios.yaml. Responses are deterministic.',
      4500);
    await clearAll(page);

    // -- IQA Blur --
    await page.goto('/ui/simulate');
    await announce(page, 'IQA Blur → Rejected', 'Image quality failure — too blurry, prompt investor to retake');
    await clearOverlay(page);
    await submitDeposit(page, { accountId: 'INV-1002', amount: '200.00', scenario: 'iqa_blur' });
    await highlight(page, '[data-state]');
    await caption(page,
      'Vendor returns FAIL with IQA_BLUR. Transfer goes Requested → Validating → Rejected. No funds posted, no ledger entry.',
      5000);
    await clearHighlights(page);
    await clearAll(page);

    // -- IQA Glare --
    await page.goto('/ui/simulate');
    await announce(page, 'IQA Glare → Rejected', 'Glare detected on check image');
    await clearOverlay(page);
    await submitDeposit(page, { accountId: 'INV-1003', amount: '300.00', scenario: 'iqa_glare' });
    await highlight(page, '[data-state]');
    await caption(page,
      'Same flow as blur — vendor FAIL, Rejected state. Different rejection code preserved for investor messaging.',
      4000);
    await clearHighlights(page);
    await clearAll(page);

    // -- Duplicate --
    await page.goto('/ui/simulate');
    await announce(page, 'Duplicate Detected → Rejected', 'Vendor detects check was previously deposited');
    await clearOverlay(page);
    await submitDeposit(page, { accountId: 'INV-1004', amount: '150.00', scenario: 'duplicate_detected' });
    await highlight(page, '[data-state]');
    await caption(page,
      'Vendor-level duplicate detection (first layer). System also has internal SHA256 fingerprint detection (second layer).',
      5000);
    await clearHighlights(page);
    await clearCaption(page);

    await caption(page,
      'Three vendor rejection scenarios shown. Each preserves its specific rejection code for investor messaging.',
      3500);
    await clearAll(page);

    // =======================================================================
    // SECTION 9 — BUSINESS RULE: OVER-LIMIT  (~5:15)
    // =======================================================================
    await announce(page, '⑧ Business Rule Enforcement', 'Funding Service applies rules independently of vendor outcome');
    await clearOverlay(page);

    await caption(page,
      'Even with a Clean Pass from the vendor, the Funding Service enforces: $5K limit, account eligibility, contribution type, duplicate fingerprint.',
      5000);
    await clearCaption(page);

    // Show a persistent caption during form fill so there's no dead frame
    await page.goto('/ui/simulate');
    await page.waitForLoadState('domcontentloaded');
    await caption(page, 'Submitting $5,500 deposit with Clean Pass vendor scenario — exceeds the $5,000 per-deposit limit.', 1000);
    // submitDeposit navigates away, so we do the form fill inline
    await page.locator('select[name="investorAccountId"]').selectOption({ value: 'INV-1005' });
    await page.locator('input[name="amount"]').fill('5500.00');
    await page.locator('input[name="frontImage"]').setInputFiles(CHECK_FRONT);
    await page.locator('input[name="backImage"]').setInputFiles(CHECK_BACK);
    await page.locator('select[name="vendorScenario"]').selectOption('clean_pass');
    await pause(page, 500);
    await page.locator('button[type="submit"]').click();
    await page.locator('[data-transfer-id]').waitFor();
    await clearCaption(page);
    await highlight(page, '[data-state]');
    await announce(page, '$5,500 → Rejected by MAX_DEPOSIT_LIMIT',
      'Vendor passed this check, but the $5,000 per-deposit business rule blocks it');
    await caption(page,
      'The rule evaluation is persisted: ACCOUNT_ELIGIBLE=PASS, MAX_DEPOSIT_LIMIT=FAIL. Visible in the transfer detail.',
      4500);
    await clearHighlights(page);
    await clearAll(page);

    // =======================================================================
    // SECTION 10 — OPERATOR REVIEW: APPROVE  (~6:00)
    // =======================================================================
    await announce(page, '⑨ Operator Review — Approve Flow',
      'MICR failure routes to manual review queue for human decision');
    await clearOverlay(page);

    await caption(page,
      'When the vendor returns REVIEW, the transfer stays in Analyzing with review_required=true. It appears in the operator queue.',
      5000);
    await clearCaption(page);

    const micrTransferId = await submitDeposit(page, {
      accountId: 'INV-1001', amount: '800.00', scenario: 'micr_failure',
    });
    await highlight(page, '[data-state]');
    await caption(page,
      'State: Analyzing. The deposit is waiting for an operator to review the MICR data and decide.',
      3500);
    await clearHighlights(page);
    await clearCaption(page);

    // Navigate to review queue
    await page.locator('a.nav-level-tab', { hasText: 'Review Queue' }).click();
    await page.locator('[data-review-item]').first().waitFor();
    await highlight(page, '[data-review-item]');
    await caption(page,
      'Review Queue: shows flagged deposits with amount, account, creation time, and a Review button.',
      4000);
    await clearHighlights(page);
    await clearCaption(page);

    // Click review
    await page.locator('[data-review-item] a.btn', { hasText: 'Review' }).first().click();
    await pause(page, 1000);

    // Show the review detail panels
    await caption(page,
      'Review Detail: transfer info, check images (front + back), vendor analysis with MICR data and confidence scores.',
      5000);
    await clearCaption(page);
    await page.evaluate(() => window.scrollBy(0, 300));
    await pause(page, 1500);

    await caption(page,
      'Rule evaluations show which rules passed and which were flagged. Operator can see all context before deciding.',
      4000);
    await clearCaption(page);
    await page.evaluate(() => window.scrollBy(0, 300));
    await pause(page, 1500);

    // Audit trail
    await caption(page,
      'Audit trail shows the deposit\'s journey so far. After operator action, their decision is added.',
      3500);
    await clearCaption(page);

    // Approve — scroll to show controls above the overlay area
    await page.evaluate(() => {
      const el = document.querySelector('[data-action="approve"]');
      if (el) el.scrollIntoView({ block: 'center' });
    });
    await pause(page, 500);
    await clearCaption(page);

    await highlight(page, '#approve-notes');
    await caption(page,
      'Approve with notes. The operator action is logged in operator_actions table + audit_events.',
      3500);
    await page.locator('#approve-notes').fill('MICR readable on manual inspection — approved');
    await pause(page, 800);
    await clearHighlights(page);
    await clearCaption(page);

    await highlight(page, '[data-action="approve"]');
    await pause(page, 500);
    await page.locator('[data-action="approve"]').click();
    await clearHighlights(page);

    await pause(page, 1500);
    await announce(page, 'Approved → FundsPosted',
      'Operator approval triggers ledger posting: credit investor, debit omnibus');
    await clearAll(page);

    // =======================================================================
    // SECTION 11 — OPERATOR REVIEW: REJECT  (~7:15)
    // =======================================================================
    await announce(page, '⑩ Operator Review — Reject Flow',
      'Amount mismatch: OCR amount differs from entered amount');
    await clearOverlay(page);

    await caption(page,
      'Amount mismatch: vendor OCR reads a different amount than the investor entered. Flagged for review.',
      4000);
    await clearCaption(page);

    await submitDeposit(page, { accountId: 'INV-1006', amount: '450.00', scenario: 'amount_mismatch' });
    await pause(page, 1000);

    await page.locator('a.nav-level-tab', { hasText: 'Review Queue' }).click();
    await page.locator('[data-review-item]').first().waitFor();
    await page.locator('[data-review-item] a.btn', { hasText: 'Review' }).first().click();
    await pause(page, 1000);

    await page.evaluate(() => {
      const el = document.querySelector('[data-action="reject"]');
      if (el) el.scrollIntoView({ block: 'center' });
    });
    await pause(page, 500);

    await highlight(page, '#reject-notes');
    await caption(page,
      'Rejecting with notes. Transfer goes to Rejected state — no ledger posting, no settlement.',
      3500);
    await page.locator('#reject-notes').fill('Amount discrepancy too large — rejecting');
    await pause(page, 800);
    await clearHighlights(page);
    await clearCaption(page);

    await highlight(page, '[data-action="reject"]');
    await pause(page, 500);
    await page.locator('[data-action="reject"]').click();
    await clearHighlights(page);
    await pause(page, 1500);

    await announce(page, 'Rejected',
      'Transfer moves to Rejected state — investor notified, no funds posted');
    await clearAll(page);

    // =======================================================================
    // SECTION 12 — HIGH-RISK REVIEW  (~8:00)
    // =======================================================================
    await announce(page, '⑪ High-Risk Deposit',
      'IQA passes but low MICR confidence triggers review');
    await clearOverlay(page);

    await caption(page,
      'iqa_pass_review: images are fine but MICR confidence is low and risk score is high. Operator decides.',
      4500);
    await clearCaption(page);

    await submitDeposit(page, { accountId: 'INV-1007', amount: '900.00', scenario: 'iqa_pass_review' });
    await highlight(page, '[data-state]');
    await pause(page, 1500);
    await clearHighlights(page);

    await page.locator('a.nav-level-tab', { hasText: 'Review Queue' }).click();
    await page.locator('[data-review-item]').first().waitFor();
    await pause(page, 1000);
    await page.locator('[data-review-item] a.btn', { hasText: 'Review' }).first().click();
    await pause(page, 1000);
    await caption(page,
      'Review detail shows vendor analysis and risk factors. Operator reviews all context before deciding.',
      3000);
    await clearCaption(page);

    await page.evaluate(() => {
      const el = document.querySelector('[data-action="approve"]');
      if (el) el.scrollIntoView({ block: 'center' });
    });
    await pause(page, 500);

    await highlight(page, '#approve-notes');
    await page.locator('#approve-notes').fill('High-risk but legitimate after manual review');
    await pause(page, 800);
    await clearHighlights(page);

    await highlight(page, '[data-action="approve"]');
    await pause(page, 500);
    await page.locator('[data-action="approve"]').click();
    await clearHighlights(page);
    await pause(page, 1500);
    await caption(page, 'Approved. This deposit is now FundsPosted and eligible for the next settlement batch.', 3500);
    await clearCaption(page);

    // =======================================================================
    // SECTION 13 — TRANSFERS OVERVIEW: ALL STATES  (~8:45)
    // =======================================================================
    await announce(page, '⑫ Transfers Overview', 'All deposits across the full range of outcomes');
    await clearOverlay(page);

    await page.locator('a.nav-level-tab', { hasText: 'Transfers' }).click();
    await page.locator('[data-transfer]').first().waitFor();
    await caption(page,
      'Multiple deposit states visible: Completed, FundsPosted, Analyzing, Rejected. Each row clickable for full detail.',
      4500);
    await clearCaption(page);
    await pause(page, 1500);
    await page.evaluate(() => window.scrollBy(0, 300));
    await pause(page, 2000);
    await page.evaluate(() => window.scrollTo(0, 0));
    await pause(page, 500);

    // =======================================================================
    // SECTION 14 — SETTLEMENT ROUND 2  (~9:15)
    // =======================================================================
    await announce(page, '⑬ Second Settlement Batch',
      'Settling the operator-approved deposits from the review workflows');
    await clearOverlay(page);

    await page.locator('a.nav-level-tab', { hasText: 'Settlement' }).click();
    await pause(page, 1000);

    await caption(page,
      'Generate a second batch: only FundsPosted deposits are included. Rejected deposits are excluded.',
      4000);
    await page.locator('[data-action="generate"]').click();
    await page.locator('[data-action="ack"]').first().waitFor();
    await pause(page, 1500);

    await caption(page,
      'Two batches now visible. Each batch is a separate X9.37 ICL file with its own items and totals.',
      4000);
    await clearCaption(page);

    await page.locator('[data-action="ack"]').first().click();
    await pause(page, 1500);
    await caption(page,
      'Acknowledged. All deposits in this batch are now Completed.',
      3000);
    await clearAll(page);
    await pause(page, 500);

    // =======================================================================
    // SECTION 15 — RETURN / REVERSAL  (~10:00)
    // =======================================================================
    await announce(page, '⑭ Return / Reversal with $30 Fee',
      'Simulating a bounced check after settlement');
    await clearOverlay(page);

    await caption(page,
      'Returns can happen after FundsPosted or Completed. Three atomic operations: reversal journal + fee journal + state transition.',
      5000);
    await clearCaption(page);

    // Get a completed transfer ID
    await page.locator('a.nav-level-tab', { hasText: 'Transfers' }).click();
    await pause(page, 500);
    const completedTransferId = await page.locator('[data-transfer]').first()
      .locator('a').first().getAttribute('href')
      .then((href) => href?.split('/').pop() ?? '');

    await page.locator('a.nav-level-tab', { hasText: 'Returns' }).click();
    await pause(page, 1000);

    await caption(page, 'Enter the transfer ID and select a reason code (NSF, ACCOUNT_CLOSED, STOP_PAYMENT, or FRAUD).', 3500);
    await highlight(page, 'input[name="transferId"]');
    await page.locator('input[name="transferId"]').fill(completedTransferId);
    await pause(page, 800);
    await clearHighlights(page);

    await highlight(page, 'select[name="reasonCode"]');
    await page.locator('select[name="reasonCode"]').selectOption('NSF');
    await pause(page, 800);
    await clearHighlights(page);

    await highlight(page, 'button[type="submit"]');
    await pause(page, 500);
    await page.locator('button[type="submit"]').click();
    await clearHighlights(page);

    await pause(page, 1500);
    await highlight(page, '[data-state]');
    await announce(page, 'Return Processed',
      'State: Returned • Reversal posted • $30 return fee charged');
    await caption(page,
      'Reversal journal: debit investor (undo credit), credit omnibus (undo debit). Fee journal: debit investor $30, credit fee revenue $30.',
      5500);
    await clearHighlights(page);
    await clearAll(page);

    // =======================================================================
    // SECTION 16 — LEDGER AFTER RETURN  (~11:00)
    // =======================================================================
    await announce(page, '⑮ Final Ledger State',
      'All account balances after deposits, settlement, and return');
    await clearOverlay(page);

    await page.locator('a.nav-level-tab', { hasText: 'Ledger' }).click();
    await pause(page, 1000);
    await highlight(page, 'table');
    await caption(page,
      'Investor balances reflect deposits minus reversals and fees. Omnibus account mirrors investor movements. Fee revenue account holds $30. All entries sum to zero — double-entry invariant holds.',
      6500);
    await clearHighlights(page);
    await clearAll(page);

    // =======================================================================
    // SECTION 17 — RETURNED TRANSFER DETAIL  (~11:30)
    // =======================================================================
    await announce(page, '⑯ Returned Transfer Detail', 'Full audit trail including return information');
    await clearOverlay(page);

    await page.locator('a.nav-level-tab', { hasText: 'Transfers' }).click();
    await page.locator('[data-transfer] a').first().click();
    await pause(page, 1000);

    await caption(page,
      'Transfer detail now shows Return panel: reason code, return fee ($30), and returned-at timestamp.',
      4500);
    await clearCaption(page);

    // Scroll to vendor result
    await page.evaluate(() => window.scrollBy(0, 350));
    await pause(page, 1000);
    await caption(page, 'Vendor result and rule evaluations preserved for full transparency.', 3000);
    await clearCaption(page);

    // Scroll down to audit trail — use scrollIntoView on the last table (audit trail)
    await page.evaluate(() => {
      const tables = document.querySelectorAll('.data-table');
      const auditTable = tables[tables.length - 1];
      if (auditTable) auditTable.scrollIntoView({ block: 'center' });
    });
    await pause(page, 500);
    // Highlight the last table (audit trail, not rule evaluations)
    await page.evaluate(() => {
      document.querySelectorAll('.tour-highlight').forEach((e) => e.remove());
      const tables = document.querySelectorAll('.data-table');
      const el = tables[tables.length - 1];
      if (!el) return;
      const rect = el.getBoundingClientRect();
      const ring = document.createElement('div');
      ring.className = 'tour-highlight';
      ring.style.cssText = `
        position: fixed; z-index: 99998;
        left: ${rect.left - 4}px; top: ${rect.top - 4}px;
        width: ${rect.width + 8}px; height: ${rect.height + 8}px;
        border: 2px solid #00d9ff;
        border-radius: 6px;
        box-shadow: 0 0 12px rgba(0,217,255,0.5);
        pointer-events: none;
        transition: all 0.3s ease;
      `;
      document.body.appendChild(ring);
    });
    await pause(page, 800);
    await caption(page,
      'Audit trail shows the complete journey: Requested → Validating → Analyzing → Approved → FundsPosted → Completed → Returned.',
      5000);
    await clearHighlights(page);
    await clearCaption(page);
    await pause(page, 1000);
    await page.evaluate(() => window.scrollTo(0, 0));

    // =======================================================================
    // SECTION 18 — SECOND RETURN (FRAUD)  (~12:15)
    // =======================================================================
    await announce(page, '⑰ Second Return — FRAUD Reason',
      'Demonstrating different reason codes');
    await clearOverlay(page);

    // Submit a new deposit, settle it, then return with FRAUD
    const fraudTransferId = await submitDeposit(page, {
      accountId: 'INV-1002', amount: '275.00', scenario: 'clean_pass',
    });
    await pause(page, 500);

    await page.locator('a.nav-level-tab', { hasText: 'Settlement' }).click();
    await page.locator('[data-action="generate"]').click();
    await page.locator('[data-action="ack"]').first().waitFor();
    await page.locator('[data-action="ack"]').first().click();
    await pause(page, 1000);

    await page.locator('a.nav-level-tab', { hasText: 'Returns' }).click();
    await page.waitForLoadState('domcontentloaded');
    // Fill form FIRST so dropdown shows FRAUD, then show caption
    await page.locator('input[name="transferId"]').fill(fraudTransferId);
    await page.locator('select[name="reasonCode"]').selectOption('FRAUD');
    await pause(page, 300);
    await highlight(page, 'select[name="reasonCode"]');
    await caption(page, 'Processing a FRAUD return on a different completed deposit.', 3000);
    await clearHighlights(page);
    await clearCaption(page);
    await page.locator('button[type="submit"]').click();
    await pause(page, 1000);

    // Navigate to the returned transfer to show the FRAUD reason cleanly
    await page.goto(`/ui/transfers/${fraudTransferId}`);
    await page.locator('[data-state]').waitFor();
    await pause(page, 500);

    // Scroll down to show the Return panel with FRAUD reason code
    await page.evaluate(() => {
      const headers = document.querySelectorAll('.panel-header-title');
      for (const h of headers) {
        if (h.textContent?.trim() === 'Return') {
          h.closest('.panel')?.scrollIntoView({ block: 'center' });
          break;
        }
      }
    });
    await pause(page, 800);

    await highlight(page, '[data-state]');
    await caption(page,
      'FRAUD return processed. Same mechanics: reversal + $30 fee. Reason code preserved for reporting.',
      4500);
    await clearHighlights(page);
    await clearAll(page);

    // =======================================================================
    // SECTION 19 — FINAL SUMMARY  (~13:00)
    // =======================================================================
    await announce(page, '⑱ Final Summary', 'All deposit states represented');
    await clearOverlay(page);

    await page.locator('a.nav-level-tab', { hasText: 'Transfers' }).click();
    await page.locator('[data-transfer]').first().waitFor();
    await caption(page,
      'Completed (settled), FundsPosted (awaiting settlement), Rejected (vendor/rule failure), Returned (bounced check).',
      5000);
    await clearCaption(page);

    await pause(page, 1000);
    await page.evaluate(() => window.scrollBy(0, 300));
    await pause(page, 1500);
    await page.evaluate(() => window.scrollTo(0, 0));
    await pause(page, 500);

    // =======================================================================
    // SECTION 20 — DESIGN DECISIONS  (~14:00)
    // =======================================================================
    // Show the ledger as a meaningful background for design decisions
    await page.locator('a.nav-level-tab', { hasText: 'Ledger' }).click();
    await page.waitForLoadState('domcontentloaded');
    await pause(page, 500);

    await announce(page, '⑲ Key Design Decisions', 'Documented in docs/decision_log.md');
    await caption(page,
      'Go (single binary, fast compile) • SQLite (zero-ops) • HTMX (no JS build) • Separate vendor stub (mirrors production architecture)',
      4500);
    await clearCaption(page);

    await caption(page,
      'Real X9.37 ICL settlement via moov-io/imagecashletter • Centralized state machine validator • SHA256 duplicate fingerprinting',
      4500);
    await clearCaption(page);

    await caption(page,
      'Testing: 14 Go tests (unit + integration) + 14 Playwright E2E specs. Settlement tests parse ICL files back and verify structure.',
      4500);
    await clearAll(page);

    // =======================================================================
    // SECTION 21 — END CARD  (~15:00)
    // =======================================================================
    // Create a clean end card using a full-screen overlay instead of overlaying the app
    await page.evaluate(() => {
      const endCard = document.createElement('div');
      endCard.id = 'tour-endcard';
      endCard.style.cssText = `
        position: fixed; top: 0; left: 0; right: 0; bottom: 0; z-index: 100000;
        background: linear-gradient(135deg, #001428 0%, #002040 40%, #001830 100%);
        display: flex; flex-direction: column; align-items: center; justify-content: center;
        font-family: 'Share Tech Mono', 'Courier New', monospace;
        color: #00d9ff;
        text-align: center;
        animation: fadeIn 0.5s ease;
      `;
      endCard.innerHTML = `
        <div style="font-size:28px;font-weight:bold;letter-spacing:3px;text-transform:uppercase;margin-bottom:16px;text-shadow:0 0 20px rgba(0,217,255,0.5);">
          APEX Mobile Check Deposit
        </div>
        <div style="width:200px;height:3px;background:linear-gradient(90deg,transparent,#00d9ff,transparent);margin-bottom:20px;"></div>
        <div style="font-size:16px;color:#88ccee;letter-spacing:1px;margin-bottom:24px;">
          Full Lifecycle Demonstrated
        </div>
        <div style="font-size:13px;color:#6699bb;line-height:2;letter-spacing:0.5px;">
          <div>Happy path • 7 vendor scenarios • Business rule enforcement</div>
          <div>Operator review (approve + reject) • X9.37 ICL settlement</div>
          <div>Return/reversal with fees • Double-entry ledger</div>
        </div>
        <div style="margin-top:32px;width:200px;height:3px;background:linear-gradient(90deg,transparent,#00d9ff,transparent);"></div>
        <div style="margin-top:16px;font-size:14px;color:#00d9ff;letter-spacing:2px;">
          Thank you for reviewing
        </div>
      `;
      document.body.appendChild(endCard);
    });
    await pause(page, 8000);
  });
});
