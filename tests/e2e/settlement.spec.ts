import { test, expect, submitDepositUI } from './fixtures';

test.describe('Settlement', () => {
  test('settlement page shows batch management', async ({ page }) => {
    await page.goto('/ui/settlement');
    await expect(page.locator('h1, h2')).toContainText(/settlement/i);
    await expect(page.locator('[data-action="generate"]')).toBeVisible();
  });

  test('generate settlement batch from posted deposits', async ({ page }) => {
    await submitDepositUI(page, { amount: '600.00', scenario: 'clean_pass' });

    // Navigate to settlement and generate
    await page.locator('a.nav-level-tab', { hasText: 'Settlement' }).click();
    await page.locator('[data-action="generate"]').click();

    // Verify batch table content: status badge, items count, total amount
    await expect(page.locator('body')).toContainText(/generated/i);
    const batchRow = page.locator('table tbody tr').first();
    await expect(batchRow.locator('[data-state]')).toContainText(/generated/i);
    await expect(batchRow).toContainText('1'); // 1 item
    await expect(batchRow).toContainText('$600.00');
  });

  test('acknowledging batch moves transfers to Completed', async ({ page }) => {
    await submitDepositUI(page, { amount: '300.00', scenario: 'clean_pass' });

    // Generate batch
    await page.locator('a.nav-level-tab', { hasText: 'Settlement' }).click();
    await page.locator('[data-action="generate"]').click();
    await expect(page.locator('body')).toContainText(/generated/i);

    // Acknowledge
    await page.locator('[data-action="ack"]').first().click();
    await expect(page.locator('body')).toContainText(/acknowledged/i);

    // Verify batch status changed
    const batchRow = page.locator('table tbody tr').first();
    await expect(batchRow.locator('[data-state]')).toContainText(/acknowledged/i);

    // Verify transfer completed by clicking through
    await page.locator('a.nav-level-tab', { hasText: 'Transfers' }).click();
    await page.locator('[data-transfer] a').first().click();
    await expect(page.locator('[data-state]')).toContainText(/completed/i);
  });
});
