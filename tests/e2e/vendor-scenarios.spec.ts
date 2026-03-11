import { test, expect, submitDepositUI } from './fixtures';

test.describe('Vendor Stub Scenarios', () => {
  test('IQA Blur results in rejection with retake message', async ({ page }) => {
    await submitDepositUI(page, { scenario: 'iqa_blur' });

    await expect(page.locator('[data-state]')).toContainText(/rejected/i);
    await expect(page.locator('body')).toContainText(/blur|retake|resubmit/i);
  });

  test('IQA Glare results in rejection with retake message', async ({ page }) => {
    await submitDepositUI(page, { scenario: 'iqa_glare' });

    await expect(page.locator('[data-state]')).toContainText(/rejected/i);
    await expect(page.locator('body')).toContainText(/glare|retake|resubmit/i);
  });

  test('MICR failure routes to review queue', async ({ page }) => {
    await submitDepositUI(page, { scenario: 'micr_failure' });

    await expect(page.locator('[data-state]')).toContainText(/analyzing/i);

    // Check it appears in review queue
    await page.locator('a.nav-level-tab', { hasText: 'Review Queue' }).click();
    await page.waitForURL(/\/ui\/review/);
    await expect(page.locator('[data-review-item]').first()).toBeVisible({ timeout: 10000 });
    await expect(page.locator('body')).toContainText(/micr_failure/);
  });

  test('Duplicate detected results in rejection', async ({ page }) => {
    await submitDepositUI(page, { scenario: 'duplicate_detected' });

    await expect(page.locator('[data-state]')).toContainText(/rejected/i);
    await expect(page.locator('body')).toContainText(/duplicate/i);
  });

  test('Amount mismatch routes to review queue', async ({ page }) => {
    await submitDepositUI(page, { scenario: 'amount_mismatch' });

    await expect(page.locator('[data-state]')).toContainText(/analyzing/i);

    // Check it appears in review queue
    await page.locator('a.nav-level-tab', { hasText: 'Review Queue' }).click();
    await page.waitForURL(/\/ui\/review/);
    await expect(page.locator('[data-review-item]').first()).toBeVisible({ timeout: 10000 });
    await expect(page.locator('body')).toContainText(/mismatch|amount/i);
  });

  test('IQA pass review routes to review queue (high risk)', async ({ page }) => {
    await submitDepositUI(page, { scenario: 'iqa_pass_review' });

    await expect(page.locator('[data-state]')).toContainText(/analyzing/i);

    // Check it appears in review queue
    await page.locator('a.nav-level-tab', { hasText: 'Review Queue' }).click();
    await page.waitForURL(/\/ui\/review/);
    await expect(page.locator('[data-review-item]').first()).toBeVisible({ timeout: 10000 });
  });
});
