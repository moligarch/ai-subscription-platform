import { test, expect } from '@playwright/test';

test('login and basic flows', async ({ page, baseURL }) => {
  // Set admin key via UI
  await page.goto('/');
  await page.fill('input[placeholder="Paste admin API key"]', process.env.E2E_ADMIN_KEY || 'test-admin-key');
  await page.click('text=Login');

  // Dashboard
  await page.waitForSelector('text=Dashboard');
  await expect(page.locator('text=Total Users')).toBeVisible();

  // Plans -> create
  await page.goto('/#/plans');
  await page.click('text=New Plan');
  await page.fill('input[placeholder="Name"]', 'E2E Plan ' + Date.now());
  await page.fill('input[placeholder="Price (IRR)"]', '1000');
  await page.click('text=Save');

  // wait for plan to appear
  await page.waitForTimeout(1000); // small wait - requires backend to be responsive
  await expect(page.locator('table')).toContainText('E2E Plan');
});
