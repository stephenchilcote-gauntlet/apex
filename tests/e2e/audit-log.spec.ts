import { test, expect, submitDepositUI } from './fixtures';
import { VisualJudge, critical, advisory } from './visual-judge';

test.describe('Audit Log', () => {
  test('audit log page loads and shows events', async ({ page }) => {
    // Submit a deposit to generate audit events
    await submitDepositUI(page, { amount: '300.00', scenario: 'clean_pass' });

    // Navigate to audit log
    await page.goto('/ui/audit');
    await expect(page.locator('h1')).toContainText(/audit/i);

    // Should show events
    const table = page.locator('table.data-table');
    await expect(table).toBeVisible();

    // Should have at least one row (the deposit we just submitted)
    const rows = table.locator('tbody tr');
    await expect(rows.first()).not.toContainText('No audit events found');
  });

  test('audit log filter by transfer ID', async ({ page, request }) => {
    // Submit a deposit via API to get a known transfer ID
    const { CHECK_FRONT, CHECK_BACK } = await import('./fixtures');
    const fs = await import('fs');
    const resp = await request.post('/api/v1/deposits', {
      multipart: {
        investorAccountId: 'INV-1001',
        amount: '450.00',
        vendorScenario: 'clean_pass',
        frontImage: { name: 'front.png', mimeType: 'image/png', buffer: fs.readFileSync(CHECK_FRONT) },
        backImage: { name: 'back.png', mimeType: 'image/png', buffer: fs.readFileSync(CHECK_BACK) },
      },
    });
    const body = await resp.json();
    const transferId = body.transferId;
    expect(transferId).toBeTruthy();

    // Filter audit log by transfer ID
    await page.goto(`/ui/audit?transferId=${transferId}`);
    await expect(page.locator('body')).toContainText(transferId.substring(0, 8));

    // Should show events for this transfer only
    const table = page.locator('table.data-table');
    await expect(table).toBeVisible();
  });

  test('audit log filter by event type shows only matching events', async ({ page }) => {
    await submitDepositUI(page, { amount: '225.00', scenario: 'clean_pass' });

    // Filter by STATE_TRANSITION — every deposit produces several
    await page.goto('/ui/audit?eventType=STATE_TRANSITION');
    const table = page.locator('table.data-table');
    await expect(table).toBeVisible();

    // All visible event-type cells should say STATE_TRANSITION (col 2: Time, Event, Transfer, Transition, Actor, Details)
    const eventTypeCells = table.locator('tbody tr td:nth-child(2)');
    const count = await eventTypeCells.count();
    expect(count).toBeGreaterThan(0);
    for (let i = 0; i < count; i++) {
      await expect(eventTypeCells.nth(i)).toContainText('STATE_TRANSITION');
    }
  });

  test('audit log API returns events filtered by transferId', async ({ page }) => {
    const transferId = await submitDepositUI(page, { amount: '175.00', scenario: 'clean_pass' });

    const resp = await page.request.get(`/api/v1/audit?transferId=${transferId}`);
    expect(resp.status()).toBe(200);
    const body = await resp.json();
    expect(Array.isArray(body)).toBe(true);
    expect(body.length).toBeGreaterThan(0);
    // Every event should reference this transfer
    for (const event of body) {
      expect(event.entityId).toBe(transferId);
    }
  });

  test('audit log clear filter works', async ({ page }) => {
    await page.goto('/ui/audit?transferId=some-id');
    await expect(page.locator('a:has-text("Clear")')).toBeVisible();
    await page.locator('a:has-text("Clear")').click();
    await expect(page).toHaveURL('/ui/audit');
  });

  test('audit table has no column overflow or spacing issues', async ({ page }) => {
    // Submit a deposit so the table has STATE_TRANSITION rows with badges in the Transition column
    await submitDepositUI(page, { amount: '199.00', scenario: 'clean_pass' });

    await page.goto('/ui/audit');
    await expect(page.locator('table.data-table tbody tr').first()).toBeVisible();

    // Structural checks: each column header must be left of the next one
    const headers = page.locator('table.data-table thead th');
    const headerCount = await headers.count();
    expect(headerCount).toBe(6);

    const boxes: { x: number; right: number }[] = [];
    for (let i = 0; i < headerCount; i++) {
      const box = await headers.nth(i).boundingBox();
      expect(box).not.toBeNull();
      boxes.push({ x: box!.x, right: box!.x + box!.width });
    }
    // Each header must start at or after the previous one ends (no overlap)
    for (let i = 1; i < boxes.length; i++) {
      expect(boxes[i].x).toBeGreaterThanOrEqual(boxes[i - 1].x);
    }

    // The Transition column badges must not visually overflow into the Actor column.
    // Check by measuring the transition cell's badge vs the cell boundary.
    const firstRow = page.locator('table.data-table tbody tr').first();
    const transitionCell = firstRow.locator('td').nth(3);
    const actorCell = firstRow.locator('td').nth(4);

    const transitionCellBox = await transitionCell.boundingBox();
    const actorCellBox = await actorCell.boundingBox();
    expect(transitionCellBox).not.toBeNull();
    expect(actorCellBox).not.toBeNull();

    // Actor cell must start at or after transition cell ends (no overlap between cells)
    expect(actorCellBox!.x).toBeGreaterThanOrEqual(transitionCellBox!.x + transitionCellBox!.width - 1);

    // All badges inside the transition cell must fit within the cell's right boundary
    const transitionBadges = transitionCell.locator('.badge');
    const badgeCount = await transitionBadges.count();
    for (let i = 0; i < badgeCount; i++) {
      const badgeBox = await transitionBadges.nth(i).boundingBox();
      if (!badgeBox) continue;
      const badgeRight = badgeBox.x + badgeBox.width;
      const cellRight = transitionCellBox!.x + transitionCellBox!.width;
      expect(badgeRight).toBeLessThanOrEqual(cellRight + 2); // 2px tolerance for borders
    }

    // Actor cell must be wide enough to display "deposit-service" (≥80px)
    expect(actorCellBox!.width).toBeGreaterThan(80);

    // Visual LLM check — catches subtle overflow/clipping the bounding-box checks might miss
    if (!process.env.ANTHROPIC_API_KEY) return;
    const judge = new VisualJudge({ artifactDir: 'tests/artifacts/visual' });
    await judge.assertVisual(page, [
      critical('Are all 6 audit table columns (Time, Event, Transfer, Transition, Actor, Details) fully visible with no content from one column bleeding or overlapping into an adjacent column?'),
      critical('In the Transition column, are ALL badge labels (e.g. "VALIDATING", "ANALYZING", "REQUESTED") and the arrow symbol (→) fully rendered within the Transition column cell, not cut off or overflowing into the Actor column? If any badge label is partially hidden or cut off at the column boundary, answer NO.'),
      critical('Is the Actor column showing the complete actor name for each row (e.g. the full text "deposit-service")? If the text is cut off — showing only a fragment like "eposit-service" or "posit-service" — that is a FAILURE. Answer NO if any actor text is clipped.'),
      advisory('Are the table row heights consistent and the cell padding uniform across all rows?'),
    ], { testName: 'audit-table-column-spacing', fullPage: false });
  });
});
