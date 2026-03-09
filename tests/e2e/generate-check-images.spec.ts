/**
 * Generate realistic placeholder check images (front + back) using Playwright.
 * Run: npx playwright test generate-check-images.ts
 * Output: tests/e2e/tests/check-front.png, tests/e2e/tests/check-back.png
 *         + defect variants: blurry, glare, no-micr, wrong-amount
 */
import { test, chromium } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';

const FRONT_HTML = `
<html>
<body style="margin:0;padding:0;background:#f8f6f0;">
<div style="
  width:600px; height:260px;
  background: linear-gradient(175deg, #f8f6f0 0%, #ede9df 100%);
  border: 2px solid #b8a88a;
  border-radius: 4px;
  font-family: 'Courier New', monospace;
  position: relative;
  overflow: hidden;
  box-sizing: border-box;
">
  <!-- Subtle security pattern -->
  <div style="position:absolute;top:0;left:0;right:0;bottom:0;
    background-image: repeating-linear-gradient(45deg, transparent, transparent 10px, rgba(180,160,130,0.06) 10px, rgba(180,160,130,0.06) 11px);
    pointer-events:none;"></div>

  <!-- Bank name -->
  <div style="position:absolute;top:12px;left:20px;">
    <div style="font-size:14px;font-weight:bold;color:#2a3a5c;letter-spacing:1px;">FIRST NATIONAL BANK</div>
    <div style="font-size:9px;color:#6a7a9c;margin-top:2px;">1200 Financial Plaza, New York, NY 10001</div>
  </div>

  <!-- Check number -->
  <div style="position:absolute;top:12px;right:20px;font-size:13px;color:#4a5a7c;">1042</div>

  <!-- Date -->
  <div style="position:absolute;top:50px;right:20px;">
    <span style="font-size:10px;color:#6a7a9c;">DATE</span>
    <span style="font-size:12px;color:#2a3a5c;border-bottom:1px solid #b8a88a;padding:0 8px;margin-left:4px;">03/07/2026</span>
  </div>

  <!-- Pay to -->
  <div style="position:absolute;top:80px;left:20px;right:20px;">
    <div style="font-size:9px;color:#6a7a9c;">PAY TO THE ORDER OF</div>
    <div style="font-size:13px;color:#2a3a5c;border-bottom:1px solid #b8a88a;padding:4px 0;margin-top:2px;">
      Apex Clearing Corporation
    </div>
  </div>

  <!-- Amount box -->
  <div style="position:absolute;top:75px;right:20px;
    border:1.5px solid #b8a88a;padding:4px 10px;background:#fff;font-size:14px;color:#2a3a5c;font-weight:bold;">
    $500.00
  </div>

  <!-- Written amount -->
  <div style="position:absolute;top:125px;left:20px;right:20px;">
    <div style="font-size:11px;color:#2a3a5c;border-bottom:1px solid #b8a88a;padding:4px 0;">
      Five Hundred and 00/100 ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ DOLLARS
    </div>
  </div>

  <!-- Bank info bottom -->
  <div style="position:absolute;bottom:45px;left:20px;">
    <div style="font-size:9px;color:#6a7a9c;">MEMO</div>
    <div style="font-size:10px;color:#2a3a5c;border-bottom:1px solid #b8a88a;width:180px;padding:2px 0;">
      Investment deposit
    </div>
  </div>

  <!-- Signature line -->
  <div style="position:absolute;bottom:40px;right:20px;">
    <div style="border-bottom:1px solid #b8a88a;width:180px;height:20px;"></div>
    <div style="font-size:8px;color:#6a7a9c;text-align:center;margin-top:2px;">AUTHORIZED SIGNATURE</div>
  </div>

  <!-- MICR line -->
  <div style="position:absolute;bottom:8px;left:20px;right:20px;
    font-family:'MICR','Courier New',monospace;font-size:12px;color:#3a4a6c;letter-spacing:3px;">
    ⑈021000089⑈ ⑆1001042⑆ 7829104538⑈
  </div>
</div>
</body>
</html>
`;

