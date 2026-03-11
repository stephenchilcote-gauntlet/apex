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

/** Section heading shown as a caption with title + subtitle. */
async function announce(page: Page, title: string, subtitle?: string) {
  const text = subtitle ? `${title} — ${subtitle}` : title;
  await caption(page, text, 3000);
}

/** On-screen caption at bottom of screen — stays until cleared or replaced. */
async function caption(page: Page, text: string, durationMs = 4000) {
  await page.waitForLoadState('domcontentloaded');
  await page.evaluate(
    ({ text }) => {
      let cap = document.getElementById('tour-caption');
      if (!cap) {
        cap = document.createElement('div');
        cap.id = 'tour-caption';
        cap.style.cssText = `
          position: fixed; bottom: 0; left: 0; right: 0; z-index: 100000;
          background: rgba(0,10,20,0.88);
          border-top: 1px solid rgba(0,217,255,0.3);
          padding: 10px 40px;
          font-family: 'Inter', 'Segoe UI', system-ui, sans-serif;
          color: #cce8ff;
          font-size: 18px; line-height: 1.4;
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
  await clearCaption(page);
}

async function clearCaption(page: Page) {
  await page.waitForLoadState('domcontentloaded');
  await page.evaluate(() => {
    const el = document.getElementById('tour-caption');
    if (el) el.style.opacity = '0';
  });
  await page.waitForTimeout(300);
}

async function clearAll(page: Page) {
  await clearCaption(page);
}

async function highlight(page: Page, selector: string) {
  // Get bounding box via Playwright locator (supports Playwright-specific selectors)
  const box = await page.locator(selector).first().boundingBox();
  if (box) {
    await page.evaluate((rect) => {
      document.querySelectorAll('.tour-highlight').forEach((e) => e.remove());
      const ring = document.createElement('div');
      ring.className = 'tour-highlight';
      ring.style.cssText = `
        position: fixed; z-index: 99997;
        left: ${rect.x - 4}px; top: ${rect.y - 4}px;
        width: ${rect.w + 8}px; height: ${rect.h + 8}px;
        border: 2px solid #00d9ff;
        border-radius: 6px;
        box-shadow: 0 0 12px rgba(0,217,255,0.5);
        pointer-events: none;
        transition: all 0.3s ease;
      `;
      document.body.appendChild(ring);
    }, { x: box.x, y: box.y, w: box.width, h: box.height });
  }
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

// ---------------------------------------------------------------------------
// Fake cursor — animated pointer so viewers can follow mouse actions
// ---------------------------------------------------------------------------

/** Inject the fake cursor element (call once after first page load). */
async function initCursor(page: Page) {
  await page.evaluate(() => {
    if (document.getElementById('tour-cursor')) return;
    const cur = document.createElement('div');
    cur.id = 'tour-cursor';
    cur.innerHTML = `<svg width="24" height="24" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
      <path d="M5 3l14 8-6.5 1.5L10 19z" fill="white" stroke="#222" stroke-width="1.5" stroke-linejoin="round"/>
    </svg>`;
    cur.style.cssText = `
      position: fixed; z-index: 100001; pointer-events: none;
      left: 960px; top: 540px;
      transition: left 0.5s cubic-bezier(0.4,0,0.2,1), top 0.5s cubic-bezier(0.4,0,0.2,1);
      filter: drop-shadow(0 2px 4px rgba(0,0,0,0.5));
    `;
    document.body.appendChild(cur);
  });
}

/** Re-inject cursor after full-page navigations. */
async function ensureCursor(page: Page) {
  await page.waitForLoadState('domcontentloaded');
  await initCursor(page);
}

/** Hide the fake cursor (e.g. during diagram overlays or non-interactive sections). */
async function hideCursor(page: Page) {
  await page.evaluate(() => {
    const cur = document.getElementById('tour-cursor');
    if (cur) cur.style.opacity = '0';
  });
}

/** Show the cursor again. */
async function showCursor(page: Page) {
  await ensureCursor(page);
  await page.evaluate(() => {
    const cur = document.getElementById('tour-cursor');
    if (cur) cur.style.opacity = '1';
  });
}

/** Animate the fake cursor to the center of an element (accepts Playwright selectors). */
async function moveTo(page: Page, selector: string) {
  await showCursor(page);
  const box = await page.locator(selector).first().boundingBox();
  if (box) {
    await page.evaluate(({ x, y }) => {
      const cur = document.getElementById('tour-cursor');
      if (!cur) return;
      cur.style.left = `${x}px`;
      cur.style.top = `${y}px`;
    }, { x: box.x + box.width / 2, y: box.y + box.height / 2 });
  }
  await page.waitForTimeout(600);
}

/** Move cursor to element, highlight it, then click. */
async function cursorClick(page: Page, selector: string) {
  await moveTo(page, selector);
  await highlight(page, selector);
  await page.waitForTimeout(300);
  await page.locator(selector).first().click();
  await clearHighlights(page);
}

/** Move cursor to element, highlight, fill text, then clear highlight. */
async function cursorFill(page: Page, selector: string, text: string) {
  await moveTo(page, selector);
  await highlight(page, selector);
  await page.waitForTimeout(200);
  await page.locator(selector).first().fill(text);
  await page.waitForTimeout(400);
  await clearHighlights(page);
}

/** Move cursor to element, highlight, select option, then clear highlight. */
async function cursorSelect(page: Page, selector: string, value: string) {
  await moveTo(page, selector);
  await highlight(page, selector);
  await page.waitForTimeout(200);
  await page.locator(selector).first().selectOption(value);
  await page.waitForTimeout(400);
  await clearHighlights(page);
}

// ---------------------------------------------------------------------------
// Full-screen diagram overlays
// ---------------------------------------------------------------------------

/** Show the architecture diagram as a full-screen overlay. */
async function showArchitectureDiagram(page: Page) {
  await page.evaluate(() => {
    const overlay = document.createElement('div');
    overlay.id = 'tour-arch-diagram';
    overlay.style.cssText = `
      position: fixed; top: 0; left: 0; right: 0; bottom: 0; z-index: 99999;
      background: linear-gradient(135deg, #001020 0%, #001830 100%);
      display: flex; flex-direction: column; align-items: center; justify-content: center;
      font-family: 'Inter', 'Segoe UI', system-ui, sans-serif;
      opacity: 0; transition: opacity 0.4s ease;
    `;
    overlay.innerHTML = `
      <div style="font-size:20px;color:#00d9ff;letter-spacing:2px;margin-bottom:28px;text-transform:uppercase;">System Architecture</div>
      <svg viewBox="0 0 800 380" width="1100" height="522" style="filter:drop-shadow(0 4px 20px rgba(0,217,255,0.15));">
        <defs>
          <filter id="glow" x="-50%" y="-50%" width="200%" height="200%">
            <feGaussianBlur stdDeviation="4" result="blur"/>
            <feMerge><feMergeNode in="blur"/><feMergeNode in="SourceGraphic"/></feMerge>
          </filter>
          <marker id="arrowhead" markerWidth="8" markerHeight="6" refX="8" refY="3" orient="auto">
            <path d="M0,0 L8,3 L0,6" fill="#00d9ff"/>
          </marker>
          <marker id="arrowhead-orange" markerWidth="8" markerHeight="6" refX="8" refY="3" orient="auto">
            <path d="M0,0 L8,3 L0,6" fill="#ff9500"/>
          </marker>
          <marker id="arrowhead-green" markerWidth="8" markerHeight="6" refX="8" refY="3" orient="auto">
            <path d="M0,0 L8,3 L0,6" fill="#44cc88"/>
          </marker>
          <marker id="arrowhead-purple" markerWidth="8" markerHeight="6" refX="8" refY="3" orient="auto">
            <path d="M0,0 L8,3 L0,6" fill="#aa66ff"/>
          </marker>
        </defs>
        <style>
          .diagram-part { transition: opacity 0.4s ease; }
        </style>

        <g id="arch-mobile" class="diagram-part">
          <rect x="20" y="150" width="130" height="70" rx="8" fill="#0a1929" stroke="#00d9ff" stroke-width="1.5"/>
          <text x="85" y="180" text-anchor="middle" fill="#cce8ff" font-size="13" font-weight="600">Mobile App</text>
          <text x="85" y="198" text-anchor="middle" fill="#6699bb" font-size="10">(Simulated)</text>
        </g>

        <g id="arch-arrows" class="diagram-part">
          <line x1="150" y1="185" x2="238" y2="185" stroke="#00d9ff" stroke-width="1.5" marker-end="url(#arrowhead)"/>
          <text x="194" y="175" text-anchor="middle" fill="#6699bb" font-size="9">HTTP POST</text>
          <line x1="440" y1="155" x2="538" y2="155" stroke="#ff9500" stroke-width="1.5" marker-end="url(#arrowhead-orange)"/>
          <text x="489" y="145" text-anchor="middle" fill="#6699bb" font-size="9">Analyze</text>
          <line x1="440" y1="275" x2="538" y2="275" stroke="#44cc88" stroke-width="1.5" marker-end="url(#arrowhead-green)"/>
          <line x1="420" y1="60" x2="538" y2="60" stroke="#aa66ff" stroke-width="1.5" marker-end="url(#arrowhead-purple)"/>
        </g>

        <g id="arch-server" class="diagram-part">
          <rect x="240" y="60" width="200" height="260" rx="10" fill="#0d2137" stroke="#00d9ff" stroke-width="2"/>
          <text x="340" y="90" text-anchor="middle" fill="#00d9ff" font-size="14" font-weight="700">App Server :8080</text>
          <rect x="260" y="105" width="160" height="35" rx="5" fill="#112a40" stroke="#335577" stroke-width="1"/>
          <text x="340" y="127" text-anchor="middle" fill="#88ccee" font-size="11">REST API /api/v1/*</text>
          <rect x="260" y="150" width="160" height="35" rx="5" fill="#112a40" stroke="#335577" stroke-width="1"/>
          <text x="340" y="172" text-anchor="middle" fill="#88ccee" font-size="11">UI Server /ui/*</text>
          <rect x="260" y="195" width="160" height="35" rx="5" fill="#112a40" stroke="#335577" stroke-width="1"/>
          <text x="340" y="212" text-anchor="middle" fill="#88ccee" font-size="10">Funding Service</text>
          <text x="340" y="224" text-anchor="middle" fill="#6699bb" font-size="9">State Machine • Rules</text>
          <rect x="260" y="240" width="160" height="35" rx="5" fill="#112a40" stroke="#335577" stroke-width="1"/>
          <text x="340" y="257" text-anchor="middle" fill="#88ccee" font-size="10">Settlement Engine</text>
          <text x="340" y="269" text-anchor="middle" fill="#6699bb" font-size="9">X9.37 ICL Generation</text>
          <rect x="260" y="285" width="160" height="25" rx="5" fill="#112a40" stroke="#335577" stroke-width="1"/>
          <text x="340" y="302" text-anchor="middle" fill="#88ccee" font-size="10">Ledger (Double-Entry)</text>
        </g>

        <g id="arch-vendor" class="diagram-part">
          <rect x="540" y="120" width="160" height="70" rx="8" fill="#0a1929" stroke="#ff9500" stroke-width="1.5"/>
          <text x="620" y="150" text-anchor="middle" fill="#ffbb44" font-size="13" font-weight="600">Vendor Stub :8081</text>
          <text x="620" y="170" text-anchor="middle" fill="#6699bb" font-size="10">IQA • MICR • Risk</text>
        </g>

        <g id="arch-sqlite" class="diagram-part">
          <rect x="540" y="240" width="160" height="60" rx="8" fill="#0a1929" stroke="#44cc88" stroke-width="1.5"/>
          <text x="620" y="266" text-anchor="middle" fill="#66eebb" font-size="13" font-weight="600">SQLite</text>
          <text x="620" y="284" text-anchor="middle" fill="#6699bb" font-size="10">Transfers • Ledger • Audit</text>
        </g>

        <g id="arch-icl" class="diagram-part">
          <rect x="540" y="40" width="160" height="50" rx="8" fill="#0a1929" stroke="#aa66ff" stroke-width="1.5"/>
          <text x="620" y="62" text-anchor="middle" fill="#cc88ff" font-size="12" font-weight="600">X9.37 ICL Files</text>
          <text x="620" y="78" text-anchor="middle" fill="#6699bb" font-size="10">Binary Settlement</text>
        </g>

        <rect id="focus-ring" visibility="hidden" rx="10" ry="10" fill="none" stroke="#ffd54f" stroke-width="3" vector-effect="non-scaling-stroke" filter="url(#glow)"/>
      </svg>
      <div style="margin-top:20px;color:#6699bb;font-size:11px;letter-spacing:0.5px;">
        Single Go binary (app) + separate vendor stub binary — mirrors production topology
      </div>
    `;
    document.body.appendChild(overlay);
    setTimeout(() => { overlay.style.opacity = '1'; }, 50);
  });
  await page.waitForTimeout(500); // wait for fade-in
}

/** Show the state machine diagram as a full-screen overlay. */
async function showStateMachineDiagram(page: Page) {
  await page.evaluate(() => {
    const overlay = document.createElement('div');
    overlay.id = 'tour-sm-diagram';
    overlay.style.cssText = `
      position: fixed; top: 0; left: 0; right: 0; bottom: 0; z-index: 99999;
      background: linear-gradient(135deg, #001020 0%, #001830 100%);
      display: flex; flex-direction: column; align-items: center; justify-content: center;
      font-family: 'Inter', 'Segoe UI', system-ui, sans-serif;
      opacity: 0; transition: opacity 0.4s ease;
    `;
    overlay.innerHTML = `
      <div style="font-size:20px;color:#00d9ff;letter-spacing:2px;margin-bottom:24px;text-transform:uppercase;">Transfer State Machine</div>
      <svg viewBox="0 0 910 380" width="1140" height="476" style="filter:drop-shadow(0 4px 20px rgba(0,217,255,0.15));">
        <defs>
          <filter id="sm-glow" x="-50%" y="-50%" width="200%" height="200%">
            <feGaussianBlur stdDeviation="4" result="blur"/>
            <feMerge><feMergeNode in="blur"/><feMergeNode in="SourceGraphic"/></feMerge>
          </filter>
          <marker id="sm-arrow" markerWidth="8" markerHeight="6" refX="8" refY="3" orient="auto">
            <path d="M0,0 L8,3 L0,6" fill="#00d9ff"/>
          </marker>
          <marker id="sm-arrow-green" markerWidth="8" markerHeight="6" refX="8" refY="3" orient="auto">
            <path d="M0,0 L8,3 L0,6" fill="#44cc88"/>
          </marker>
          <marker id="sm-arrow-red" markerWidth="8" markerHeight="6" refX="8" refY="3" orient="auto">
            <path d="M0,0 L8,3 L0,6" fill="#ff4444"/>
          </marker>
          <marker id="sm-arrow-orange" markerWidth="8" markerHeight="6" refX="8" refY="3" orient="auto">
            <path d="M0,0 L8,3 L0,6" fill="#ff9500"/>
          </marker>
          <marker id="sm-arrow-purple" markerWidth="8" markerHeight="6" refX="8" refY="3" orient="auto">
            <path d="M0,0 L8,3 L0,6" fill="#aa66ff"/>
          </marker>
        </defs>
        <style>
          .diagram-part { transition: opacity 0.4s ease; }
        </style>

        <g id="sm-happy" class="diagram-part">
          <rect x="20" y="155" width="100" height="40" rx="6" fill="#112a40" stroke="#00d9ff" stroke-width="1.5"/>
          <text x="70" y="180" text-anchor="middle" fill="#cce8ff" font-size="11" font-weight="600">Requested</text>
          <rect x="170" y="155" width="100" height="40" rx="6" fill="#112a40" stroke="#00d9ff" stroke-width="1.5"/>
          <text x="220" y="180" text-anchor="middle" fill="#cce8ff" font-size="11" font-weight="600">Validating</text>
          <rect x="320" y="155" width="100" height="40" rx="6" fill="#112a40" stroke="#00d9ff" stroke-width="1.5"/>
          <text x="370" y="180" text-anchor="middle" fill="#cce8ff" font-size="11" font-weight="600">Analyzing</text>
          <rect x="470" y="155" width="100" height="40" rx="6" fill="#112a40" stroke="#00d9ff" stroke-width="1.5"/>
          <text x="520" y="180" text-anchor="middle" fill="#cce8ff" font-size="11" font-weight="600">Approved</text>
          <rect x="620" y="155" width="110" height="40" rx="6" fill="#0d3320" stroke="#44cc88" stroke-width="1.5"/>
          <text x="675" y="180" text-anchor="middle" fill="#66eebb" font-size="11" font-weight="600">FundsPosted</text>
          <rect x="780" y="155" width="100" height="40" rx="6" fill="#0d3320" stroke="#44cc88" stroke-width="2"/>
          <text x="830" y="175" text-anchor="middle" fill="#66eebb" font-size="12" font-weight="700">✓</text>
          <text x="830" y="190" text-anchor="middle" fill="#66eebb" font-size="10">Completed</text>
          <line x1="120" y1="175" x2="168" y2="175" stroke="#00d9ff" stroke-width="1.5" marker-end="url(#sm-arrow)"/>
          <line x1="270" y1="175" x2="318" y2="175" stroke="#00d9ff" stroke-width="1.5" marker-end="url(#sm-arrow)"/>
          <line x1="420" y1="175" x2="468" y2="175" stroke="#00d9ff" stroke-width="1.5" marker-end="url(#sm-arrow)"/>
          <line x1="570" y1="175" x2="618" y2="175" stroke="#44cc88" stroke-width="1.5" marker-end="url(#sm-arrow-green)"/>
          <line x1="730" y1="175" x2="778" y2="175" stroke="#44cc88" stroke-width="1.5" marker-end="url(#sm-arrow-green)"/>
          <text x="145" y="166" text-anchor="middle" fill="#4488aa" font-size="8">submit</text>
          <text x="295" y="166" text-anchor="middle" fill="#4488aa" font-size="8">vendor</text>
          <text x="445" y="166" text-anchor="middle" fill="#4488aa" font-size="8">rules pass</text>
          <text x="595" y="166" text-anchor="middle" fill="#4488aa" font-size="8">ledger post</text>
          <text x="755" y="166" text-anchor="middle" fill="#4488aa" font-size="8">settle</text>
        </g>

        <g id="sm-rejected" class="diagram-part">
          <rect x="230" y="290" width="110" height="40" rx="6" fill="#2d1010" stroke="#ff4444" stroke-width="1.5"/>
          <text x="285" y="315" text-anchor="middle" fill="#ff6666" font-size="11" font-weight="600">✗ Rejected</text>
          <line x1="220" y1="195" x2="270" y2="288" stroke="#ff4444" stroke-width="1.2" stroke-dasharray="4,3" marker-end="url(#sm-arrow-red)"/>
          <text x="222" y="248" fill="#bb5555" font-size="8">vendor fail</text>
          <line x1="370" y1="195" x2="300" y2="288" stroke="#ff4444" stroke-width="1.2" stroke-dasharray="4,3" marker-end="url(#sm-arrow-red)"/>
          <text x="358" y="248" fill="#bb5555" font-size="8">rule fail</text>
        </g>

        <g id="sm-returned" class="diagram-part">
          <rect x="680" y="290" width="110" height="40" rx="6" fill="#2d1d00" stroke="#ff9500" stroke-width="1.5"/>
          <text x="735" y="315" text-anchor="middle" fill="#ffbb44" font-size="11" font-weight="600">↩ Returned</text>
          <line x1="675" y1="195" x2="720" y2="288" stroke="#ff9500" stroke-width="1.2" stroke-dasharray="4,3" marker-end="url(#sm-arrow-orange)"/>
          <line x1="830" y1="195" x2="750" y2="288" stroke="#ff9500" stroke-width="1.2" stroke-dasharray="4,3" marker-end="url(#sm-arrow-orange)"/>
          <text x="760" y="248" fill="#bb8833" font-size="8">bounced check</text>
        </g>

        <g id="sm-review" class="diagram-part">
          <rect x="320" y="55" width="100" height="32" rx="5" fill="#1a1a30" stroke="#aa66ff" stroke-width="1"/>
          <text x="370" y="71" text-anchor="middle" fill="#cc88ff" font-size="10">Operator</text>
          <text x="370" y="82" text-anchor="middle" fill="#cc88ff" font-size="10">Review</text>
          <path d="M370,155 L370,87" stroke="#aa66ff" stroke-width="1" stroke-dasharray="3,3" marker-end="url(#sm-arrow-purple)"/>
          <text x="392" y="125" fill="#9966cc" font-size="8">review</text>
        </g>

        <rect id="focus-ring" visibility="hidden" rx="8" ry="8" fill="none" stroke="#ffd54f" stroke-width="3" vector-effect="non-scaling-stroke" filter="url(#sm-glow)"/>
      </svg>
      <div style="display:flex;gap:24px;margin-top:18px;font-size:10px;color:#6699bb;">
        <span><span style="color:#00d9ff;">━━</span> Happy path</span>
        <span><span style="color:#ff4444;">╌╌</span> Vendor/rule failure</span>
        <span><span style="color:#ff9500;">╌╌</span> Bounced check (post-settlement)</span>
        <span><span style="color:#aa66ff;">╌╌</span> Operator review queue</span>
      </div>
    `;
    document.body.appendChild(overlay);
    setTimeout(() => { overlay.style.opacity = '1'; }, 50);
  });
  await page.waitForTimeout(500); // wait for fade-in
}

/** Remove a full-screen diagram overlay by ID. */
async function removeDiagram(page: Page, id: string) {
  await page.evaluate((id) => {
    const el = document.getElementById(id);
    if (el) {
      el.style.opacity = '0';
      setTimeout(() => el.remove(), 400);
    }
  }, id);
  await page.waitForTimeout(500);
}

/** Highlight one part of a diagram, dimming all others. */
async function highlightDiagramPart(page: Page, targetId: string, captionText: string, durationMs = 4000) {
  await page.evaluate(({ targetId }) => {
    const svg = document.querySelector('[id^="tour-"] svg') as SVGSVGElement;
    if (!svg) return;
    // Dim all parts, un-dim target
    svg.querySelectorAll('.diagram-part').forEach(el => {
      (el as SVGElement).style.opacity = el.id === targetId ? '1' : '0.2';
    });
    // Move focus ring
    const target = svg.querySelector(`#${targetId}`) as SVGGraphicsElement;
    const ring = svg.querySelector('#focus-ring') as SVGRectElement;
    if (target && ring) {
      const box = target.getBBox();
      const pad = 6;
      ring.setAttribute('x', String(box.x - pad));
      ring.setAttribute('y', String(box.y - pad));
      ring.setAttribute('width', String(box.width + pad * 2));
      ring.setAttribute('height', String(box.height + pad * 2));
      ring.setAttribute('visibility', 'visible');
      // Re-append to ensure it paints on top
      ring.parentElement?.appendChild(ring);
    }
  }, { targetId });
  await caption(page, captionText, durationMs);
}

/** Reset diagram highlighting — all parts fully visible, ring hidden. */
async function resetDiagramHighlight(page: Page) {
  await page.evaluate(() => {
    const svg = document.querySelector('[id^="tour-"] svg') as SVGSVGElement;
    if (!svg) return;
    svg.querySelectorAll('.diagram-part').forEach(el => {
      (el as SVGElement).style.opacity = '1';
    });
    const ring = svg.querySelector('#focus-ring') as SVGRectElement;
    if (ring) ring.setAttribute('visibility', 'hidden');
  });
  await clearCaption(page);
}

const SCENARIO_ACCOUNT_MAP: Record<string, string> = {
  'clean_pass': 'INV-1001',
  'iqa_blur': 'INV-1002',
  'iqa_glare': 'INV-1003',
  'micr_failure': 'INV-1004',
  'duplicate_detected': 'INV-1005',
  'amount_mismatch': 'INV-1006',
  'iqa_pass_review': 'INV-1007',
};

// Reusable submit helper
async function submitDeposit(
  page: Page,
  opts: { accountId?: string; amount?: string; scenario?: string } = {},
) {
  const scenario = opts.scenario ?? 'clean_pass';
  const accountId = opts.accountId ?? SCENARIO_ACCOUNT_MAP[scenario] ?? 'INV-1001';
  const amount = opts.amount ?? '500.00';

  await page.goto('/ui/simulate');
  await pause(page, 500);
  await page.locator('select[name="investorAccountId"]').selectOption({ value: accountId });
  await page.locator('input[name="amount"]').fill(amount);
  await page.locator('input[name="frontImage"]').setInputFiles(CHECK_FRONT);
  await page.locator('input[name="backImage"]').setInputFiles(CHECK_BACK);
  await page.locator('button[type="submit"]').click();
  await page.locator('[data-transfer-id]').waitFor();
  const transferId = await page.locator('[data-transfer-id]').getAttribute('data-transfer-id');
  return transferId!;
}

// ===========================================================================
// THE TOUR — single continuous test → one video file
// ===========================================================================

test.use({
  video: { mode: 'on', size: { width: 1920, height: 1080 } },
  viewport: { width: 1920, height: 1080 },
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
    await initCursor(page);
    await hideCursor(page);
    await announce(page, 'Mobile Check Deposit System', 'A complete deposit lifecycle for brokerage accounts');
    await caption(page,
      'Built in Go • SQLite • HTMX • X9.37 ICL settlement • 14 Go tests + 14 Playwright E2E tests',
      4000);
    await clearAll(page);

    // =======================================================================
    // SECTION 2 — ARCHITECTURE OVERVIEW  (~0:10)
    // =======================================================================
    await hideCursor(page);
    await showArchitectureDiagram(page);
    await caption(page,
      '① System Architecture — Two Go binaries working together to process mobile check deposits.',
      4000);
    await clearCaption(page);

    // Walk through each component
    await highlightDiagramPart(page, 'arch-mobile',
      'Mobile App — Simulated via the UI. Captures front/back check images and submits deposits via HTTP POST.', 4000);
    await highlightDiagramPart(page, 'arch-server',
      'App Server (:8080) — The core Go binary. Hosts the REST API, HTMX UI, funding service (state machine + business rules), settlement engine (X9.37 ICL), and double-entry ledger.', 5500);
    await highlightDiagramPart(page, 'arch-vendor',
      'Vendor Stub (:8081) — Separate Go binary simulating a third-party image analysis service. Returns IQA, MICR, and risk scoring results.', 4500);
    await highlightDiagramPart(page, 'arch-sqlite',
      'SQLite — Single-file database storing transfers, ledger entries, audit trail, and settlement batches.', 4000);
    await highlightDiagramPart(page, 'arch-icl',
      'X9.37 ICL Files — Real binary settlement files with proper record types and embedded check images. Generated by the settlement engine.', 4000);
    await resetDiagramHighlight(page);
    await removeDiagram(page, 'tour-arch-diagram');

    // Flash through every tab so the viewer sees the full UI surface
    await showCursor(page);
    await announce(page, 'Application Pages', '6 pages covering the full deposit lifecycle');
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
      await ensureCursor(page);
      await moveTo(page, `a.nav-level-tab:has-text("${tab}")`);
      await page.locator('a.nav-level-tab', { hasText: tab }).click();
      await ensureCursor(page);
      await caption(page, `${tab}: ${desc}`, 2500);
    }
    await clearCaption(page);

    // =======================================================================
    // SECTION 3 — STATE MACHINE EXPLANATION  (~0:50)
    // =======================================================================
    await hideCursor(page);
    await showStateMachineDiagram(page);
    await caption(page,
      '② Transfer State Machine — Every deposit follows this lifecycle. All transitions are enforced by a centralized validator.',
      4000);
    await clearCaption(page);

    // Walk through each path
    await highlightDiagramPart(page, 'sm-happy',
      'Happy Path (blue → green): Requested → Validating → Analyzing → Approved → FundsPosted → Completed. Six states, left to right.', 5500);
    await highlightDiagramPart(page, 'sm-rejected',
      'Rejected (red): Vendor failures (bad IQA, MICR errors) or business rule violations (over limit, duplicate) route here. Terminal state.', 5000);
    await highlightDiagramPart(page, 'sm-review',
      'Operator Review (purple): When the vendor returns REVIEW instead of PASS/FAIL, a human operator must approve or reject.', 4500);
    await highlightDiagramPart(page, 'sm-returned',
      'Returned (orange): Post-settlement bounced checks. Triggers ledger reversal of the original credit plus a $30 return fee.', 4500);
    await resetDiagramHighlight(page);
    await removeDiagram(page, 'tour-sm-diagram');
    await showCursor(page);

    // =======================================================================
    // SECTION 4 — HAPPY PATH: DEPOSIT SUBMISSION  (~1:00)
    // =======================================================================
    await announce(page, '③ Happy Path — Clean Pass Deposit', 'End-to-end: submit → auto-approve → post funds');
    await clearOverlay(page);

    await page.goto('/ui/simulate');
    await ensureCursor(page);
    await pause(page, 800);

    // Fill form with step-by-step cursor movements
    await caption(page, 'Selecting investor account INV-1001 — this account is mapped to a "Clean Pass" vendor scenario.', 3000);
    await cursorSelect(page, 'select[name="investorAccountId"]', 'INV-1001');

    await caption(page, 'Entering deposit amount: $500.00 (under the $5,000 per-deposit limit).', 3000);
    await cursorFill(page, 'input[name="amount"]', '500.00');

    await page.locator('input[name="frontImage"]').setInputFiles(CHECK_FRONT);
    await page.locator('input[name="backImage"]').setInputFiles(CHECK_BACK);
    await caption(page, 'Front and back check images uploaded. The system computes SHA256 hashes for duplicate detection.', 2500);
    await clearCaption(page);

    await caption(page, 'Submitting — this single API call triggers: image save → vendor call → 4 business rules → ledger posting.', 3500);
    await cursorClick(page, 'button[type="submit"]');

    await page.locator('[data-transfer-id]').waitFor();
    await ensureCursor(page);
    await highlight(page, '[data-state]');
    await announce(page, 'Result: FundsPosted',
      'Vendor PASS + all 4 rules pass → auto-approved → ledger posted → funds available');
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

    await cursorClick(page, 'a.nav-level-tab:has-text("Transfers")');
    await ensureCursor(page);
    await page.locator('[data-transfer]').first().waitFor();
    await caption(page, 'The Transfers page lists every deposit with its current state badge, amount, and business date.', 2500);
    await clearCaption(page);

    await cursorClick(page, '[data-transfer] a');
    await ensureCursor(page);
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
    await pause(page, 800);
    await caption(page, 'Check images: front and back stored on disk, served for operator review.', 3000);
    await clearCaption(page);

    // Vendor Result panel
    await page.evaluate(() => window.scrollBy(0, 350));
    await pause(page, 800);
    await caption(page,
      'Vendor Result: decision (PASS/FAIL/REVIEW), IQA status, MICR routing/account/serial, confidence score, risk score, amount match.',
      5000);
    await clearCaption(page);

    // Rule Evaluations panel
    await page.evaluate(() => window.scrollBy(0, 300));
    await pause(page, 800);
    await highlight(page, 'table');
    await caption(page,
      'Rule Evaluations: 4 business rules — account eligibility, $5K limit, contribution type, duplicate fingerprint. Each logged with pass/fail + details.',
      5000);
    await clearHighlights(page);
    await clearCaption(page);

    // Audit Trail panel
    await page.evaluate(() => window.scrollBy(0, 300));
    await pause(page, 800);
    await highlight(page, 'table');
    await caption(page,
      'Audit Trail: every state transition with timestamp, from/to state, actor, and event details. This is the complete decision trace.',
      5000);
    await clearHighlights(page);
    await clearCaption(page);
    await page.evaluate(() => window.scrollTo(0, 0));
    await pause(page, 800);

    // =======================================================================
    // SECTION 6 — LEDGER: DOUBLE-ENTRY BOOKKEEPING  (~2:45)
    // =======================================================================
    await announce(page, '⑤ Double-Entry Ledger', 'Every deposit creates balanced journal entries — credits and debits sum to zero');
    await clearOverlay(page);

    await cursorClick(page, 'a.nav-level-tab:has-text("Ledger")');
    await ensureCursor(page);
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

    await cursorClick(page, 'a.nav-level-tab:has-text("Settlement")');
    await ensureCursor(page);
    await pause(page, 1000);

    await caption(page,
      'Generate Batch: collects all FundsPosted deposits for the current business date and writes an X9.37 ICL binary file.',
      4000);
    await cursorClick(page, '[data-action="generate"]');
    await ensureCursor(page);

    await page.locator('[data-action="ack"]').first().waitFor();
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
    await cursorClick(page, '[data-action="ack"]');
    await ensureCursor(page);
    await pause(page, 1500);
    await clearCaption(page);

    // Verify Completed state
    await cursorClick(page, 'a.nav-level-tab:has-text("Transfers")');
    await ensureCursor(page);
    await cursorClick(page, '[data-transfer] a');
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
    // Navigate to review queue first so the section header appears in context
    await page.goto('/ui/review');
    await ensureCursor(page);
    await announce(page, '⑨ Operator Review — Approve Flow',
      'MICR failure routes to manual review queue for human decision');
    await clearOverlay(page);

    await caption(page,
      'When the vendor returns REVIEW (instead of PASS or FAIL), the transfer stays in Analyzing with review_required=true. It appears in the operator queue.',
      5000);
    await clearCaption(page);

    const micrTransferId = await submitDeposit(page, {
      accountId: 'INV-1001', amount: '800.00', scenario: 'micr_failure',
    });
    await ensureCursor(page);
    await highlight(page, '[data-state]');
    await caption(page,
      'State: Analyzing — the deposit is waiting for an operator to review the MICR data and make a decision.',
      3500);
    await clearHighlights(page);
    await clearCaption(page);

    // Navigate to review queue
    await cursorClick(page, 'a.nav-level-tab:has-text("Review Queue")');
    await ensureCursor(page);
    await page.locator('[data-review-item]').first().waitFor();
    await highlight(page, '[data-review-item]');
    await caption(page,
      'Review Queue: shows flagged deposits with amount, account, creation time, and a Review button.',
      4000);
    await clearHighlights(page);
    await clearCaption(page);

    // Click review
    await cursorClick(page, '[data-review-item] a.btn:has-text("Review")');
    await ensureCursor(page);
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
      if (el) el.scrollIntoView({ block: 'center', behavior: 'smooth' });
    });
    await pause(page, 500);
    await clearCaption(page);

    await caption(page,
      'Adding operator notes. Every operator action is logged in the operator_actions table and audit trail.',
      3500);
    await cursorFill(page, '#approve-notes', 'MICR readable on manual inspection — approved');
    await clearCaption(page);

    await cursorClick(page, '[data-action="approve"]');
    await ensureCursor(page);

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
      'Amount mismatch: the vendor\'s OCR reads a different dollar amount than what the investor entered. This is flagged for review.',
      4000);
    await clearCaption(page);

    await submitDeposit(page, { accountId: 'INV-1006', amount: '450.00', scenario: 'amount_mismatch' });
    await ensureCursor(page);
    await pause(page, 1000);

    await cursorClick(page, 'a.nav-level-tab:has-text("Review Queue")');
    await ensureCursor(page);
    await page.locator('[data-review-item]').first().waitFor();
    await cursorClick(page, '[data-review-item] a.btn:has-text("Review")');
    await ensureCursor(page);
    await pause(page, 1000);

    await page.evaluate(() => {
      const el = document.querySelector('[data-action="reject"]');
      if (el) el.scrollIntoView({ block: 'center', behavior: 'smooth' });
    });
    await pause(page, 500);

    await caption(page,
      'Rejecting with notes. Transfer goes to Rejected state — no ledger posting, no settlement.',
      3500);
    await cursorFill(page, '#reject-notes', 'Amount discrepancy too large — rejecting');
    await clearCaption(page);

    await cursorClick(page, '[data-action="reject"]');
    await ensureCursor(page);
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
      'iqa_pass_review: images pass quality checks, but MICR confidence is low and risk score is high. Operator decides.',
      4500);
    await clearCaption(page);

    await submitDeposit(page, { accountId: 'INV-1007', amount: '900.00', scenario: 'iqa_pass_review' });
    await ensureCursor(page);
    await highlight(page, '[data-state]');
    await pause(page, 1500);
    await clearHighlights(page);

    await cursorClick(page, 'a.nav-level-tab:has-text("Review Queue")');
    await ensureCursor(page);
    await page.locator('[data-review-item]').first().waitFor();
    await pause(page, 1000);
    await cursorClick(page, '[data-review-item] a.btn:has-text("Review")');
    await ensureCursor(page);
    await pause(page, 1000);
    await caption(page,
      'Review detail shows vendor analysis and risk factors. Operator reviews all context before deciding.',
      3000);
    await clearCaption(page);

    await page.evaluate(() => {
      const el = document.querySelector('[data-action="approve"]');
      if (el) el.scrollIntoView({ block: 'center', behavior: 'smooth' });
    });
    await pause(page, 500);

    await cursorFill(page, '#approve-notes', 'High-risk but legitimate after manual review');

    await cursorClick(page, '[data-action="approve"]');
    await ensureCursor(page);
    await pause(page, 1500);
    await caption(page, 'Approved. This deposit is now FundsPosted and eligible for the next settlement batch.', 3500);
    await clearCaption(page);

    // =======================================================================
    // SECTION 13 — TRANSFERS OVERVIEW: ALL STATES  (~8:45)
    // =======================================================================
    await announce(page, '⑫ Transfers Overview', 'All deposits across the full range of outcomes');
    await clearOverlay(page);

    await cursorClick(page, 'a.nav-level-tab:has-text("Transfers")');
    await ensureCursor(page);
    await page.locator('[data-transfer]').first().waitFor();
    await caption(page,
      'Multiple deposit states visible: Completed, FundsPosted, Analyzing, Rejected. Each row clickable for full detail.',
      4500);
    await clearCaption(page);
    await pause(page, 1500);
    await page.evaluate(() => window.scrollBy(0, 300));
    await pause(page, 2000);
    await page.evaluate(() => window.scrollTo(0, 0));
    await pause(page, 800);

    // =======================================================================
    // SECTION 14 — SETTLEMENT ROUND 2  (~9:15)
    // =======================================================================
    await announce(page, '⑬ Second Settlement Batch',
      'Settling the operator-approved deposits from the review workflows');
    await clearOverlay(page);

    await cursorClick(page, 'a.nav-level-tab:has-text("Settlement")');
    await ensureCursor(page);
    await pause(page, 1000);

    await caption(page,
      'Generate a second batch: only FundsPosted deposits are included. Rejected deposits are excluded.',
      4000);
    await cursorClick(page, '[data-action="generate"]');
    await ensureCursor(page);
    await page.locator('[data-action="ack"]').first().waitFor();
    await pause(page, 1500);

    await caption(page,
      'Two batches now visible. Each batch is a separate X9.37 ICL file with its own items and totals.',
      4000);
    await clearCaption(page);

    await cursorClick(page, '[data-action="ack"]');
    await ensureCursor(page);
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
    await cursorClick(page, 'a.nav-level-tab:has-text("Transfers")');
    await ensureCursor(page);
    await pause(page, 500);
    const completedTransferId = await page.locator('[data-transfer]').first()
      .locator('a').first().getAttribute('href')
      .then((href) => href?.split('/').pop() ?? '');

    await cursorClick(page, 'a.nav-level-tab:has-text("Returns")');
    await ensureCursor(page);
    await pause(page, 1000);

    await caption(page, 'Enter the transfer ID and select a reason code (NSF, ACCOUNT_CLOSED, STOP_PAYMENT, or FRAUD).', 3500);
    await cursorFill(page, 'input[name="transferId"]', completedTransferId);

    await cursorSelect(page, 'select[name="reasonCode"]', 'NSF');

    await cursorClick(page, 'button[type="submit"]');

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

    await cursorClick(page, 'a.nav-level-tab:has-text("Ledger")');
    await ensureCursor(page);
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

    await cursorClick(page, 'a.nav-level-tab:has-text("Transfers")');
    await ensureCursor(page);
    await cursorClick(page, '[data-transfer] a');
    await ensureCursor(page);
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
      if (auditTable) auditTable.scrollIntoView({ block: 'start', behavior: 'smooth' });
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
        position: fixed; z-index: 99997;
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
    await pause(page, 800);

    // =======================================================================
    // SECTION 18 — SECOND RETURN (FRAUD)  (~12:15)
    // =======================================================================
    await announce(page, '⑰ Second Return — FRAUD Reason',
      'Demonstrating different reason codes');
    await clearOverlay(page);

    // Submit a new deposit, settle it, then return with FRAUD
    const fraudTransferId = await submitDeposit(page, {
      accountId: 'INV-1001', amount: '275.00', scenario: 'clean_pass',
    });
    await pause(page, 500);

    await cursorClick(page, 'a.nav-level-tab:has-text("Settlement")');
    await ensureCursor(page);
    await cursorClick(page, '[data-action="generate"]');
    await ensureCursor(page);
    await page.locator('[data-action="ack"]').first().waitFor();
    await cursorClick(page, '[data-action="ack"]');
    await ensureCursor(page);
    await pause(page, 1000);

    await cursorClick(page, 'a.nav-level-tab:has-text("Returns")');
    await ensureCursor(page);
    // Fill form FIRST so dropdown shows FRAUD, then show caption
    await cursorFill(page, 'input[name="transferId"]', fraudTransferId);
    await cursorSelect(page, 'select[name="reasonCode"]', 'FRAUD');
    await caption(page, 'Processing a FRAUD return on a different completed deposit.', 3000);
    await clearCaption(page);
    await cursorClick(page, 'button[type="submit"]');
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
          h.closest('.panel')?.scrollIntoView({ block: 'center', behavior: 'smooth' });
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

    await cursorClick(page, 'a.nav-level-tab:has-text("Transfers")');
    await ensureCursor(page);
    await page.locator('[data-transfer]').first().waitFor();
    await caption(page,
      'Completed (settled), FundsPosted (awaiting settlement), Rejected (vendor/rule failure), Returned (bounced check).',
      5000);
    await clearCaption(page);

    await pause(page, 1000);
    await page.evaluate(() => window.scrollBy(0, 300));
    await pause(page, 1500);
    await page.evaluate(() => window.scrollTo(0, 0));
    await pause(page, 800);

    // =======================================================================
    // SECTION 20 — DESIGN DECISIONS  (~14:00)
    // =======================================================================
    // Show the ledger as a meaningful background for design decisions
    await cursorClick(page, 'a.nav-level-tab:has-text("Ledger")');
    await ensureCursor(page);
    await pause(page, 500);
    await hideCursor(page);

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
    await hideCursor(page);
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
