import { test, expect } from './fixtures';

test.describe('Happy Path End-to-End', () => {
  test('deposit flows through to FundsPosted automatically on clean pass', async ({ page }) => {
    // Submit a deposit with clean_pass scenario
    await page.goto('/ui/simulate');

    await page.locator('select[name="investorAccountId"]').selectOption({ value: 'INV-1001' });
    await page.locator('input[name="amount"]').fill('500.00');
    await page.locator('input[name="frontImage"]').setInputFiles({
      name: 'front.png', mimeType: 'image/png', buffer: Buffer.from('fake-front')
    });
    await page.locator('input[name="backImage"]').setInputFiles({
      name: 'back.png', mimeType: 'image/png', buffer: Buffer.from('fake-back')
    });
    await page.locator('select[name="vendorScenario"]').selectOption('clean_pass');
    await page.locator('button[type="submit"]').click();

    // Should redirect to or show transfer detail
    await expect(page.locator('body')).toContainText(/fundsposted/i);

    // Get the transfer ID from the page
    const transferId = await page.locator('[data-transfer-id]').getAttribute('data-transfer-id');
    expect(transferId).toBeTruthy();

    // Navigate to transfer detail
    await page.goto(`/ui/transfers/${transferId}`);
    await expect(page.locator('[data-state]')).toContainText(/fundsposted/i);

    // Verify ledger was updated
    await page.goto('/ui/ledger');
    await expect(page.locator('body')).toContainText(/500/);
  });

  test('settlement completes the lifecycle', async ({ page, request }) => {
    // First submit a deposit via the API for speed
    const depositResp = await request.post('/api/v1/deposits', {
      multipart: {
        investorAccountId: 'INV-1001',
        amount: '750.00',
        frontImage: { name: 'front.png', mimeType: 'image/png', buffer: Buffer.from('fake-front') },
        backImage: { name: 'back.png', mimeType: 'image/png', buffer: Buffer.from('fake-back') },
        vendorScenario: 'clean_pass',
      },
    });
    expect(depositResp.ok()).toBeTruthy();
    const { transferId } = await depositResp.json();

    // Generate settlement batch
    await page.goto('/ui/settlement');
    await page.locator('button:has-text("Generate"), [data-action="generate"]').click();
    await expect(page.locator('body')).toContainText(/generated/i);

    // Acknowledge the batch
    await page.locator('button:has-text("Acknowledge"), [data-action="ack"]').first().click();
    await expect(page.locator('body')).toContainText(/acknowledged/i);

    // Verify transfer is now Completed
    await page.goto(`/ui/transfers/${transferId}`);
    await expect(page.locator('[data-state]')).toContainText(/completed/i);
  });
});
