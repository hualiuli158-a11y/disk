import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests/integration',
  testMatch: '**/*.integration.ts',
  fullyParallel: false,
  workers: 1,
  timeout: 180_000,
  expect: { timeout: 15_000 },
  outputDir: '../build/browser-e2e-results',
  reporter: [
    ['list', { printSteps: true }],
    ['html', { outputFolder: '../build/browser-e2e-report', open: 'never' }],
  ],
  use: {
    baseURL: 'http://127.0.0.1:5174',
    channel: 'chrome',
    headless: true,
    actionTimeout: 10_000,
    navigationTimeout: 20_000,
    viewport: { width: 1440, height: 1000 },
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  webServer: {
    command: 'npm run dev:api -- --host 127.0.0.1 --port 5174 --strictPort',
    url: 'http://127.0.0.1:5174',
    reuseExistingServer: false,
    env: {
      DISK_API_TARGET: process.env.DISK_API_TARGET || 'http://127.0.0.1:8080',
      VITE_API_BASE_URL: '',
    },
  },
});
