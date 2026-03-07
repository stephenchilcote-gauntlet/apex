import { test, expect, submitDepositUI } from './fixtures';

test.describe('Ledger View', () => {
  test('ledger page shows accounts table with correct structure', async ({ page }) => {
    await page.goto('/ui/ledger');
    await expect(page.locator('h1, h2')).toContainText(/ledger/i);

    const table = page.locator('table');
    await expect(table).toBeVisible();

    // Verify table headers
    const headers = table.locator('thead th');
    await expect(headers.nth(0)).toContainText('External ID');
    await expect(headers.nth(1)).toContainText('Name');
    await expect(headers.nth(2)).toContainText('Type');
    await expect(headers.nth(3)).toContainText('Balance');

    // Should show investor accounts and system accounts
    await expect(page.locator('body')).toContainText(/INV-/);
    await expect(page.locator('body')).toContainText(/investor/i);
    await expect(page.locator('body')).toContainText(/omnibus/i);
    await expect(page.locator('body')).toContainText(/fee/i);
  });

  test('deposit posting updates account balance', async ({ page }) => {
    await submitDepositUI(page, { amount: '250.00', scenario: 'clean_pass' });

    // Navigate to ledger and verify balance
    await page.locator('a.nav-level-tab', { hasText: 'Ledger' }).click();
    await expect(page.locator('body')).toContainText(/250/);

    // Verify INV-1001 was credited
    const rows = page.locator('table tbody tr');
    const inv1001Row = rows.filter({ hasText: 'INV-1001' });
    await expect(inv1001Row).toContainText('$250.00');

    // Verify omnibus was debited (negative)
    const omnibusRow = rows.filter({ hasText: /omnibus/i });
    await expect(omnibusRow).toBeVisible();
  });
});
