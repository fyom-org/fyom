# Playwright E2E Testing Guide

## Overview
This document describes how to run Playwright E2E tests for the fyom frontend.

Test files are under `frontend/tests/`

## Prerequisites

### Start Backend Server (Required)
The E2E tests require a running fyom backend server on port 27402

```bash
# 1. Build the Go backend
cd fyom && CGO_ENABLED=0 go build -o /tmp/fyom-server ./cmd/fyom/

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
cd fyom && nix develop --command pnpm exec playwright test --config frontend/playwright.config.ts
```

### Direct Command
```bash
cd fyom/frontend
PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 \
PLAYWRIGHT_SKIP_VALIDATE_HOST_REQUIREMENTS=true \
NO_PROXY="localhost,127.0.0.1,::1" \
no_proxy="localhost,127.0.0.1,::1" \
npx playwright test --config playwright.config.ts
```

### Common Test Commands
```bash
# Enter nix flake development environment
nix develop

# Go to web directory
cd web

# Run all E2E tests
pnpm exec playwright test --config playwright.config.ts

# Run with headed browser (for debugging)
pnpm exec playwright test --headed

# Show HTML report after tests
pnpm exec playwright show-report
```

## Configuration Files

### Playwright Config (`frontend/playwright.config.ts`)
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
    url: 'http://127.0.0.1:27402',
    reuseExistingServer: true,
    timeout: 120_000,
  },
  use: {
    baseURL: 'http://127.0.0.1:27402',
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

## Common Issues & Troubleshooting

### 1. Browser Not Found

Need to setup nix flake development environment 

```bash
nix develop
```

### 2. Test Isolation Issues
Tests share browser context by default.
For proper isolation, each test should:
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
The `webServer` config in `playwright.config.ts` uses `reuseExistingServer: true` and expects server at `http://127.0.0.1:27402`.
Ensure the backend is running before tests.

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
