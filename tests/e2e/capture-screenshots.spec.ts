import { test as base, expect } from '@playwright/test';
import * as path from 'path';

const screenshotDir = path.resolve(__dirname, '../../docs/screenshots');
const CHECK_FRONT = path.join(__dirname, 'tests', 'check-front.png');
const CHECK_BACK = path.join(__dirname, 'tests', 'check-back.png');

// Use base test without the auto-reset fixture so we can accumulate data
const test = base;

async function submitViaUI(page: any, accountId: string, amount: string) {
  await page.goto('/ui/simulate');
  await page.locator('select[name="investorAccountId"]').selectOption({ value: accountId });
  await page.locator('input[name="amount"]').fill(amount);
  // Uncheck sample images to enable file upload
  const sampleChk = page.locator('input[name="useSampleImages"]');
  if (await sampleChk.isChecked()) await sampleChk.uncheck();
  await page.locator('input[name="frontImage"]').setInputFiles(CHECK_FRONT);
  await page.locator('input[name="backImage"]').setInputFiles(CHECK_BACK);
  await page.locator('button[type="submit"]').click();
  await page.waitForLoadState('networkidle');
  const transferId = await page.locator('[data-transfer-id]').getAttribute('data-transfer-id');
  return transferId;
}

test.describe('Screenshot Gallery', () => {
  test('capture all UI pages', async ({ page, request }) => {
    // Reset state first
    await request.post('/api/v1/test/reset');

    // 1. Deposit Simulator (empty)
    await page.goto('/ui/simulate');
    await page.waitForLoadState('networkidle');
    await page.screenshot({ path: path.join(screenshotDir, '01-deposit-simulator.png'), fullPage: true });

    // 2. Submit a clean_pass deposit (INV-1001)
    const transferId1 = await submitViaUI(page, 'INV-1001', '250.00');
    await page.screenshot({ path: path.join(screenshotDir, '02-deposit-result.png'), fullPage: true });

    // 3. Submit a review scenario (INV-1007)
    await submitViaUI(page, 'INV-1007', '500.00');

    // 4. Submit a rejection scenario (INV-1002)
    await submitViaUI(page, 'INV-1002', '100.00');

    // 5. Transfers list
    await page.goto('/ui/transfers');
    await page.waitForLoadState('networkidle');
    await page.screenshot({ path: path.join(screenshotDir, '03-transfers-list.png'), fullPage: true });

    // 6. Transfer detail
    await page.goto(`/ui/transfers/${transferId1}`);
    await page.waitForLoadState('networkidle');
    await page.screenshot({ path: path.join(screenshotDir, '04-transfer-detail.png'), fullPage: true });

    // 7. Operator review queue
    await page.goto('/ui/review');
    await page.waitForLoadState('networkidle');
    await page.screenshot({ path: path.join(screenshotDir, '05-operator-review.png'), fullPage: true });

    // 8. Ledger
    await page.goto('/ui/ledger');
    await page.waitForLoadState('networkidle');
    await page.screenshot({ path: path.join(screenshotDir, '06-ledger.png'), fullPage: true });

    // 9. Settlement - generate a batch
    await page.goto('/ui/settlement');
    await page.waitForLoadState('networkidle');
    await page.locator('[data-action="generate"]').click();
    await page.waitForLoadState('networkidle');
    await page.screenshot({ path: path.join(screenshotDir, '07-settlement.png'), fullPage: true });

    // 10. Returns
    await page.goto('/ui/returns');
    await page.waitForLoadState('networkidle');
    await page.screenshot({ path: path.join(screenshotDir, '08-returns.png'), fullPage: true });
  });
});
