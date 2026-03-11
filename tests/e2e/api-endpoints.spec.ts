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

  test('GET /api/v1/deposits?state=FundsPosted filters by state', async ({ page }) => {
    await submitDepositUI(page, { amount: '275.00', scenario: 'clean_pass' });

    const resp = await page.request.get('/api/v1/deposits?state=FundsPosted');
    expect(resp.status()).toBe(200);
    const body = await resp.json();
    expect(Array.isArray(body)).toBe(true);
    expect(body.length).toBeGreaterThan(0);
    // All results should be in FundsPosted state
    for (const deposit of body) {
      expect(deposit.State).toBe('FundsPosted');
    }
  });

  test('GET /api/v1/deposits?state=Rejected filters to rejected only', async ({ page }) => {
    await submitDepositUI(page, { scenario: 'iqa_blur' });

    const resp = await page.request.get('/api/v1/deposits?state=Rejected');
    expect(resp.status()).toBe(200);
    const body = await resp.json();
    expect(Array.isArray(body)).toBe(true);
    expect(body.length).toBeGreaterThan(0);
    for (const deposit of body) {
      expect(deposit.State).toBe('Rejected');
    }
  });

  test('GET /api/v1/deposits?investorAccountId filters to specific account', async ({ page }) => {
    // Submit using INV-1002 (iqa_blur scenario routes to INV-1002)
    await submitDepositUI(page, { scenario: 'iqa_blur' });

    // Get the account UUID for INV-1002
    const acctResp = await page.request.get('/api/v1/ledger/accounts');
    const accounts = await acctResp.json();
    const acct1002 = accounts.find((a: any) => a.externalAccountId === 'INV-1002');
    expect(acct1002).toBeTruthy();

    const resp = await page.request.get(`/api/v1/deposits?investorAccountId=${acct1002.id}`);
    expect(resp.status()).toBe(200);
    const body = await resp.json();
    expect(Array.isArray(body)).toBe(true);
    expect(body.length).toBeGreaterThan(0);
    for (const deposit of body) {
      expect(deposit.InvestorAccountID).toBe(acct1002.id);
    }
  });

  test('GET /api/v1/deposits?reviewRequired=true filters to review queue only', async ({ page }) => {
    await submitDepositUI(page, { scenario: 'micr_failure' });

    const resp = await page.request.get('/api/v1/deposits?reviewRequired=true');
    expect(resp.status()).toBe(200);
    const body = await resp.json();
    expect(Array.isArray(body)).toBe(true);
    expect(body.length).toBeGreaterThan(0);
    for (const deposit of body) {
      expect(deposit.ReviewRequired).toBe(true);
    }
  });

  test('GET /api/v1/ledger/accounts/{accountId} returns account with entries', async ({ page }) => {
    await submitDepositUI(page, { amount: '125.00', scenario: 'clean_pass' });

    // Get account list to find a valid account ID
    const listResp = await page.request.get('/api/v1/ledger/accounts');
    const accounts = await listResp.json();
    const account = accounts.find((a: any) => a.externalAccountId === 'INV-1001');
    expect(account).toBeTruthy();

    const resp = await page.request.get(`/api/v1/ledger/accounts/${account.id}`);
    expect(resp.status()).toBe(200);
    const body = await resp.json();
    expect(body).toHaveProperty('account');
    expect(body).toHaveProperty('entries');
    expect(body.account.externalAccountId).toBe('INV-1001');
  });

  test('GET /api/v1/ledger/journals returns journals for a transfer', async ({ page }) => {
    const transferId = await submitDepositUI(page, { amount: '225.00', scenario: 'clean_pass' });

    const resp = await page.request.get(`/api/v1/ledger/journals?transferId=${transferId}`);
    expect(resp.status()).toBe(200);
    const body = await resp.json();
    expect(Array.isArray(body)).toBe(true);
    expect(body.length).toBeGreaterThan(0);
    // Each journal should reference this transfer
    // Journal struct has no json tags — fields are PascalCase
    for (const journal of body) {
      expect(journal).toHaveProperty('ID');
      expect(journal).toHaveProperty('JournalType');
    }
    // A clean_pass deposit should produce a DEPOSIT_POSTING journal
    const depositJournal = body.find((j: any) => j.JournalType === 'DEPOSIT_POSTING');
    expect(depositJournal).toBeTruthy();
  });

  test('GET /api/v1/deposits/{id}/decision-trace returns full audit trail', async ({ page }) => {
    const transferId = await submitDepositUI(page, { amount: '350.00', scenario: 'clean_pass' });

    const resp = await page.request.get(`/api/v1/deposits/${transferId}/decision-trace`);
    expect(resp.status()).toBe(200);
    const body = await resp.json();
    expect(Array.isArray(body)).toBe(true);
    expect(body.length).toBeGreaterThan(0);
    // Each trace event should have required fields (audit.Event uses PascalCase — no json tags)
    const event = body[0];
    expect(event).toHaveProperty('EventType');
    expect(event).toHaveProperty('EntityID');
    expect(event.EntityID).toBe(transferId);
  });

  test('POST /api/v1/settlement/batches/generate creates a batch', async ({ page }) => {
    await submitDepositUI(page, { amount: '600.00', scenario: 'clean_pass' });

    const resp = await page.request.post('/api/v1/settlement/batches/generate', {
      data: {},
    });
    expect(resp.status()).toBe(200);
    const body = await resp.json();
    expect(body).toHaveProperty('ID');
    expect(body).toHaveProperty('Status');
    expect(body.Status).toBe('GENERATED');
    expect(body).toHaveProperty('TotalItems');
    expect(body.TotalItems).toBeGreaterThan(0);
    expect(body).toHaveProperty('TotalAmountCents');
    expect(body.TotalAmountCents).toBeGreaterThan(0);
  });

  test('GET /api/v1/settlement/batches/{batchId} returns batch detail', async ({ page }) => {
    await submitDepositUI(page, { amount: '550.00', scenario: 'clean_pass' });

    const genResp = await page.request.post('/api/v1/settlement/batches/generate', { data: {} });
    expect(genResp.status()).toBe(200);
    const batch = await genResp.json();
    const batchId = batch.ID;

    const resp = await page.request.get(`/api/v1/settlement/batches/${batchId}`);
    expect(resp.status()).toBe(200);
    const body = await resp.json();
    expect(body).toHaveProperty('batch');
    expect(body).toHaveProperty('items');
    expect(body.batch.ID).toBe(batchId);
    expect(Array.isArray(body.items)).toBe(true);
    expect(body.items.length).toBeGreaterThan(0);
    // BatchItem PascalCase fields
    expect(body.items[0]).toHaveProperty('TransferID');
    expect(body.items[0]).toHaveProperty('AmountCents');
  });

  test('POST /api/v1/settlement/batches/{batchId}/ack acknowledges batch', async ({ page }) => {
    await submitDepositUI(page, { amount: '475.00', scenario: 'clean_pass' });

    const genResp = await page.request.post('/api/v1/settlement/batches/generate', { data: {} });
    expect(genResp.status()).toBe(200);
    const batch = await genResp.json();
    const batchId = batch.ID;

    const resp = await page.request.post(`/api/v1/settlement/batches/${batchId}/ack`, {
      data: { ackReference: 'ACK-API-TEST-001' },
    });
    expect(resp.status()).toBe(200);
    const body = await resp.json();
    expect(body).toHaveProperty('batchId');
    expect(body.batchId).toBe(batchId);
    expect(body).toHaveProperty('status');
    expect(body.status).toBe('ACKNOWLEDGED');
  });

  test('POST /api/v1/returns processes a return on a completed deposit', async ({ page }) => {
    const transferId = await submitDepositUI(page, { amount: '325.00', scenario: 'clean_pass' });

    // Complete the deposit via settlement
    const genResp = await page.request.post('/api/v1/settlement/batches/generate', { data: {} });
    expect(genResp.status()).toBe(200);
    const batch = await genResp.json();
    await page.request.post(`/api/v1/settlement/batches/${batch.ID}/ack`, {
      data: { ackReference: 'ACK-RETURN-TEST' },
    });

    const resp = await page.request.post('/api/v1/returns', {
      data: { transferId, reasonCode: 'NSF', reasonText: 'Insufficient funds' },
    });
    expect(resp.status()).toBe(200);
    const body = await resp.json();
    expect(body).toHaveProperty('transferId');
    expect(body.transferId).toBe(transferId);
    expect(body).toHaveProperty('status');
    expect(body.status).toBe('RETURNED');
  });

  test('POST /api/v1/operator/transfers/{id}/approve approves flagged deposit', async ({ page }) => {
    const transferId = await submitDepositUI(page, { scenario: 'micr_failure' });

    const resp = await page.request.post(`/api/v1/operator/transfers/${transferId}/approve`, {
      data: { notes: 'Approved via API test', operatorId: 'test-operator' },
    });
    expect(resp.status()).toBe(200);
    const body = await resp.json();
    expect(body).toHaveProperty('state');
    expect(body.state).toMatch(/fundsposted|approved/i);
  });

  test('POST /api/v1/operator/transfers/{id}/reject rejects flagged deposit', async ({ page }) => {
    const transferId = await submitDepositUI(page, { scenario: 'amount_mismatch' });

    const resp = await page.request.post(`/api/v1/operator/transfers/${transferId}/reject`, {
      data: { notes: 'Rejected via API test', rejectionCode: 'MANUAL_REVIEW_FAILED', operatorId: 'test-operator' },
    });
    expect(resp.status()).toBe(200);
    const body = await resp.json();
    expect(body).toHaveProperty('state');
    expect(body.state).toBe('Rejected');
  });
});
