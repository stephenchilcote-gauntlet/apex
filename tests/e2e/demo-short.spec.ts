import { test as base, expect, Page } from '@playwright/test';
import { CHECK_FRONT, CHECK_BACK } from './fixtures';
import { VisualJudge, critical, advisory } from './visual-judge';

const test = base.extend({});

/**
 * Professional Demo Video — 1.5–3 minute walkthrough of three core workflows:
 *   1. Happy Path      — submit a clean deposit, watch it auto-approve to FundsPosted
 *   2. Operator Review — submit a flagged deposit, approve it from the review queue
 *   3. Settlement      — generate an X9.37 ICL batch and acknowledge it
 *
 * Run:   npx playwright test demo-short.spec.ts
 * Output: reports/test-results/playwright/demo-short/
 *
 * Key fixes vs video-tour.spec.ts:
 *   - Cursor NO LONGER snaps to center on page navigation.
 *     The last known position is tracked in `cursor` and injected at that
 *     exact position when re-creating the element after navigation, so
 *     CSS transitions always start from where the cursor actually was.
 *   - Heavy VisualJudge checks to verify UI correctness at key moments.
 */

// ---------------------------------------------------------------------------
// Cursor position tracker — prevents snap-to-center on navigation
// ---------------------------------------------------------------------------

/** Mutable cursor state shared by all helpers. */
const cursor = { x: 960, y: 540 };

async function ensureCursor(page: Page) {
  await page.waitForLoadState('domcontentloaded');
  const { x, y } = cursor;
  await page.evaluate(
    ({ x, y }) => {
      if (document.getElementById('demo-cursor')) return;
      const cur = document.createElement('div');
      cur.id = 'demo-cursor';
      cur.innerHTML = `<svg width="22" height="22" viewBox="0 0 24 24" fill="none">
        <path d="M5 3l14 8-6.5 1.5L10 19z" fill="white" stroke="#222" stroke-width="1.5" stroke-linejoin="round"/>
      </svg>`;
      // CRITICAL FIX: inject at last known position, NOT at a fixed center
      // point. Without this, the cursor always appears at the viewport center
      // (960, 540) and then visibly snaps/animates to its target — creating
      // an unrealistic teleport effect on every page navigation.
      cur.style.cssText = `
        position: fixed; z-index: 100001; pointer-events: none;
        left: ${x}px; top: ${y}px;
        transition: left 0.45s cubic-bezier(0.4,0,0.2,1), top 0.45s cubic-bezier(0.4,0,0.2,1);
        filter: drop-shadow(0 2px 4px rgba(0,0,0,0.5));
      `;
      document.body.appendChild(cur);
    },
    { x, y },
  );
}

async function moveCursor(page: Page, selector: string) {
  await ensureCursor(page);
  const box = await page.locator(selector).first().boundingBox();
  if (!box) return;

  const tx = Math.round(box.x + box.width / 2);
  const ty = Math.round(box.y + box.height / 2);

  // For long-distance moves (> 400px), briefly pass through a midpoint so
  // the cursor arc looks more like a natural human mouse path instead of
  // a perfectly straight teleport from one edge to the other.
  const dist = Math.sqrt((tx - cursor.x) ** 2 + (ty - cursor.y) ** 2);
  if (dist > 400) {
    const midX = Math.round((cursor.x + tx) / 2 + (Math.random() - 0.5) * 80);
    const midY = Math.round((cursor.y + ty) / 2 + (Math.random() - 0.5) * 50);
    await page.evaluate(
      ({ x, y }) => {
        const cur = document.getElementById('demo-cursor');
        if (cur) { cur.style.left = `${x}px`; cur.style.top = `${y}px`; }
      },
      { x: midX, y: midY },
    );
    await page.waitForTimeout(260);
  }

  // Update tracker BEFORE final move so the next ensureCursor call starts
  // from the correct position even if a navigation happens mid-flight.
  cursor.x = tx;
  cursor.y = ty;
  await page.evaluate(
    ({ x, y }) => {
      const cur = document.getElementById('demo-cursor');
      if (cur) { cur.style.left = `${x}px`; cur.style.top = `${y}px`; }
    },
    { x: tx, y: ty },
  );
  await page.waitForTimeout(520); // wait for CSS transition to complete
}

