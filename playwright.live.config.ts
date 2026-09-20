import { defineConfig, devices } from '@playwright/test';
import { cpSync, mkdirSync, rmSync } from 'node:fs';
import { resolve } from 'node:path';

const baseURL = 'http://127.0.0.1:8099';

// The live box broadcasts from a COPY of the schedule, not from the one in the
// working tree. TC-6.9 proves the guide follows an edit, which means editing a
// file while the test runs, and a test that rewrites committed content leaves
// the repository dirty when it fails half way through.
const liveListings = resolve('.artifacts/live-listings');
rmSync(liveListings, { recursive: true, force: true });
mkdirSync(liveListings, { recursive: true });
cpSync(resolve('listings'), liveListings, { recursive: true });

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
      GORETROTV_LISTINGS_PATH: liveListings,
      GORETROTV_DICTIONARY_PATH: resolve('dictionaries/skyuk.dict'),
      GORETROTV_BROADCAST_DATE: '1998-12-24',
    },
  },
});
