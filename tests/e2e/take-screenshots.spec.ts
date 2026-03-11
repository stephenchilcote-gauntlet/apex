/**
 * Screenshot regenerator — captures fresh screenshots for docs/screenshots/
 * Run: cd tests/e2e && npx playwright test take-screenshots.spec.ts
 */

import { test } from '@playwright/test';
import * as path from 'path';
import * as fs from 'fs';

const DOCS = path.resolve(__dirname, '../../docs/screenshots');

test.use({ viewport: { width: 1440, height: 900 } });

test.describe('Screenshot Regenerator', () => {
  test.setTimeout(120_000);

  test('capture all pages', async ({ page, request }) => {
    fs.mkdirSync(DOCS, { recursive: true });

    // Reset to clean state and seed demo data
    await request.post('/api/v1/test/reset').catch(() => {});
    await request.post('/api/v1/test/seed').catch(() => {});

    // Submit a real clean-pass deposit so the ledger and transfer detail
    // show actual journal entries (seed data alone doesn't create them)
    const frontBuf = fs.readFileSync(path.join(__dirname, 'tests', 'check-front.png'));
    const backBuf  = fs.readFileSync(path.join(__dirname, 'tests', 'check-back.png'));
    const depositResp = await request.post('/api/v1/deposits', {
      multipart: {
        investorAccountId: 'INV-1001',
        amount: '750.00',
        vendorScenario: 'clean_pass',
        frontImage: { name: 'front.png', mimeType: 'image/png', buffer: frontBuf },
        backImage:  { name: 'back.png',  mimeType: 'image/png', buffer: backBuf },
      },
    }).catch(() => null);
    const depositJson = depositResp ? await depositResp.json().catch(() => null) : null;
    const liveTransferId = depositJson?.transferId ?? null;

    // Get IDs we'll need
    const depositsResp = await request.get('/api/v1/deposits?limit=50');
    const deposits = (await depositsResp.json()) as any[];
    const completed = deposits.find((d: any) => d.State === 'Completed' || d.State === 'FundsPosted');
    const analyzing = deposits.find((d: any) => d.State === 'Analyzing' && d.ReviewRequired);
    // Prefer the live FundsPosted transfer for transfer detail screenshots
    const transferId = liveTransferId ?? completed?.ID ?? deposits[0]?.ID;
    const reviewId = analyzing?.ID;

    // 01 Dashboard
    await page.goto('/ui');
    await page.waitForLoadState('networkidle');
    await page.screenshot({ path: path.join(DOCS, '01-dashboard.png'), fullPage: false });
    console.log('01-dashboard.png ✓');

    // 01 Deposit Simulator
    await page.goto('/ui/simulate');
    await page.waitForLoadState('networkidle');
    await page.screenshot({ path: path.join(DOCS, '01-deposit-simulator.png'), fullPage: false });
    console.log('01-deposit-simulator.png ✓');

    // 02 Deposit Result (transfer detail of a completed transfer)
    if (transferId) {
      await page.goto(`/ui/transfers/${transferId}`);
      await page.waitForLoadState('networkidle');
      await page.screenshot({ path: path.join(DOCS, '02-deposit-result.png'), fullPage: false });
      console.log('02-deposit-result.png ✓');
    }

    // 03 Transfers List
    await page.goto('/ui/transfers');
    await page.waitForLoadState('networkidle');
    await page.screenshot({ path: path.join(DOCS, '03-transfers-list.png'), fullPage: false });
    console.log('03-transfers-list.png ✓');

    // 04 Transfer Detail
    if (transferId) {
      await page.goto(`/ui/transfers/${transferId}`);
      await page.waitForLoadState('networkidle');
      await page.evaluate(() => window.scrollBy(0, 300));
      await page.waitForTimeout(300);
      await page.screenshot({ path: path.join(DOCS, '04-transfer-detail.png'), fullPage: false });
      console.log('04-transfer-detail.png ✓');
    }

    // 05 Operator Review
    if (reviewId) {
      await page.goto(`/ui/review/${reviewId}`);
      await page.waitForLoadState('networkidle');
      await page.screenshot({ path: path.join(DOCS, '05-operator-review.png'), fullPage: false });
      console.log('05-operator-review.png ✓');
    } else {
      await page.goto('/ui/review');
      await page.waitForLoadState('networkidle');
      await page.screenshot({ path: path.join(DOCS, '05-operator-review.png'), fullPage: false });
      console.log('05-operator-review.png (queue) ✓');
    }

    // 06 Ledger
    await page.goto('/ui/ledger');
    await page.waitForLoadState('networkidle');
    await page.screenshot({ path: path.join(DOCS, '06-ledger.png'), fullPage: false });
    console.log('06-ledger.png ✓');

    // 07 Settlement
    await page.goto('/ui/settlement');
    await page.waitForLoadState('networkidle');
    await page.screenshot({ path: path.join(DOCS, '07-settlement.png'), fullPage: false });
    console.log('07-settlement.png ✓');

    // 08 Returns
    await page.goto('/ui/returns');
    await page.waitForLoadState('networkidle');
    await page.screenshot({ path: path.join(DOCS, '08-returns.png'), fullPage: false });
    console.log('08-returns.png ✓');

    console.log(`\nAll screenshots saved to ${DOCS}/`);
  });
});
