import { test, expect, submitDepositUI } from './fixtures';
import { VisualJudge, critical, advisory } from './visual-judge';

let judge: VisualJudge;

test.beforeAll(() => {
  judge = new VisualJudge({
    artifactDir: 'tests/artifacts/visual',
  });
});

test.describe('Visual Regression', () => {

  // ── Layout & Navigation ──────────────────────────────────────────

  test('simulate page has proper layout and form structure', async ({ page }) => {
    await page.goto('/ui/simulate');
    await judge.assertVisual(page, [
      critical('Is there a navigation bar at the top with multiple tab links including "Simulate", "Transfers", "Review Queue", "Ledger", "Settlement", and "Returns"?'),
      critical('Is there a form with labeled fields for investor account, amount, front image, back image, and a submit button?'),
      advisory('Is the overall layout clean with consistent spacing and alignment?'),
    ], { testName: 'layout-simulate-form', fullPage: true });
  });

  // ── Deposit Result States ────────────────────────────────────────

  test('successful deposit shows result panel with FundsPosted badge', async ({ page }) => {
    await submitDepositUI(page, { amount: '500.00', scenario: 'clean_pass' });
    await judge.assertVisual(page, [
      critical('Is there a "Deposit Result" panel showing a Transfer ID, a State badge, and a Message?'),
      critical('Does the State badge show "FundsPosted" or similar success state?'),
      advisory('Is the result panel visually distinct from the form below it?'),
    ], { testName: 'deposit-result-success', fullPage: true });
  });

  test('rejected deposit shows rejection info in result panel', async ({ page }) => {
    await submitDepositUI(page, { scenario: 'iqa_blur' });
    await judge.assertVisual(page, [
      critical('Is there a "Deposit Result" panel visible with a State badge showing "Rejected"?'),
      critical('Is there a Rejection field visible that contains the text "FAIL_BLUR" or "blur"?'),
    ], { testName: 'deposit-result-rejection', fullPage: true });
  });

  // ── Transfers List ───────────────────────────────────────────────

  test('transfers list shows table with correct columns', async ({ page }) => {
    await submitDepositUI(page, { amount: '100.00', scenario: 'clean_pass' });
    await page.locator('a.nav-level-tab', { hasText: 'Transfers' }).click();
    await expect(page.locator('[data-transfer]').first()).toBeVisible();

    await judge.assertVisual(page, [
      critical('Is there a table with column headers including "ID", "Account", "Amount", "State", and date-related columns?'),
      critical('Does the table contain at least one row of transfer data with a clickable ID link, dollar amount, and state badge?'),
      advisory('Is the "Transfers" tab highlighted as active in the navigation?'),
    ], { testName: 'transfers-list', fullPage: true });
  });

  // ── Transfer Detail ──────────────────────────────────────────────

  test('transfer detail shows all panels', async ({ page }) => {
    await submitDepositUI(page, { amount: '200.00', scenario: 'clean_pass' });
    await page.locator('a.nav-level-tab', { hasText: 'Transfers' }).click();
    await page.locator('[data-transfer] a').first().click();
    await expect(page.locator('[data-state]')).toBeVisible();

    await judge.assertVisual(page, [
      critical('Is there a "Transfer Info" panel showing ID, State badge, Amount, Account, and timestamps?'),
      critical('Is there a "Check Images" panel with front and back image placeholders or images?'),
      critical('Is there a "Vendor Result" panel showing decision details like IQA Status, MICR data, and Risk Score?'),
      critical('Is there an "Audit Trail" panel with a table of events showing Time, Event, From, To, and Actor columns?'),
    ], { testName: 'transfer-detail-panels', fullPage: true });
  });

  // ── Review Queue ─────────────────────────────────────────────────

  test('review queue shows flagged items in table', async ({ page }) => {
    await submitDepositUI(page, { scenario: 'micr_failure' });
    await page.locator('a.nav-level-tab', { hasText: 'Review Queue' }).click();
    await expect(page.locator('[data-review-item]').first()).toBeVisible();

    await judge.assertVisual(page, [
      critical('Is there a "Pending Reviews" table with columns for ID, Account, Amount, Scenario, Created, and Action?'),
      critical('Does the table have at least one row with a "Review" button or link?'),
      advisory('Is the "Review Queue" tab highlighted as active in the navigation?'),
    ], { testName: 'review-queue-populated', fullPage: true });
  });

  // ── Review Detail ────────────────────────────────────────────────

  test('review detail shows transfer info and approve/reject forms', async ({ page }) => {
    await submitDepositUI(page, { scenario: 'amount_mismatch' });
    await page.locator('a.nav-level-tab', { hasText: 'Review Queue' }).click();
    await page.locator('[data-review-item] a.btn', { hasText: 'Review' }).first().click();

    await judge.assertVisual(page, [
      critical('Is there a "Transfer Info" panel showing the transfer details including State badge and amounts?'),
      critical('Are there two side-by-side or stacked sections: one for "Approve" with a notes textarea and approve button, and one for "Reject" with a notes textarea and reject button?'),
      critical('Is there a "Check Images" panel showing front and back check images?'),
    ], { testName: 'review-detail-forms', fullPage: true });
  });

  // ── Settlement ───────────────────────────────────────────────────

  test('settlement shows batch table after generation', async ({ page }) => {
    await submitDepositUI(page, { amount: '600.00', scenario: 'clean_pass' });
    await page.locator('a.nav-level-tab', { hasText: 'Settlement' }).click();
    await page.locator('[data-action="generate"]').click();
    await expect(page.locator('body')).toContainText(/generated/i);

    await judge.assertVisual(page, [
      critical('Is there a "Generate Settlement Batch" button in an Actions panel?'),
      critical('Is there a Batches table with columns including ID, Business Date, Status, Items, Total Amount, and Action?'),
      critical('Does the batch table have at least one row with a status badge showing "GENERATED" and an "Acknowledge" button?'),
    ], { testName: 'settlement-batch-generated', fullPage: true });
  });

  // ── Returns ──────────────────────────────────────────────────────

  test('returns page shows form and returned transfer after processing', async ({ page }) => {
    // Create a completed deposit first
    const transferId = await submitDepositUI(page, { amount: '400.00', scenario: 'clean_pass' });
    await page.locator('a.nav-level-tab', { hasText: 'Settlement' }).click();
    await page.locator('[data-action="generate"]').click();
    await expect(page.locator('body')).toContainText(/generated/i);
    await page.locator('[data-action="ack"]').first().click();
    await expect(page.locator('body')).toContainText(/acknowledged/i);

    // Process the return
    await page.locator('a.nav-level-tab', { hasText: 'Returns' }).click();
    await page.locator('input[name="transferId"]').fill(transferId);
    await page.locator('select[name="reasonCode"]').selectOption('NSF');
    await page.locator('button[type="submit"]').click();
    await expect(page.locator('[data-state]')).toContainText(/returned/i);

    await judge.assertVisual(page, [
      critical('Is there a "Process Return" form with fields for Transfer ID and Reason Code, plus a submit button?'),
      critical('Is there a "Returned Transfer" panel showing ID, State badge with "Returned", Amount, Return Reason "NSF", Return Fee, and Returned At timestamp?'),
      advisory('Is the "Returns" tab highlighted as active in the navigation?'),
    ], { testName: 'returns-processed', fullPage: true });
  });

  // ── Ledger ───────────────────────────────────────────────────────

  test('ledger shows accounts table with balances', async ({ page }) => {
    await submitDepositUI(page, { amount: '250.00', scenario: 'clean_pass' });
    await page.locator('a.nav-level-tab', { hasText: 'Ledger' }).click();
    await expect(page.locator('body')).toContainText(/250/);

    await judge.assertVisual(page, [
      critical('Is there an "Account Balances" table with columns "External ID", "Name", "Type", and "Balance"?'),
      critical('Does the table show investor accounts (INV-prefixed), an omnibus account, and a fee revenue account?'),
      critical('Are dollar amounts displayed with proper currency formatting (e.g., $250.00)?'),
    ], { testName: 'ledger-with-balances', fullPage: true });
  });

  // ── Empty States ─────────────────────────────────────────────────

  test('empty transfers page shows proper empty state', async ({ page }) => {
    await page.goto('/ui/transfers');

    await judge.assertVisual(page, [
      critical('Is there a table structure visible with column headers but showing a "No transfers found" message in the body?'),
      advisory('Is the empty state message centered and clearly visible?'),
    ], { testName: 'empty-transfers', fullPage: true });
  });

});
