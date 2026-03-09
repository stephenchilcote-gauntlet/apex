import { test as base, expect, Page } from '@playwright/test';
import * as path from 'path';

export const test = base.extend({
  resetState: [async ({ request }, use) => {
    const resp = await request.post('/api/v1/test/reset');
    expect(resp.ok()).toBeTruthy();
    await use();
  }, { auto: true, scope: 'test' }],
});

export { expect };

/** Paths to realistic placeholder check images */
export const CHECK_FRONT = path.join(__dirname, 'tests', 'check-front.png');
export const CHECK_BACK = path.join(__dirname, 'tests', 'check-back.png');

/**
 * Submit a deposit through the UI simulate form. Returns the transfer ID.
 */
const SCENARIO_ACCOUNT_MAP: Record<string, string> = {
  'clean_pass': 'INV-1001',
  'iqa_blur': 'INV-1002',
  'iqa_glare': 'INV-1003',
  'micr_failure': 'INV-1004',
  'duplicate_detected': 'INV-1005',
  'amount_mismatch': 'INV-1006',
  'iqa_pass_review': 'INV-1007',
};

export async function submitDepositUI(
  page: Page,
  opts: {
    accountId?: string;
    amount?: string;
    scenario?: string;
  } = {},
): Promise<string> {
  const scenario = opts.scenario ?? 'clean_pass';
  const accountId = opts.accountId ?? SCENARIO_ACCOUNT_MAP[scenario] ?? 'INV-1001';
  const amount = opts.amount ?? '500.00';

  await page.goto('/ui/simulate');
  await page.locator('select[name="investorAccountId"]').selectOption({ value: accountId });
  await page.locator('input[name="amount"]').fill(amount);
  await page.locator('input[name="frontImage"]').setInputFiles(CHECK_FRONT);
  await page.locator('input[name="backImage"]').setInputFiles(CHECK_BACK);
  await page.locator('button[type="submit"]').click();

  // Wait for result panel and extract transfer ID
  const transferId = await page.locator('[data-transfer-id]').getAttribute('data-transfer-id');
  expect(transferId).toBeTruthy();
  return transferId!;
}
