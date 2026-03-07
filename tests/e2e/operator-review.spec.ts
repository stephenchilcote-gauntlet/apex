import { test, expect } from './fixtures';

async function submitForReview(page: any, scenario = 'micr_failure') {
  await page.goto('/ui/simulate');
  await page.locator('select[name="investorAccountId"]').selectOption({ value: 'INV-1001' });
  await page.locator('input[name="amount"]').fill('800.00');
  await page.locator('input[name="frontImage"]').setInputFiles({
    name: 'front.png', mimeType: 'image/png', buffer: Buffer.from('fake-front')
  });
  await page.locator('input[name="backImage"]').setInputFiles({
    name: 'back.png', mimeType: 'image/png', buffer: Buffer.from('fake-back')
  });
  await page.locator('select[name="vendorScenario"]').selectOption(scenario);
  await page.locator('button[type="submit"]').click();
}

test.describe('Operator Review Workflow', () => {
  test('review queue shows flagged deposits with required info', async ({ page }) => {
    await submitForReview(page, 'micr_failure');

    await page.goto('/ui/review');
    const row = page.locator('table tbody tr, [data-review-item]').first();
    await expect(row).toBeVisible();

    // Should show key review information (amount and scenario)
    await expect(page.locator('body')).toContainText(/800/);
    await expect(page.locator('body')).toContainText(/micr_failure/);
  });

  test('operator can approve a flagged deposit', async ({ page }) => {
    await submitForReview(page, 'amount_mismatch');

    await page.goto('/ui/review');
    const reviewItem = page.locator('table tbody tr, [data-review-item]').first();
    await expect(reviewItem).toBeVisible();

    // Click into the review detail or use inline approve
    await reviewItem.locator('a, button').filter({ hasText: /review|detail|view/i }).first().click();

    // Should see check images, MICR data, risk indicators
    await expect(page.locator('body')).toContainText(/amount/i);

    // Fill operator notes and approve
    await page.locator('#approve-notes').fill('Amounts close enough after manual review');
    await page.locator('[data-action="approve"]').click();

    // Should see updated state
    await expect(page.locator('body')).toContainText(/approved|fundsposted/i);
  });

  test('operator can reject a flagged deposit', async ({ page }) => {
    await submitForReview(page, 'micr_failure');

    await page.goto('/ui/review');
    const reviewItem = page.locator('table tbody tr, [data-review-item]').first();
    await expect(reviewItem).toBeVisible();

    await reviewItem.locator('a, button').filter({ hasText: /review|detail|view/i }).first().click();

    // Fill notes and reject
    await page.locator('#reject-notes').fill('MICR data unreadable, cannot verify');
    await page.locator('[data-action="reject"]').click();

    // Should see rejected state
    await expect(page.locator('body')).toContainText(/rejected/i);
  });
});
