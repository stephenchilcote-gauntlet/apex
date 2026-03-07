import { test, expect } from './fixtures';

test.describe('Settlement', () => {
  test('settlement page shows batch management', async ({ page }) => {
    await page.goto('/ui/settlement');
    await expect(page.locator('h1, h2')).toContainText(/settlement/i);
    await expect(page.locator('button:has-text("Generate"), [data-action="generate"]')).toBeVisible();
  });

  test('generate settlement batch from posted deposits', async ({ page, request }) => {
    // Create a deposit that will auto-approve
    const resp = await request.post('/api/v1/deposits', {
      multipart: {
        investorAccountId: 'INV-1001',
        amount: '600.00',
        frontImage: { name: 'front.png', mimeType: 'image/png', buffer: Buffer.from('fake-front') },
        backImage: { name: 'back.png', mimeType: 'image/png', buffer: Buffer.from('fake-back') },
        vendorScenario: 'clean_pass',
      },
    });
    expect(resp.ok()).toBeTruthy();

    // Generate settlement batch
    await page.goto('/ui/settlement');
    await page.locator('button:has-text("Generate"), [data-action="generate"]').click();

    // Should show generated batch with item count and total
    await expect(page.locator('body')).toContainText(/generated/i);
    await expect(page.locator('body')).toContainText(/600/);
  });

  test('acknowledging batch moves transfers to Completed', async ({ page, request }) => {
    // Submit deposit
    const depositResp = await request.post('/api/v1/deposits', {
      multipart: {
        investorAccountId: 'INV-1001',
        amount: '300.00',
        frontImage: { name: 'front.png', mimeType: 'image/png', buffer: Buffer.from('fake-front') },
        backImage: { name: 'back.png', mimeType: 'image/png', buffer: Buffer.from('fake-back') },
        vendorScenario: 'clean_pass',
      },
    });
    expect(depositResp.ok()).toBeTruthy();
    const { transferId } = await depositResp.json();

    // Generate batch
    await page.goto('/ui/settlement');
    await page.locator('button:has-text("Generate"), [data-action="generate"]').click();
    await expect(page.locator('body')).toContainText(/generated/i);

    // Acknowledge
    await page.locator('button:has-text("Acknowledge"), [data-action="ack"]').first().click();

    // Verify transfer completed
    await page.goto(`/ui/transfers/${transferId}`);
    await expect(page.locator('[data-state]')).toContainText(/completed/i);
  });
});
