import { test, expect } from './e2e/fixtures';

test.describe('fyom bootstrap and auth lifecycle', () => {
  
  test('Scenario 1: Anonymous cold start → lands on /login (not /setup)', async ({ freshPage, baseURL }) => {
    await freshPage.goto(baseURL + '/');
    await freshPage.waitForURL('**/login', { timeout: 10000 });
    expect(freshPage.url()).toContain('/login');
    // Login form should be visible
    await expect(freshPage.locator('text=Welcome back')).toBeVisible();
    // Should NOT be on setup
    expect(freshPage.url()).not.toContain('/setup');
  });

  test('Scenario 2: /setup is no longer usable after initialization', async ({ freshPage, baseURL }) => {
    // Initialize system first
    await freshPage.request.post(baseURL + '/api/v1/system/initialize', {
      data: { username: 'admin', password: 'bootstrap123', allow_registration: false }
    });
    
    // Now try to access /setup directly
    await freshPage.goto(baseURL + '/setup');
    // Should be redirected away from /setup (to /login)
    await freshPage.waitForURL('**/login', { timeout: 10000 });
    expect(freshPage.url()).not.toContain('/setup');
  });

  test('Scenario 3: Fresh anonymous session cannot access protected pages', async ({ freshPage, baseURL }) => {
    // Initialize system
    await freshPage.request.post(baseURL + '/api/v1/system/initialize', {
      data: { username: 'admin', password: 'bootstrap123', allow_registration: false }
    });
    
    // Fresh anonymous session
    await freshPage.goto(baseURL + '/');
    await freshPage.waitForURL('**/login', { timeout: 10000 });
    
    // Try accessing protected pages - should be redirected to login
    await freshPage.goto(baseURL + '/profile');
    await freshPage.waitForURL('**/login', { timeout: 10000 });
    
    await freshPage.goto(baseURL + '/library');
    await freshPage.waitForURL('**/login', { timeout: 10000 });
  });
});