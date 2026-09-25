import { defineConfig, devices } from '@playwright/test';

const domain = process.env.GORETROTV_DOMAIN;
if (!domain) throw new Error('GORETROTV_DOMAIN must name the public host');

export default defineConfig({
  testDir: './tests/demo',
  outputDir: './.artifacts/playwright-demo-results',
  timeout: 150_000,
  workers: 1,
  use: {
    baseURL: `https://${domain}`,
    ...devices['Desktop Chrome'],
    viewport: { width: 1280, height: 720 },
    video: {
      mode: 'on',
      size: { width: 1280, height: 720 },
    },
  },
});
