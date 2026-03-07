import { test, expect } from './fixtures';

async function submitDeposit(page: any, accountId: string, scenario: string, amount = '500.00') {
  await page.goto('/ui/simulate');
  await page.locator('select[name="investorAccountId"]').selectOption({ value: accountId });
  await page.locator('input[name="amount"]').fill(amount);
  await page.locator('input[name="frontImage"]').setInputFiles({
    name: 'front.png', mimeType: 'image/png', buffer: Buffer.from('fake-front')
  });
  await page.locator('input[name="backImage"]').setInputFiles({
    name: 'back.png', mimeType: 'image/png', buffer: Buffer.from('fake-back')
  });
  await page.locator('select[name="vendorScenario"]').selectOption(scenario);
  await page.locator('button[type="submit"]').click();
}

test.describe('Vendor Stub Scenarios', () => {
  test('IQA Blur results in rejection with retake message', async ({ page }) => {
    await submitDeposit(page, 'INV-1001', 'iqa_blur');

    await expect(page.locator('body')).toContainText(/rejected/i);
    await expect(page.locator('body')).toContainText(/blur|retake|resubmit/i);
  });

  test('IQA Glare results in rejection with retake message', async ({ page }) => {
    await submitDeposit(page, 'INV-1001', 'iqa_glare');

    await expect(page.locator('body')).toContainText(/rejected/i);
    await expect(page.locator('body')).toContainText(/glare|retake|resubmit/i);
  });

  test('MICR failure routes to review queue', async ({ page }) => {
    await submitDeposit(page, 'INV-1001', 'micr_failure');

    await expect(page.locator('body')).toContainText(/analyzing|review/i);

    // Check it appears in review queue
    await page.goto('/ui/review');
    await expect(page.locator('table tbody tr, [data-review-item]')).toHaveCount(1, { timeout: 5000 });
    await expect(page.locator('body')).toContainText(/micr/i);
  });

  test('Duplicate detected results in rejection', async ({ page }) => {
    await submitDeposit(page, 'INV-1001', 'duplicate_detected');

    await expect(page.locator('body')).toContainText(/rejected/i);
    await expect(page.locator('body')).toContainText(/duplicate/i);
  });

  test('Amount mismatch routes to review queue', async ({ page }) => {
    await submitDeposit(page, 'INV-1001', 'amount_mismatch');

    await expect(page.locator('body')).toContainText(/analyzing|review/i);

    // Check it appears in review queue with amount comparison
    await page.goto('/ui/review');
    await expect(page.locator('table tbody tr, [data-review-item]')).toHaveCount(1, { timeout: 5000 });
    await expect(page.locator('body')).toContainText(/mismatch|amount/i);
  });
});
