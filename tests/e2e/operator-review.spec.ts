import { test, expect, submitDepositUI } from './fixtures';

test.describe('Operator Review Workflow', () => {
  test('review queue shows flagged deposits with required info', async ({ page }) => {
    await submitDepositUI(page, { amount: '800.00', scenario: 'micr_failure' });

    await page.locator('a.nav-level-tab', { hasText: 'Review Queue' }).click();
    const row = page.locator('[data-review-item]').first();
    await expect(row).toBeVisible();

    // Should show key review information
    await expect(page.locator('body')).toContainText(/800/);
    await expect(page.locator('body')).toContainText(/micr_failure/);
  });

  test('operator can approve a flagged deposit', async ({ page }) => {
    await submitDepositUI(page, { scenario: 'amount_mismatch' });

    await page.locator('a.nav-level-tab', { hasText: 'Review Queue' }).click();
    await expect(page.locator('[data-review-item]').first()).toBeVisible();

    // Click the Review link (exact match on the btn class link)
    await page.locator('[data-review-item] a.btn', { hasText: 'Review' }).first().click();

    // Should see review detail page with transfer info
    await expect(page.locator('body')).toContainText(/amount/i);

    // Fill operator notes and approve
    await page.locator('#approve-notes').fill('Amounts close enough after manual review');
    await page.locator('[data-action="approve"]').click();

    // Should see updated state
    await expect(page.locator('body')).toContainText(/approved|fundsposted/i);
  });

  test('operator can reject a flagged deposit', async ({ page }) => {
    await submitDepositUI(page, { scenario: 'micr_failure' });

    await page.locator('a.nav-level-tab', { hasText: 'Review Queue' }).click();
    await expect(page.locator('[data-review-item]').first()).toBeVisible();

    // Click the Review link directly
    await page.locator('[data-review-item] a.btn', { hasText: 'Review' }).first().click();

    // Fill notes and reject
    await page.locator('#reject-notes').fill('MICR data unreadable, cannot verify');
    await page.locator('[data-action="reject"]').click();

    // Should see rejected state
    await expect(page.locator('body')).toContainText(/rejected/i);
  });
});
