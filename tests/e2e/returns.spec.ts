import { test, expect } from './fixtures';

test.describe('Return / Reversal Handling', () => {
  test('return page allows triggering a return', async ({ page }) => {
    await page.goto('/ui/returns');
    await expect(page.locator('h1, h2')).toContainText(/return/i);
  });

  test('returning a completed deposit creates reversal with $30 fee', async ({ page, request }) => {
    // Submit and complete a deposit via API
    const depositResp = await request.post('/api/v1/deposits', {
      multipart: {
        investorAccountId: 'INV-1001',
        amount: '400.00',
        frontImage: { name: 'front.png', mimeType: 'image/png', buffer: Buffer.from('fake-front') },
        backImage: { name: 'back.png', mimeType: 'image/png', buffer: Buffer.from('fake-back') },
        vendorScenario: 'clean_pass',
      },
    });
    expect(depositResp.ok()).toBeTruthy();
    const { transferId } = await depositResp.json();

    // Generate and ack settlement to get to Completed
    await request.post('/api/v1/settlement/batches/generate', {
      data: { businessDateCT: new Date().toISOString().split('T')[0] },
    });
    const batchesResp = await request.get('/api/v1/settlement/batches');
    const batches = await batchesResp.json();
    const batchId = batches[batches.length - 1]?.id;
    if (batchId) {
      await request.post(`/api/v1/settlement/batches/${batchId}/ack`, {
        data: { ackReference: 'ACK-TEST-001' },
      });
    }

    // Navigate to returns page and trigger return
    await page.goto('/ui/returns');
    await page.locator('input[name="transferId"]').fill(transferId);
    await page.locator('select[name="reasonCode"]').selectOption('NSF');
    await page.locator('button[type="submit"]').click();

    // Should confirm the return
    await expect(page.locator('body')).toContainText(/returned/i);

    // Verify on transfer detail
    await page.goto(`/ui/transfers/${transferId}`);
    await expect(page.locator('[data-state]')).toContainText(/returned/i);

    // Verify ledger shows reversal and fee
    await page.goto('/ui/ledger');
    await expect(page.locator('body')).toContainText(/30/); // $30 fee
  });
});
