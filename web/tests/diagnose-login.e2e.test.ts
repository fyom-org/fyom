import { test, expect } from '@playwright/test';

const BASE = 'http://127.0.0.1:8080';

test('diagnose login flow', async ({ page }) => {
  // Step 1: Initialize if needed
  const statusRes = await page.request.get(BASE + '/api/v1/system/status');
  const statusData = await statusRes.json();
  if (!statusData.data.initialized) {
    await page.request.post(BASE + '/api/v1/system/initialize', {
      data: { username: 'admin', password: 'admin123', allow_registration: false },
    });
  }

  // Step 2: Navigate to /login
  await page.goto(BASE + '/login');
  await page.waitForURL('**/login', { timeout: 10000 });

  // Step 3: Intercept the request module's getMe call
  const result = await page.evaluate(async () => {
    const app = document.querySelector('#app')?.__vue_app__;
    if (!app) return { error: 'no vue app' };
    const pinia = app.config.globalProperties.$pinia;
    if (!pinia) return { error: 'no pinia' };
    const userStore = pinia._s.get('user');
    if (!userStore) return { error: 'no user store' };

    // Manually set token in localStorage
    const testToken = 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.test';
    localStorage.setItem('token', testToken);

    // Check if the token is there
    const lsToken = localStorage.getItem('token');

    // Now call getMe via the request module
    try {
      const { default: request } = await import('@/api/request');
      const meRes = await request.get('/auth/me');
      return {
        lsToken: lsToken?.substring(0, 20) + '...',
        meStatus: 'success',
        meData: meRes.data
      };
    } catch (e: any) {
      return {
        lsToken: lsToken?.substring(0, 20) + '...',
        meStatus: 'error',
        error: e.message,
        errorResponse: e.response?.status
      };
    }
  });

  console.log('getMe via request module:', JSON.stringify(result));

  expect(true).toBe(true);
});
