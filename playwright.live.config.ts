import { defineConfig, devices } from '@playwright/test';
import { resolve } from 'node:path';

const baseURL = 'http://127.0.0.1:8099';

export default defineConfig({
  testDir: './tests/live',
  outputDir: './.artifacts/playwright-live-results',
  timeout: 75_000,
  use: { baseURL, ...devices['Desktop Chrome'] },
  webServer: {
    command: './ctl.sh run',
    url: `${baseURL}/health`,
    timeout: 30_000,
    reuseExistingServer: false,
    env: {
      GORETROTV_ENV: 'development',
      GORETROTV_FIRMWARE_DIR: resolve('firmware'),
      GORETROTV_SNAPSHOT_PATH: resolve('snapshots/post-acquisition.snapshot'),
      GORETROTV_WEB_DIR: resolve('web'),
      GORETROTV_HTTP_ADDR: '127.0.0.1:8099',
    },
  },
});
