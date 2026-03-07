import { test as base, expect, Page } from '@playwright/test';

export const test = base.extend({
  resetState: [async ({ request }, use) => {
    const resp = await request.post('/api/v1/test/reset');
    expect(resp.ok()).toBeTruthy();
    await use();
  }, { auto: true, scope: 'test' }],
});

export { expect };

/**
 * Submit a deposit through the UI simulate form. Returns the transfer ID.
 */
export async function submitDepositUI(
  page: Page,
  opts: {
    accountId?: string;
    amount?: string;
    scenario?: string;
  } = {},
): Promise<string> {
  const accountId = opts.accountId ?? 'INV-1001';
  const amount = opts.amount ?? '500.00';
  const scenario = opts.scenario ?? 'clean_pass';

  await page.goto('/ui/simulate');
  await page.locator('select[name="investorAccountId"]').selectOption({ value: accountId });
  await page.locator('input[name="amount"]').fill(amount);
  await page.locator('input[name="frontImage"]').setInputFiles({
    name: 'front.png', mimeType: 'image/png', buffer: Buffer.from('fake-front'),
  });
  await page.locator('input[name="backImage"]').setInputFiles({
    name: 'back.png', mimeType: 'image/png', buffer: Buffer.from('fake-back'),
  });
  await page.locator('select[name="vendorScenario"]').selectOption(scenario);
  await page.locator('button[type="submit"]').click();

  // Wait for result panel and extract transfer ID
  const transferId = await page.locator('[data-transfer-id]').getAttribute('data-transfer-id');
  expect(transferId).toBeTruthy();
  return transferId!;
}
