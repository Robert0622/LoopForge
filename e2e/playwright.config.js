// @ts-check
const { defineConfig } = require('@playwright/test');

const port = process.env.E2E_PORT || '18499';
const setupPort = process.env.E2E_SETUP_PORT || '18498';
// E2E_BASE_URL points the suite at an already running instance (for example
// the Docker image started with LOOPFORGE_DEMO_MODE=true and a fresh volume).
const external = process.env.E2E_BASE_URL;

module.exports = defineConfig({
  testDir: './tests',
  // Tests share one server and database; run them in file order.
  workers: 1,
  fullyParallel: false,
  timeout: 60_000,
  expect: { timeout: 15_000 },
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: external || `http://127.0.0.1:${port}`,
    viewport: { width: 1440, height: 900 },
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
    launchOptions: process.env.E2E_CHROMIUM ? { executablePath: process.env.E2E_CHROMIUM, args: ['--no-sandbox'] } : {},
  },
  // The second server has no account yet, for the first-run setup test.
  webServer: external ? undefined : [
    {
      command: 'bash ./serve.sh',
      url: `http://127.0.0.1:${port}/api/health`,
      timeout: 60_000,
      reuseExistingServer: false,
      env: { E2E_PORT: port },
    },
    {
      command: 'bash ./serve.sh',
      url: `http://127.0.0.1:${setupPort}/api/health`,
      timeout: 60_000,
      reuseExistingServer: false,
      env: { E2E_PORT: setupPort, E2E_NO_ADMIN: '1' },
    },
  ],
});
