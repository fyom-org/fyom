/**
 * Playwright E2E tests for fyom auth lifecycle.
 *
 * Covers the exact user scenarios that were broken:
 * 1. Setup → login → lands on / (not stuck on /login)
 * 2. Login from /login page → redirected to /
 * 3. Logout from /profile → redirected to /login
 * 4. Anonymous user → / → redirected to /login
 * 5. /setup inaccessible after initialization
 */

import { test, expect } from '@playwright/test';

const BASE = 'http://127.0.0.1:8080';

test.describe('fyom auth lifecycle', () => {

  test('V1: Anonymous user cold start → /login (not /setup)', async ({ page }) => {
    await page.goto(BASE + '/');
    await page.waitForURL('**/login', { timeout: 10000 });
    // Should be on /login, not /setup
    expect(page.url()).toContain('/login');
    // Login form should be visible
    await expect(page.locator('text=Welcome back')).toBeVisible();
  });

  test('V2: Setup success → login → lands on / (not stuck on /login)', async ({ page }) => {
    // Step 1: System status should be uninitialized
    const statusRes = await page.request.get(BASE + '/api/v1/system/status');
    const statusData = await statusRes.json();
    expect(statusData.data.initialized).toBe(false);

    // Step 2: Initialize system
    const initRes = await page.request.post(BASE + '/api/v1/system/initialize', {
      data: { username: 'admin', password: 'admin123', allow_registration: false },
    });
    expect(initRes.status()).toBe(200);

    // Step 3: Login
    const loginRes = await page.request.post(BASE + '/api/v1/auth/login', {
      data: { username: 'admin', password: 'admin123' },
    });
    expect(loginRes.status()).toBe(200);
    const loginData = await loginRes.json();
    expect(loginData.data.access_token).toBeDefined();

    // Step 4: Navigate to / — should work (not stuck on /login)
    await page.goto(BASE + '/');
    await page.waitForURL('**/', { timeout: 10000 });
    // Should see dashboard content, not login form
    const url = page.url();
    expect(url).not.toContain('/login');
    expect(url).not.toContain('/setup');
  });

  test('V3: Login from /login page → lands on /', async ({ page }) => {
    // Navigate to /login
    await page.goto(BASE + '/login');
    await page.waitForURL('**/login', { timeout: 10000 });

    // Fill login form
    await page.fill('input[autocomplete="username"]', 'admin');
    await page.fill('input[autocomplete="current-password"]', 'admin123');
    await page.click('button:has-text("Sign In")');

    // Should redirect to / (not stay on /login)
    await page.waitForURL('**/', { timeout: 15000 });
    const url = page.url();
    expect(url).not.toContain('/login');
  });

  test('V4: Logout from /profile → redirected to /login', async ({ page }) => {
    // Login first
    await page.goto(BASE + '/login');
    await page.waitForURL('**/login', { timeout: 10000 });
    await page.fill('input[autocomplete="username"]', 'admin');
    await page.fill('input[autocomplete="current-password"]', 'admin123');
    await page.click('button:has-text("Sign In")');
    await page.waitForURL('**/', { timeout: 15000 });

    // Navigate to /profile
    await page.goto(BASE + '/profile');
    await page.waitForURL('**/profile', { timeout: 10000 });

    // Click logout
    await page.click('button:has-text("Logout")');

    // Should redirect to /login
    await page.waitForURL('**/login', { timeout: 10000 });
    expect(page.url()).toContain('/login');
  });

  test('V5: /setup inaccessible after initialization', async ({ page }) => {
    // System is already initialized from previous tests
    await page.goto(BASE + '/setup');
    // Should be redirected away from /setup
    await page.waitForURL('**/', { timeout: 10000 });
    const url = page.url();
    expect(url).not.toContain('/setup');
  });

  test('V6: Protected pages not accessible to anonymous users', async ({ page }) => {
    // Clear cookies/localStorage to simulate fresh session
    await page.goto(BASE + '/');
    await page.evaluate(() => { localStorage.clear(); });
    await page.goto(BASE + '/');
    await page.waitForURL('**/login', { timeout: 10000 });

    // Try accessing protected pages
    await page.goto(BASE + '/profile');
    await page.waitForURL('**/login', { timeout: 10000 });

    await page.goto(BASE + '/library');
    await page.waitForURL('**/login', { timeout: 10000 });
  });
});
