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
 * Run:   cd tests/e2e && npx playwright test demo-short.spec.ts
 * Output: tests/e2e/test-results/demo-short-.../video.webm
 *
 * Design principles:
 *   - Cursor NEVER snaps to center. Last known position is tracked in the
 *     `cursor` module variable and injected verbatim after each navigation.
 *   - Progress indicator persists across pages (tracked in `progress` and
 *     restored alongside the cursor in afterNav()).
 *   - Fills use character-by-character typing for a human-looking pace.
 *   - Visual checks at every key moment via VisualJudge (non-fatal).
 */

// ============================================================================
// Module-level state — survives across page navigations
// ============================================================================

const cursor = { x: 960, y: 540 };
const progress = { current: 0, total: 3, label: '' };

// ============================================================================
// Post-navigation restore — call after every page.goto() or nav click
// ============================================================================

/**
 * After every full-page navigation, re-inject the cursor at its last known
 * position AND restore the workflow progress indicator.
 * All DOM elements are wiped on navigation so we must recreate them.
 */
async function afterNav(page: Page) {
  await page.waitForLoadState('domcontentloaded');

  await page.evaluate(
    ({ cx, cy, pcurrent, ptotal, plabel }) => {
      // ── cursor ──────────────────────────────────────────────────────
      if (!document.getElementById('demo-cursor')) {
        const cur = document.createElement('div');
        cur.id = 'demo-cursor';
        cur.innerHTML = `<svg width="22" height="22" viewBox="0 0 24 24" fill="none">
          <path d="M5 3l14 8-6.5 1.5L10 19z" fill="white" stroke="#222" stroke-width="1.5" stroke-linejoin="round"/>
        </svg>`;
        // Inject at last known position — NOT at a hardcoded center.
        // This eliminates the "snap to (960,540) then jump to target" glitch.
        cur.style.cssText = `
          position: fixed; z-index: 100001; pointer-events: none;
          left: ${cx}px; top: ${cy}px;
          transition: left 0.45s cubic-bezier(0.4,0,0.2,1), top 0.45s cubic-bezier(0.4,0,0.2,1);
          filter: drop-shadow(0 2px 4px rgba(0,0,0,0.5));
        `;
        document.body.appendChild(cur);
      }

      // ── progress indicator ──────────────────────────────────────────
      if (pcurrent > 0 && !document.getElementById('demo-progress')) {
        const el = document.createElement('div');
        el.id = 'demo-progress';
        el.style.cssText = `
          position: fixed; top: 16px; right: 16px; z-index: 99997;
          background: rgba(0,0,0,0.78);
          border: 1px solid rgba(243,78,63,0.35);
          border-radius: 6px;
          padding: 5px 12px;
          font-family: -apple-system, 'Segoe UI', system-ui, sans-serif;
          font-size: 11px; color: #aaa; letter-spacing: 0.04em;
          pointer-events: none;
        `;
        const dots = Array.from({ length: ptotal }, (_, i) => {
          const done = i < pcurrent;
          const active = i === pcurrent - 1;
          return `<span style="color:${done ? (active ? '#f34e3f' : '#b03030') : '#333'};font-size:9px;">●</span>`;
        }).join(' ');
        el.innerHTML = `${dots} &nbsp;<span style="color:#e8e8e8;font-weight:600;">${plabel}</span>`;
        document.body.appendChild(el);
      }
    },
    { cx: cursor.x, cy: cursor.y, pcurrent: progress.current, ptotal: progress.total, plabel: progress.label },
  );
}

// ============================================================================
// Cursor movement — smooth arcs with midpoint for long distances
// ============================================================================

