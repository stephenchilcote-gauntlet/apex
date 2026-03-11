import { test as base, expect, Page } from '@playwright/test';
import { CHECK_FRONT, CHECK_BACK } from './fixtures';

/**
 * Short Demo Video — 1.5–3 minute walkthrough of three core workflows:
 *   1. Happy Path     — submit a clean deposit, watch it auto-approve to FundsPosted
 *   2. Operator Review — submit a flagged deposit, approve it from the review queue
 *   3. Settlement      — generate an X9.37 ICL batch and acknowledge it
 *
 * Run:   npx playwright test demo-short.spec.ts
 * Output: reports/demo-short.mp4
 */

const test = base.extend({});

// ---------------------------------------------------------------------------
// Caption / highlight helpers (minimal set — no diagram overlays)
// ---------------------------------------------------------------------------

async function caption(page: Page, text: string, durationMs = 2500) {
  await page.waitForLoadState('domcontentloaded');
  await page.evaluate(
    ({ text }) => {
      let cap = document.getElementById('demo-caption');
      if (!cap) {
        cap = document.createElement('div');
        cap.id = 'demo-caption';
        cap.style.cssText = `
          position: fixed; bottom: 0; left: 0; right: 0; z-index: 100000;
          background: rgba(0,0,0,0.82);
          border-top: 2px solid #f34e3f;
          padding: 12px 48px;
          font-family: system-ui, sans-serif;
          color: #f0f0f0;
          font-size: 16px; line-height: 1.45;
          text-align: center;
          pointer-events: none;
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
  await page.waitForTimeout(200);
}

async function title(page: Page, heading: string, sub?: string) {
  await page.evaluate(
    ({ heading, sub }) => {
      let card = document.getElementById('demo-title-card');
      if (!card) {
        card = document.createElement('div');
        card.id = 'demo-title-card';
        card.style.cssText = `
          position: fixed; top: 0; left: 0; right: 0; bottom: 0; z-index: 99999;
          background: #111111;
          display: flex; flex-direction: column; align-items: center; justify-content: center;
          font-family: system-ui, sans-serif;
          opacity: 0; transition: opacity 0.3s ease;
        `;
        document.body.appendChild(card);
      }
      card.innerHTML = `
        <div style="font-size:11px;font-weight:700;letter-spacing:0.18em;text-transform:uppercase;color:#f34e3f;margin-bottom:16px;">
          Apex MCD
        </div>
        <div style="font-size:32px;font-weight:700;color:#f0f0f0;margin-bottom:10px;letter-spacing:-0.02em;">
          ${heading}
        </div>
        ${sub ? `<div style="font-size:16px;color:#737373;">${sub}</div>` : ''}
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
    if (el) { el.style.opacity = '0'; setTimeout(() => el.remove(), 350); }
  });
  await page.waitForTimeout(400);
}

async function highlight(page: Page, selector: string) {
  const box = await page.locator(selector).first().boundingBox();
  if (box) {
    await page.evaluate((r) => {
      document.querySelectorAll('.demo-hl').forEach(e => e.remove());
      const ring = document.createElement('div');
      ring.className = 'demo-hl';
      ring.style.cssText = `
        position: fixed; z-index: 99998;
        left: ${r.x - 4}px; top: ${r.y - 4}px;
        width: ${r.w + 8}px; height: ${r.h + 8}px;
        border: 2px solid #f34e3f;
        border-radius: 6px;
        box-shadow: 0 0 10px rgba(243,78,63,0.45);
        pointer-events: none;
      `;
      document.body.appendChild(ring);
    }, { x: box.x, y: box.y, w: box.width, h: box.height });
  }
  await page.waitForTimeout(600);
}

async function clearHighlights(page: Page) {
  await page.evaluate(() => {
    document.querySelectorAll('.demo-hl').forEach(e => e.remove());
  });
}

async function moveCursor(page: Page, selector: string) {
  await page.evaluate(() => {
    if (document.getElementById('demo-cursor')) return;
    const cur = document.createElement('div');
    cur.id = 'demo-cursor';
    cur.innerHTML = `<svg width="22" height="22" viewBox="0 0 24 24" fill="none">
      <path d="M5 3l14 8-6.5 1.5L10 19z" fill="white" stroke="#333" stroke-width="1.5" stroke-linejoin="round"/>
    </svg>`;
    cur.style.cssText = `
      position: fixed; z-index: 100001; pointer-events: none;
      left: 960px; top: 540px;
      transition: left 0.45s cubic-bezier(0.4,0,0.2,1), top 0.45s cubic-bezier(0.4,0,0.2,1);
      filter: drop-shadow(0 2px 4px rgba(0,0,0,0.5));
    `;
    document.body.appendChild(cur);
  });
  const box = await page.locator(selector).first().boundingBox();
  if (box) {
    await page.evaluate(({ x, y }) => {
      const cur = document.getElementById('demo-cursor');
      if (cur) { cur.style.left = `${x}px`; cur.style.top = `${y}px`; }
    }, { x: box.x + box.width / 2, y: box.y + box.height / 2 });
  }
  await page.waitForTimeout(500);
}

async function ensureCursor(page: Page) {
  await page.waitForLoadState('domcontentloaded');
  await page.evaluate(() => {
    if (!document.getElementById('demo-cursor')) {
      const cur = document.createElement('div');
      cur.id = 'demo-cursor';
      cur.innerHTML = `<svg width="22" height="22" viewBox="0 0 24 24" fill="none">
        <path d="M5 3l14 8-6.5 1.5L10 19z" fill="white" stroke="#333" stroke-width="1.5" stroke-linejoin="round"/>
      </svg>`;
      cur.style.cssText = `
        position: fixed; z-index: 100001; pointer-events: none;
        left: 960px; top: 540px;
        transition: left 0.45s cubic-bezier(0.4,0,0.2,1), top 0.45s cubic-bezier(0.4,0,0.2,1);
        filter: drop-shadow(0 2px 4px rgba(0,0,0,0.5));
      `;
      document.body.appendChild(cur);
    }
  });
}

async function click(page: Page, selector: string) {
  await moveCursor(page, selector);
  await highlight(page, selector);
  await page.waitForTimeout(200);
  await page.locator(selector).first().click();
  await clearHighlights(page);
}

async function fill(page: Page, selector: string, text: string) {
  await moveCursor(page, selector);
  await highlight(page, selector);
  await page.locator(selector).first().fill(text);
  await clearHighlights(page);
}

async function selectOption(page: Page, selector: string, value: string) {
  await moveCursor(page, selector);
  await highlight(page, selector);
  await page.locator(selector).first().selectOption({ value });
  await page.waitForTimeout(300);
  await clearHighlights(page);
}

async function waitForState(page: Page, state: string, maxMs = 20000) {
  await page.waitForFunction(
    (s) => {
      const el = document.querySelector('[data-state]');
      return el && el.textContent?.trim() === s;
    },
    state,
    { timeout: maxMs },
  );
}

// ===========================================================================
// THE SHORT DEMO
// ===========================================================================

test.use({
  video: { mode: 'on', size: { width: 1920, height: 1080 } },
  viewport: { width: 1920, height: 1080 },
});

test.describe('Short Demo', () => {
  test.setTimeout(240_000); // 4 min hard cap

  test('Three Core Workflows', async ({ page, request }) => {

    // Reset to clean state
    const resp = await request.post('/api/v1/test/reset');
    expect(resp.ok()).toBeTruthy();

    // =========================================================================
    // INTRO TITLE CARD  (~0:00–0:05)
    // =========================================================================
    await page.goto('/ui');
    await title(page, 'Mobile Check Deposit', 'Three core workflows — 2 minute demo');
    await page.waitForTimeout(2500);
    await removeTitle(page);
    await ensureCursor(page);

    // =========================================================================
    // OVERVIEW — Dashboard  (~0:05–0:15)
    // =========================================================================
    await caption(page, 'Overview dashboard — live stats and quick navigation', 2000);
    await page.waitForTimeout(1000);
    await clearCaption(page);

    // =========================================================================
    // WORKFLOW 1 — Happy Path  (~0:15–0:55)
    // =========================================================================
    await title(page, 'Workflow 1: Happy Path', 'Submit → auto-approve → funds posted');
    await page.waitForTimeout(2000);
    await removeTitle(page);

    await page.goto('/ui/simulate');
    await ensureCursor(page);
    await caption(page, 'Simulate a clean deposit using INV-1001 (auto-approve account)', 2000);
    await clearCaption(page);

    await selectOption(page, 'select[name="investorAccountId"]', 'INV-1001');
    await fill(page, 'input[name="amount"]', '1250.00');
    await page.locator('input[name="frontImage"]').setInputFiles(CHECK_FRONT);
    await page.locator('input[name="backImage"]').setInputFiles(CHECK_BACK);
    await page.waitForTimeout(500);

    await caption(page, 'Submitting deposit — the API creates a transfer and queues vendor analysis', 1500);
    await clearCaption(page);

    await click(page, 'button[type="submit"]');
    await page.locator('[data-transfer-id]').waitFor({ timeout: 15000 });
    const transferId1 = await page.locator('[data-transfer-id]').getAttribute('data-transfer-id');
    expect(transferId1).toBeTruthy();

    await caption(page, 'Transfer created — navigating to detail page to watch live state transitions', 2000);
    await clearCaption(page);

    await page.goto(`/ui/transfers/${transferId1}`);
    await ensureCursor(page);
    await caption(page, 'State machine: Requested → Validating → Analyzing → Approved → FundsPosted', 2500);
    await clearCaption(page);

    // Wait for terminal state
    await page.waitForFunction(
      () => {
        const el = document.querySelector('[data-state]');
        const s = el?.textContent?.trim();
        return s === 'FundsPosted' || s === 'Completed' || s === 'Rejected';
      },
      { timeout: 25000 },
    );

    const finalState1 = await page.locator('[data-state]').first().textContent();
    await highlight(page, '[data-state]');
    await caption(page, `Funds Posted ✓ — the check passed all rules and the ledger was credited`, 2500);
    await clearHighlights(page);
    await clearCaption(page);

    // =========================================================================
    // WORKFLOW 2 — Operator Review  (~0:55–1:45)
    // =========================================================================
    await title(page, 'Workflow 2: Operator Review', 'Amount mismatch flagged → review queue → approve');
    await page.waitForTimeout(2000);
    await removeTitle(page);

    await page.goto('/ui/simulate');
    await ensureCursor(page);
    await caption(page, 'INV-1006 triggers an amount mismatch — OCR vs declared amount differs', 2000);
    await clearCaption(page);

    await selectOption(page, 'select[name="investorAccountId"]', 'INV-1006');
    await fill(page, 'input[name="amount"]', '500.00');
    await page.locator('input[name="frontImage"]').setInputFiles(CHECK_FRONT);
    await page.locator('input[name="backImage"]').setInputFiles(CHECK_BACK);

    await click(page, 'button[type="submit"]');
    await page.locator('[data-transfer-id]').waitFor({ timeout: 15000 });
    const transferId2 = await page.locator('[data-transfer-id]').getAttribute('data-transfer-id');
    expect(transferId2).toBeTruthy();

    // Wait for it to reach review queue (Analyzing + review_required)
    await page.goto(`/ui/transfers/${transferId2}`);
    await ensureCursor(page);
    await page.waitForFunction(
      () => {
        const el = document.querySelector('[data-state]');
        const s = el?.textContent?.trim();
        return s === 'Analyzing' || s === 'Approved' || s === 'FundsPosted' || s === 'Rejected';
      },
      { timeout: 25000 },
    );

    await caption(page, 'Vendor flagged this deposit — it\'s now waiting in the operator review queue', 2000);
    await clearCaption(page);

    // Go to review queue
    await click(page, 'a.nav-level-tab:has-text("Review")');
    await ensureCursor(page);
    await caption(page, 'Review Queue — operators see all flagged deposits with reason codes', 2000);
    await clearCaption(page);

    // Find the review link for this transfer and click it
    const reviewLink = page.locator(`a[href="/ui/review/${transferId2}"]`).first();
    const reviewLinkAlt = page.locator('a[href^="/ui/review/"]').first();
    const targetReview = (await reviewLink.count() > 0) ? reviewLink : reviewLinkAlt;

    await moveCursor(page, 'a[href^="/ui/review/"]');
    await highlight(page, 'a[href^="/ui/review/"]');
    await page.waitForTimeout(500);
    await clearHighlights(page);
    await targetReview.click();
    await ensureCursor(page);

    await caption(page, 'Review detail — check images, vendor analysis, rule evaluations, and audit trail', 2500);
    await clearCaption(page);

    // Approve it
    await highlight(page, 'button[value="approve"], input[value="approve"], button:has-text("Approve")');
    await caption(page, 'Approving — operator decision triggers state transition to Approved → FundsPosted', 2000);
    await clearHighlights(page);
    await clearCaption(page);

    const approveBtn = page.locator('button:has-text("Approve"), input[value="approve"]').first();
    await approveBtn.click();

    // Wait for result
    await page.waitForURL(/\/ui\/transfers\/|\/ui\/review/, { timeout: 15000 }).catch(() => {});
    await ensureCursor(page);
    await caption(page, 'Approved ✓ — funds posted to the investor account ledger', 2000);
    await clearCaption(page);

    // =========================================================================
    // WORKFLOW 3 — Settlement  (~1:45–2:20)
    // =========================================================================
    await title(page, 'Workflow 3: Settlement', 'Package FundsPosted transfers into X9.37 ICL batch');
    await page.waitForTimeout(2000);
    await removeTitle(page);

    await click(page, 'a.nav-level-tab:has-text("Settlement")');
    await ensureCursor(page);
    await caption(page, 'Settlement — packages all FundsPosted transfers into a binary X9.37 ICL file', 2000);
    await clearCaption(page);

    // Generate batch
    await highlight(page, '#gen-btn, button:has-text("Generate Settlement Batch")');
    await caption(page, 'Generating ICL batch — the settlement engine writes check images into binary format', 1500);
    await clearHighlights(page);
    await clearCaption(page);

    await click(page, '#gen-btn, button:has-text("Generate Settlement Batch")');

    // Wait for batch to appear
    await page.waitForSelector('.badge--GENERATED, .badge--ACKNOWLEDGED', { timeout: 20000 }).catch(() => {});
    await ensureCursor(page);
    await caption(page, 'Batch generated — X9.37 ICL file ready for download', 2000);
    await clearCaption(page);

    // Acknowledge if present
    const ackBtn = page.locator('button:has-text("Acknowledge")').first();
    if (await ackBtn.count() > 0) {
      await highlight(page, 'button:has-text("Acknowledge")');
      await caption(page, 'Acknowledging — confirms the file was transmitted to the clearing network', 1500);
      await clearHighlights(page);
      await clearCaption(page);
      await ackBtn.click();
      await page.waitForSelector('.badge--ACKNOWLEDGED', { timeout: 10000 }).catch(() => {});
      await ensureCursor(page);
      await caption(page, 'Acknowledged ✓ — settlement complete, transfers marked as Completed', 2000);
      await clearCaption(page);
    }

    // =========================================================================
    // OUTRO — Dashboard summary  (~2:20–2:30)
    // =========================================================================
    await page.goto('/ui');
    await ensureCursor(page);
    await caption(page, 'Overview — all three workflows reflected in live dashboard stats', 2500);
    await clearCaption(page);

    await title(page, 'Apex Mobile Check Deposit', 'Go • SQLite • HTMX • X9.37 ICL • Operator UI');
    await page.waitForTimeout(3000);
  });
});
