# Playwright E2E Testing Guide

## Overview
This document describes how to run Playwright E2E tests for the fyom frontend.

## Prerequisites

### Start Backend Server (Required)
The E2E tests require a running fyom backend server on port 8080.

```bash
# 1. Build the Go backend
cd /root/fyom && CGO_ENABLED=0 go build -o /tmp/fyom-server ./cmd/fyom/

# 2. Start server with a temporary database
FYOM_AUTH_JWT_SECRET=***RE_SECRET=*** /tmp/fyom-server -db-path /tmp/fyom-e2e.db &
```

### Environment Variables (Required)
```bash
export PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1
export PLAYWRIGHT_SKIP_VALIDATE_HOST_REQUIREMENTS=true
export NO_PROXY="localhost,127.0.0.1,::1"
export no_proxy="localhost,127.0.0.1,::1"
```

## Running Playwright Tests

### Using Taskfile (Recommended)
```bash
cd /root/fyom && nix develop --command pnpm exec playwright test --config web/playwright.config.ts
```

### Direct Command
```bash
cd /root/fyom/web
PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 \
PLAYWRIGHT_SKIP_VALIDATE_HOST_REQUIREMENTS=true \
NO_PROXY="localhost,127.0.0.1,::1" \
no_proxy="localhost,127.0.0.1,::1" \
npx playwright test --config playwright.config.ts
```

### Common Test Commands
```bash
# Run all E2E tests
pnpm exec playwright test --config playwright.config.ts

# Run specific test by name pattern
pnpm exec playwright test -g "V3"

# Run with headed browser (for debugging)
pnpm exec playwright test --headed

# Show HTML report after tests
pnpm exec playwright show-report
```

## Configuration Files

### Playwright Config (`web/playwright.config.ts`)
```typescript
import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  testMatch: '**/*-lifecycle.e2e.test.ts',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [['list']],
  webServer: {
    command: 'sleep 1',
    url: 'http://127.0.0.1:8080',
    reuseExistingServer: true,
    timeout: 120_000,
  },
  use: {
    baseURL: 'http://127.0.0.1:8080',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
});
```

### Test Files
- `web/tests/auth-lifecycle.e2e.test.ts` - Main E2E test suite covering:
  - V1: Anonymous user cold start → `/login`
  - V2: Setup → login → lands on `/`
  - V3: Login from `/login` page → lands on `/`
  - V4: Logout from `/profile` → redirected to `/login`
  - V5: `/setup` inaccessible after initialization
  - V6: Protected pages not accessible to anonymous users

## Common Issues & Troubleshooting

### 1. Browser Not Found
```bash
nix develop /root/fyom --command pnpm exec playwright install chromium
```

### 2. Test Isolation Issues
Tests share browser context by default. For proper isolation, each test should:
- Use fresh browser context
- Clear localStorage/cookies between tests
- Reset database state if needed

### 3. Network Proxy Issues
Ensure proxy bypass for local addresses:
```bash
export NO_PROXY="localhost,127.0.0.1,::1"
export no_proxy="localhost,127.0.0.1,::1"
```

### 4. Server Not Ready
The `webServer` config in `playwright.config.ts` uses `reuseExistingServer: true` and expects server at `http://127.0.0.1:8080`. Ensure the backend is running before tests.

## Test Scenarios Covered

| Test | Scenario | Expected |
|------|----------|----------|
| V1 | Fresh browser → `/` | Redirect to `/login` (not `/setup`) |
| V2 | Setup → login | Redirect to `/` |
| V3 | Login from `/login` | Redirect to `/` |
| V4 | Logout from `/profile` | Redirect to `/login` |
| V5 | Access `/setup` after init | Redirect away from `/setup` |
| V6 | Anonymous access to protected pages | Redirect to `/login` |

## Known Issues & Fixes

| Issue | Status | Fix |
|-------|--------|-----|
| Login stuck on `/login` | Fixed | Use `fetch` with explicit token in `doLogin()` |
| `authStatus=unknown` leak | Fixed | Guard sets `anonymous` when no token |
| Double navigation on logout | Fixed | Removed `router.replace` from `ProfileView` |
| Route revalidation on redirect | Fixed | `setupRouteRevalidation()` called unconditionally |
| `wait` decision silent passthrough | Fixed | Return `false` to cancel navigation |

## Debugging Tips

1. **Check browser console**: `page.on('console', msg => console.log(msg.text()))`
2. **Capture network errors**: `page.on('response', r => r.status() >= 400 && console.log(r.url(), r.status()))`
3. **View screenshots**: `test-results/<test-name>/test-failed-1.png`
4. **Watch videos**: `test-results/<test-name>/video.webm`

## CI/CD Integration

For CI/CD pipelines, ensure:
1. Backend server starts before tests
2. Playwright browsers installed in CI image
3. Environment variables set correctly
4. Test timeout configured appropriately (default 30s, increase for slow CI)