const BACK_HTML = `
<html>
<body style="margin:0;padding:0;background:#f8f6f0;">
<div style="
  width:600px; height:260px;
  background: linear-gradient(175deg, #f8f6f0 0%, #ede9df 100%);
  border: 2px solid #b8a88a;
  border-radius: 4px;
  font-family: 'Courier New', monospace;
  position: relative;
  overflow: hidden;
  box-sizing: border-box;
">
  <!-- Security pattern -->
  <div style="position:absolute;top:0;left:0;right:0;bottom:0;
    background-image: repeating-linear-gradient(45deg, transparent, transparent 10px, rgba(180,160,130,0.06) 10px, rgba(180,160,130,0.06) 11px);
    pointer-events:none;"></div>

  <!-- Endorsement area -->
  <div style="position:absolute;top:15px;left:30px;right:30px;">
    <div style="font-size:10px;color:#6a7a9c;text-transform:uppercase;letter-spacing:1px;margin-bottom:8px;">
      Endorse Here
    </div>
    <div style="border-bottom:1px solid #b8a88a;height:22px;margin-bottom:6px;"></div>
    <div style="border-bottom:1px solid #b8a88a;height:22px;margin-bottom:6px;"></div>
    <div style="border-bottom:1px solid #b8a88a;height:22px;margin-bottom:10px;"></div>
    <div style="font-size:8px;color:#9aaa9c;text-align:center;border-top:2px solid #b8a88a;padding-top:4px;">
      DO NOT WRITE, STAMP, OR SIGN BELOW THIS LINE
    </div>
  </div>

  <!-- For mobile deposit text -->
  <div style="position:absolute;bottom:60px;left:30px;right:30px;text-align:center;">
    <div style="font-size:11px;color:#4a5a7c;font-style:italic;">
      "For Mobile Deposit Only"
    </div>
    <div style="font-size:10px;color:#6a7a9c;margin-top:4px;">
      Account #7829104538
    </div>
  </div>

  <!-- Bank processing stamp area -->
  <div style="position:absolute;bottom:15px;left:30px;right:30px;">
    <div style="font-size:7px;color:#aabbaa;text-align:center;letter-spacing:2px;">
      FINANCIAL INSTITUTION USE ONLY
    </div>
  </div>
</div>
</body>
</html>
`;

function applyBlur(html: string): string {
  return html.replace(
    'overflow: hidden;',
    'overflow: hidden; filter: blur(3px);'
  );
}

function applyGlare(html: string): string {
  // Two overlapping hotspots to create a harsh, blown-out glare that obscures text
  const glareDiv = `
    <div style="position:absolute;top:10%;left:20%;width:70%;height:60%;
      background: radial-gradient(ellipse, rgba(255,255,255,1) 0%, rgba(255,255,255,0.95) 25%, rgba(255,255,255,0.6) 50%, transparent 75%);
      pointer-events:none;transform:rotate(-10deg);z-index:999;"></div>
    <div style="position:absolute;top:25%;left:40%;width:40%;height:35%;
      background: radial-gradient(ellipse, rgba(255,255,255,1) 0%, rgba(255,255,255,0.8) 40%, transparent 70%);
      pointer-events:none;transform:rotate(5deg);z-index:999;"></div>`;
  return html.replace('</div>\n</body>', glareDiv + '\n</div>\n</body>');
}

function removeMICR(html: string): string {
  return html.replace(
    '⑈021000089⑈ ⑆1001042⑆ 7829104538⑈',
    ''
  );
}

function wrongAmount(html: string): string {
  return html
    .replace('$500.00', '$750.00')
    .replace('Five Hundred and 00/100', 'Seven Hundred Fifty and 00/100');
}

test('generate check images', async () => {
  const browser = await chromium.launch();
  const context = await browser.newContext({
    viewport: { width: 620, height: 280 },
    deviceScaleFactor: 2,
  });

  const outDir = path.join(__dirname, 'tests');
  fs.mkdirSync(outDir, { recursive: true });

  // Front
  const frontPage = await context.newPage();
  await frontPage.setContent(FRONT_HTML, { waitUntil: 'load' });
  await frontPage.locator('div').first().screenshot({
    path: path.join(outDir, 'check-front.png'),
  });

  // Back
  const backPage = await context.newPage();
  await backPage.setContent(BACK_HTML, { waitUntil: 'load' });
  await backPage.locator('div').first().screenshot({
    path: path.join(outDir, 'check-back.png'),
  });

  // Defect variants
  const variants: Array<{name: string; transformFront: (h: string) => string; transformBack?: (h: string) => string}> = [
    { name: 'blurry', transformFront: applyBlur, transformBack: applyBlur },
    { name: 'glare', transformFront: applyGlare, transformBack: applyGlare },
    { name: 'no-micr', transformFront: removeMICR },
    { name: 'wrong-amount', transformFront: wrongAmount },
  ];

  for (const variant of variants) {
    const vFrontHTML = variant.transformFront(FRONT_HTML);
    const vBackHTML = variant.transformBack ? variant.transformBack(BACK_HTML) : BACK_HTML;

    const vFrontPage = await context.newPage();
    await vFrontPage.setContent(vFrontHTML, { waitUntil: 'load' });
    await vFrontPage.locator('div').first().screenshot({
      path: path.join(outDir, `check-front-${variant.name}.png`),
    });

    const vBackPage = await context.newPage();
    await vBackPage.setContent(vBackHTML, { waitUntil: 'load' });
    await vBackPage.locator('div').first().screenshot({
      path: path.join(outDir, `check-back-${variant.name}.png`),
    });
  }

  await browser.close();
});
