import { test, expect } from './fixtures';

test.describe('Empty States', () => {
  test('transfers page shows empty message when no transfers', async ({ page }) => {
    await page.goto('/ui/transfers');
    await expect(page.locator('body')).toContainText('No transfers found');
  });

  test('review queue shows empty message when no items pending', async ({ page }) => {
    await page.goto('/ui/review');
    await expect(page.locator('body')).toContainText('No items pending review');
  });

  test('settlement page shows empty message when no batches', async ({ page }) => {
    await page.goto('/ui/settlement');
    await expect(page.locator('body')).toContainText('No batches found');
  });
});
