import { test, expect, CHECK_FRONT, CHECK_BACK } from './fixtures';

test.describe('Deposit Submission', () => {
  test('can navigate to deposit simulation page', async ({ page }) => {
    await page.goto('/ui/simulate');
    await expect(page.locator('h1, h2')).toContainText(/deposit|simulate/i);
    await expect(page.locator('form')).toBeVisible();
  });

  test('deposit form has required fields', async ({ page }) => {
    await page.goto('/ui/simulate');
    await expect(page.locator('input[name="investorAccountId"], select[name="investorAccountId"]')).toBeVisible();
    await expect(page.locator('input[name="amount"]')).toBeVisible();
    // Uncheck sample images to reveal file inputs
    const sampleChk = page.locator('input[name="useSampleImages"]');
    if (await sampleChk.isChecked()) await sampleChk.uncheck();
    await expect(page.locator('input[name="frontImage"]')).toBeVisible();
    await expect(page.locator('input[name="backImage"]')).toBeVisible();
    await expect(page.locator('button[type="submit"]')).toBeVisible();
  });

  test('successful deposit submission shows confirmation', async ({ page }) => {
    await page.goto('/ui/simulate');

    // Fill out the deposit form
    await page.locator('select[name="investorAccountId"]').selectOption({ value: 'INV-1001' });
    await page.locator('input[name="amount"]').fill('1250.00');

    // Use sample images (default) — no file upload needed

    await page.locator('button[type="submit"]').click();

    // Should see confirmation with transfer ID and status
    await expect(page.locator('body')).toContainText(/transfer|deposit/i);
    await expect(page.locator('body')).toContainText(/requested|validating|analyzing|approved|fundsposted/i);
  });
});
