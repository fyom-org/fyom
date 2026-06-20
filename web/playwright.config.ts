import { defineConfig, devices } from '@playwright/test';
import { fileURLToPath } from 'url';
import { dirname, join } from 'path';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const projectRoot = join(__dirname, '..');

export default defineConfig({
  testDir: './tests',
  testMatch: '**/*.e2e.test.ts',
  fullyParallel: false,
  workers: 1,
  retries: 0,
  reporter: [['list']],
  webServer: {
    command: `cd "${projectRoot}" && pnpm --filter web run build && CGO_ENABLED=0 go run ./cmd/fyom/main.go --db-path ${join(projectRoot, 'build', 'test-e2e.db')}`,
    url: 'http://127.0.0.1:27402',
    reuseExistingServer: true,
    timeout: 180_000,
    cwd: projectRoot,
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
