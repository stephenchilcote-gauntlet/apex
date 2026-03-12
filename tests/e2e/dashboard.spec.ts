import { test, expect } from './fixtures';

test.describe('Dashboard', () => {
  test.beforeEach(async ({ page, request }) => {
    await request.post('/api/v1/test/seed');
    await page.goto('/ui');
  });

  test('honeycomb state cells link to transfers filtered by state', async ({ page }) => {
    const stateCells = page.locator('a.hive-cell[href*="/ui/transfers?state="]');
    await expect(stateCells.first()).toBeVisible();
    const count = await stateCells.count();
    expect(count).toBeGreaterThanOrEqual(1);
  });

  test('FundsPosted honeycomb cell is present', async ({ page }) => {
    await expect(page.locator('.hive-cell--FundsPosted')).toBeVisible();
  });

  test('clicking FundsPosted cell navigates to filtered transfers', async ({ page }) => {
    await page.locator('.hive-cell--FundsPosted').click();
    await expect(page).toHaveURL(/\/ui\/transfers\?state=FundsPosted/);
  });

  test('action cell for Simulate is present', async ({ page }) => {
    await expect(page.locator('a.hive-cell--action[href="/ui/simulate"]')).toBeVisible();
  });

  test('action cell for Settlement is present', async ({ page }) => {
    await expect(page.locator('a.hive-cell--action[href="/ui/settlement"]')).toBeVisible();
  });

  test('action cell for Returns is present', async ({ page }) => {
    await expect(page.locator('a.hive-cell--action[href="/ui/returns"]')).toBeVisible();
  });

  test('action cell for Ledger is present', async ({ page }) => {
    await expect(page.locator('a.hive-cell--action[href="/ui/ledger"]')).toBeVisible();
  });

  test('KPI action cards row is present', async ({ page }) => {
    await expect(page.locator('.dash-action-row')).toBeVisible();
  });

  test('KPI action card values are present', async ({ page }) => {
    const values = page.locator('.action-card-value');
    await expect(values.first()).toBeVisible();
    const count = await values.count();
    expect(count).toBeGreaterThanOrEqual(1);
  });
});
