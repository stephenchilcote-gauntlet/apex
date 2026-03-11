import { test, expect, submitDepositUI } from './fixtures';
import { VisualJudge, critical } from './visual-judge';

let judge: VisualJudge | undefined;

test.describe('Settlement', () => {
  test('settlement page shows batch management', async ({ page }) => {
    await page.goto('/ui/settlement');
    await expect(page.locator('h1, h2')).toContainText(/settlement/i);
    await expect(page.locator('[data-action="generate"]')).toBeVisible();
  });

  test('generate settlement batch from posted deposits', async ({ page }) => {
    await submitDepositUI(page, { amount: '600.00', scenario: 'clean_pass' });

    // Navigate to settlement and generate
    await page.locator('a.nav-level-tab', { hasText: 'Settlement' }).click();
    await page.locator('[data-action="generate"]').click();

    // Verify batch table content: status badge, items count, total amount
    await expect(page.locator('body')).toContainText(/generated/i);
    const batchRow = page.locator('table tbody tr').first();
    await expect(batchRow.locator('[data-state]')).toContainText(/generated/i);
    await expect(batchRow).toContainText('1'); // 1 item
    await expect(batchRow).toContainText('$600.00');

    // Visual layout checks — catch overlapping/overflow issues
    if (!judge) {
      judge = new VisualJudge({ artifactDir: 'tests/artifacts/visual' });
    }
    await judge.assertVisual(page, [
      critical('Is the Batches table fully contained within its panel, with no content overflowing or clipped outside the panel borders?'),
      critical('Are all table columns (ID, Business Date, Status, Items, Total Amount, File, Created, Action) visible and non-overlapping, with each column header aligned above its data?'),
      critical('Are all table cell values readable with no text overlapping adjacent cells? Note: the ID column intentionally shows a truncated UUID with ellipsis, and the File column uses CSS text-overflow ellipsis for long filenames — these are expected.'),
      critical('Is the Actions panel with the "Generate Settlement Batch" button visually separate from the Batches table, with no overlapping between the two panels?'),
    ], { testName: 'settlement-table-layout', fullPage: true });
  });

  test('batch ID link opens batch detail page', async ({ page }) => {
    await submitDepositUI(page, { amount: '400.00', scenario: 'clean_pass' });

    await page.goto('/ui/settlement');
    await page.locator('[data-action="generate"]').click();
    await expect(page.locator('body')).toContainText(/generated/i);

    // Click the batch ID link
    await page.locator('table tbody tr a[href*="/ui/settlement/"]').first().click();

    // Verify batch detail page
    await expect(page.locator('h1')).toContainText(/batch detail/i);
    await expect(page.locator('.detail-grid')).toBeVisible();
    await expect(page.locator('body')).toContainText(/GENERATED/i);
    await expect(page.locator('body')).toContainText('$400.00');

    // Items table should show the transfer
    const itemsTable = page.locator('table.data-table');
    await expect(itemsTable).toBeVisible();
    const rows = itemsTable.locator('tbody tr');
    await expect(rows.first()).not.toContainText('No');
  });

  test('acknowledging batch moves transfers to Completed', async ({ page }) => {
    await submitDepositUI(page, { amount: '300.00', scenario: 'clean_pass' });

    // Generate batch
    await page.locator('a.nav-level-tab', { hasText: 'Settlement' }).click();
    await page.locator('[data-action="generate"]').click();
    await expect(page.locator('body')).toContainText(/generated/i);

    // Acknowledge
    await page.locator('[data-action="ack"]').first().click();
    await expect(page.locator('body')).toContainText(/acknowledged/i);

    // Verify batch status changed
    const batchRow = page.locator('table tbody tr').first();
    await expect(batchRow.locator('[data-state]')).toContainText(/acknowledged/i);

    // Verify transfer completed by clicking through
    await page.locator('a.nav-level-tab', { hasText: 'Transfers' }).click();
    await page.locator('[data-transfer] a').first().click();
    await expect(page.locator('[data-state]')).toContainText(/completed/i);
  });
});
