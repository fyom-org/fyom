import { test as base, type Page } from '@playwright/test';

interface E2EFixtures {
  baseURL: string;
  freshPage: Page;
}

export const test = base.extend<E2EFixtures>({
  baseURL: 'http://127.0.0.1:8080',
  
  freshPage: async ({ page, baseURL }, use) => {
    // Capture console messages for debugging
    page.on('console', msg => {
      if (msg.type() === 'error' || msg.text().includes('[router]')) {
        console.log(`[BROWSER CONSOLE ${msg.type()}] ${msg.text()}`);
      }
    });
    
    page.on('pageerror', err => {
      console.error('[BROWSER PAGE ERROR]', err.message);
    });

    // Navigate to base URL first to establish a valid origin
    await page.goto(baseURL + '/', { waitUntil: 'networkidle' });
    // Clear storage AFTER establishing a valid origin
    await page.evaluate(() => {
      localStorage.clear();
      sessionStorage.clear();
    });
    // Reload to trigger the router guard with clean storage
    await page.reload({ waitUntil: 'networkidle' });
    await use(page);
  },
});

export { expect } from '@playwright/test';