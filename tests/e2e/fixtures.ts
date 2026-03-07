import { test as base, expect } from '@playwright/test';

export const test = base.extend({
  resetState: [async ({ request }, use) => {
    const resp = await request.post('/api/v1/test/reset');
    expect(resp.ok()).toBeTruthy();
    await use();
  }, { auto: true, scope: 'test' }],
});

export { expect };
