import * as fs from 'fs';
import { test, expect, CHECK_FRONT, CHECK_BACK } from './fixtures';

test.describe('Business Rules', () => {
  test('deposit over $5,000 is rejected', async ({ page }) => {
    await page.goto('/ui/simulate');
    await page.locator('select[name="investorAccountId"]').selectOption({ value: 'INV-1001' });
    await page.locator('input[name="amount"]').fill('5500.00');
    await page.locator('input[name="frontImage"]').setInputFiles(CHECK_FRONT);
    await page.locator('input[name="backImage"]').setInputFiles(CHECK_BACK);
    await page.locator('button[type="submit"]').click();

    await expect(page.locator('body')).toContainText(/rejected|limit|exceed/i);
  });

  test('transfer detail shows 5 rule evaluations including DAILY_DEPOSIT_LIMIT', async ({ page, request }) => {
    // Submit a clean deposit using the proper check image fixtures
    const resp = await request.post('/api/v1/deposits', {
      multipart: {
        investorAccountId: 'INV-1001',
        amount: '100.00',
        frontImage: { name: 'front.jpg', mimeType: 'image/jpeg', buffer: fs.readFileSync(CHECK_FRONT) },
        backImage: { name: 'back.jpg', mimeType: 'image/jpeg', buffer: fs.readFileSync(CHECK_BACK) },
      },
    });
    expect(resp.ok()).toBeTruthy();
    const deposit = await resp.json();
    const transferId = deposit.transferId;
    expect(transferId).toBeTruthy();

    await page.goto(`/ui/transfers/${transferId}`);
    // Wait for rule evaluations section (appears once vendor analysis completes)
    await page.waitForSelector('.panel:has-text("Rule Evaluations")', { timeout: 20000 });
    const rows = page.locator('.panel:has-text("Rule Evaluations") tbody tr');
    await expect(rows).toHaveCount(5);

    // Verify the daily limit rule appears and passes
    const dailyRow = rows.filter({ hasText: 'DAILY_DEPOSIT_LIMIT' });
    await expect(dailyRow).toHaveCount(1);
    await expect(dailyRow.locator('.badge')).toContainText('PASS');
  });
});
