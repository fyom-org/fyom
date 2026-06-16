import { test as base, type Page } from '@playwright/test';

interface E2EFixtures {
  baseURL: string;
  freshPage: Page;
}

export const test = base.extend<E2EFixtures>({
  baseURL: 'http://127.0.0.1:8080',
  
  freshPage: async ({ page, baseURL }, use) => {
    // Clear all storage to simulate fresh anonymous session
    await page.context().clearCookies();
    await page.evaluate(() => {
      localStorage.clear();
      sessionStorage.clear();
    });
    // Ensure we start fresh by going to base and clearing again
    await page.goto(baseURL + '/', { waitUntil: 'networkidle' });
    await page.evaluate(() => {
      localStorage.clear();
      sessionStorage.clear();
    });
    await use(page);
  },
});

export { expect } from '@playwright/test';
