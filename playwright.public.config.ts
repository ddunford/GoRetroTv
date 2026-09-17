import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests/public',
  outputDir: './.artifacts/playwright-public-results',
  timeout: 75_000,
  workers: 1,
  use: {
    baseURL: 'https://goretrotv.demosrv.uk',
    ...devices['Desktop Chrome'],
  },
});
