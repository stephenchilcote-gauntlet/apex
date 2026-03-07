import { test, expect, CHECK_FRONT, CHECK_BACK } from './fixtures';

test.describe('Business Rules', () => {
  test('deposit over $5,000 is rejected', async ({ page }) => {
    await page.goto('/ui/simulate');
    await page.locator('select[name="investorAccountId"]').selectOption({ value: 'INV-1001' });
    await page.locator('input[name="amount"]').fill('5500.00');
    await page.locator('input[name="frontImage"]').setInputFiles(CHECK_FRONT);
    await page.locator('input[name="backImage"]').setInputFiles(CHECK_BACK);
    await page.locator('select[name="vendorScenario"]').selectOption('clean_pass');
    await page.locator('button[type="submit"]').click();

    await expect(page.locator('body')).toContainText(/rejected|limit|exceed/i);
  });
});
