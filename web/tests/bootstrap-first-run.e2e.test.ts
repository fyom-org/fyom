import { test, expect } from './e2e/fixtures';

test.describe('fyom bootstrap and auth lifecycle (routing behavior)', () => {
  
  test('Scenario 1: Anonymous cold start → redirects to /login', async ({ freshPage, baseURL }) => {
    // The fixture already navigated to / and cleared storage
    // Wait for client-side redirect to /login
    await freshPage.waitForURL('**/login', { timeout: 15000 });
    await expect(freshPage.locator('text=Welcome back')).toBeVisible({ timeout: 5000 });
    expect(freshPage.url()).toContain('/login');
    expect(freshPage.url()).not.toContain('/setup');
  });

  test('Scenario 2: /setup redirects to /login', async ({ freshPage, baseURL }) => {
    // Navigate to /setup (the fixture already navigated to /)
    const response = await freshPage.goto(baseURL + '/setup', { waitUntil: 'networkidle' });
    expect(response).not.toBeNull();
    if (response) expect(response.status()).toBe(200);
    
    // Wait for client-side redirect to /login
    await freshPage.waitForURL('**/login', { timeout: 15000 });
    expect(freshPage.url()).not.toContain('/setup');
    expect(freshPage.url()).toContain('/login');
  });

  test('Scenario 3: Fresh anonymous session cannot access protected pages', async ({ freshPage, baseURL }) => {
    // The fixture already navigated to / and cleared storage
    // Wait for redirect to login
    await freshPage.waitForURL('**/login', { timeout: 15000 });
    
    // Try accessing protected pages - should be redirected to login
    const profileResponse = await freshPage.goto(baseURL + '/profile', { waitUntil: 'networkidle' });
    expect(profileResponse).not.toBeNull();
    if (profileResponse) expect(profileResponse.status()).toBe(200);
    await freshPage.waitForURL('**/login', { timeout: 15000 });
    
    const libraryResponse = await freshPage.goto(baseURL + '/library', { waitUntil: 'networkidle' });
    if (libraryResponse) expect(libraryResponse.status()).toBe(200);
    await freshPage.waitForURL('**/login', { timeout: 15000 });
  });

});