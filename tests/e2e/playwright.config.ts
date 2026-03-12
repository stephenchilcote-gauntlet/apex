import { defineConfig } from '@playwright/test';
import * as dotenv from 'dotenv';
import * as path from 'path';

dotenv.config({ path: path.resolve(__dirname, '../../.env') });

export default defineConfig({
  globalSetup: require.resolve('./global-setup'),
  testDir: '.',
  timeout: 60000,
  retries: 1,
  workers: 1,
  use: {
    baseURL: 'http://localhost:8080',
    screenshot: 'only-on-failure',
    trace: 'on-first-retry',
    navigationTimeout: 30000,
    actionTimeout: 30000,
    // Persists the session cookie obtained in globalSetup across all tests.
    storageState: 'storageState.json',
    // Sends API key on every request fixture call (no-op when API_KEY is unset).
    extraHTTPHeaders: process.env.API_KEY
      ? { 'X-API-Key': process.env.API_KEY }
      : {},
  },
  projects: [
    {
      name: 'chromium',
      use: { browserName: 'chromium' },
    },
  ],
  reporter: [
    ['html', { outputFolder: '../../reports/test-results/playwright' }],
    ['list'],
  ],
});
