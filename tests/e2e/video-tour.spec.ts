import { test as base, expect, Page } from '@playwright/test';
import { CHECK_FRONT, CHECK_BACK } from './fixtures';

/**
 * Video Tour — exercises every workflow in the APEX Mobile Check Deposit system
 * with on-screen annotations so the viewer can follow along.
 *
 * Run:  npx playwright test video-tour.spec.ts
 * Output: tests/e2e/test-results/video-tour/
 */

// Custom fixture: no auto-reset between sections (single continuous test),
// but we do reset once at the start. Video recording enabled.
const test = base.extend({});

// ---------------------------------------------------------------------------
// Annotation helpers — inject an overlay banner + optional highlight ring
// ---------------------------------------------------------------------------

async function announce(page: Page, title: string, subtitle?: string) {
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
          padding: 18px 32px;
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
  // Let the viewer read it
  await page.waitForTimeout(2500);
}

async function clearOverlay(page: Page) {
  await page.evaluate(() => {
    const el = document.getElementById('tour-overlay');
    if (el) el.style.opacity = '0';
  });
  await page.waitForTimeout(300);
}

async function highlight(page: Page, selector: string) {
  await page.evaluate((sel) => {
    // Remove previous highlights
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

// Reusable submit helper (mirrors fixtures.ts but inline, no import dependency)
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
// THE TOUR — single continuous test so it records one video
// ===========================================================================

test.use({
  video: { mode: 'on', size: { width: 1280, height: 900 } },
  viewport: { width: 1280, height: 900 },
});

test.describe('Video Tour', () => {
  test.setTimeout(300_000); // 5 minutes — this is a long narrated walkthrough

  test('Full Application Walkthrough', async ({ page, request }) => {
    // -----------------------------------------------------------------------
    // 0. Reset state
    // -----------------------------------------------------------------------
    const resp = await request.post('/api/v1/test/reset');
    expect(resp.ok()).toBeTruthy();

    // -----------------------------------------------------------------------
    // 1. TITLE CARD
    // -----------------------------------------------------------------------
    await page.goto('/ui/simulate');
    await announce(page, 'APEX Mobile Check Deposit', 'Complete Application Walkthrough');
    await pause(page, 2000);

    // -----------------------------------------------------------------------
    // 2. NAVIGATION TOUR
    // -----------------------------------------------------------------------
    await announce(page, '① Navigation', 'All six tabs in the application');
    await clearOverlay(page);
    await highlight(page, '.nav-level-tabs');
    await pause(page, 1500);
    await clearHighlights(page);

    const tabs = ['Simulate', 'Transfers', 'Review Queue', 'Ledger', 'Settlement', 'Returns'];
    for (const tab of tabs) {
      await page.locator('a.nav-level-tab', { hasText: tab }).click();
      await pause(page, 1200);
    }

    // -----------------------------------------------------------------------
    // 3. EMPTY STATES
    // -----------------------------------------------------------------------
    await announce(page, '② Empty States', 'Pages before any data exists');
    await clearOverlay(page);

    await page.goto('/ui/transfers');
    await pause(page, 1500);
    await page.goto('/ui/review');
    await pause(page, 1500);
    await page.goto('/ui/settlement');
    await pause(page, 1500);

    // -----------------------------------------------------------------------
    // 4. SIMULATE — Happy Path (clean_pass)
    // -----------------------------------------------------------------------
    await announce(page, '③ Deposit Submission — Clean Pass', 'Auto-approved → FundsPosted');
    await clearOverlay(page);

    await page.goto('/ui/simulate');
    await pause(page, 800);

    // Show form filling step by step
    await highlight(page, 'select[name="investorAccountId"]');
    await page.locator('select[name="investorAccountId"]').selectOption({ value: 'INV-1001' });
    await pause(page, 800);
    await clearHighlights(page);

    await highlight(page, 'input[name="amount"]');
    await page.locator('input[name="amount"]').fill('500.00');
    await pause(page, 800);
    await clearHighlights(page);

    await page.locator('input[name="frontImage"]').setInputFiles(CHECK_FRONT);
    await page.locator('input[name="backImage"]').setInputFiles(CHECK_BACK);

    await highlight(page, 'select[name="vendorScenario"]');
    await page.locator('select[name="vendorScenario"]').selectOption('clean_pass');
    await pause(page, 800);
    await clearHighlights(page);

    await highlight(page, 'button[type="submit"]');
    await pause(page, 500);
    await page.locator('button[type="submit"]').click();
    await clearHighlights(page);

    await page.locator('[data-transfer-id]').waitFor();
    await highlight(page, '[data-state]');
    await announce(page, 'Result: FundsPosted', 'Clean pass auto-approves and posts funds immediately');
    await clearOverlay(page);
    await clearHighlights(page);
    const happyTransferId = await page.locator('[data-transfer-id]').getAttribute('data-transfer-id');

    // -----------------------------------------------------------------------
    // 5. TRANSFERS LIST — see the new deposit
    // -----------------------------------------------------------------------
    await announce(page, '④ Transfers List', 'All deposits are tracked here');
    await clearOverlay(page);

    await page.locator('a.nav-level-tab', { hasText: 'Transfers' }).click();
    await page.locator('[data-transfer]').first().waitFor();
    await highlight(page, '[data-transfer]');
    await pause(page, 2000);
    await clearHighlights(page);

    // -----------------------------------------------------------------------
    // 6. TRANSFER DETAIL
    // -----------------------------------------------------------------------
    await announce(page, '⑤ Transfer Detail', 'Full information for a single deposit');
    await clearOverlay(page);

    await page.locator('[data-transfer] a').first().click();
    await page.locator('[data-state]').waitFor();
    await pause(page, 2000);

    // Scroll to show more panels
    await page.evaluate(() => window.scrollBy(0, 400));
    await pause(page, 2000);
    await page.evaluate(() => window.scrollBy(0, 400));
    await pause(page, 2000);
    await page.evaluate(() => window.scrollTo(0, 0));
    await pause(page, 500);

    // -----------------------------------------------------------------------
    // 7. LEDGER — see balance update
    // -----------------------------------------------------------------------
    await announce(page, '⑥ Ledger', 'Account balances reflect the posted deposit');
    await clearOverlay(page);

    await page.locator('a.nav-level-tab', { hasText: 'Ledger' }).click();
    await pause(page, 2000);

    // Highlight the investor account row
    await highlight(page, 'table');
    await pause(page, 2000);
    await clearHighlights(page);

    // -----------------------------------------------------------------------
    // 8. VENDOR SCENARIOS — Rejections
    // -----------------------------------------------------------------------
    await announce(page, '⑦ Vendor Scenarios — Rejections', 'IQA Blur, IQA Glare, Duplicate');

    // -- IQA Blur --
    await announce(page, 'IQA Blur → Rejected', 'Image quality failure (blur)');
    await clearOverlay(page);
    await submitDeposit(page, { accountId: 'INV-1002', amount: '200.00', scenario: 'iqa_blur' });
    await highlight(page, '[data-state]');
    await pause(page, 2000);
    await clearHighlights(page);

    // -- IQA Glare --
    await announce(page, 'IQA Glare → Rejected', 'Image quality failure (glare)');
    await clearOverlay(page);
    await submitDeposit(page, { accountId: 'INV-1003', amount: '300.00', scenario: 'iqa_glare' });
    await highlight(page, '[data-state]');
    await pause(page, 2000);
    await clearHighlights(page);

    // -- Duplicate --
    await announce(page, 'Duplicate Detected → Rejected', 'Vendor detects duplicate check');
    await clearOverlay(page);
    await submitDeposit(page, { accountId: 'INV-1004', amount: '150.00', scenario: 'duplicate_detected' });
    await highlight(page, '[data-state]');
    await pause(page, 2000);
    await clearHighlights(page);

    // -----------------------------------------------------------------------
    // 9. BUSINESS RULE — Over $5,000 rejection
    // -----------------------------------------------------------------------
    await announce(page, '⑧ Business Rule: $5,000 Limit', 'Deposits over $5,000 are automatically rejected');
    await clearOverlay(page);
    await submitDeposit(page, { accountId: 'INV-1005', amount: '5500.00', scenario: 'clean_pass' });
    await highlight(page, '[data-state]');
    await pause(page, 2500);
    await clearHighlights(page);

    // -----------------------------------------------------------------------
    // 10. REVIEW WORKFLOW — MICR Failure → Approve
    // -----------------------------------------------------------------------
    await announce(page, '⑨ Operator Review — Approve', 'MICR failure routes to manual review queue');
    await clearOverlay(page);

    const micrTransferId = await submitDeposit(page, {
      accountId: 'INV-1001', amount: '800.00', scenario: 'micr_failure',
    });
    await highlight(page, '[data-state]');
    await pause(page, 1500);
    await clearHighlights(page);

    // Navigate to review queue
    await announce(page, 'Review Queue', 'Operator sees flagged deposit awaiting review');
    await clearOverlay(page);
    await page.locator('a.nav-level-tab', { hasText: 'Review Queue' }).click();
    await page.locator('[data-review-item]').first().waitFor();
    await highlight(page, '[data-review-item]');
    await pause(page, 2000);
    await clearHighlights(page);

    // Click review
    await page.locator('[data-review-item] a.btn', { hasText: 'Review' }).first().click();
    await pause(page, 1500);

    // Scroll to see all panels
    await page.evaluate(() => window.scrollBy(0, 300));
    await pause(page, 1500);

    // Show approve form
    await announce(page, 'Approving Deposit', 'Operator adds notes and approves');
    await clearOverlay(page);
    await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
    await pause(page, 500);

    await highlight(page, '#approve-notes');
    await page.locator('#approve-notes').fill('MICR readable on manual inspection — approved');
    await pause(page, 1000);
    await clearHighlights(page);

    await highlight(page, '[data-action="approve"]');
    await pause(page, 500);
    await page.locator('[data-action="approve"]').click();
    await clearHighlights(page);
    await pause(page, 2000);

    // -----------------------------------------------------------------------
    // 11. REVIEW WORKFLOW — Amount Mismatch → Reject
    // -----------------------------------------------------------------------
    await announce(page, '⑩ Operator Review — Reject', 'Amount mismatch flagged for manual review');
    await clearOverlay(page);

    await submitDeposit(page, { accountId: 'INV-1006', amount: '450.00', scenario: 'amount_mismatch' });
    await pause(page, 1000);

    await page.locator('a.nav-level-tab', { hasText: 'Review Queue' }).click();
    await page.locator('[data-review-item]').first().waitFor();
    await page.locator('[data-review-item] a.btn', { hasText: 'Review' }).first().click();
    await pause(page, 1500);

    await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
    await pause(page, 500);

    await highlight(page, '#reject-notes');
    await page.locator('#reject-notes').fill('Amount discrepancy too large — rejecting');
    await pause(page, 1000);
    await clearHighlights(page);

    await highlight(page, '[data-action="reject"]');
    await pause(page, 500);
    await page.locator('[data-action="reject"]').click();
    await clearHighlights(page);
    await pause(page, 2000);

    // -----------------------------------------------------------------------
    // 12. REVIEW WORKFLOW — High Risk (iqa_pass_review)
    // -----------------------------------------------------------------------
    await announce(page, '⑪ High-Risk Deposit Review', 'IQA passes but risk score triggers review');
    await clearOverlay(page);

    await submitDeposit(page, { accountId: 'INV-1007', amount: '900.00', scenario: 'iqa_pass_review' });
    await highlight(page, '[data-state]');
    await pause(page, 1500);
    await clearHighlights(page);

    await page.locator('a.nav-level-tab', { hasText: 'Review Queue' }).click();
    await page.locator('[data-review-item]').first().waitFor();
    await pause(page, 1500);

    // Approve this one too so we have more for settlement
    await page.locator('[data-review-item] a.btn', { hasText: 'Review' }).first().click();
    await pause(page, 1000);
    await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
    await page.locator('#approve-notes').fill('High-risk but legitimate after review');
    await page.locator('[data-action="approve"]').click();
    await pause(page, 2000);

    // -----------------------------------------------------------------------
    // 13. TRANSFERS LIST — Multiple deposits now visible
    // -----------------------------------------------------------------------
    await announce(page, '⑫ Transfers List — Multiple Deposits', 'Showing all deposits with their various states');
    await clearOverlay(page);

    await page.locator('a.nav-level-tab', { hasText: 'Transfers' }).click();
    await page.locator('[data-transfer]').first().waitFor();
    await pause(page, 2500);

    // Scroll down if many rows
    await page.evaluate(() => window.scrollBy(0, 300));
    await pause(page, 1500);
    await page.evaluate(() => window.scrollTo(0, 0));
    await pause(page, 500);

    // -----------------------------------------------------------------------
    // 14. SETTLEMENT — Generate and Acknowledge
    // -----------------------------------------------------------------------
    await announce(page, '⑬ Settlement', 'Generate a settlement batch from posted deposits');
    await clearOverlay(page);

    await page.locator('a.nav-level-tab', { hasText: 'Settlement' }).click();
    await pause(page, 1000);

    await highlight(page, '[data-action="generate"]');
    await pause(page, 800);
    await page.locator('[data-action="generate"]').click();
    await clearHighlights(page);

    // Wait for batch to appear
    await page.locator('[data-state]').first().waitFor();
    await highlight(page, 'table');
    await pause(page, 2500);
    await clearHighlights(page);

    // Acknowledge the batch
    await announce(page, 'Acknowledging Settlement Batch', 'Transitions deposits from FundsPosted → Completed');
    await clearOverlay(page);

    await highlight(page, '[data-action="ack"]');
    await pause(page, 800);
    await page.locator('[data-action="ack"]').first().click();
    await clearHighlights(page);
    await pause(page, 2000);

    // Verify a transfer is now Completed
    await page.locator('a.nav-level-tab', { hasText: 'Transfers' }).click();
    await page.locator('[data-transfer] a').first().click();
    await highlight(page, '[data-state]');
    await pause(page, 2000);
    await clearHighlights(page);

    // -----------------------------------------------------------------------
    // 15. RETURNS — Process a return with $30 fee
    // -----------------------------------------------------------------------
    await announce(page, '⑭ Return / Reversal', 'Returning a completed deposit with NSF reason');
    await clearOverlay(page);

    // We need a completed transfer — the first happy-path one should be completed now
    await page.locator('a.nav-level-tab', { hasText: 'Returns' }).click();
    await pause(page, 1000);

    // Get a completed transfer ID from the transfers page
    await page.locator('a.nav-level-tab', { hasText: 'Transfers' }).click();
    await pause(page, 500);

    // Find the first completed transfer ID
    const completedTransferId = await page.locator('[data-transfer]').first()
      .locator('a').first().getAttribute('href')
      .then((href) => href?.split('/').pop() ?? '');

    await page.locator('a.nav-level-tab', { hasText: 'Returns' }).click();
    await pause(page, 1000);

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
    await announce(page, 'Return Processed', 'State: Returned • $30 return fee applied');
    await clearOverlay(page);
    await clearHighlights(page);
    await pause(page, 1500);

    // -----------------------------------------------------------------------
    // 16. LEDGER — Final state with all movements
    // -----------------------------------------------------------------------
    await announce(page, '⑮ Final Ledger State', 'All account balances reflecting deposits, settlement, and return');
    await clearOverlay(page);

    await page.locator('a.nav-level-tab', { hasText: 'Ledger' }).click();
    await pause(page, 1000);
    await highlight(page, 'table');
    await pause(page, 3000);
    await clearHighlights(page);

    // -----------------------------------------------------------------------
    // 17. TRANSFER DETAIL — Returned transfer shows return info
    // -----------------------------------------------------------------------
    await announce(page, '⑯ Returned Transfer Detail', 'Transfer detail shows return info and fee');
    await clearOverlay(page);

    await page.locator('a.nav-level-tab', { hasText: 'Transfers' }).click();
    await page.locator('[data-transfer] a').first().click();
    await pause(page, 1500);
    await page.evaluate(() => window.scrollBy(0, 400));
    await pause(page, 2000);
    await page.evaluate(() => window.scrollBy(0, 400));
    await pause(page, 2000);
    await page.evaluate(() => window.scrollTo(0, 0));

    // -----------------------------------------------------------------------
    // 18. SECOND RETURN — FRAUD reason code
    // -----------------------------------------------------------------------
    await announce(page, '⑰ Return with FRAUD Reason', 'Different reason code demonstration');
    await clearOverlay(page);

    // Need another completed deposit — submit, settle, ack
    // Reset and create a fresh one for cleanliness
    const fraudTransferId = await submitDeposit(page, {
      accountId: 'INV-1002', amount: '275.00', scenario: 'clean_pass',
    });
    await pause(page, 500);

    // Settle it
    await page.locator('a.nav-level-tab', { hasText: 'Settlement' }).click();
    await page.locator('[data-action="generate"]').click();
    await page.locator('[data-state]').first().waitFor();
    await page.locator('[data-action="ack"]').first().click();
    await pause(page, 1000);

    // Return it with FRAUD
    await page.locator('a.nav-level-tab', { hasText: 'Returns' }).click();
    await page.locator('input[name="transferId"]').fill(fraudTransferId);
    await page.locator('select[name="reasonCode"]').selectOption('FRAUD');
    await page.locator('button[type="submit"]').click();
    await pause(page, 1500);

    await highlight(page, '[data-state]');
    await pause(page, 2000);
    await clearHighlights(page);

    // -----------------------------------------------------------------------
    // 19. FINAL SUMMARY — Transfers page showing all states
    // -----------------------------------------------------------------------
    await announce(page, '⑱ Final Overview', 'All transfers showing the full range of outcomes');
    await clearOverlay(page);

    await page.locator('a.nav-level-tab', { hasText: 'Transfers' }).click();
    await page.locator('[data-transfer]').first().waitFor();
    await pause(page, 2000);
    await page.evaluate(() => window.scrollBy(0, 300));
    await pause(page, 2000);
    await page.evaluate(() => window.scrollTo(0, 0));
    await pause(page, 1000);

    // Click through a couple details for final inspection
    const transferRows = page.locator('[data-transfer] a');
    const count = await transferRows.count();
    for (let i = 0; i < Math.min(count, 3); i++) {
      await page.locator('a.nav-level-tab', { hasText: 'Transfers' }).click();
      await page.locator('[data-transfer]').first().waitFor();
      await transferRows.nth(i).click();
      await pause(page, 1500);
      await page.evaluate(() => window.scrollBy(0, 500));
      await pause(page, 1000);
      await page.evaluate(() => window.scrollTo(0, 0));
      await pause(page, 500);
    }

    // -----------------------------------------------------------------------
    // 20. END CARD
    // -----------------------------------------------------------------------
    await page.locator('a.nav-level-tab', { hasText: 'Simulate' }).click();
    await page.waitForLoadState('domcontentloaded');
    await announce(page, 'Tour Complete', 'APEX Mobile Check Deposit — All Workflows Demonstrated');
    await pause(page, 4000);
    await clearOverlay(page);
  });
});
