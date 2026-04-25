import { test, expect } from '@playwright/test'
import { E2E_ADMIN_EMAIL, E2E_ADMIN_PASSWORD } from '../playwright.config'

// Smoke flow per PMS_11 T3.3.
//
// Coverage delivered today: login → create property → logout. This exercises
// the full system end-to-end (frontend build, CSRF header, cookie session,
// migrations, encrypted store, RBAC, navigation guards, logout).
//
// The remaining steps from the spec scenario — add occupancy → generate
// invoice → download PDF — depend on UI flows that are still in flux and
// would require non-trivial fixture data (ICS feeds or manual occupancy
// entry, plus invoice numbering/payout selection). Tracked as a follow-up
// inside this same file so the harness is reused once those flows stabilise.

test.describe('PMS smoke', () => {
  test('admin can sign in, create a property, and sign out', async ({ page }) => {
    // 1. Sign in.
    await page.goto('/login')
    await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible()
    await page.getByLabel('Email').fill(E2E_ADMIN_EMAIL)
    await page.getByLabel('Password').fill(E2E_ADMIN_PASSWORD)
    await page.getByRole('button', { name: 'Sign in' }).click()

    // Bootstrap super_admin always lands on /provisioning to rotate the
    // temp password. PMS_2FA_DEV_BYPASS=true (set by start-backend.sh)
    // waives the TOTP enrolment step, so the password stage is the only
    // one we need to clear before the SPA lets us into the app proper.
    await page.waitForURL(/\/provisioning(\?|$)/)
    const rotatedPassword = `${E2E_ADMIN_PASSWORD}-rotated!`
    await page.getByLabel('New password').fill(rotatedPassword)
    await page.getByLabel('Confirm new password').fill(rotatedPassword)
    await page.getByRole('button', { name: 'Save password' }).click()

    // Land on the dashboard. URL should no longer be /provisioning or /login.
    await expect(page).not.toHaveURL(/\/login(\?|$)/)
    await expect(page).not.toHaveURL(/\/provisioning(\?|$)/)

    // 2. Navigate to Properties and start a new one.
    await page.goto('/properties')
    await page.getByRole('button', { name: 'New property' }).click()
    await expect(page).toHaveURL(/\/properties\/new$/)

    const propertyName = `E2E Property ${Date.now()}`
    await page.getByLabel('Name').fill(propertyName)
    // Timezone and language have sensible defaults from PropertyFormView,
    // but we touch them to ensure the form actually receives a value.
    await page.getByLabel('Timezone').fill('Europe/Bratislava')
    await page.getByLabel('Default language').fill('en')

    await page.getByRole('button', { name: 'Create property' }).click()

    // After creation we either land on the detail page or the list — both
    // count as success as long as the new property shows up in the table.
    await page.waitForURL(/\/properties(\/\d+)?$/)
    await expect(page.getByRole('cell', { name: propertyName })).toBeVisible()

    // 3. Logout from the topbar.
    await page.getByRole('button', { name: 'Logout' }).click()
    await page.waitForURL(/\/login(\?|$)/)
    await expect(page.getByRole('heading', { name: 'Sign in' })).toBeVisible()
  })
})
