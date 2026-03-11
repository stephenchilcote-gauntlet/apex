import { test, expect, submitDepositUI } from './fixtures';

test.describe('Audit Log', () => {
  test('audit log page loads and shows events', async ({ page }) => {
    // Submit a deposit to generate audit events
    await submitDepositUI(page, { amount: '300.00', scenario: 'clean_pass' });

    // Navigate to audit log
    await page.goto('/ui/audit');
    await expect(page.locator('h1')).toContainText(/audit/i);

    // Should show events
    const table = page.locator('table.data-table');
    await expect(table).toBeVisible();

    // Should have at least one row (the deposit we just submitted)
    const rows = table.locator('tbody tr');
    await expect(rows.first()).not.toContainText('No audit events found');
  });

  test('audit log filter by transfer ID', async ({ page, request }) => {
    // Submit a deposit via API to get a known transfer ID
    const { CHECK_FRONT, CHECK_BACK } = await import('./fixtures');
    const fs = await import('fs');
    const resp = await request.post('/api/v1/deposits', {
      multipart: {
        investorAccountId: 'INV-1001',
        amount: '450.00',
        vendorScenario: 'clean_pass',
        frontImage: { name: 'front.png', mimeType: 'image/png', buffer: fs.readFileSync(CHECK_FRONT) },
        backImage: { name: 'back.png', mimeType: 'image/png', buffer: fs.readFileSync(CHECK_BACK) },
      },
    });
    const body = await resp.json();
    const transferId = body.transferId;
    expect(transferId).toBeTruthy();

    // Filter audit log by transfer ID
    await page.goto(`/ui/audit?transferId=${transferId}`);
    await expect(page.locator('body')).toContainText(transferId.substring(0, 8));

    // Should show events for this transfer only
    const table = page.locator('table.data-table');
    await expect(table).toBeVisible();
  });

  test('audit log clear filter works', async ({ page }) => {
    await page.goto('/ui/audit?transferId=some-id');
    await expect(page.locator('a:has-text("Clear")')).toBeVisible();
    await page.locator('a:has-text("Clear")').click();
    await expect(page).toHaveURL('/ui/audit');
  });
});
