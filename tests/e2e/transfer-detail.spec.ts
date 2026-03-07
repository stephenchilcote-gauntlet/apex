import { test, expect } from './fixtures';

test.describe('Transfer Detail & Decision Trace', () => {
  test('transfer list page shows all deposits', async ({ page, request }) => {
    // Create a deposit
    await request.post('/api/v1/deposits', {
      multipart: {
        investorAccountId: 'INV-1001',
        amount: '100.00',
        frontImage: { name: 'front.png', mimeType: 'image/png', buffer: Buffer.from('fake') },
        backImage: { name: 'back.png', mimeType: 'image/png', buffer: Buffer.from('fake') },
        vendorScenario: 'clean_pass',
      },
    });

    await page.goto('/ui/transfers');
    await expect(page.locator('h1, h2')).toContainText(/transfer|deposit/i);
    await expect(page.locator('table tbody tr, [data-transfer]')).toHaveCount(1, { timeout: 5000 });
  });

  test('transfer detail shows full decision trace', async ({ page, request }) => {
    const resp = await request.post('/api/v1/deposits', {
      multipart: {
        investorAccountId: 'INV-1001',
        amount: '200.00',
        frontImage: { name: 'front.png', mimeType: 'image/png', buffer: Buffer.from('fake') },
        backImage: { name: 'back.png', mimeType: 'image/png', buffer: Buffer.from('fake') },
        vendorScenario: 'clean_pass',
      },
    });
    const { transferId } = await resp.json();

    await page.goto(`/ui/transfers/${transferId}`);

    // Should show state
    await expect(page.locator('[data-state]')).toBeVisible();

    // Should show decision trace / audit trail
    await expect(page.locator('body')).toContainText(/vendor|validation/i);
    await expect(page.locator('body')).toContainText(/rule|business/i);
    await expect(page.locator('body')).toContainText(/clean_pass|pass/i);

    // Should show images
    await expect(page.locator('img[alt*="front" i], img[data-side="front"]')).toBeVisible();
    await expect(page.locator('img[alt*="back" i], img[data-side="back"]')).toBeVisible();
  });
});