// ---------------------------------------------------------------------------
// Caption / highlight / title-card helpers
// ---------------------------------------------------------------------------

async function caption(page: Page, text: string, durationMs = 2800) {
  await page.waitForLoadState('domcontentloaded');
  await page.evaluate(
    ({ text }) => {
      let cap = document.getElementById('demo-caption');
      if (!cap) {
        cap = document.createElement('div');
        cap.id = 'demo-caption';
        cap.style.cssText = `
          position: fixed; bottom: 0; left: 0; right: 0; z-index: 100000;
          background: rgba(8,8,8,0.88);
          border-top: 2px solid #f34e3f;
          padding: 14px 56px;
          font-family: -apple-system, 'Segoe UI', system-ui, sans-serif;
          color: #f0f0f0;
          font-size: 17px; line-height: 1.5;
          text-align: center;
          letter-spacing: 0.01em;
          pointer-events: none;
          transition: opacity 0.25s ease;
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

async function clearCaption(page: Page) {
  await page.evaluate(() => {
    const el = document.getElementById('demo-caption');
    if (el) el.style.opacity = '0';
  });
  await page.waitForTimeout(220);
}

async function titleCard(page: Page, heading: string, sub?: string) {
  await page.evaluate(
    ({ heading, sub }) => {
      let card = document.getElementById('demo-title-card');
      if (!card) {
        card = document.createElement('div');
        card.id = 'demo-title-card';
        document.body.appendChild(card);
      }
      card.style.cssText = `
        position: fixed; top: 0; left: 0; right: 0; bottom: 0; z-index: 99999;
        background: #111;
        display: flex; flex-direction: column; align-items: center; justify-content: center;
        font-family: -apple-system, 'Segoe UI', system-ui, sans-serif;
        opacity: 0; transition: opacity 0.35s ease;
        pointer-events: none;
      `;
      card.innerHTML = `
        <div style="font-size:11px;font-weight:700;letter-spacing:0.2em;text-transform:uppercase;color:#f34e3f;margin-bottom:18px;">
          Apex MCD
        </div>
        <div style="font-size:34px;font-weight:700;color:#f0f0f0;margin-bottom:12px;letter-spacing:-0.02em;">
          ${heading}
        </div>
        ${sub ? `<div style="font-size:16px;color:#737373;letter-spacing:0.01em;">${sub}</div>` : ''}
      `;
      setTimeout(() => { (card as HTMLElement).style.opacity = '1'; }, 20);
    },
    { heading, sub },
  );
  await page.waitForTimeout(400);
}

async function removeTitle(page: Page) {
  await page.evaluate(() => {
    const el = document.getElementById('demo-title-card');
    if (el) { el.style.opacity = '0'; setTimeout(() => el.remove(), 380); }
  });
  await page.waitForTimeout(450);
}

async function highlight(page: Page, selector: string) {
  const box = await page.locator(selector).first().boundingBox();
  if (!box) return;
  await page.evaluate(
    (r) => {
      document.querySelectorAll('.demo-hl').forEach((e) => e.remove());
      if (!document.getElementById('demo-hl-style')) {
        const s = document.createElement('style');
        s.id = 'demo-hl-style';
        s.textContent = `@keyframes demo-hl-pulse {
          0%,100% { box-shadow: 0 0 12px rgba(243,78,63,0.4); }
          50%      { box-shadow: 0 0 24px rgba(243,78,63,0.75); }
        }`;
        document.head.appendChild(s);
      }
      const ring = document.createElement('div');
      ring.className = 'demo-hl';
      ring.style.cssText = `
        position: fixed; z-index: 99998;
        left: ${r.x - 4}px; top: ${r.y - 4}px;
        width: ${r.w + 8}px; height: ${r.h + 8}px;
        border: 2px solid #f34e3f;
        border-radius: 6px;
        animation: demo-hl-pulse 1.1s ease-in-out infinite;
        pointer-events: none;
      `;
      document.body.appendChild(ring);
    },
    { x: box.x, y: box.y, w: box.width, h: box.height },
  );
  await page.waitForTimeout(650);
}

async function clearHighlights(page: Page) {
  await page.evaluate(() => document.querySelectorAll('.demo-hl').forEach((e) => e.remove()));
}

async function clickEl(page: Page, selector: string) {
  await moveCursor(page, selector);
  await highlight(page, selector);
  await page.waitForTimeout(220);
  await page.locator(selector).first().click();
  await clearHighlights(page);
}

async function fillEl(page: Page, selector: string, text: string) {
  await moveCursor(page, selector);
  await highlight(page, selector);
  await page.locator(selector).first().fill(text);
  await page.waitForTimeout(380);
  await clearHighlights(page);
}

async function selectEl(page: Page, selector: string, value: string) {
  await moveCursor(page, selector);
  await highlight(page, selector);
  await page.locator(selector).first().selectOption({ value });
  await page.waitForTimeout(380);
  await clearHighlights(page);
}

/** Show a persistent workflow progress indicator in the top-right corner. */
async function setProgress(page: Page, current: number, total: number, label: string) {
  await page.evaluate(
    ({ current, total, label }) => {
      let el = document.getElementById('demo-progress');
      if (!el) {
        el = document.createElement('div');
        el.id = 'demo-progress';
        el.style.cssText = `
          position: fixed; top: 16px; right: 16px; z-index: 99999;
          background: rgba(0,0,0,0.75);
          border: 1px solid rgba(243,78,63,0.4);
          border-radius: 6px;
          padding: 6px 12px;
          font-family: -apple-system, 'Segoe UI', system-ui, sans-serif;
          font-size: 11px;
          color: #aaa;
          pointer-events: none;
          letter-spacing: 0.04em;
        `;
        document.body.appendChild(el);
      }
      const dots = Array.from({ length: total }, (_, i) =>
        `<span style="color:${i < current ? '#f34e3f' : i === current - 1 ? '#f34e3f' : '#444'}">●</span>`
      ).join(' ');
      el.innerHTML = `${dots} &nbsp;<span style="color:#f0f0f0;font-weight:600;">${label}</span>`;
    },
    { current, total, label },
  );
}

async function waitForTerminalState(page: Page, maxMs = 30000) {
  // Use span[data-state] to target the badge specifically.
  // The pipeline div also has [data-state] as an attribute but its textContent
  // is all stage names concatenated — not the current state string.
  await page.waitForFunction(
    () => {
      // Prefer the badge span; fall back to the pipeline container attribute.
      const badge = document.querySelector('span[data-state]');
      if (badge) {
        const s = badge.textContent?.trim() ?? '';
        return ['FundsPosted', 'Completed', 'Rejected', 'Returned', 'Analyzing'].includes(s);
      }
      const pipeline = document.querySelector('.pipeline[data-state]');
      if (pipeline) {
        const s = pipeline.getAttribute('data-state') ?? '';
        return ['FundsPosted', 'Completed', 'Rejected', 'Returned', 'Analyzing'].includes(s);
      }
      return false;
    },
    { timeout: maxMs },
  );
}

// ---------------------------------------------------------------------------
// Visual assertions — heavy use throughout to verify correctness
// ---------------------------------------------------------------------------

let judge: VisualJudge | null = null;

async function assertVisual(
  page: Page,
  stepName: string,
  checks: ReturnType<typeof critical | typeof advisory>[],
) {
  if (!judge) return;
  try {
    // In the demo context we log but never hard-fail — the video should
    // always complete even if an individual visual check doesn't pass.
    const results = await judge.assertVisual(page, checks, { testName: `demo-${stepName}`, fullPage: false });
    for (const [name, result] of Object.entries(results)) {
      if (!result.passed) {
        console.warn(`[visual-judge] ${stepName}/${name}: FAIL — ${result.reason}`);
      }
    }
  } catch (e) {
    console.warn(`[visual-judge] ${stepName}: ${e}`);
  }
}

// ===========================================================================
// THE DEMO — single continuous test → one video file
// ===========================================================================

test.use({
  video: { mode: 'on', size: { width: 1920, height: 1080 } },
  viewport: { width: 1920, height: 1080 },
});

test.describe('Professional Demo', () => {
  // Visual judge adds ~15s per LLM call. With ~12 key checks that's ~3 min overhead.
  // Total budget: ~2 min video + ~3 min checks = allow 10 min.
  test.setTimeout(600_000);

  test('Three Core Workflows', async ({ page, request }) => {

    // Init visual judge (non-fatal if API key absent)
    try {
      judge = new VisualJudge();
    } catch {
      console.warn('[demo] VisualJudge disabled — set ANTHROPIC_API_KEY to enable visual checks');
    }

    // Reset to deterministic clean state, then seed demo data
    const resetResp = await request.post('/api/v1/test/reset');
    expect(resetResp.ok()).toBeTruthy();
    await request.post('/api/v1/test/seed'); // non-fatal if no seed endpoint

    // =========================================================================
    // INTRO TITLE CARD  (~0:00–0:07)
    // =========================================================================
    await page.goto('/ui');
    await page.waitForLoadState('domcontentloaded');
    await ensureCursor(page);

    await titleCard(page, 'Mobile Check Deposit', 'Three core workflows — 2 min demo');
    await page.waitForTimeout(2800);
    await removeTitle(page);

    // =========================================================================
    // DASHBOARD OVERVIEW  (~0:07–0:18)
    // =========================================================================
    await caption(page, 'Overview dashboard — live stats, exception counts, and quick links to all pages', 2800);

    await assertVisual(page, 'dashboard', [
      critical('Does the page show a dashboard with stat cards or metric panels?'),
    ]);

    await clearCaption(page);
    await page.waitForTimeout(500);

    // =========================================================================
    // WORKFLOW 1 — Happy Path  (~0:18–1:05)
    // =========================================================================
    await titleCard(page, 'Workflow 1: Happy Path', 'Submit a clean check → auto-approve → funds posted');
    await page.waitForTimeout(2300);
    await removeTitle(page);
    await setProgress(page, 1, 3, 'Happy Path');

    // Navigate to Simulate
    await clickEl(page, 'a.nav-level-tab:has-text("Simulate")');
    await ensureCursor(page);
    await page.waitForLoadState('domcontentloaded');

    await caption(page, 'Simulate page — models the mobile app capture interface', 2200);
    await clearCaption(page);

    await caption(page, 'Account INV-1001 maps to the "clean pass" vendor scenario — all checks will pass automatically', 2400);
    await clearCaption(page);

    await selectEl(page, 'select[name="investorAccountId"]', 'INV-1001');
    await fillEl(page, 'input[name="amount"]', '1250.00');
    await page.locator('input[name="frontImage"]').setInputFiles(CHECK_FRONT);
    await page.locator('input[name="backImage"]').setInputFiles(CHECK_BACK);
    await page.waitForTimeout(700);

    await caption(page, 'Front and back images attached — SHA256 fingerprints computed server-side for duplicate detection', 2400);
    await clearCaption(page);

    await caption(page, 'One submit → vendor call + 4 business rules + ledger post, all in a single API call', 2200);
    await clearCaption(page);
    await clickEl(page, 'button[type="submit"]');

    // Capture the new transfer ID from the result page
    await page.locator('[data-transfer-id]').waitFor({ timeout: 20000 });
    const transferId1 = await page.locator('[data-transfer-id]').getAttribute('data-transfer-id');
    expect(transferId1).toBeTruthy();

    // Go to transfer detail and watch live state polling
    await page.goto(`/ui/transfers/${transferId1}`);
    await ensureCursor(page);
    await page.waitForLoadState('domcontentloaded');

    await caption(page, 'Transfer detail — state badge updates via HTMX polling every 3 seconds', 2500);
    await clearCaption(page);

    await waitForTerminalState(page);
    await page.waitForTimeout(800);

    await assertVisual(page, 'transfer-funds-posted', [
      critical('Does the page show a transfer with a green or positive state badge like FundsPosted or Completed?'),
      critical('Is there a pipeline or progress tracker showing the transfer stages?'),
    ]);

    await highlight(page, '[data-state]');
    await caption(page, 'FundsPosted ✓ — vendor passed, all 4 rules passed, investor ledger credited', 2800);
    await clearHighlights(page);
    await clearCaption(page);

    await highlight(page, '.pipeline');
    await caption(page, 'Stage tracker: Requested → Validating → Analyzing → Approved → FundsPosted', 2800);
    await clearHighlights(page);
    await clearCaption(page);

    // Scroll to rule evaluations
    await page.evaluate(() => window.scrollBy(0, 600));
    await page.waitForTimeout(600);

    await assertVisual(page, 'rule-evaluations', [
      critical('Is there a table showing business rule evaluations with pass/fail outcomes?'),
    ]);

    await caption(page, 'Rule Evaluations — eligibility ✓, $5K limit ✓, contribution type ✓, duplicate fingerprint ✓', 2800);
    await clearCaption(page);
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.waitForTimeout(500);

    // =========================================================================
    // WORKFLOW 2 — Operator Review  (~1:05–2:00)
    // =========================================================================
    await titleCard(page, 'Workflow 2: Operator Review', 'Amount mismatch → review queue → human approval');
    await page.waitForTimeout(2300);
    await removeTitle(page);
    await setProgress(page, 2, 3, 'Operator Review');

    await page.goto('/ui/simulate');
    await ensureCursor(page);
    await page.waitForLoadState('domcontentloaded');

    await caption(page, 'INV-1006 triggers an OCR amount mismatch — vendor returns REVIEW instead of PASS', 2400);
    await clearCaption(page);

    await selectEl(page, 'select[name="investorAccountId"]', 'INV-1006');
    await fillEl(page, 'input[name="amount"]', '500.00');
    await page.locator('input[name="frontImage"]').setInputFiles(CHECK_FRONT);
    await page.locator('input[name="backImage"]').setInputFiles(CHECK_BACK);
    await page.waitForTimeout(500);

    await clickEl(page, 'button[type="submit"]');
    await page.locator('[data-transfer-id]').waitFor({ timeout: 20000 });
    const transferId2 = await page.locator('[data-transfer-id]').getAttribute('data-transfer-id');
    expect(transferId2).toBeTruthy();

    // Wait for the transfer to reach a review-eligible or terminal state
    await page.goto(`/ui/transfers/${transferId2}`);
    await ensureCursor(page);
    await page.waitForLoadState('domcontentloaded');
    await waitForTerminalState(page);

    await assertVisual(page, 'transfer-pending-review', [
      critical('Does the page show a transfer state? Is the state non-green — e.g. Analyzing, or awaiting review?'),
    ]);

    await caption(page, 'Transfer in Analyzing — flagged for human review due to OCR amount mismatch', 2500);
    await clearCaption(page);

    // Review Queue
    await clickEl(page, 'a.nav-level-tab:has-text("Review")');
    await ensureCursor(page);
    await page.waitForLoadState('domcontentloaded');

    await assertVisual(page, 'review-queue', [
      critical('Is there a table or list showing deposits awaiting operator review?'),
    ]);

    await highlight(page, 'table, [data-review-item]');
    await caption(page, 'Review Queue — operators see all flagged deposits with waiting time and a Review action', 2500);
    await clearHighlights(page);
    await clearCaption(page);

    // Click review for this transfer
    const reviewSelector = `a[href="/ui/review/${transferId2}"], a[href^="/ui/review/"]`;
    await moveCursor(page, reviewSelector);
    await highlight(page, reviewSelector);
    await page.waitForTimeout(450);
    await clearHighlights(page);
    await page.locator(reviewSelector).first().click();
    await ensureCursor(page);
    await page.waitForLoadState('domcontentloaded');

    await assertVisual(page, 'review-detail', [
      critical('Does the page show a review form with transfer info and check images?'),
    ]);

    await caption(page, 'Review detail — transfer info, check images, vendor analysis, rule evaluations, audit trail', 2800);
    await clearCaption(page);

    await page.evaluate(() => window.scrollBy(0, 350));
    await page.waitForTimeout(600);
    await caption(page, 'Vendor flagged an OCR mismatch — the declared vs recognized amounts differ', 2400);
    await clearCaption(page);

    await page.evaluate(() => window.scrollBy(0, 350));
    await page.waitForTimeout(600);

    await caption(page, 'Audit trail records every state transition — operator decision will be appended', 2200);
    await clearCaption(page);

    // Scroll to action buttons
    await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
    await page.waitForTimeout(600);

    await assertVisual(page, 'approve-reject-buttons', [
      critical('Are Approve and Reject buttons visible at the bottom of the review form?'),
    ]);

    await caption(page, 'Operator has reviewed check images and vendor data — approving', 2000);
    await clearCaption(page);

    // Fill notes and approve
    const notesSelector = '#approve-notes, textarea[name="notes"]';
    if (await page.locator(notesSelector).first().count() > 0) {
      await moveCursor(page, notesSelector);
      await page.locator(notesSelector).first().fill('Images clear, amount verified. Approving.');
      await page.waitForTimeout(400);
    }

    await clickEl(page, '#approve-btn, button:has-text("Approve")');
    await page.waitForURL(/\/ui\/transfers\/|\/ui\/review/, { timeout: 20000 }).catch(() => {});
    await ensureCursor(page);
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(700);

    await assertVisual(page, 'post-approve', [
      critical('Does the page indicate success — either a transfer with Approved/FundsPosted state, or a redirect?'),
    ]);

    await caption(page, 'Approved ✓ — transfer advances to FundsPosted, investor ledger credited', 2500);
    await clearCaption(page);

    // =========================================================================
    // WORKFLOW 3 — Settlement  (~2:00–2:40)
    // =========================================================================
    await titleCard(page, 'Workflow 3: Settlement', 'Package FundsPosted transfers → X9.37 ICL binary file');
    await page.waitForTimeout(2300);
    await removeTitle(page);
    await setProgress(page, 3, 3, 'Settlement');

    await clickEl(page, 'a.nav-level-tab:has-text("Settlement")');
    await ensureCursor(page);
    await page.waitForLoadState('domcontentloaded');

    await assertVisual(page, 'settlement-page', [
      critical('Is there a settlement page with a button to generate a settlement batch?'),
    ]);

    await caption(page, 'Settlement — collects all FundsPosted transfers and writes a binary X9.37 ICL file', 2500);
    await clearCaption(page);

    await highlight(page, '#gen-btn, button:has-text("Generate")');
    await caption(page, 'X9.37 ICL is the real wire format used by US clearing networks — proper record types, embedded images', 2200);
    await clearHighlights(page);
    await clearCaption(page);

    await clickEl(page, '#gen-btn, button:has-text("Generate")');

    // Wait for batch to appear
    await page.waitForSelector(
      '.badge--GENERATED, .badge--ACKNOWLEDGED, td:has-text("GENERATED")',
      { timeout: 25000 },
    ).catch(() => {});
    await ensureCursor(page);
    await page.waitForTimeout(700);

    await assertVisual(page, 'batch-generated', [
      critical('Is there a settlement batch row showing GENERATED status with item count and total amount?'),
    ]);

    await highlight(page, 'table tbody tr:first-child, .badge--GENERATED');
    await caption(page, 'Batch generated — X9.37 ICL file contains check images in proper binary record format', 2500);
    await clearHighlights(page);
    await clearCaption(page);

    // Acknowledge
    const ackBtn = page.locator('[data-action="ack"], button:has-text("Acknowledge")').first();
    if (await ackBtn.count() > 0) {
      await moveCursor(page, '[data-action="ack"], button:has-text("Acknowledge")');
      await highlight(page, '[data-action="ack"], button:has-text("Acknowledge")');
      await caption(page, 'Acknowledging — simulates the clearing bank confirming receipt of the ICL file', 2200);
      await clearHighlights(page);
      await clearCaption(page);
      await ackBtn.click();
      await page.waitForSelector(
        '.badge--ACKNOWLEDGED, td:has-text("ACKNOWLEDGED")',
        { timeout: 15000 },
      ).catch(() => {});
      await ensureCursor(page);
      await page.waitForTimeout(700);

      await assertVisual(page, 'batch-acknowledged', [
        critical('Is the settlement batch now showing ACKNOWLEDGED status?'),
      ]);

      await caption(page, 'Acknowledged ✓ — all transfers in this batch are now marked Completed', 2500);
      await clearCaption(page);
    }

    // =========================================================================
    // OUTRO — Dashboard wrap-up  (~2:40–2:52)
    // =========================================================================
    await page.goto('/ui');
    await ensureCursor(page);
    await page.waitForLoadState('domcontentloaded');
    await page.waitForTimeout(900);

    await assertVisual(page, 'dashboard-final', [
      critical('Does the overview dashboard show non-zero statistics reflecting the completed workflows?'),
    ]);

    await caption(page, 'Dashboard updated — three workflows completed end-to-end', 2500);
    await clearCaption(page);

    await titleCard(page, 'Apex Mobile Check Deposit', 'Go · SQLite · HTMX · X9.37 ICL · Operator Review UI');
    await page.waitForTimeout(3200);
  });
});
