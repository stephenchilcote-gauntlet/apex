import { test, expect } from './fixtures';

test.describe('Navigation', () => {
  const tabs = [
    { label: 'Simulate', url: '/ui/simulate', heading: /simulate/i },
    { label: 'Transfers', url: '/ui/transfers', heading: /transfer/i },
    { label: 'Review Queue', url: '/ui/review', heading: /review/i },
    { label: 'Ledger', url: '/ui/ledger', heading: /ledger/i },
    { label: 'Settlement', url: '/ui/settlement', heading: /settlement/i },
    { label: 'Returns', url: '/ui/returns', heading: /return/i },
    { label: 'Audit Log', url: '/ui/audit', heading: /audit/i },
  ];

  for (const tab of tabs) {
    test(`clicking "${tab.label}" tab navigates to correct page`, async ({ page }) => {
      // Start from a different page
      await page.goto('/ui/simulate');

      await page.locator('a.nav-level-tab', { hasText: tab.label }).click();

      await expect(page).toHaveURL(new RegExp(tab.url));
      await expect(page.locator('h1, h2')).toContainText(tab.heading);

      // Active tab should have aria-selected
      await expect(page.locator('a.nav-level-tab', { hasText: tab.label })).toHaveAttribute('aria-selected', 'true');
    });
  }
});
