import { test, expect, submitDepositUI } from './fixtures';

test.describe('Transfers List', () => {
  test('transfers list shows page total in tfoot', async ({ page }) => {
    await submitDepositUI(page, { amount: '350.00', scenario: 'clean_pass' });

    await page.goto('/ui/transfers');
    // tfoot should show page total
    const tfoot = page.locator('table tfoot');
    await expect(tfoot).toBeVisible();
    await expect(tfoot).toContainText('$350.00');
  });

  test('CSV export download returns csv content', async ({ page }) => {
    await submitDepositUI(page, { amount: '125.00', scenario: 'clean_pass' });

    // CSV export via page.request preserves the browser session/cookies
    const resp = await page.request.get('/ui/transfers?format=csv');
    expect(resp.status()).toBe(200);
    const contentType = resp.headers()['content-type'];
    expect(contentType).toMatch(/text\/csv/i);
    const body = await resp.text();
    // CSV has header row with column names
    expect(body).toContain('ID,Account,Amount,State');
    expect(body).toContain('125.00');
  });
});

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

  test('check image lightbox opens and closes', async ({ page }) => {
    await submitDepositUI(page, { amount: '150.00', scenario: 'clean_pass' });

    await page.locator('a.nav-level-tab', { hasText: 'Transfers' }).click();
    await page.locator('[data-transfer] a').first().click();

    // Lightbox should be hidden initially
    await expect(page.locator('#check-lightbox')).not.toHaveClass(/open/);

    // Click front image to open lightbox
    await page.locator('img[data-side="front"]').click();
    await expect(page.locator('#check-lightbox')).toHaveClass(/open/);
    await expect(page.locator('#lightbox-label')).toContainText(/front/i);

    // Press Escape to close
    await page.keyboard.press('Escape');
    await expect(page.locator('#check-lightbox')).not.toHaveClass(/open/);

    // Open again and close by clicking backdrop
    await page.locator('img[data-side="back"]').click();
    await expect(page.locator('#check-lightbox')).toHaveClass(/open/);
    await expect(page.locator('#lightbox-label')).toContainText(/back/i);
    await page.locator('#check-lightbox').click({ position: { x: 5, y: 5 } });
    await expect(page.locator('#check-lightbox')).not.toHaveClass(/open/);
  });
});
