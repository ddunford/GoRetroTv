import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests/e2e',
  outputDir: './.artifacts/playwright-results',
  projects: [
    { name: 'chromium' },
    { name: 'reduced-motion', grep: /@motion/, use: { contextOptions: { reducedMotion: 'reduce' } } },
  ],
  use: {
    baseURL: 'http://127.0.0.1:8766',
    ...devices['Desktop Chrome'],
  },
  webServer: {
    command: 'npm run build:web && python3 -m http.server 8766 --bind 127.0.0.1 --directory web',
    url: 'http://127.0.0.1:8766/',
    reuseExistingServer: !process.env.CI,
    timeout: 30_000,
  },
});