async function moveCursor(page: Page, selector: string) {
  await afterNav(page); // ensure cursor exists
  const box = await page.locator(selector).first().boundingBox();
  if (!box) return;

  const tx = Math.round(box.x + box.width / 2);
  const ty = Math.round(box.y + box.height / 2);

  // For long moves (> 400px), arc through a jittered midpoint so the
  // cursor path looks like a natural human mouse arc, not a teleport.
  const dist = Math.sqrt((tx - cursor.x) ** 2 + (ty - cursor.y) ** 2);
  if (dist > 400) {
    const jx = (Math.random() - 0.5) * 90;
    const jy = (Math.random() - 0.5) * 55;
    const midX = Math.round((cursor.x + tx) / 2 + jx);
    const midY = Math.round((cursor.y + ty) / 2 + jy);
    await page.evaluate(
      ({ x, y }) => {
        const cur = document.getElementById('demo-cursor');
        if (cur) { cur.style.left = `${x}px`; cur.style.top = `${y}px`; }
      },
      { x: midX, y: midY },
    );
    await page.waitForTimeout(270);
  }

  // Update tracker BEFORE final move so the next afterNav() call starts
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
  await page.waitForTimeout(520);
}

// ============================================================================
// Caption
// ============================================================================

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
          text-align: center; letter-spacing: 0.01em;
          pointer-events: none;
          transition: opacity 0.22s ease;
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

// ============================================================================
// Title card
// ============================================================================

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
        opacity: 0; transition: opacity 0.35s ease; pointer-events: none;
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

// ============================================================================
// Highlight ring
// ============================================================================

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
          0%,100% { box-shadow: 0 0 10px rgba(243,78,63,0.35); }
          50%      { box-shadow: 0 0 22px rgba(243,78,63,0.7); }
        }`;
        document.head.appendChild(s);
      }
      const ring = document.createElement('div');
      ring.className = 'demo-hl';
      ring.style.cssText = `
        position: fixed; z-index: 99998;
        left: ${r.x - 4}px; top: ${r.y - 4}px;
        width: ${r.w + 8}px; height: ${r.h + 8}px;
        border: 2px solid #f34e3f; border-radius: 6px;
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

// ============================================================================
// Interaction helpers
// ============================================================================

async function clickEl(page: Page, selector: string) {
  await moveCursor(page, selector);
  await highlight(page, selector);
  await page.waitForTimeout(200);
  await page.locator(selector).first().click();
  await clearHighlights(page);
}

/** Type text character-by-character for a human-looking pace. */
async function typeEl(page: Page, selector: string, text: string) {
  await moveCursor(page, selector);
  await highlight(page, selector);
  await page.locator(selector).first().fill(''); // clear first
  for (const ch of text) {
    await page.locator(selector).first().pressSequentially(ch, { delay: 55 });
  }
  await page.waitForTimeout(300);
  await clearHighlights(page);
}

async function selectEl(page: Page, selector: string, value: string) {
  await moveCursor(page, selector);
  await highlight(page, selector);
  await page.locator(selector).first().selectOption({ value });
  await page.waitForTimeout(380);
  await clearHighlights(page);
}

// ============================================================================
// Progress indicator — persists via module state + afterNav() restore
// ============================================================================

async function setProgress(page: Page, current: number, label: string) {
  progress.current = current;
  progress.label = label;
  // Remove stale element and let afterNav() re-inject with fresh state
  await page.evaluate(() => { document.getElementById('demo-progress')?.remove(); });
  await afterNav(page);
}

// ============================================================================
// State waiter
// ============================================================================

async function waitForTerminalState(page: Page, maxMs = 30000) {
  // Prefer span[data-state] (the badge), not .pipeline[data-state] (the
  // container whose textContent is all stage names concatenated).
  await page.waitForFunction(
    () => {
      const badge = document.querySelector('span[data-state]');
      if (badge) {
        const s = (badge.textContent ?? '').trim();
        return ['FundsPosted', 'Completed', 'Rejected', 'Returned', 'Analyzing'].includes(s);
      }
      const pipeline = document.querySelector('.pipeline[data-pipeline-state]');
      if (pipeline) {
        const s = pipeline.getAttribute('data-pipeline-state') ?? '';
        return ['FundsPosted', 'Completed', 'Rejected', 'Returned', 'Analyzing'].includes(s);
      }
      return false;
    },
    { timeout: maxMs },
  );
}

// ============================================================================
// Visual assertions — screenshots captured inline, LLM analysis deferred
// to run concurrently after demo completes (keeps video ~2 min not 5 min)
// ============================================================================

let judge: VisualJudge | null = null;

