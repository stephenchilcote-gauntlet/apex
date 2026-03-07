import { test, expect, submitDepositUI } from './fixtures';

test.describe('Transfer Detail & Decision Trace', () => {
  test('transfer list shows deposit and clicking navigates to detail', async ({ page }) => {
    await submitDepositUI(page, { amount: '100.00', scenario: 'clean_pass' });

    // Navigate to transfer list
    await page.locator('a.nav-level-tab', { hasText: 'Transfers' }).click();
    await expect(page.locator('h1, h2')).toContainText(/transfer/i);

    // Verify transfer row exists with content
    const row = page.locator('[data-transfer]').first();
    await expect(row).toBeVisible();
    await expect(row).toContainText('$100.00');

    // Click the transfer link to navigate to detail
    await row.locator('a').first().click();

    // Should be on transfer detail page
    await expect(page.locator('h1, h2')).toContainText(/transfer detail/i);
    await expect(page.locator('[data-state]')).toBeVisible();
  });

  test('transfer detail shows full decision trace', async ({ page }) => {
    await submitDepositUI(page, { amount: '200.00', scenario: 'clean_pass' });

    // Navigate to transfer detail via click-through
    await page.locator('a.nav-level-tab', { hasText: 'Transfers' }).click();
    await page.locator('[data-transfer] a').first().click();

    // Should show state
    await expect(page.locator('[data-state]')).toBeVisible();

    // Should show decision trace / audit trail
    await expect(page.locator('body')).toContainText(/vendor|validation/i);
    await expect(page.locator('body')).toContainText(/rule|business/i);
    await expect(page.locator('body')).toContainText(/clean_pass|pass/i);

    // Should show images
    await expect(page.locator('img[alt*="Front" i], img[data-side="front"]')).toBeVisible();
    await expect(page.locator('img[alt*="Back" i], img[data-side="back"]')).toBeVisible();
  });
});
