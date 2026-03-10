import { chromium } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';

const STORAGE_STATE = path.join(__dirname, 'storageState.json');

export default async function globalSetup() {
  const username = process.env.UI_USERNAME;
  const password = process.env.UI_PASSWORD;

  if (!username || !password) {
    // Auth is disabled — write an empty state so the storageState ref in config doesn't fail.
    fs.writeFileSync(STORAGE_STATE, JSON.stringify({ cookies: [], origins: [] }));
    return;
  }

  const browser = await chromium.launch();
  const context = await browser.newContext();
  const page = await context.newPage();

  const baseURL = process.env.BASE_URL ?? 'http://localhost:8080';
  await page.goto(`${baseURL}/ui/login`);
  await page.fill('input[name="username"]', username);
  await page.fill('input[name="password"]', password);
  await page.click('button[type="submit"]');
  // Wait until we've been redirected away from /ui/login
  await page.waitForURL((url) => !url.pathname.startsWith('/ui/login'));

  await context.storageState({ path: STORAGE_STATE });
  await browser.close();
}
