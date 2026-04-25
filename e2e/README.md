# PMS end-to-end tests (Playwright)

Hermetic browser tests that drive the full stack: backend + frontend
production build + Chromium.

## Run locally

```bash
cd e2e
npm install
npx playwright install --with-deps chromium
npm test
```

The Playwright `webServer` config builds the frontend, starts a clean
backend on port `18080` against a fresh SQLite database under
`e2e/.runtime/`, and serves the frontend on port `15173`. A bootstrap
super-admin (`e2e-admin@example.com`) is provisioned on first start.

## What is covered

- Login → Create property → Logout (`tests/smoke.spec.ts`).

The PMS_11 T3.3 scenario also names occupancy/invoice/PDF download. Those
flows depend on fixture-heavy UI paths (ICS sync or manual entry, invoice
numbering, payout selection) and are intentionally deferred until the
shapes stabilise — see the `// follow-up` note in `tests/smoke.spec.ts`.

## CI

The `e2e` job in `.github/workflows/ci.yml` runs the same harness against
ubuntu-latest with Go and Node already cached.
