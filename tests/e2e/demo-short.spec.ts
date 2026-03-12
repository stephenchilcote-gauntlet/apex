import * as fs from 'fs';
import { test as base, expect, Page } from '@playwright/test';
import { CHECK_FRONT, CHECK_BACK, CHECK_FRONT_WRONG_AMOUNT, CHECK_BACK_WRONG_AMOUNT } from './fixtures';
import { VisualJudge, critical, advisory } from './visual-judge';

const test = base.extend({});

/**
 * Professional Demo Video — 3 minute walkthrough of four core workflows:
 *   1. Happy Path      — submit a clean deposit, watch it auto-approve to FundsPosted
 *   2. Operator Review — submit a flagged deposit, approve it from the review queue
 *   3. Settlement      — generate an X9.37 ICL batch and acknowledge it
 *   4. Returns         — process a bounced check with reversal and $30 NSF fee
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
const progress = { current: 0, total: 4, label: '' };

// Timing log — records when each caption fires (ms since test start)
// Written to audio-clips/timing.json at end of test for audio assembly.
let _t0 = 0;
const timingLog: { id: string; t: number; duration: number }[] = [];
let _captionSeq = 0;

// ============================================================================
// Post-navigation restore — call after every page.goto() or nav click
// ============================================================================

/**
 * After every full-page navigation, re-inject the cursor at its last known
 * position AND restore the workflow progress indicator.
 * All DOM elements are wiped on navigation so we must recreate them.
 */
