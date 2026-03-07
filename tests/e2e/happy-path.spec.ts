import { test, expect, submitDepositUI } from './fixtures';

test.describe('Happy Path End-to-End', () => {
  test('deposit flows through to FundsPosted automatically on clean pass', async ({ page }) => {
    const transferId = await submitDepositUI(page, { amount: '500.00', scenario: 'clean_pass' });

    await expect(page.locator('[data-state]')).toContainText(/fundsposted/i);

    // Navigate to transfers list and click through to detail
    await page.locator('a.nav-level-tab', { hasText: 'Transfers' }).click();
    await page.locator('[data-transfer] a').first().click();
    await expect(page.locator('[data-state]')).toContainText(/fundsposted/i);

    // Verify ledger was updated
    await page.locator('a.nav-level-tab', { hasText: 'Ledger' }).click();
    await expect(page.locator('body')).toContainText(/500/);
  });

  test('settlement completes the lifecycle', async ({ page }) => {
    await submitDepositUI(page, { amount: '750.00', scenario: 'clean_pass' });

    // Generate settlement batch via UI
    await page.locator('a.nav-level-tab', { hasText: 'Settlement' }).click();
    await page.locator('[data-action="generate"]').click();
    await expect(page.locator('body')).toContainText(/generated/i);

    // Acknowledge the batch
    await page.locator('[data-action="ack"]').first().click();
    await expect(page.locator('body')).toContainText(/acknowledged/i);

    // Verify transfer is now Completed by clicking through transfers list
    await page.locator('a.nav-level-tab', { hasText: 'Transfers' }).click();
    await page.locator('[data-transfer] a').first().click();
    await expect(page.locator('[data-state]')).toContainText(/completed/i);
  });
});
