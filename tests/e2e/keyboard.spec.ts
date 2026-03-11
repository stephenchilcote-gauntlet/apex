import { test, expect, submitDepositUI } from './fixtures';

test.describe('Keyboard Shortcuts', () => {
  test('? key opens keyboard shortcuts modal', async ({ page }) => {
    await page.goto('/ui');
    // Focus somewhere that isn't an input
    await page.locator('body').click();
    await page.keyboard.press('?');
    const modal = page.locator('#kbd-modal');
    await expect(modal).toBeVisible();
    await expect(modal).toContainText('Keyboard shortcuts');
    // Close with Escape
    await page.keyboard.press('Escape');
    await expect(modal).not.toBeVisible();
  });

  test('g+h navigates to Overview', async ({ page }) => {
    await page.goto('/ui/transfers');
    await page.locator('body').click();
    await page.keyboard.press('g');
    await page.keyboard.press('h');
    await expect(page).toHaveURL('/ui');
  });

  test('g+t navigates to Transfers', async ({ page }) => {
    await page.goto('/ui');
    await page.locator('body').click();
    await page.keyboard.press('g');
    await page.keyboard.press('t');
    await expect(page).toHaveURL('/ui/transfers');
  });

  test('g+s navigates to Simulate', async ({ page }) => {
    await page.goto('/ui');
    await page.locator('body').click();
    await page.keyboard.press('g');
    await page.keyboard.press('s');
    await expect(page).toHaveURL('/ui/simulate');
  });

  test('g+l navigates to Ledger', async ({ page }) => {
    await page.goto('/ui');
    await page.locator('body').click();
    await page.keyboard.press('g');
    await page.keyboard.press('l');
    await expect(page).toHaveURL('/ui/ledger');
  });

  test('Ctrl+K opens command palette', async ({ page }) => {
    await page.goto('/ui');
    await page.keyboard.press('Control+k');
    const modal = page.locator('#cmd-modal');
    await expect(modal).toBeVisible();
    await expect(page.locator('#cmd-input')).toBeFocused();
    await page.keyboard.press('Escape');
    await expect(modal).not.toBeVisible();
  });

  test('command palette shows results when typing an account ID', async ({ page }) => {
    await submitDepositUI(page, { amount: '175.00', scenario: 'clean_pass' });

    await page.goto('/ui');
    await page.keyboard.press('Control+k');
    await expect(page.locator('#cmd-modal')).toBeVisible();

    await page.locator('#cmd-input').fill('INV-1001');
    // Wait for HTMX debounce (150ms) + network response
    await expect(page.locator('#cmd-results .cmd-result').first()).toBeVisible({ timeout: 3000 });
    await expect(page.locator('#cmd-results')).toContainText('175.00');
  });

  test('command palette shows results for partial account name', async ({ page }) => {
    await submitDepositUI(page, { amount: '299.00', scenario: 'clean_pass' });

    await page.goto('/ui');
    await page.keyboard.press('Control+k');
    await page.locator('#cmd-input').fill('INV');
    await expect(page.locator('#cmd-results .cmd-result')).not.toHaveCount(0, { timeout: 3000 });
  });

  test('command palette shows no-match message for unknown query', async ({ page }) => {
    await page.goto('/ui');
    await page.keyboard.press('Control+k');
    await page.locator('#cmd-input').fill('XYZZY_NO_MATCH_99999');
    await expect(page.locator('#cmd-results')).toContainText('No transfers matching', { timeout: 3000 });
  });

  test('command palette result click navigates to transfer detail', async ({ page }) => {
    await submitDepositUI(page, { amount: '350.00', scenario: 'clean_pass' });

    await page.goto('/ui');
    await page.keyboard.press('Control+k');
    await page.locator('#cmd-input').fill('INV-1001');
    await page.locator('#cmd-results .cmd-result').first().waitFor({ timeout: 3000 });
    await page.locator('#cmd-results .cmd-result').first().click();
    await expect(page).toHaveURL(/\/ui\/transfers\/.+/);
  });

  test('search endpoint returns matching transfers', async ({ page }) => {
    await submitDepositUI(page, { amount: '175.00', scenario: 'clean_pass' });

    const resp = await page.request.get('/ui/search?q=INV');
    expect(resp.status()).toBe(200);
    const body = await resp.text();
    expect(body).toContain('cmd-result');
    expect(body).toContain('175.00');
  });

  test('search endpoint empty query returns placeholder', async ({ page }) => {
    const resp = await page.request.get('/ui/search?q=');
    expect(resp.status()).toBe(200);
    expect(await resp.text()).toContain('Type to search');
  });

  test('search endpoint no-match returns empty message', async ({ page }) => {
    const resp = await page.request.get('/ui/search?q=XYZZY_NO_MATCH_12345');
    expect(resp.status()).toBe(200);
    expect(await resp.text()).toContain('No transfers matching');
  });

  test('j/k keys navigate transfer rows', async ({ page }) => {
    await submitDepositUI(page, { amount: '225.00', scenario: 'clean_pass' });

    await page.goto('/ui/transfers');
    const firstRow = page.locator('tr[data-transfer]').first();
    await expect(firstRow).toBeVisible();

    // Focus the page body (not an input) then press j to focus first row
    await page.locator('body').click();
    await page.keyboard.press('j');

    // First row should be focused
    await expect(firstRow).toHaveClass(/row-focused/);

    // Press j again — stays on last row if only one row
    await page.keyboard.press('j');
    // Still visible
    await expect(firstRow).toBeVisible();
  });
});
