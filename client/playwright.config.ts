import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  retries: 0,
  use: {
    baseURL: 'http://localhost:5174',
    // API calls are intercepted via route mocking — no real backend needed.
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
  ],
  webServer: {
    command: 'npm run dev -- --port 5174',
    url: 'http://localhost:5174/chores/',
    reuseExistingServer: false,
    stdout: 'ignore',
    stderr: 'pipe',
  },
})
