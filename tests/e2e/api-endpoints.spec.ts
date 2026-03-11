import { test, expect, submitDepositUI } from './fixtures';

/**
 * API endpoint tests — verify core REST API endpoints return correct
 * status codes, content types, and response shapes.
 */
test.describe('API Endpoints', () => {
  test('GET /healthz returns healthy status', async ({ page }) => {
    const resp = await page.request.get('/healthz');
    expect(resp.status()).toBe(200);
    const body = await resp.json();
    expect(body.status).toBe('ok');
    expect(body.db).toBe('ok');
    expect(body.vendor).toBe('ok');
  });

  test('GET /api/v1/metrics returns transfer statistics', async ({ page }) => {
    await submitDepositUI(page, { amount: '500.00', scenario: 'clean_pass' });

    const resp = await page.request.get('/api/v1/metrics');
    expect(resp.status()).toBe(200);
    const body = await resp.json();
    expect(body).toHaveProperty('transfers');
    expect(body.transfers).toHaveProperty('total');
    expect(body.transfers.total).toBeGreaterThan(0);
    expect(body).toHaveProperty('volume');
    expect(body.volume).toHaveProperty('total_cents');
  });

  test('GET /api/v1/deposits lists deposits', async ({ page }) => {
    await submitDepositUI(page, { amount: '350.00', scenario: 'clean_pass' });

    const resp = await page.request.get('/api/v1/deposits');
    expect(resp.status()).toBe(200);
    const body = await resp.json();
    // API returns [] (not null) for empty list
    expect(Array.isArray(body)).toBe(true);
    // Find our $350 deposit
    const deposit = body.find((d: any) => d.AmountCents === 35000);
    expect(deposit).toBeTruthy();
    // Verify Go struct PascalCase field names
    expect(deposit).toHaveProperty('ID');
    expect(deposit).toHaveProperty('State');
    expect(deposit).toHaveProperty('AmountCents');
  });

  test('GET /api/v1/deposits/{id} returns deposit detail', async ({ page }) => {
    const transferId = await submitDepositUI(page, { amount: '250.00', scenario: 'clean_pass' });

    const resp = await page.request.get(`/api/v1/deposits/${transferId}`);
    expect(resp.status()).toBe(200);
    const body = await resp.json();
    // Response is a map with 'transfer', 'vendorResult', 'ruleEvaluations', 'auditEvents'
    expect(body).toHaveProperty('transfer');
    expect(body.transfer.ID).toBe(transferId);
    expect(body).toHaveProperty('vendorResult');
    expect(body).toHaveProperty('ruleEvaluations');
  });

  test('GET /api/v1/audit returns audit events', async ({ page }) => {
    await submitDepositUI(page, { amount: '450.00', scenario: 'clean_pass' });

    const resp = await page.request.get('/api/v1/audit');
    expect(resp.status()).toBe(200);
    const body = await resp.json();
    expect(Array.isArray(body)).toBe(true);
    expect(body.length).toBeGreaterThan(0);
    // Verify event shape
    const event = body[0];
    expect(event).toHaveProperty('eventType');
    expect(event).toHaveProperty('createdAt');
  });

  test('GET /api/v1/ledger/accounts returns account list', async ({ page }) => {
    const resp = await page.request.get('/api/v1/ledger/accounts');
    expect(resp.status()).toBe(200);
    const body = await resp.json();
    expect(Array.isArray(body)).toBe(true);
    expect(body.length).toBeGreaterThan(0);
    const account = body[0];
    expect(account).toHaveProperty('externalAccountId');
    expect(account).toHaveProperty('balanceCents');
  });

  test('GET /api/v1/settlement/batches returns empty array when no batches', async ({ page }) => {
    const resp = await page.request.get('/api/v1/settlement/batches');
    expect(resp.status()).toBe(200);
    const body = await resp.json();
    // API returns [] (not null) for empty list
    expect(Array.isArray(body)).toBe(true);
  });

  test('GET /api/v1/operator/review-queue returns flagged deposits', async ({ page }) => {
    await submitDepositUI(page, { scenario: 'micr_failure' });

    const resp = await page.request.get('/api/v1/operator/review-queue');
    expect(resp.status()).toBe(200);
    const body = await resp.json();
    expect(Array.isArray(body)).toBe(true);
    expect(body.length).toBeGreaterThan(0);
    // Verify the deposit is flagged for review
    expect(body[0].ReviewRequired).toBe(true);
    expect(body[0].State).toBe('Analyzing');
  });

  test('GET /api/v1/operator/review-queue returns empty array when no items', async ({ page }) => {
    const resp = await page.request.get('/api/v1/operator/review-queue');
    expect(resp.status()).toBe(200);
    const body = await resp.json();
    expect(Array.isArray(body)).toBe(true);
    expect(body.length).toBe(0);
  });
});