async function afterNav(page: Page) {
  await page.waitForLoadState('domcontentloaded', { timeout: 5000 }).catch(() => {});

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
          position: fixed; top: 58px; right: 16px; z-index: 99997;
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
// Caption — context-aware positioning
//
// When anchorSelector is provided, the caption floats near that element.
// Otherwise it falls back to a slim bar positioned relative to the cursor:
//   - cursor in top 55% of screen → bottom bar
//   - cursor in bottom 45% of screen → top bar
// ============================================================================

async function caption(page: Page, text: string, durationMs = 2800, anchorSelector?: string) {
  await page.waitForLoadState('domcontentloaded', { timeout: 5000 }).catch(() => {});

  // Record timing for audio assembly
  const tOffset = _t0 ? Date.now() - _t0 : 0;
  timingLog.push({ id: `cap-${++_captionSeq}`, t: tOffset, duration: durationMs });

  // Resolve anchor bounding box in Node.js context (not available in evaluate)
  let anchorBox: { x: number; y: number; width: number; height: number } | null = null;
  if (anchorSelector) {
    anchorBox = await page.locator(anchorSelector).first().boundingBox().catch(() => null);
  }

  const cx = cursor.x;
  const cy = cursor.y;

  await page.evaluate(
    ({ text, ab, cx, cy }) => {
      const VW = 1920;
      const VH = 1080;

      let cap = document.getElementById('demo-caption');
      if (!cap) {
        cap = document.createElement('div');
        cap.id = 'demo-caption';
        document.body.appendChild(cap);
      }
      cap.style.opacity = '0';
      cap.style.transition = 'opacity 0.22s ease';

      if (ab) {
        // ── Floating bubble near anchor element ──────────────────────────────
        const capW = Math.min(520, VW - 80);
        const elemCY = ab.y + ab.height / 2;

        let top: number;
        if (elemCY < VH * 0.5) {
          // Element in upper half — place caption below it
          top = Math.min(ab.y + ab.height + 14, VH - 130);
        } else {
          // Element in lower half — place caption above it (estimate height ~90px)
          top = Math.max(ab.y - 104, 80);
        }

        let left = ab.x + ab.width / 2 - capW / 2;
        left = Math.max(24, Math.min(left, VW - capW - 24));

        cap.style.cssText = `
          position: fixed; z-index: 100000;
          left: ${left}px; top: ${top}px;
          width: ${capW}px;
          background: rgba(6,6,6,0.93);
          border: 1.5px solid rgba(243,78,63,0.55);
          border-radius: 10px;
          padding: 11px 18px;
          font-family: -apple-system, 'Segoe UI', system-ui, sans-serif;
          color: #f0f0f0;
          font-size: 15px; line-height: 1.5;
          text-align: left; letter-spacing: 0.01em;
          pointer-events: none;
          transition: opacity 0.22s ease;
          box-shadow: 0 6px 28px rgba(0,0,0,0.65);
        `;
      } else {
        // ── Slim bar, top or bottom based on cursor position ─────────────────
        const barAtBottom = cy < VH * 0.55;
        if (barAtBottom) {
          cap.style.cssText = `
            position: fixed; bottom: 0; left: 0; right: 0; z-index: 100000;
            background: rgba(8,8,8,0.90);
            border-top: 2px solid #f34e3f;
            padding: 12px 80px;
            font-family: -apple-system, 'Segoe UI', system-ui, sans-serif;
            color: #f0f0f0;
            font-size: 16px; line-height: 1.5;
            text-align: center; letter-spacing: 0.01em;
            pointer-events: none;
            transition: opacity 0.22s ease;
          `;
        } else {
          cap.style.cssText = `
            position: fixed; top: 54px; left: 0; right: 0; z-index: 100000;
            background: rgba(8,8,8,0.90);
            border-bottom: 2px solid #f34e3f;
            padding: 12px 80px;
            font-family: -apple-system, 'Segoe UI', system-ui, sans-serif;
            color: #f0f0f0;
            font-size: 16px; line-height: 1.5;
            text-align: center; letter-spacing: 0.01em;
            pointer-events: none;
            transition: opacity 0.22s ease;
          `;
        }
      }

      cap.textContent = text;
      requestAnimationFrame(() => { (cap as HTMLElement).style.opacity = '1'; });
    },
    { text, ab: anchorBox, cx, cy },
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
// Key-press badge — shows a floating key label on screen while a key is pressed
// ============================================================================

async function showKeyBadge(page: Page, label: string, holdMs = 700) {
  await page.evaluate(({ label }) => {
    document.querySelectorAll('.demo-key-badge').forEach(e => e.remove());
    const el = document.createElement('div');
    el.className = 'demo-key-badge';
    el.style.cssText = `
      position: fixed; bottom: 88px; left: 50%; transform: translateX(-50%);
      z-index: 100002;
      background: #1a1a1a; border: 1.5px solid #f34e3f; border-radius: 8px;
      padding: 10px 24px;
      font-family: 'SF Mono','Fira Mono','Consolas',monospace;
      font-size: 22px; font-weight: 700; color: #f0f0f0; letter-spacing: 0.04em;
      box-shadow: 0 4px 24px rgba(243,78,63,0.4);
      opacity: 0; transition: opacity 0.1s ease; pointer-events: none;
    `;
    el.textContent = label;
    document.body.appendChild(el);
    requestAnimationFrame(() => { (el as HTMLElement).style.opacity = '1'; });
  }, { label });
  await page.waitForTimeout(holdMs);
  // Cleanup is best-effort — swallow if navigation has already destroyed the context.
  await page.evaluate(() => {
    document.querySelectorAll('.demo-key-badge').forEach(e => {
      (e as HTMLElement).style.opacity = '0';
      setTimeout(() => e.remove(), 120);
    });
  }).catch(() => {});
  await page.waitForTimeout(130);
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
  // Caption durations (clip_ms + 1500) sum to ~5 min. Visual judge disabled for clean recording run.
  test.setTimeout(2_700_000); // 45 min

  test('Four Core Workflows', async ({ page, request }) => {

    // Visual judge disabled for recording run (saves ~3.5 min)
    // try { judge = new VisualJudge(); } catch {
    //   console.warn('[demo] VisualJudge disabled — set ANTHROPIC_API_KEY to enable');
    // }

    _t0 = Date.now();

    // Force dark mode for the entire demo (set before first navigation)
    await page.addInitScript(() => {
      localStorage.setItem('apex-theme', 'dark');
    });

    // Reset to deterministic clean state, then seed demo data
    const resetResp = await request.post('/api/v1/test/reset');
    expect(resetResp.ok()).toBeTruthy();
    await request.post('/api/v1/test/seed');

    // =========================================================================
    // INTRO TITLE CARD  (~0:00–0:07)
    // =========================================================================
    await page.goto('/ui');
    await afterNav(page);

    await titleCard(page, 'Mobile Check Deposit', 'Four core workflows — 3 min demo');
    await page.waitForTimeout(2800);
    await removeTitle(page);

    // =========================================================================
    // DASHBOARD OVERVIEW  (~0:07–0:18)
    // =========================================================================
    console.log(`[demo] ${Date.now() - _t0}ms — Dashboard overview`);
    await caption(page, 'Live activity across all investor accounts — deposits, exceptions, and what needs attention now', 9905, '.dash-action-row');

    await assertVisual(page, 'dashboard', [
      critical('Does the page show a dashboard with stat cards or metric panels?'),
    ]);

    await clearCaption(page);

    // ── Keyboard power-user demo ──────────────────────────────────────────────
    await caption(page, 'Operators can navigate the entire system without touching the mouse', 7862);
    await clearCaption(page);

    // g → t  (Transfers)
    await showKeyBadge(page, 'g → t', 900);
    await page.keyboard.press('g');
    await page.waitForTimeout(80);
    await page.keyboard.press('t');
    await page.waitForURL('**/ui/transfers', { timeout: 8000 });
    await afterNav(page);
    await page.waitForTimeout(500);
    await caption(page, 'Single-key shortcuts — anywhere in the system, instantly', 10370, 'nav.nav-level-tabs, .nav-level-tabs');
    await clearCaption(page);

    // g → h  (back to Overview)
    await showKeyBadge(page, 'g → h', 900);
    await page.keyboard.press('g');
    await page.waitForTimeout(80);
    await page.keyboard.press('h');
    await page.waitForURL('**/ui', { timeout: 8000 });
    await afterNav(page);
    await page.waitForTimeout(400);

    // Ctrl+K — command palette search
    await showKeyBadge(page, 'Ctrl+K', 700);
    await page.keyboard.press('Control+k');
    await page.waitForSelector('#cmd-modal[open]', { timeout: 3000 }).catch(() => {});
    await page.waitForTimeout(300);
    await page.locator('#cmd-input').pressSequentially('INV-1001', { delay: 65 });
    await page.waitForTimeout(900);
    await caption(page, 'Search any transfer or account instantly — from anywhere in the app', 7072, '#cmd-modal');
    await clearCaption(page);
    await page.keyboard.press('Escape');
    await page.waitForTimeout(400);

    // =========================================================================
    // WORKFLOW 1 — Happy Path  (~0:18–1:05)
    // =========================================================================
    console.log(`[demo] ${Date.now() - _t0}ms — Workflow 1: Happy Path`);
    await titleCard(page, 'Workflow 1: Happy Path', 'Submit a clean check → auto-approve → funds posted');
    await page.waitForTimeout(2300);
    await removeTitle(page);
    await setProgress(page, 1, 'Happy Path');

    // Navigate to Simulate
    await clickEl(page, 'a.nav-level-tab:has-text("Simulate")');
    await afterNav(page);

    await caption(page, 'Submit a deposit — front and back check images, investor account, and amount', 8419, 'form[action="/ui/simulate"]');
    await clearCaption(page);

    // Briefly show the Recent Deposits section
    const recentSection = page.locator('.panel:has-text("Recent deposits")');
    if (await recentSection.count() > 0) {
      await page.evaluate(() => window.scrollBy({ top: 400, behavior: 'smooth' }));
      await page.waitForTimeout(600);
      await highlight(page, '.panel:has-text("Recent deposits") table');
      await caption(page, 'Recent submissions — live status updates as each deposit processes', 7537, '.panel:has-text("Recent deposits") table');
      await clearHighlights(page);
      await clearCaption(page);
      await page.evaluate(() => window.scrollTo({ top: 0, behavior: 'smooth' }));
      await page.waitForTimeout(500);
    }

    await selectEl(page, 'select[name="investorAccountId"]', 'INV-1001');
    await typeEl(page, 'input[name="amount"]', '1250.00');
    // Uncheck sample images to enable file upload (shows previews)
    const sampleChk = page.locator('input[name="useSampleImages"]');
    if (await sampleChk.isChecked()) await sampleChk.uncheck();
    await page.locator('input[name="frontImage"]').setInputFiles(CHECK_FRONT);
    await page.locator('input[name="backImage"]').setInputFiles(CHECK_BACK);
    // Give FileReader time to render previews
    await page.waitForSelector('#frontPreview[src]', { timeout: 5000 }).catch(() => {});
    await page.waitForTimeout(500);

    await moveCursor(page, '#frontPreview');
    await highlight(page, '#frontPreview');
    await caption(page, 'AI analyzes image quality, the printed amount, and routing information — on every submission', 10555, '#frontPreview');
    await clearHighlights(page);
    await clearCaption(page);

    await caption(page, 'One submission — automated analysis, compliance checks, and accounting. All in seconds.', 8140, 'button[type="submit"]');
    await clearCaption(page);
    await clickEl(page, 'button[type="submit"]');

    await page.locator('[data-transfer-id]').waitFor({ timeout: 20000 });
    const transferId1 = await page.locator('[data-transfer-id]').getAttribute('data-transfer-id');
    expect(transferId1).toBeTruthy();

    await page.goto(`/ui/transfers/${transferId1}`);
    await afterNav(page);

    await caption(page, 'Transfer status updates automatically in real time — no refresh needed', 7119, 'span[data-state]');
    await clearCaption(page);

    await waitForTerminalState(page);
    await page.waitForTimeout(800);

    await assertVisual(page, 'transfer-funds-posted', [
      critical('Does the page show a transfer with a green or positive state badge like FundsPosted or Completed?'),
      critical('Is there a pipeline or progress tracker showing the transfer stages?'),
    ]);

    // Highlight the state badge specifically (span[data-state], not the pipeline)
    await highlight(page, 'span[data-state]');
    await caption(page, 'Funds Posted ✓ — all checks passed, investor account credited', 6515, 'span[data-state]');
    await clearHighlights(page);
    await clearCaption(page);

    await highlight(page, '.pipeline');
    await caption(page, 'Every stage is recorded for compliance and audit', 2200, '.pipeline');
    await clearHighlights(page);
    await clearCaption(page);

    // Show the Process Return button — demonstrates return flow awareness
    const returnBtn = page.locator('a:has-text("Process Return")').first();
    if (await returnBtn.count() > 0) {
      await moveCursor(page, 'a:has-text("Process Return")');
      await highlight(page, 'a:has-text("Process Return")');
      await caption(page, 'If a check bounces later, a return can be initiated in one click', 7769, 'a:has-text("Process Return")');
      await clearHighlights(page);
      await clearCaption(page);
    }

    // Scroll to rule evaluations
    await page.evaluate(() => window.scrollBy(0, 600));
    await page.waitForTimeout(600);

    await assertVisual(page, 'rule-evaluations', [
      critical('Is there a table showing business rule evaluations with pass/fail outcomes?'),
    ]);

    await caption(page, 'Compliance checks — account eligibility, deposit limits, contribution type, and duplicate detection — all passed', 9905);
    await clearCaption(page);
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.waitForTimeout(500);

    // =========================================================================
    // WORKFLOW 2 — Operator Review  (~1:05–2:00)
    // =========================================================================
    console.log(`[demo] ${Date.now() - _t0}ms — Workflow 2: Operator Review`);
    await titleCard(page, 'Workflow 2: Operator Review', 'Amount mismatch → review queue → human approval');
    await page.waitForTimeout(2300);
    await removeTitle(page);
    await setProgress(page, 2, 'Operator Review');

    await page.goto('/ui/simulate');
    await afterNav(page);

    await selectEl(page, 'select[name="investorAccountId"]', 'INV-1006');
    await typeEl(page, 'input[name="amount"]', '500.00');
    // Uncheck sample images to enable file upload — use wrong-amount check (printed $750, declared $500)
    const sampleChk2 = page.locator('input[name="useSampleImages"]');
    if (await sampleChk2.isChecked()) await sampleChk2.uncheck();
    await page.locator('input[name="frontImage"]').setInputFiles(CHECK_FRONT_WRONG_AMOUNT);
    await page.locator('input[name="backImage"]').setInputFiles(CHECK_BACK_WRONG_AMOUNT);
    await page.waitForSelector('#frontPreview[src]', { timeout: 5000 }).catch(() => {});
    await page.waitForTimeout(400);
    await caption(page, 'This check shows $750 — declared amount is $500. The system catches the discrepancy automatically.', 11298, '#frontPreview');
    await clearCaption(page);
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
    await caption(page, 'Flagged for review — amount discrepancy detected, routed to the review queue', 5679, 'span[data-state]');
    await clearHighlights(page);
    await clearCaption(page);

    // Review Queue
    await clickEl(page, 'a.nav-level-tab:has-text("Review")');
    await afterNav(page);

    await assertVisual(page, 'review-queue', [
      critical('Is there a table or list showing deposits awaiting operator review?'),
    ]);

    await highlight(page, 'table');
    await caption(page, 'Review Queue — everything waiting for an operator decision, with time elapsed', 7444, 'table');
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

    await caption(page, 'Review detail — transfer information and check images side by side', 6654);
    await clearCaption(page);

    // Highlight check images panel
    await highlight(page, '.check-images');
    await caption(page, 'Compare the images directly against the AI\'s findings', 6051, '.check-images');
    await clearHighlights(page);
    await clearCaption(page);

    await page.evaluate(() => window.scrollBy({ top: 360, behavior: 'smooth' }));
    await page.waitForTimeout(700);
    await caption(page, 'The AI read the printed amount as $750 — that discrepancy triggered this review', 7165);
    await clearCaption(page);

    await page.evaluate(() => window.scrollBy({ top: 360, behavior: 'smooth' }));
    await page.waitForTimeout(700);
    await caption(page, 'Full audit trail — every action, who took it, and when', 7304);
    await clearCaption(page);

    // Scroll to action panel (approve button) — use element-based scroll for reliability
    await page.locator('#approve-btn, button:has-text("Approve")').first().scrollIntoViewIfNeeded();
    await page.waitForTimeout(600);

    await assertVisual(page, 'approve-reject-buttons', [
      critical('Are Approve and Reject buttons visible at the bottom of the review form?'),
    ]);

    await caption(page, 'After reviewing images and AI findings, the operator approves with a note', 6747, '#approve-btn, button:has-text("Approve")');
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

    await caption(page, 'Approved ✓ — funds posted to the investor account immediately', 6329, 'span[data-state]');
    await clearCaption(page);

    // =========================================================================
    // WORKFLOW 3 — Settlement  (~2:00–2:40)
    // =========================================================================
    console.log(`[demo] ${Date.now() - _t0}ms — Workflow 3: Settlement`);
    await titleCard(page, 'Workflow 3: Settlement', 'Package FundsPosted transfers → X9.37 ICL binary file');
    await page.waitForTimeout(2300);
    await removeTitle(page);
    await setProgress(page, 3, 'Settlement');

    await clickEl(page, 'a.nav-level-tab:has-text("Settlement")');
    await afterNav(page);

    await assertVisual(page, 'settlement-page', [
      critical('Is there a settlement page with a button to generate a settlement batch?'),
    ]);

    await caption(page, 'Settlement — packages cleared deposits into the format required by the Federal Reserve', 7676);
    await clearCaption(page);

    await highlight(page, '#gen-btn, button:has-text("Generate")');
    await caption(page, 'The same clearing format used by US banks — with check images embedded as required', 7676, '#gen-btn, button:has-text("Generate")');
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
    await caption(page, 'Settlement file ready — queued for transmission to the clearing network', 5493, 'table tbody tr:first-child');
    await clearHighlights(page);
    await clearCaption(page);

    // Acknowledge
    const ackBtn = page.locator('[data-action="ack"], button:has-text("Acknowledge")').first();
    if (await ackBtn.count() > 0) {
      await moveCursor(page, '[data-action="ack"], button:has-text("Acknowledge")');
      await highlight(page, '[data-action="ack"], button:has-text("Acknowledge")');
      await caption(page, 'Acknowledge — confirm the clearing bank has received the settlement file', 6190, '[data-action="ack"], button:has-text("Acknowledge")');
      await clearHighlights(page);
      await clearCaption(page);
      await ackBtn.click();
      await page.waitForSelector('.badge--ACKNOWLEDGED, td:has-text("ACKNOWLEDGED")', { timeout: 15000 }).catch(() => {});
      await afterNav(page);
      await page.waitForTimeout(700);

      await assertVisual(page, 'batch-acknowledged', [
        critical('Is the settlement batch now showing ACKNOWLEDGED status?'),
      ]);

      await caption(page, 'Settlement complete ✓ — all deposits in this batch are fully settled', 5586, '.badge--ACKNOWLEDGED, td:has-text("ACKNOWLEDGED")');
      await clearCaption(page);
    }

    // =========================================================================
    // WORKFLOW 4 — Returns  (~2:45–3:15)
    // =========================================================================
    console.log(`[demo] ${Date.now() - _t0}ms — Workflow 4: Returns`);
    await titleCard(page, 'Workflow 4: Return Processing', 'Completed check bounces → reversal + $30 NSF fee');
    await page.waitForTimeout(2300);
    await removeTitle(page);
    await setProgress(page, 4, 'Returns');

    await clickEl(page, 'a.nav-level-tab:has-text("Returns")');
    await afterNav(page);

    await caption(page, 'Returns — process a bounced check using standard bank return reason codes', 6422);
    await clearCaption(page);

    // Show UUID autocomplete: type first 8 chars, wait for dropdown, Tab to complete
    await page.locator('#transferId').fill('');
    await page.locator('#transferId').focus();
    await page.locator('#transferId').fill(transferId1!.substring(0, 8));
    await page.waitForTimeout(600); // let HTMX fetch + render dropdown
    await caption(page, 'Type a few characters, Tab to auto-complete the transfer ID', 6051, '#transferId');
    await clearCaption(page);
    await page.locator('#transferId').press('Tab');
    await page.waitForTimeout(300);

    await highlight(page, 'select[name="reasonCode"]');
    await caption(page, 'NSF — Non-Sufficient Funds — the most common return reason', 5865, 'select[name="reasonCode"]');
    await clearHighlights(page);
    await clearCaption(page);

    await caption(page, 'Reversal posted and NSF fee applied — automatically', 7026);
    await clearCaption(page);

    await clickEl(page, '#returns-submit-btn, button:has-text("Process Return")');

    // Wait for the returned transfer panel (POST renders inline, but browser reloads the page)
    await page.waitForSelector('.badge--Returned, span[data-state]:has-text("Returned")', { timeout: 20000 }).catch(() => {});
    await afterNav(page); // restore cursor + progress overlay after form POST navigation
    await page.waitForTimeout(800);

    await assertVisual(page, 'return-processed', [
      critical('Does the page show a successfully processed return with a Returned state badge?'),
      advisory('Is there a section showing the returned transfer details including amount and reason code?'),
    ]);

    await highlight(page, '.flash--success, .badge--Returned');
    await caption(page, 'Return processed ✓ — deposit reversed, $30 NSF fee recorded against investor account', 8048, '.flash--success, .badge--Returned');
    await clearHighlights(page);
    await clearCaption(page);

    // =========================================================================
    // OUTRO — Ledger then Dashboard wrap-up
    // =========================================================================

    // Brief ledger view — now shows deposit + reversal + fee entries
    await clickEl(page, 'a.nav-level-tab:has-text("Ledger")');
    await afterNav(page);
    await page.waitForTimeout(600);

    await highlight(page, 'table');
    await caption(page, 'Every transaction is reflected in the ledger — investor accounts, clearing, and fee revenue', 8233, 'table');
    await clearHighlights(page);
    await clearCaption(page);

    // Scroll to show the recent journal entries panel
    await page.evaluate(() => window.scrollBy({ top: 400, behavior: 'smooth' }));
    await page.waitForTimeout(700);

    const journalSection = page.locator('.panel-header-title:has-text("Recent journal")');
    if (await journalSection.count() > 0) {
      await highlight(page, '.panel:has(.panel-header-title:has-text("Recent journal")) table');
      await caption(page, 'Deposit, reversal, and fee — every dollar accounted for, every debit matched by a credit', 8698, '.panel:has(.panel-header-title:has-text("Recent journal")) table');
      await clearHighlights(page);
      await clearCaption(page);
    }

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

    console.log(`[demo] ${Date.now() - _t0}ms — Outro: dashboard`);
    await highlight(page, '.dash-action-row');
    await caption(page, 'Four core workflows complete — dashboard reflects all live activity', 6608, '.dash-action-row');
    await clearHighlights(page);
    await clearCaption(page);

    // Scroll to state breakdown
    await page.evaluate(() => window.scrollBy({ top: 500, behavior: 'smooth' }));
    await page.waitForTimeout(700);
    await highlight(page, 'table');
    await caption(page, 'Live activity by status — click any row to filter instantly', 6840, 'table');
    await clearHighlights(page);
    await clearCaption(page);
    await page.evaluate(() => window.scrollTo(0, 0));
    await page.waitForTimeout(400);

    await titleCard(page, 'Apex Mobile Check Deposit', 'Automated Analysis · Operator Review · Settlement · Full Audit Trail');
    await page.waitForTimeout(3200);

    // =========================================================================
    // Write timing log for audio assembly
    // =========================================================================
    try {
      const timingPath = new URL('audio-clips/timing.json', `file://${__dirname}/`).pathname;
      fs.mkdirSync(new URL('audio-clips/', `file://${__dirname}/`).pathname, { recursive: true });
      fs.writeFileSync(timingPath, JSON.stringify(timingLog, null, 2));
      console.log(`\n  ⏱  Timing log written to ${timingPath}`);
    } catch (e) {
      console.warn(`  ⚠  Could not write timing log: ${e}`);
    }

    // =========================================================================
    // DEFERRED VISUAL CHECKS — run after video ends to avoid dead time in recording
    // =========================================================================
    await runDeferredChecks();
  });
});
