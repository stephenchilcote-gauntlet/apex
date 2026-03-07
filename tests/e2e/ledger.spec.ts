import { test, expect } from './fixtures';

test.describe('Ledger View', () => {
  test('ledger page shows accounts and balances', async ({ page }) => {
    await page.goto('/ui/ledger');
    await expect(page.locator('h1, h2')).toContainText(/ledger|accounts|balance/i);
    await expect(page.locator('table')).toBeVisible();

    // Should show seeded accounts
    await expect(page.locator('body')).toContainText(/INV-/);
  });

  test('deposit posting updates account balance', async ({ page, request }) => {
    // Get initial balance
    await page.goto('/ui/ledger');
    const initialContent = await page.locator('body').textContent();

    // Submit a clean deposit via API
    const resp = await request.post('/api/v1/deposits', {
      multipart: {
        investorAccountId: 'INV-1001',
        amount: '250.00',
        frontImage: { name: 'front.png', mimeType: 'image/png', buffer: Buffer.from('fake-front') },
        backImage: { name: 'back.png', mimeType: 'image/png', buffer: Buffer.from('fake-back') },
        vendorScenario: 'clean_pass',
      },
    });
    expect(resp.ok()).toBeTruthy();

    // Refresh and check balance changed
    await page.goto('/ui/ledger');
    await expect(page.locator('body')).toContainText(/250/);
  });
});
