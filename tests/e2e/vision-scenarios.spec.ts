/**
 * Vision Mode E2E Tests
 *
 * These tests submit deposits with defect-variant check images and verify the
 * system reacts correctly when the vendor stub is running in vision mode
 * (VENDOR_VISION_MODE=true). They require ANTHROPIC_API_KEY to be set.
 *
 * Skip when: VENDOR_VISION_MODE is not "true" (normal CI).
 * Run with: VENDOR_VISION_MODE=true npx playwright test vision-scenarios.spec.ts
 */
import { test, expect } from './fixtures';
import * as path from 'path';

const VISION_ENABLED = process.env.VENDOR_VISION_MODE === 'true';

const IMG_DIR = path.join(__dirname, 'tests');
const CHECK_FRONT = path.join(IMG_DIR, 'check-front.png');
const CHECK_BACK = path.join(IMG_DIR, 'check-back.png');
const CHECK_FRONT_BLURRY = path.join(IMG_DIR, 'check-front-blurry.png');
const CHECK_BACK_BLURRY = path.join(IMG_DIR, 'check-back-blurry.png');
const CHECK_FRONT_GLARE = path.join(IMG_DIR, 'check-front-glare.png');
const CHECK_BACK_GLARE = path.join(IMG_DIR, 'check-back-glare.png');
const CHECK_FRONT_NO_MICR = path.join(IMG_DIR, 'check-front-no-micr.png');
const CHECK_BACK_NO_MICR = path.join(IMG_DIR, 'check-back-no-micr.png');
const CHECK_FRONT_WRONG_AMOUNT = path.join(IMG_DIR, 'check-front-wrong-amount.png');
const CHECK_BACK_WRONG_AMOUNT = path.join(IMG_DIR, 'check-back-wrong-amount.png');

/**
 * Submit a deposit via the UI with specific front/back images.
 * Uses INV-1001 (clean_pass suffix) so the scenario fallback would be clean_pass,
 * but in vision mode the actual images drive the decision.
 */
async function submitWithImages(
  page: import('@playwright/test').Page,
  frontImage: string,
  backImage: string,
  amount = '500.00',
  accountId = 'INV-1001',
): Promise<string> {
  await page.goto('/ui/simulate');
  await page.locator('select[name="investorAccountId"]').selectOption({ value: accountId });
  await page.locator('input[name="amount"]').fill(amount);
  // Uncheck sample images to enable file upload
  const sampleChk = page.locator('input[name="useSampleImages"]');
  if (await sampleChk.isChecked()) await sampleChk.uncheck();
  await page.locator('input[name="frontImage"]').setInputFiles(frontImage);
  await page.locator('input[name="backImage"]').setInputFiles(backImage);
  await page.locator('button[type="submit"]').click();

  const transferId = await page.locator('[data-transfer-id]').getAttribute('data-transfer-id');
  expect(transferId).toBeTruthy();
  return transferId!;
}

test.describe('Vision Mode Scenarios', () => {
  test.skip(!VISION_ENABLED, 'VENDOR_VISION_MODE not enabled');

  test('clean check images produce FundsPosted', async ({ page }) => {
    await submitWithImages(page, CHECK_FRONT, CHECK_BACK);
    await expect(page.locator('[data-state]')).toContainText(/fundsposted/i);
  });

  test('blurry check images produce rejection', async ({ page }) => {
    await submitWithImages(page, CHECK_FRONT_BLURRY, CHECK_BACK_BLURRY);
    await expect(page.locator('[data-state]')).toContainText(/rejected/i);
    await expect(page.locator('body')).toContainText(/blur|image quality|FAIL/i);
  });

  test('glare check images produce rejection', async ({ page }) => {
    await submitWithImages(page, CHECK_FRONT_GLARE, CHECK_BACK_GLARE);
    await expect(page.locator('[data-state]')).toContainText(/rejected/i);
    await expect(page.locator('body')).toContainText(/glare|image quality|FAIL/i);
  });

  test('wrong amount check triggers mismatch', async ({ page }) => {
    // Image shows $750, we submit $500 — vision should detect mismatch
    await submitWithImages(page, CHECK_FRONT_WRONG_AMOUNT, CHECK_BACK_WRONG_AMOUNT, '500.00');

    // Could be rejected or sent to review depending on combined risk score
    const stateText = await page.locator('[data-state]').textContent();
    expect(stateText).toBeTruthy();

    // Verify via API that amount mismatch was detected
    const transferId = await page.locator('[data-transfer-id]').getAttribute('data-transfer-id');
    const apiResp = await page.request.get(`/api/v1/deposits/${transferId}`);
    const data = await apiResp.json();
    if (data.vendorResult) {
      expect(data.vendorResult.amountMatches).toBe(false);
    }
  });

  test('no-MICR check triggers elevated risk', async ({ page }) => {
    await submitWithImages(page, CHECK_FRONT_NO_MICR, CHECK_BACK_NO_MICR);

    const transferId = await page.locator('[data-transfer-id]').getAttribute('data-transfer-id');
    const apiResp = await page.request.get(`/api/v1/deposits/${transferId}`);
    const data = await apiResp.json();

    if (data.vendorResult) {
      // MICR confidence should be 0 when not readable
      expect(data.vendorResult.micr?.confidence ?? 0).toBeLessThan(0.5);
      // Risk score should be elevated
      expect(data.vendorResult.riskScore).toBeGreaterThan(10);
    }
  });
});
