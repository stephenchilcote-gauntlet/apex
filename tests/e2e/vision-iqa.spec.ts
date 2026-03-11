import * as fs from 'fs';
import { test, expect } from './fixtures';
import {
  CHECK_FRONT,
  CHECK_BACK,
  CHECK_FRONT_WRONG_AMOUNT,
  CHECK_BACK_WRONG_AMOUNT,
} from './fixtures';

/**
 * Vision IQA/OCR end-to-end tests.
 *
 * These run against the live app with VENDOR_VISION_MODE=true — real Claude
 * Haiku analysis on each submission.  They catch logic bugs that the unit
 * integration test misses because that test skips when ANTHROPIC_API_KEY is
 * not exported to the Go test environment.
 *
 * Each test submits images via the REST API (base64) and polls until the
 * transfer reaches a terminal state, then asserts the decision.
 */

const API_KEY = 'apex-dev-key-e2e';
const ACCOUNT_ID = 'INV-1001'; // any valid account — vision mode ignores account suffix

/** Poll GET /api/v1/deposits/{id} until state is terminal or timeout. */
async function waitForTerminal(
  request: any,
  transferId: string,
  maxMs = 30000,
): Promise<string> {
  const terminal = new Set(['FundsPosted', 'Analyzing', 'Rejected', 'Completed', 'Returned']);
  const deadline = Date.now() + maxMs;
  while (Date.now() < deadline) {
    const resp = await request.get(`/api/v1/deposits/${transferId}`, {
      headers: { 'X-API-Key': API_KEY },
    });
    const body = await resp.json();
    const state = body.transfer?.State ?? body.State;
    if (terminal.has(state)) return state;
    await new Promise(r => setTimeout(r, 800));
  }
  throw new Error(`Transfer ${transferId} did not reach terminal state within ${maxMs}ms`);
}

async function submitViaAPI(request: any, frontPath: string, backPath: string, amountCents: number): Promise<string> {
  const dollars = (amountCents / 100).toFixed(2);
  const resp = await request.post('/api/v1/deposits', {
    headers: { 'X-API-Key': API_KEY },
    multipart: {
      investorAccountId: ACCOUNT_ID,
      amount: dollars,
      frontImage: { name: 'front.png', mimeType: 'image/png', buffer: fs.readFileSync(frontPath) },
      backImage:  { name: 'back.png',  mimeType: 'image/png', buffer: fs.readFileSync(backPath)  },
    },
  });
  expect(resp.status(), 'deposit submission should succeed').toBe(200);
  const body = await resp.json();
  expect(body.transferId, 'response must include a transfer ID').toBeTruthy();
  return body.transferId;
}

test.describe('Vision IQA/OCR (real Claude Haiku)', () => {
  test('clean check → FundsPosted (PASS decision)', async ({ request }) => {
    const id = await submitViaAPI(request, CHECK_FRONT, CHECK_BACK, 50000);
    const state = await waitForTerminal(request, id);
    expect(state, `clean check should reach FundsPosted, got ${state}`).toBe('FundsPosted');
  });

  test('wrong-amount check → Analyzing/review (REVIEW decision)', async ({ request }) => {
    // Image shows $750, declared amount is $500 — Claude Haiku should detect mismatch
    const id = await submitViaAPI(request, CHECK_FRONT_WRONG_AMOUNT, CHECK_BACK_WRONG_AMOUNT, 50000);
    const state = await waitForTerminal(request, id);
    expect(
      state,
      `amount-mismatch check must be routed to review (Analyzing), got ${state} — ` +
      `vision mode may not be enabled or mapEvidenceToResponse is not treating mismatch as hard REVIEW`,
    ).toBe('Analyzing');

    // Also confirm review_required flag is set
    const resp = await request.get(`/api/v1/deposits/${id}`, {
      headers: { 'X-API-Key': API_KEY },
    });
    const body = await resp.json();
    const reviewRequired = body.transfer?.ReviewRequired ?? body.ReviewRequired;
    expect(reviewRequired, 'ReviewRequired must be true for amount-mismatch deposits').toBe(true);
  });
});