interface DeferredCheck {
  screenshot: Buffer;
  stepName: string;
  checks: ReturnType<typeof critical | typeof advisory>[];
}
const deferredChecks: DeferredCheck[] = [];

/**
 * Capture a screenshot NOW (fast, inline with demo) and queue LLM analysis
 * for later. Call runDeferredChecks() at the end of the test to process all.
 */
async function assertVisual(
  page: Page,
  stepName: string,
  checks: ReturnType<typeof critical | typeof advisory>[],
) {
  if (!judge) return;
  try {
    const screenshot = await page.screenshot({ fullPage: false });
    deferredChecks.push({ screenshot, stepName, checks });
  } catch (e) {
    console.warn(`[visual-judge] ${stepName}: screenshot failed — ${e}`);
  }
}

async function runDeferredChecks() {
  if (!judge || deferredChecks.length === 0) return;
  console.log(`\n  🔍 Running ${deferredChecks.length} deferred visual checks in parallel...`);
  await Promise.all(
    deferredChecks.map(async ({ screenshot, stepName, checks }) => {
      try {
        const results = await judge!.assertVisualFromBuffer(screenshot, checks, {
          testName: `demo-${stepName}`,
          fullPage: false,
        });
        for (const [, result] of Object.entries(results)) {
          if (!result.passed) {
            console.warn(`[visual-judge] ${stepName}: FAIL — ${result.reason}`);
          }
        }
      } catch (e) {
        console.warn(`[visual-judge] ${stepName}: ${e}`);
      }
    }),
  );
}

// ============================================================================
// THE DEMO — single continuous test → one video file
// ============================================================================

test.use({
  video: { mode: 'on', size: { width: 1920, height: 1080 } },
  viewport: { width: 1920, height: 1080 },
});

