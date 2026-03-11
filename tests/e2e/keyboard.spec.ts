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
    // Close with Escape
    await page.keyboard.press('Escape');
    await expect(modal).not.toBeVisible();
  });

  test('search endpoint returns matching transfers', async ({ page }) => {
    await submitDepositUI(page, { amount: '175.00', scenario: 'clean_pass' });

    // Test /ui/search endpoint directly — it powers the command palette
    const resp = await page.request.get('/ui/search?q=INV');
    expect(resp.status()).toBe(200);
    const body = await resp.text();
    // Should return HTML with transfer result links
    expect(body).toContain('cmd-result');
    expect(body).toContain('175.00');
  });

  test('search endpoint empty query returns placeholder', async ({ page }) => {
    await page.goto('/ui');
    const resp = await page.request.get('/ui/search?q=');
    expect(resp.status()).toBe(200);
    const body = await resp.text();
    expect(body).toContain('Type to search');
  });

  test('search endpoint no-match returns empty message', async ({ page }) => {
    await page.goto('/ui');
    const resp = await page.request.get('/ui/search?q=XYZZY_NO_MATCH_12345');
    expect(resp.status()).toBe(200);
    const body = await resp.text();
    expect(body).toContain('No transfers matching');
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
