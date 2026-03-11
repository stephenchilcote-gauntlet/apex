import { test, expect, submitDepositUI } from './fixtures';

test.describe('Empty States', () => {
  test('transfers page shows empty message when no transfers', async ({ page }) => {
    await page.goto('/ui/transfers');
    await expect(page.locator('body')).toContainText('No transfers found');
  });

  test('review queue shows empty message when no items pending', async ({ page }) => {
    await page.goto('/ui/review');
    await expect(page.locator('body')).toContainText('No items pending review');
  });

  test('settlement page shows empty message when no batches', async ({ page }) => {
    await page.goto('/ui/settlement');
    await expect(page.locator('body')).toContainText('No batches found');
  });
});

test.describe('Dashboard', () => {
  test('dashboard loads and shows stat cards', async ({ page }) => {
    await page.goto('/ui');
    await expect(page.locator('h1')).toContainText(/overview/i);

    // Should show stat cards
    await expect(page.locator('.dash-card').first()).toBeVisible();
    await expect(page.locator('body')).toContainText(/total transfers/i);
    await expect(page.locator('body')).toContainText(/pending review/i);
  });

  test('dashboard reflects submitted deposit in counts', async ({ page }) => {
    await submitDepositUI(page, { amount: '300.00', scenario: 'clean_pass' });

    await page.goto('/ui');
    await page.waitForLoadState('networkidle');

    // Total should be > 0
    const totalCard = page.locator('.dash-card', { hasText: 'total transfers' });
    const value = await totalCard.locator('.dash-card-value').textContent();
    expect(parseInt(value || '0')).toBeGreaterThan(0);
  });
});
