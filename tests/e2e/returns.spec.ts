import { test, expect, submitDepositUI } from './fixtures';

test.describe('Return / Reversal Handling', () => {
  test('return page allows triggering a return', async ({ page }) => {
    await page.goto('/ui/returns');
    await expect(page.locator('h1, h2')).toContainText(/return/i);
  });

  test('eligible for return table appears and prefills transfer ID on click', async ({ page }) => {
    // Create a FundsPosted deposit (not completing settlement so it stays in FundsPosted)
    await submitDepositUI(page, { amount: '200.00', scenario: 'clean_pass' });

    // Navigate to returns page
    await page.locator('a.nav-level-tab', { hasText: 'Returns' }).click();

    // Should show eligible transfers table
    const eligiblePanel = page.locator('.panel', { hasText: 'Eligible for Return' });
    await expect(eligiblePanel).toBeVisible();

    // Should show our $200 deposit
    await expect(eligiblePanel).toContainText('$200.00');

    // Click the row to prefill the transfer ID
    await eligiblePanel.locator('tbody tr').first().click();

    // Transfer ID field should be populated
    const idInput = page.locator('input[name="transferId"]');
    const value = await idInput.inputValue();
    expect(value).toMatch(/^[0-9a-f-]{36}$/);
  });

  async function createCompletedDeposit(page: any, amount: string): Promise<string> {
    const transferId = await submitDepositUI(page, { amount, scenario: 'clean_pass' });

    // Generate and ack settlement via UI
    await page.locator('a.nav-level-tab', { hasText: 'Settlement' }).click();
    await page.locator('[data-action="generate"]').click();
    await expect(page.locator('body')).toContainText(/generated/i);
    await page.locator('[data-action="ack"]').first().click();
    await expect(page.locator('body')).toContainText(/acknowledged/i);

    return transferId;
  }

  test('returning a completed deposit with NSF creates reversal with $30 fee', async ({ page }) => {
    const transferId = await createCompletedDeposit(page, '400.00');

    // Navigate to returns page and trigger return
    await page.locator('a.nav-level-tab', { hasText: 'Returns' }).click();
    await page.locator('input[name="transferId"]').fill(transferId);
    await page.locator('select[name="reasonCode"]').selectOption('NSF');
    await page.locator('button[type="submit"]').click();

    // Should show the returned transfer panel
    await expect(page.locator('body')).toContainText(/returned/i);
    await expect(page.locator('[data-state]')).toContainText(/returned/i);
    await expect(page.locator('body')).toContainText(/NSF/);
    await expect(page.locator('body')).toContainText(/\$30\.00/);

    // Verify on transfer detail via click-through
    await page.locator('a.nav-level-tab', { hasText: 'Transfers' }).click();
    await page.locator('[data-transfer] a').first().click();
    await expect(page.locator('[data-state]')).toContainText(/returned/i);

    // Verify ledger shows fee
    await page.locator('a.nav-level-tab', { hasText: 'Ledger' }).click();
    await expect(page.locator('body')).toContainText(/30/);
  });

  test('returning a completed deposit with FRAUD reason code', async ({ page }) => {
    const transferId = await createCompletedDeposit(page, '350.00');

    await page.locator('a.nav-level-tab', { hasText: 'Returns' }).click();
    await page.locator('input[name="transferId"]').fill(transferId);
    await page.locator('select[name="reasonCode"]').selectOption('FRAUD');
    await page.locator('button[type="submit"]').click();

    await expect(page.locator('[data-state]')).toContainText(/returned/i);
    await expect(page.locator('body')).toContainText(/FRAUD/);
    await expect(page.locator('body')).toContainText(/\$30\.00/);
  });
});