test.describe('Professional Demo', () => {
  // Visual judge adds ~15s per LLM call. With ~12 key checks that's ~3 min overhead.
  // Total budget: ~2 min video + ~3 min checks = allow 10 min.
  test.setTimeout(600_000);

  test('Three Core Workflows', async ({ page, request }) => {

    try { judge = new VisualJudge(); } catch {
      console.warn('[demo] VisualJudge disabled — set ANTHROPIC_API_KEY to enable');
    }

    // Reset to deterministic clean state, then seed demo data
    const resetResp = await request.post('/api/v1/test/reset');
    expect(resetResp.ok()).toBeTruthy();
    await request.post('/api/v1/test/seed');

    // =========================================================================
    // INTRO TITLE CARD  (~0:00–0:07)
    // =========================================================================
    await page.goto('/ui');
    await afterNav(page);

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

    // Show keyboard shortcuts modal — demonstrates UX polish
    await moveCursor(page, '.kbd-help-btn');
    await highlight(page, '.kbd-help-btn');
    await caption(page, 'Press ? for keyboard shortcuts — navigate with g+key, ? for help, / for command palette', 2200);
    await clearHighlights(page);
    await clearCaption(page);
    await page.locator('.kbd-help-btn').first().click();
    await page.waitForSelector('#kbd-modal[open]', { timeout: 3000 }).catch(() => {});
    await page.waitForTimeout(1800);
    await page.keyboard.press('Escape');
    await page.waitForTimeout(500);

    // =========================================================================
    // WORKFLOW 1 — Happy Path  (~0:18–1:05)
    // =========================================================================
    await titleCard(page, 'Workflow 1: Happy Path', 'Submit a clean check → auto-approve → funds posted');
    await page.waitForTimeout(2300);
    await removeTitle(page);
    await setProgress(page, 1, 'Happy Path');

    // Navigate to Simulate
    await clickEl(page, 'a.nav-level-tab:has-text("Simulate")');
    await afterNav(page);

    await caption(page, 'Simulate page — models the mobile app capture interface', 2200);
    await clearCaption(page);

    await caption(page, 'INV-1001 is the "clean pass" account — all vendor checks pass automatically', 2400);
    await clearCaption(page);

    await selectEl(page, 'select[name="investorAccountId"]', 'INV-1001');
    await typeEl(page, 'input[name="amount"]', '1250.00');
    await page.locator('input[name="frontImage"]').setInputFiles(CHECK_FRONT);
    await page.locator('input[name="backImage"]').setInputFiles(CHECK_BACK);
    // Give FileReader time to render previews
    await page.waitForSelector('#frontPreview[src]', { timeout: 5000 }).catch(() => {});
    await page.waitForTimeout(500);

    await moveCursor(page, '#frontPreview');
    await highlight(page, '#frontPreview');
    await caption(page, 'Check images attached — thumbnails shown before upload, SHA256 fingerprints for dedup', 2600);
    await clearHighlights(page);
    await clearCaption(page);

    await caption(page, 'One submit → vendor analysis + 5 business rules + ledger post — all in one API call', 2200);
    await clearCaption(page);
    await clickEl(page, 'button[type="submit"]');

    await page.locator('[data-transfer-id]').waitFor({ timeout: 20000 });
    const transferId1 = await page.locator('[data-transfer-id]').getAttribute('data-transfer-id');
    expect(transferId1).toBeTruthy();

    await page.goto(`/ui/transfers/${transferId1}`);
    await afterNav(page);

    await caption(page, 'Transfer detail — state badge live-polls via HTMX every 3 seconds', 2500);
    await clearCaption(page);

    await waitForTerminalState(page);
    await page.waitForTimeout(800);

    await assertVisual(page, 'transfer-funds-posted', [
      critical('Does the page show a transfer with a green or positive state badge like FundsPosted or Completed?'),
      critical('Is there a pipeline or progress tracker showing the transfer stages?'),
    ]);

    // Highlight the state badge specifically (span[data-state], not the pipeline)
    await highlight(page, 'span[data-state]');
    await caption(page, 'FundsPosted ✓ — vendor passed, all 4 rules passed, investor ledger credited', 2800);
    await clearHighlights(page);
    await clearCaption(page);

    await highlight(page, '.pipeline');
    await caption(page, 'Stage pipeline: Requested → Validating → Analyzing → Approved → FundsPosted', 2600);
    await clearHighlights(page);
    await clearCaption(page);

    // Show the Process Return button — demonstrates return flow awareness
    const returnBtn = page.locator('a:has-text("Process Return")').first();
    if (await returnBtn.count() > 0) {
      await moveCursor(page, 'a:has-text("Process Return")');
      await highlight(page, 'a:has-text("Process Return")');
      await caption(page, 'Process Return → available for FundsPosted transfers — R codes, fee calculation, reversal posting', 2400);
      await clearHighlights(page);
      await clearCaption(page);
    }

    // Scroll to rule evaluations
    await page.evaluate(() => window.scrollBy(0, 600));
    await page.waitForTimeout(600);

    await assertVisual(page, 'rule-evaluations', [
      critical('Is there a table showing business rule evaluations with pass/fail outcomes?'),
    ]);

    await caption(page, 'Rule Evaluations — eligibility ✓  $5K/deposit ✓  $10K/day ✓  contribution type ✓  duplicate check ✓', 2800);
    await clearCaption(page);
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.waitForTimeout(500);

    // =========================================================================
    // WORKFLOW 2 — Operator Review  (~1:05–2:00)
    // =========================================================================
    await titleCard(page, 'Workflow 2: Operator Review', 'Amount mismatch → review queue → human approval');
    await page.waitForTimeout(2300);
    await removeTitle(page);
    await setProgress(page, 2, 'Operator Review');

    await page.goto('/ui/simulate');
    await afterNav(page);

    await caption(page, 'INV-1006 triggers an OCR amount mismatch — vendor returns REVIEW instead of PASS', 2400);
    await clearCaption(page);

    await selectEl(page, 'select[name="investorAccountId"]', 'INV-1006');
    await typeEl(page, 'input[name="amount"]', '500.00');
    await page.locator('input[name="frontImage"]').setInputFiles(CHECK_FRONT);
    await page.locator('input[name="backImage"]').setInputFiles(CHECK_BACK);
    await page.waitForSelector('#frontPreview[src]', { timeout: 5000 }).catch(() => {});
    await page.waitForTimeout(500);

    await clickEl(page, 'button[type="submit"]');
    await page.locator('[data-transfer-id]').waitFor({ timeout: 20000 });
    const transferId2 = await page.locator('[data-transfer-id]').getAttribute('data-transfer-id');
    expect(transferId2).toBeTruthy();

    await page.goto(`/ui/transfers/${transferId2}`);
    await afterNav(page);
    await waitForTerminalState(page);

    await assertVisual(page, 'transfer-pending-review', [
      critical('Does the page show a non-green transfer state — e.g. Analyzing, or awaiting review?'),
    ]);

    await highlight(page, 'span[data-state]');
    await caption(page, 'State: Analyzing — flagged for human review due to OCR amount mismatch', 2500);
    await clearHighlights(page);
    await clearCaption(page);

    // Review Queue
    await clickEl(page, 'a.nav-level-tab:has-text("Review")');
    await afterNav(page);

    await assertVisual(page, 'review-queue', [
      critical('Is there a table or list showing deposits awaiting operator review?'),
    ]);

    await highlight(page, 'table');
    await caption(page, 'Review Queue — flagged deposits with waiting time, reason, and a Review action', 2500);
    await clearHighlights(page);
    await clearCaption(page);

    // Navigate to the specific review for transferId2
    const reviewSelector = `a[href="/ui/review/${transferId2}"], a[href^="/ui/review/"]`;
    await moveCursor(page, reviewSelector);
    await highlight(page, reviewSelector);
    await page.waitForTimeout(450);
    await clearHighlights(page);
    await page.locator(reviewSelector).first().click();
    await afterNav(page);

    await assertVisual(page, 'review-detail', [
      critical('Does the page show a review form with transfer info and check images?'),
    ]);

    await caption(page, 'Review detail — transfer info and check images at a glance', 2000);
    await clearCaption(page);

    // Highlight check images panel
    await highlight(page, '.check-images');
    await caption(page, 'Front and back check images — operator verifies against vendor OCR results', 2400);
    await clearHighlights(page);
    await clearCaption(page);

    await page.evaluate(() => window.scrollBy({ top: 360, behavior: 'smooth' }));
    await page.waitForTimeout(700);
    await caption(page, 'Vendor Analysis — OCR detected $505, entered $500 — amount mismatch triggered review', 2400);
    await clearCaption(page);

    await page.evaluate(() => window.scrollBy({ top: 360, behavior: 'smooth' }));
    await page.waitForTimeout(700);
    await caption(page, 'Activity timeline — complete state history with actor, reason, and timestamps', 2200);
    await clearCaption(page);

    // Scroll to action panel (approve button) — use element-based scroll for reliability
    await page.locator('#approve-btn, button:has-text("Approve")').first().scrollIntoViewIfNeeded();
    await page.waitForTimeout(600);

    await assertVisual(page, 'approve-reject-buttons', [
      critical('Are Approve and Reject buttons visible at the bottom of the review form?'),
    ]);

    await caption(page, 'Operator has verified the images and vendor data — approving this deposit', 2000);
    await clearCaption(page);

    // Type notes and approve
    const notesSelector = '#approve-notes, textarea[name="notes"]';
    if (await page.locator(notesSelector).first().count() > 0) {
      await moveCursor(page, notesSelector);
      await page.locator(notesSelector).first().pressSequentially('Images clear, amount verified. Approving.', { delay: 30 });
      await page.waitForTimeout(300);
    }

    await clickEl(page, '#approve-btn, button:has-text("Approve")');
    await page.waitForURL(/\/ui\/transfers\/|\/ui\/review/, { timeout: 20000 }).catch(() => {});
    await afterNav(page);
    await page.waitForTimeout(700);

    await assertVisual(page, 'post-approve', [
      critical('Does the page indicate success — transfer in Approved or FundsPosted state?'),
    ]);

    await caption(page, 'Approved ✓ — transfer advances to FundsPosted, investor ledger credited', 2500);
    await clearCaption(page);

    // =========================================================================
    // WORKFLOW 3 — Settlement  (~2:00–2:40)
    // =========================================================================
    await titleCard(page, 'Workflow 3: Settlement', 'Package FundsPosted transfers → X9.37 ICL binary file');
    await page.waitForTimeout(2300);
    await removeTitle(page);
    await setProgress(page, 3, 'Settlement');

    await clickEl(page, 'a.nav-level-tab:has-text("Settlement")');
    await afterNav(page);

    await assertVisual(page, 'settlement-page', [
      critical('Is there a settlement page with a button to generate a settlement batch?'),
    ]);

    await caption(page, 'Settlement — collects FundsPosted transfers and writes a binary X9.37 ICL file', 2500);
    await clearCaption(page);

    await highlight(page, '#gen-btn, button:has-text("Generate")');
    await caption(page, 'X9.37 ICL is the real US clearing network format — proper record types, embedded images', 2200);
    await clearHighlights(page);
    await clearCaption(page);

    await clickEl(page, '#gen-btn, button:has-text("Generate")');
    await page.waitForSelector('.badge--GENERATED, td:has-text("GENERATED")', { timeout: 25000 }).catch(() => {});
    await afterNav(page); // restore overlays even though no full navigation occurred
    await page.waitForTimeout(700);

    await assertVisual(page, 'batch-generated', [
      critical('Is there a settlement batch row showing GENERATED status with item count and total amount?'),
    ]);

    await highlight(page, 'table tbody tr:first-child');
    await caption(page, 'Batch generated — X9.37 ICL file contains embedded check images in binary record format', 2500);
    await clearHighlights(page);
    await clearCaption(page);

    // Acknowledge
    const ackBtn = page.locator('[data-action="ack"], button:has-text("Acknowledge")').first();
    if (await ackBtn.count() > 0) {
      await moveCursor(page, '[data-action="ack"], button:has-text("Acknowledge")');
      await highlight(page, '[data-action="ack"], button:has-text("Acknowledge")');
      await caption(page, 'Acknowledge — simulates the clearing bank confirming receipt of the ICL file', 2200);
      await clearHighlights(page);
      await clearCaption(page);
      await ackBtn.click();
      await page.waitForSelector('.badge--ACKNOWLEDGED, td:has-text("ACKNOWLEDGED")', { timeout: 15000 }).catch(() => {});
      await afterNav(page);
      await page.waitForTimeout(700);

      await assertVisual(page, 'batch-acknowledged', [
        critical('Is the settlement batch now showing ACKNOWLEDGED status?'),
      ]);

      await caption(page, 'Acknowledged ✓ — all transfers in this batch are now marked Completed', 2500);
      await clearCaption(page);
    }

    // =========================================================================
    // OUTRO — Ledger then Dashboard wrap-up
    // =========================================================================

    // Brief ledger view — shows the double-entry accounting entries
    await clickEl(page, 'a.nav-level-tab:has-text("Ledger")');
    await afterNav(page);
    await page.waitForTimeout(600);

    await highlight(page, 'table');
    await caption(page, 'Ledger — every deposit generates matching debit/credit entries across asset and liability accounts', 2800);
    await clearHighlights(page);
    await clearCaption(page);

    await assertVisual(page, 'ledger-entries', [
      critical('Is there a financial ledger table showing entries with amounts and account types?'),
    ]);

    // Final dashboard
    await page.goto('/ui');
    await afterNav(page);
    await page.waitForTimeout(900);

    await assertVisual(page, 'dashboard-final', [
      critical('Does the overview dashboard show non-zero statistics reflecting the completed workflows?'),
    ]);

    await highlight(page, '.dash-cards');
    await caption(page, 'Three workflows complete — all key metrics updated', 2000);
    await clearHighlights(page);
    await clearCaption(page);

    // Scroll to state breakdown
    await page.evaluate(() => window.scrollBy({ top: 500, behavior: 'smooth' }));
    await page.waitForTimeout(700);
    await highlight(page, 'table');
    await caption(page, 'Transfers by State — 6 completed, 2 pending review, 3 exceptions — clickable for filtered views', 2800);
    await clearHighlights(page);
    await clearCaption(page);
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.waitForTimeout(400);

    await titleCard(page, 'Apex Mobile Check Deposit', 'Go · SQLite · HTMX · X9.37 ICL · Operator Review UI');
    await page.waitForTimeout(3200);

    // =========================================================================
    // DEFERRED VISUAL CHECKS — run after video ends to avoid dead time in recording
    // =========================================================================
    await runDeferredChecks();
  });
});
