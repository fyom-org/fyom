import { test, expect } from './e2e/fixtures';

test.describe('fyom login and logout flows', () => {

  test('Scenario 4: Login from /login page → lands on /', async ({ freshPage, baseURL }) => {
    // Initialize system
    await freshPage.request.post(baseURL + '/api/v1/system/initialize', {
      data: { username: 'admin', password: 'bootstrap123', allow_registration: false }
    });

    // Navigate to /login
    await freshPage.goto(baseURL + '/login');
    await freshPage.waitForURL('**/login', { timeout: 10000 });

    // Fill login form
    await freshPage.fill('input[autocomplete="username"]', 'admin');
    await freshPage.fill('input[autocomplete="current-password"]', 'bootstrap123');
    await freshPage.click('button:has-text("Sign In")');

    // Should redirect to / (not stay on /login)
    await freshPage.waitForURL('**/', { timeout: 15000 });
    const url = freshPage.url();
    expect(url).not.toContain('/login');
  });

  test('Scenario 5: Logout from /profile → redirected to /login', async ({ freshPage, baseURL }) => {
    // Initialize system
    await freshPage.request.post(baseURL + '/api/v1/system/initialize', {
      data: { username: 'admin', password: 'bootstrap123', allow_registration: false }
    });

    // Login first
    await freshPage.goto(baseURL + '/login');
    await freshPage.waitForURL('**/login', { timeout: 10000 });
    await freshPage.fill('input[autocomplete="username"]', 'admin');
    await freshPage.fill('input[autocomplete="current-password"]', 'bootstrap123');
    await freshPage.click('button:has-text("Sign In")');
    await freshPage.waitForURL('**/', { timeout: 15000 });

    // Navigate to /profile
    await freshPage.goto(baseURL + '/profile');
    await freshPage.waitForURL('**/profile', { timeout: 10000 });

    // Click logout
    await freshPage.click('button:has-text("Logout")');

    // Should redirect to /login
    await freshPage.waitForURL('**/login', { timeout: 10000 });
    expect(freshPage.url()).toContain('/login');
  });

});
