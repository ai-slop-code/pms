import { defineConfig, devices } from '@playwright/test'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const here = path.dirname(fileURLToPath(import.meta.url))
const repoRoot = path.resolve(here, '..')
const runtimeDir = path.join(here, '.runtime')

// Deterministic credentials for the bootstrap super-admin used by every test.
// These never escape this file (the database is wiped before every run).
export const E2E_ADMIN_EMAIL = 'e2e-admin@example.com'
export const E2E_ADMIN_PASSWORD = 'e2e-test-password-1234'

const backendPort = Number(process.env.E2E_BACKEND_PORT ?? '18080')
const frontendPort = Number(process.env.E2E_FRONTEND_PORT ?? '15173')

export default defineConfig({
  testDir: './tests',
  timeout: 60_000,
  expect: { timeout: 10_000 },
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : [['list']],
  fullyParallel: false, // single backend, sequential is safer
  workers: 1,
  use: {
    baseURL: `http://127.0.0.1:${frontendPort}`,
    trace: 'retain-on-failure',
    video: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: [
    {
      // Backend: start a clean SQLite DB under e2e/.runtime so each run is
      // hermetic. The bootstrap super-admin is provisioned via env vars on
      // first start. PMS_ENV=test relaxes production guards and allows the
      // 2FA dev bypass if a future test wants to enable it.
      command: `bash ${path.join(here, 'scripts', 'start-backend.sh')}`,
      cwd: repoRoot,
      url: `http://127.0.0.1:${backendPort}/healthz`,
      timeout: 120_000,
      reuseExistingServer: !process.env.CI,
      env: {
        E2E_BACKEND_PORT: String(backendPort),
        E2E_FRONTEND_PORT: String(frontendPort),
        E2E_RUNTIME_DIR: runtimeDir,
        E2E_ADMIN_EMAIL,
        E2E_ADMIN_PASSWORD,
      },
      stdout: 'pipe',
      stderr: 'pipe',
    },
    {
      // Frontend: vite dev server with the `/api` proxy pointed at the
      // hermetic test backend. We deliberately use dev (not preview) so the
      // Playwright run uses a same-origin SPA → API path, mirroring the
      // production deployment that puts both behind a single reverse proxy.
      // VITE_API_BASE_URL is intentionally NOT set (empty string default
      // routes through the dev proxy).
      command: `npm run dev -- --host 127.0.0.1 --port ${frontendPort} --strictPort`,
      cwd: path.join(repoRoot, 'frontend'),
      url: `http://127.0.0.1:${frontendPort}`,
      timeout: 180_000,
      reuseExistingServer: !process.env.CI,
      env: {
        VITE_DEV_API_PROXY: `http://127.0.0.1:${backendPort}`,
      },
      stdout: 'pipe',
      stderr: 'pipe',
    },
  ],
})
