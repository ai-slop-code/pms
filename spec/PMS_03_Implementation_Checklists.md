# PMS Implementation Checklists

## How to Use
This file is a verification checklist for the AI developer agent. Each item should be demonstrably implemented, not only scaffolded.

## Implementation status (do not lose track)

**Last updated:** 2026-04-23 (UI/UX polish pass — spec/PMS_08_UI_UX_Polish_Spec.md — closed out: added `UiDateInput` primitive; AnalyticsView stripped of every inline hex, with class-based bar charts consuming `--color-primary` / `--success-fg` and token-scaled spacing; Finance already tabbed (Overview / Transactions / Recurring / Categories / Monthly breakdown); `UiTable` gained a `stack` prop that collapses rows into stacked cards under 640 px using `data-label` attributes (§5 of PMS_08); Vitest coverage expanded to all 19 primitives — new specs for `UiDialog` (focus trap, Escape + backdrop close, focus restore, persistent lockout), `UiCard`, `UiSelect`, `UiEmptyState`, `UiIconButton`, `UiPageHeader`, `UiSection`, `UiSkeleton`, `UiTable`, `UiTag`, `UiToast` / `ToastStack`, `UiToolbar`, and `UiDateInput`; copy pass swept display-side `.slice(0, 10)` ISO dates in AnalyticsView and BookingPayoutsView through `formatShortDate` + `isoTitle` so the exact timestamp lives in `title=""`; `--color-on-primary` token added so LoginView and other brand-on-primary surfaces no longer reach for raw `#fff`; last hex fallbacks inside `var(--success-fg, #047857)` in Messages/Occupancy removed — a repo-wide grep for `#[0-9a-fA-F]{3,6}` outside `tokens.css` is now clean. ShellView already wires a polite `aria-live` route-change announcer. Per user directive the `@axe-core/cli` npm script and Playwright visual-snapshot suite remain the only explicit exclusions from PMS_08 and are tracked as future hardening. Previous: Analytics Module shipped + Analytics polish pass …)

| Area | Status |
|------|--------|
| **Phase 1 — Foundations** (auth, roles, properties, permissions, audit, dashboard API/UI) | **Done** — see `backend/`, `frontend/` |
| **Phase 2 — Occupancy / ICS** | **Done** *(see migration `000002_occupancy`, `internal/occupancy`, Occupancy UI, token export)* |
| **Phase 3 — Nuki / Cleaning** | **Done** *(Nuki lifecycle + retries; cleaning analytics + salary + finance linkage)* |
| **Phase 4 — Finance / Invoices / Messages** | **Done** *(booking payout import + mapping + mojibake repair; invoice PDF versioning; property-scoped multilingual templates)* |
| **Phase 5 — Hardening** | **Shipped** *(Phase A security blockers H1–H3, Phase B reliability items M1–M3 + M5, and Phase C scale items M4a/M4b + L1 all landed; frontend Vitest scaffold and error-state reliability tests in place. Full Playwright end-to-end coverage and optional observability polish remain as non-blocking follow-ups — see §11)* |
| **Phase 6 — Analytics Module** | **Done** *(BI surface layered over occupancy + finance + cleaning; Outlook / Performance / Demand tabs; migrated yearly finance + cleaning rollups; freshness banner with ok/warn/stale tiers; returning-guest drill-down paginated; see `backend/internal/store/analytics.go`, `backend/internal/api/analytics_handlers.go`, `frontend/src/views/AnalyticsView.vue`, `spec/PMS_02_Module_Specifications.md` §9)* |
| **Phase 6.1 — Analytics polish** | **Done** *(glossary, money-first Performance layout, horizontal net-per-stay bars, unsold-range roll-up, gap/orphan weekday context, ISO-week heatmap axis, per-property `week_starts_on`; see `spec/PMS_06_Analytics_Fix_Plan.md`)* |
| **Phase 7 — UI/UX foundation & shell** | **Done** *(tokens.css + base.css landed; Inter Variable + JetBrains Mono Variable now self-hosted via `@fontsource-variable/*` and declared in `--font-family-*` tokens with the bundled woff2 files emitted to `dist/assets/`. Lucide icon set installed; core `Ui*` primitives — UiButton, UiCard, UiBadge, UiKpiCard, UiSection, UiPageHeader, UiInlineBanner, UiEmptyState, UiSkeleton, UiToolbar, UiTabs, UiInput, UiSelect, UiFileInput, UiTable, UiDialog, UiTag, UiIconButton, UiToast + ToastStack + `useToast()` composable — built; Vitest specs cover prop/ARIA variants for UiButton, UiBadge, UiInput, UiInlineBanner, UiTabs, UiKpiCard, UiFileInput, the toast composable, and the new `@/utils/format` helpers (`formatEmpty`, `formatShortDate`, `formatShortDateTime`, `formatYesNo`, `isoTitle`) — 73 passing. Dev-only `/ui-kit` route showcases every primitive. New AppTopbar + AppSidebar drawer with responsive rules replace the old text-link top nav; skip-link + `<main id="main-content">` landmark added; ShellView now emits a polite `aria-live` route-change announcer (A14) and updates `document.title` per route. Per-view migration (`PMS_08_UI_UX_Polish_Spec.md` §8) shipped: Dashboard (hero KPI row + Alerts card with ISO tooltip dates), PropertyForm/Detail, Users/UserDetail, BookingPayouts, Occupancy, Nuki (pin reveal is now a persistent `UiDialog` with 30 s countdown, clipboard copy, and server-side audit trail — no PIN ever sits inline), Cleaning, Messages, Invoices, Finance (tabbed Overview / Transactions / Recurring / Categories / Monthly breakdown workspace with `UiFileInput` adoption and toast notifications), and Analytics now compose the new primitives end-to-end and consume design tokens via `var(--…)` — SVG charts ship `<title>`/`<desc>` + `role="img"` and `--viz-*` strokes in place of hex. UiBadge gained a `label` prop (used by DashboardView). Inline-banner / empty-state / KPI cards / dialogs are unified across all property-scoped surfaces; legacy `.card` / raw button styles only remain inside hand-rolled SVG charts that intentionally keep their bespoke layout.)* |

Use `✅` = demonstrably done, `⬜` = not done or not yet verifiable. Notes in *italics* clarify partial or scope limits.

## 1. Global Platform Checklist

### Authentication
- ✅ Email/password login exists.
- ✅ Passwords are securely hashed.
- ✅ Authenticated session/token lifecycle works.
- ✅ Logout works.
- ✅ Protected API endpoints reject unauthenticated access.

### Users and Roles
- ✅ Roles `super_admin`, `owner`, `property_manager`, and `read_only` exist.
- ✅ Super admin can create users.
- ✅ Super admin can assign properties to users. *(via `owner_user_id` on property create and `property_user_permissions`; UI: Users → permissions.)*
- ✅ Owners can create their own properties.
- ✅ Property managers cannot exceed assigned permissions. *(enforced on implemented write APIs; re-verify as new module routes are added.)*
- ✅ Read-only users cannot perform writes. *(same scope note as above.)*

### Property and Permission Model
- ✅ Multiple properties can be created.
- ✅ A user can see only assigned properties unless super admin.
- ✅ Permissions are scoped by property.
- ✅ Permissions are scoped by module.
- ✅ Backend enforces access checks on every property/module endpoint. *(all **current** property-scoped routes; new modules must follow the same pattern.)*

### Logging
- ✅ Backend/API audit logging is implemented.
- ✅ Audit logs record actor, action, entity, outcome, and timestamp.
- ✅ Sensitive data is excluded from logs.

### Dashboard
- ✅ Dashboard exists. *(API + UI; cross-module widgets populated.)*
- ✅ Dashboard respects property context. *(`property_id` query param + UI property switcher.)*
- ✅ Dashboard hides widgets the user cannot access. *(backend `getDashboardSummary` gates each widget on `UserCan` per module; frontend `ShellView` filters nav via `auth.canAccessPropertyModule`.)*

## 2. Occupancy and ICS Checklist

*ICS URL remains on **Profile & integrations**; `occupancy_sources` tracks `source_type` + `active` (toggle on **Occupancy → Sync & export**).*

### Configuration
- ✅ Each property has configurable ICS URL.
- ✅ ICS source settings are editable in the UI.

### Sync
- ✅ Hourly sync job exists. *(interval configurable via `OCCUPANCY_SYNC_INTERVAL_MINUTES`, default 60; ticker in `cmd/server`.)*
- ✅ Manual sync can be triggered from the UI or API.
- ✅ Raw ICS events are stored. *(per sync run in `occupancy_raw_events`.)*
- ✅ Normalized occupancies are stored. *(`occupancies` table, upsert by `source_event_uid`.)*
- ✅ Duplicate events are not inserted repeatedly. *(same UID upserts; raw rows unique per run+uid.)*
- ✅ Changed upstream events update normalized occupancies. *(content hash + upsert; explicit `updated`, `cancelled`, `deleted_from_source` states.)*
- ✅ Sync runs are logged with success/failure status. *(`occupancy_sync_runs`; includes `partial` on mid-run errors.)*

### UI
- ✅ Month calendar view exists.
- ✅ Occupancy list view exists.
- ✅ Occupancies can be filtered meaningfully. *(month + status on list tab.)*
- ✅ Sync errors/statuses are visible. *(sync history table + dashboard occupancy sync summary when user has occupancy read.)*

### JSON Export
- ✅ Authenticated JSON occupancy endpoint exists. *(`GET /api/properties/{id}/occupancy-export?token=…`, no session.)*
- ✅ Export token can be managed securely. *(hashed at rest; plaintext shown once on create; revoke.)*
- ✅ JSON output includes stay dates and source metadata.

### Future-Proofing
- ✅ Source model supports future providers beyond Booking.com. *(`occupancy_sources.source_type` string, default `booking_ics`.)*

### Tests / gaps (optional follow-up)
- ✅ Broader automated tests for sync behavior. *(HTTP sync tests cover changed events => `updated`, mixed valid+broken ICS => `partial`, and state transitions.)*
- ✅ Explicit `updated` status on content change.

## 3. Nuki Access Checklist

*Module shipped in v1 scope: Nuki code lifecycle API/UI, sync runs, daily cleanup, and scheduler wiring are implemented. Real Nuki HTTP calls are supported, with optional mock mode for local development.*
*Status lifecycle business decision in code/spec: `not_generated` → `generated` → `revoked` (with `error_message` for failures).*

### Configuration
- ✅ Property stores Nuki credentials. *(secrets + settings UI; write-only token in API responses.)*
- ✅ Property stores Nuki `authID`. *(`cleaner_nuki_auth_id` on profile + smart lock id in secrets; naming differs slightly from spec wording but fields exist.)*
- ✅ Default check-in time is configurable.
- ✅ Default check-out time is configurable.

### Generation
- ✅ Access code can be generated from an occupancy.
- ✅ Validity window uses occupancy dates plus configured check-in/check-out times.
- ✅ Automatic generation after occupancy sync is supported or clearly queued. *(manual occupancy sync triggers Nuki sync; scheduler also chains occupancy -> Nuki sync.)*
- ✅ Re-imported occupancies do not create duplicate codes. *(unique key by `property_id + occupancy_id` and upsert behavior.)*

### Lifecycle
- ✅ Generated/current code list exists.
- ✅ Historical code list exists.
- ✅ Daily cleanup job removes or deactivates expired codes.
- ✅ Revocation is possible.
- ✅ Occupancy date changes reconcile existing codes safely.

### Reliability
- ✅ Nuki failures are stored visibly. *(`nuki_access_codes.error_message` + lifecycle statuses and `nuki_sync_runs` are exposed in API/UI.)*
- ✅ Retry flow exists for failed code operations. *(rerun generate/sync retries failed operations.)*

## 4. Cleaning Log Checklist

### Data Capture
- ✅ Cleaner activity is derived from Nuki only. *(reconcile ingests Nuki smartlock logs, keyed by configured cleaner auth ID.)*
- ✅ Only the first entry per day is counted.
- ✅ Later entries that day are ignored for metrics.
- ✅ Counted cleaning logs are stored per property/day.

### Fees and Salary
- ✅ Cleaning fee is configurable.
- ✅ Washing fee is configurable.
- ✅ Fee history supports effective dates.
- ✅ Monthly base salary is calculated from counted days.
- ✅ Manual monthly adjustment or bonus can be added.
- ✅ Final monthly salary reflects adjustments.

### Analytics
- ✅ Monthly cleaning count is shown.
- ✅ Monthly amount to pay is shown.
- ✅ Daily first-entry timestamps are visible.
- ✅ Heatmap of arrival times exists. *(UI rendered as horizontal bar chart for non-zero hourly buckets.)*
- ✅ Filters by property, month, and year exist. *(property context selector + month filter + yearly stats year selector.)*

### Integration
- ✅ Cleaner monthly salary can be linked into finance as an expense draft or entry. *(generated/updated as auto transaction `source_type=cleaning_salary` during Finance month open.)*

## 5. Finance Checklist

### Transactions
- ✅ Transaction create/edit/delete exists.
- ✅ Transaction fields include date, direction, amount, and category.
- ✅ EUR is treated as the only currency. *(amounts stored as integer cents; UI formatting and input are EUR-only.)*
- ✅ Manual booking income entries are supported.
- ✅ Attachments can be uploaded and stored. *(multipart upload stored under data dir attachments path; see §11 M3 — layout still `attachments/<property>/<timestamp>_<file>` rather than architecture-suggested `<property>/<transaction>/<file>`.)*

### Booking.com CSV Import
- ✅ CSV parse + row persistence exists. *(`parseBookingPayoutCSV` in `finance_handlers.go`; store in `store/finance_booking_payouts.go`.)*
- ✅ Reference-based mapping to occupancy + month idempotent. *(rematch + create-stay-from-payout flows.)*
- ✅ UTF-8 mojibake from Booking.com exports is repaired on ingest and at read-time. *(`fixCSVMojibake` in `api/booking_payout_display.go`; Windows-1252 reverse map covers 0x80–0x9F glyphs; covers guest name, raw row JSON, host name, occupancy summary; applied also in `resolveBookingPayoutOccupancy` inputs so fuzzy matching works on already-imported rows without a DB migration.)*
- ✅ Unit tests cover Turkish, Czech, German, and ASCII passthrough. *(`booking_payout_display_test.go`.)*

### Categories
- ✅ Categories exist and are manageable.
- ✅ Categories support reporting breakdowns.
- ✅ Categories can distinguish property income from other incoming cashflow. *(`counts_toward_property_income`.)*

### Summary and Reporting
- ✅ Total incoming is calculated.
- ✅ Total outgoing is calculated.
- ✅ Monthly incoming is calculated.
- ✅ Monthly outgoing is calculated.
- ✅ Total property income is calculated.
- ✅ Category breakdown is calculated.

### Recurring Expenses
- ✅ Recurring monthly expense rules exist.
- ✅ Opening a month generates missing recurring entries.
- ✅ Opening the same month twice does not duplicate recurring entries.
- ✅ Recurring amount changes affect only future months. *(future month-open runs use current rule values; already generated past months are keyed by `rule:YYYY-MM`.)*
- ✅ Historical generated entries remain unchanged. *(unless that exact month is explicitly re-opened, which intentionally reconciles generated auto rows.)*

### Cleaner Margin
- ✅ Cleaner salary expense is visible in finance.
- ✅ Cleaner margin against monthly property income is calculated.

## 6. Invoice Checklist

### Invoice Creation
- ✅ Manual invoice creation flow exists. *(POST endpoint + Vue form in `InvoicesView.vue`.)*
- ✅ One stay can be linked to one invoice. *(partial unique index `ux_invoices_property_occupancy`; occupancy picker in UI.)*
- ✅ Invoice language supports Slovak and English. *(language field on invoice; PDF renders SK/EN labels, footer, service summary.)*
- ✅ Issue date is captured.
- ✅ Taxable supply date is captured.
- ✅ Due date is captured.
- ✅ Stay start and end are captured.
- ✅ Amount is captured.

### Numbering
- ✅ Invoice numbering is unique per property and year. *(unique constraints on `invoice_number` and `(property_id, sequence_year, sequence_value)`.)*
- ✅ Variable symbol / invoice number is shown on the invoice. *(first row in details table: "Číslo faktúry (variabilný symbol)".)*
- ✅ Next invoice number preview exists or numbering is otherwise deterministic. *(API `GET .../invoice-sequence/next-preview`; shown in UI list card.)*

### Invoice Content
- ✅ Supplier data is included. *(snapshot from property profile at generation time.)*
- ✅ Customer data is included. *(manually entered; snapshot stored in `invoices.customer_snapshot_json`.)*
- ✅ Stay period is included. *(detail row + service summary section in PDF.)*
- ✅ Amount is included. *(highlighted total row in details card.)*
- ✅ Invoice states that payment was already made via Booking.com. *(payment note section; default text if none provided.)*
- ✅ PDF branding is applied. *(mockup-aligned design: navy header, rounded cards, clipboard icons, footer with heart + euro doodle, green paid badge.)*

### Storage and History
- ✅ Invoice metadata is stored in DB. *(`invoices` + `invoice_files` tables.)*
- ✅ PDF file is stored on disk. *(under `data/invoices/{propertyID}/{year}/`.)*
- ✅ PDF can be downloaded. *(API `GET .../download`; UI button.)*
- ✅ Invoice can be edited. *(PATCH endpoint + form pre-fill in UI.)*
- ✅ Invoice can be regenerated. *(POST `.../regenerate`; UI button.)*
- ✅ Regeneration preserves version history. *(new `invoice_files` row per regeneration; version history table in UI.)*

## 7. Customer Messages Checklist

### Templates
- ✅ Templates are property-specific. *(unique constraint `(property_id, language_code, template_type)`; property-scoped API endpoints.)*
- ✅ Templates are editable in the UI. *(Templates tab with inline editor, title/body fields, placeholder insertion buttons.)*
- ✅ Templates exist for English, Slovak, German, Ukrainian, and Hungarian. *(default templates auto-created on first access via `EnsureDefaultMessageTemplates`.)*
- ✅ Template placeholders are validated. *(`ValidateTemplatePlaceholders` rejects unsupported `{{...}}` tokens; API returns 400 with details.)*

### Generation
- ✅ Messages are generated per occupancy/stay row. *(occupancy picker in Generate tab; API `GET .../messages/generate?occupancy_id=...`.)*
- ✅ Generated message includes stay dates. *(`{{stay_start}}` / `{{stay_end}}` resolved from occupancy dates in property timezone.)*
- ✅ Generated message includes property name and address. *(`{{property_name}}` / `{{property_address}}` from property + profile.)*
- ✅ Generated message includes Wi-Fi details. *(`{{wifi_name}}` / `{{wifi_password}}` from `property_profiles.wifi_ssid` / `wifi_password`.)*
- ✅ Generated message includes parking details. *(`{{parking_info}}` from `property_profiles.parking_instructions`.)*
- ✅ Generated message includes contact phone. *(`{{contact_phone}}` from `property_profiles.contact_phone`.)*
- ✅ Generated message includes Nuki code. *(`{{nuki_code}}` resolved from `GetNukiCodeByOccupancyID`; shows "—" if unavailable.)*
- ✅ Generated message includes check-in/check-out times. *(`{{check_in_time}}` / `{{check_out_time}}` from property profile defaults.)*

### UX
- ✅ Per-language copy-to-clipboard action exists. *(Copy button per rendered message card; visual "Copied" feedback.)*
- ✅ Message preview exists. *(generated messages rendered in styled cards with all placeholders resolved.)*
- ✅ Missing Nuki code is handled clearly. *(warning banner shown when `nuki_available=false`; placeholder shows "—".)*

## 8. Dashboard Checklist

### Widgets
- ✅ Upcoming stays widget exists. *(`ListUpcomingOccupancies` feeds `dashboardUpcomingStayRow`; rendered with stay dates + status.)*
- ✅ Active Nuki codes widget exists. *(`ListUpcomingStaysForNuki` filtered to `generated` status; shows label/masked/validity/error.)*
- ✅ Latest sync status widget exists. *(per-module `sync_status` map — occupancy + Nuki — with `not_configured` / `no_sync_yet` / success/partial/error.)*
- ✅ Monthly cleaning widget exists. *(`ComputeCleaningMonthlySummary` → counted days + salary draft in EUR.)*
- ✅ Monthly finance summary widget exists. *(incoming / outgoing / net, property-timezone aware.)*
- ✅ Invoice overview widget exists. *(last 3 invoices from `ListInvoices` with number, customer, total, version.)*

### Behavior
- ✅ Widgets respect user permissions. *(each widget branch gated by `UserCan` on the corresponding module + read level; nav in `ShellView` hidden accordingly.)*
- ✅ Widgets handle missing module configuration gracefully. *(sync widget explicitly returns `not_configured` when secrets are absent; other widgets omit the key when data retrieval fails or module is not accessible.)*

## 9. Non-Functional Checklist

### Architecture Quality
- ✅ Backend uses clear module boundaries. *(layered `cmd` / `internal/api` / `internal/store` / `internal/migrate`; room to split by domain as features land.)*
- ✅ Repository/persistence layer is structured to ease PostgreSQL migration later. *(SQL schema and types chosen to be PG-friendly; still SQLite-only today.)*
- ✅ Database migrations are present. *(embedded SQL in `backend/internal/migrate/`.)*
- ✅ External integrations are behind interfaces/services. *(occupancy and Nuki each run behind dedicated services; Nuki uses a client interface with real/mock implementations.)*

### Reliability
- ✅ Scheduled jobs are idempotent. *(occupancy and Nuki/cleaning schedulers are rerun-safe; finance month-open generation is idempotent by source reference.)*
- ✅ Integration failures are visible in UI or admin views. *(occupancy sync runs + error column / dashboard hint.)*
- ⬜ Error states do not silently corrupt data. *(partially; occupancy marks `deleted_from_source` only after a successful fetch+parse.)*

### Security
- ✅ Secrets are not exposed in normal API responses. *(settings API uses flags / masking for integration secrets.)*
- ✅ Secrets are not written to logs.
- ✅ Property-scoped access is enforced server-side. *(for implemented endpoints.)*
- ✅ Automation export endpoint uses token protection. *(occupancy JSON export via `occupancy_api_tokens`.)*

### Time Handling
- ✅ Property timezone is stored.
- ✅ Monthly calculations use property timezone consistently. *(occupancy calendar/list month windows use property `timezone` for overlap queries.)*

### Testing
- ✅ Automated tests exist for authentication and authorization. *(basic API tests in `backend/internal/api/server_test.go`.)*
- ✅ Automated tests exist for occupancy sync logic. *(ICS parse unit tests + HTTP sync behavior tests in `internal/occupancy`.)*
- ✅ Automated tests exist for Nuki access lifecycle. *(`internal/nuki/service_test.go`: create/update dedupe, failure states, cleanup, and revocation reconciliation.)*
- ✅ Automated tests exist for cleaning salary calculations. *(`internal/store/cleaning_test.go` + reconcile behavior tests in `internal/nuki/service_test.go`.)*
- ✅ Automated tests exist for finance recurring rules and summaries. *(`internal/store/finance_test.go` covers recurring idempotency/timezone and cleaner margin summary; API tests cover booking payout import/mapping flows.)*
- ✅ Automated tests exist for invoice numbering and PDF generation. *(API tests: create, regenerate versioned PDF, duplicate occupancy/payout guards, invoice code prefix; PDF smoke test in `invoicepdf_test.go`.)*
- ✅ Automated tests exist for message placeholder rendering. *(store unit tests for `RenderMessageTemplate` and `ValidateTemplatePlaceholders`; API integration tests for template CRUD, default creation, invalid placeholder rejection, and end-to-end message generation.)*

## 10. Recommended Delivery Order Checklist
- ✅ Foundation and auth completed first.
- ✅ Property and permission system completed next.
- ✅ Occupancy sync completed before Nuki/messages.
- ✅ Nuki module completed before message generation.
- ✅ Cleaning module completed before cleaner finance integration.
- ✅ Finance and invoicing completed after occupancy foundation is stable.
- ✅ Dashboard summaries landed after all source modules were live.
- ⬜ Hardening pass before commercial launch (see §11).

## 11. Commercial-Readiness Hardening Checklist

*Tracks the outstanding items from `PMS_04_Audit_Report_2026-04-13.md` with their current status in the code. Complete this section before marking v1 as production-ready.*

### Phase A — Launch blockers (security)
- ✅ **H1 — Session cookie `Secure` is env-driven.** `config.Load` reads `PMS_ENV`, `PMS_COOKIE_SECURE` and `PMS_COOKIE_SAMESITE`; `Secure` defaults to `true` in production and is wired through `api.Server.CookieSecure` / `CookieSameSite` for both login and logout cookies. `SameSite=None` without `Secure=true` is rejected at config load. *Files:* `backend/internal/config/config.go`, `backend/internal/api/server.go`, `backend/cmd/server/main.go`.
- ✅ **H2 — Occupancy export token moves out of the query string.** `getOccupancyExportPublic` now reads the token from `Authorization: Bearer …`, falls back to `X-Export-Token`, and treats the legacy `?token=` query parameter as deprecated (emits a `Warning` response header + audit-log entry). A new `internal/middleware.AccessLog` replaces chi's default logger and redacts `token`, `access_token`, `api_key`, `secret`, `password` query keys before writing to the request log. The Occupancy UI now surfaces a copy-curl button that uses the header form. *Files:* `backend/internal/api/occupancy_handlers.go`, `backend/internal/middleware/accesslog.go`, `backend/cmd/server/main.go`, `frontend/src/views/OccupancyView.vue`.
- ✅ **H3 — Nuki PIN stops being returned in plaintext.** `generated_pin` is removed from `GET /properties/{id}/nuki/upcoming-stays`; only the masked code remains. A new `GET /properties/{id}/nuki/codes/{codeId}/reveal-pin` endpoint returns the plaintext PIN, is gated on `NukiAccess` **write**-level (so read-only viewers cannot enumerate PINs), and writes a `nuki_reveal_pin` audit entry for every call (including the "empty" outcome when no PIN is stored). The Nuki UI replaces the inline PIN cell with a "Reveal"/"Hide" toggle and also reveals the PIN automatically right after a successful generation. *Follow-up (separate ticket):* encrypt `nuki_access_codes.generated_pin_plain` at rest with a master key once key management is in place. *Files:* `backend/internal/api/nuki_handlers.go`, `backend/internal/api/server.go`, `frontend/src/views/NukiView.vue`.
- ✅ **Scope truthfulness.** Invoices and Messages are shipped, matching the v1 spec.

### Phase B — Commercial hardening (reliability / UX)
- ✅ **M2 — Payout import is per-row transactional.** *(resolved; `Store.ImportBookingPayoutRow` wraps each create-transaction + create-payout pair in a single `BeginTx`/`Commit`, so a failure can't leave a finance row without its payout mapping or vice versa. `finance_handlers.go` now delegates to it.)*
- ✅ **M1 — Frontend navigation is permission-aware.** *(resolved; `ShellView` filters `APP_NAV_ITEMS` via `auth.canAccessPropertyModule` and redirects disallowed routes.)*
- ✅ **M3 — Attachment storage matches the architecture spec.** *(resolved; `saveFinanceAttachment` now stores under `attachments/<property_id>/<transaction_id>/<filename>`. JSON payloads no longer accept a client-supplied `attachment_path`; multipart uploads are persisted only after the transaction row exists. A startup relocation step — `relocateLegacyFinanceAttachments` in `backend/cmd/server/attachments_relocate.go` — moves files from the legacy layout and rewrites DB paths on first boot.)*
- ✅ **M5 — Scheduler is safe for multi-instance deploys.** *(resolved; new `job_leases` table (migration `000015_job_leases`) plus `Store.TryAcquireJobLease`/`ReleaseJobLease` helpers gate each scheduler tick. `main.go` generates a per-process instance ID and wraps the occupancy sync, Nuki cleanup, and cleaning reconcile tickers in a cooperative lease so a second replica can't double-run hourly jobs.)*

### Phase C — Scale & maintainability
- ✅ **M4a — Drop `db.SetMaxOpenConns(1)`.** *(resolved; `dbconn.Open` now enables WAL journaling, sets `busy_timeout=5000`, keeps `foreign_keys=ON` + `synchronous=NORMAL`, and raises the pool to `MaxOpenConns=8`/`MaxIdleConns=8`. SQLite serializes writes at the file level through `busy_timeout`, so reads can run concurrently without the historical single-connection bottleneck.)*
- ✅ **M4b — Replace list-then-filter lookups.** *(resolved; `Store.GetFinanceTransactionByID` and `Store.GetFinanceRecurringRuleByID` now issue targeted `SELECT … WHERE property_id = ? AND id = ?` queries instead of loading the full list and scanning in Go.)*
- ✅ **L1 — Dashboard API path canonicalization.** *(resolved; `GET /api/properties/{id}/dashboard` is registered alongside the legacy `GET /api/dashboard/summary?property_id=…`. `DashboardView.vue` now calls the canonical path; the legacy route is kept for backward compatibility and shares the same handler.)*
- ✅ **Automated frontend tests for high-risk flows.** *(scaffolded; Vitest + jsdom test runner added under `frontend/`. Initial suite covers the `api()` HTTP wrapper's error/success paths and the `useAuthStore.canAccessPropertyModule` permission matrix. Root-level `make test` now runs both backend and frontend suites. Full end-to-end coverage — login round-trip, property switching, invoice create + regenerate, message generation with a missing Nuki code — is tracked as a follow-up requiring Playwright + a fixture server.)*
- ✅ **Error-state reliability sweep.** *(resolved; new `backend/internal/store/reliability_test.go` exercises the M2 atomicity contract (a duplicate payout import must leave neither an orphan finance transaction nor a payout row) and the M5 lease semantics (mutual exclusion between owners, renewal by the current owner, and takeover of an expired lease). Partial-write contract documented implicitly by the tests.)*

### Non-blocking polish (post-v1 candidates)
- ✅ **Backup / export hooks for SQLite + data dir.** *(resolved; `GET /api/admin/backup` streams a gzipped tar containing a consistent SQLite snapshot — produced via `VACUUM INTO` so it is WAL-safe — plus the `invoices/` and `attachments/` subtrees under the configured data dir. The endpoint is gated on `super_admin`, audited, and never stages the full archive on disk. Covered by `TestGetAdminBackup_SuperAdminGetsTarGzWithDB` and `TestGetAdminBackup_NonAdminForbidden` in `backend/internal/api/admin_backup_test.go`.)*
- ✅ **Observability.** *(resolved; access logger now supports structured JSON output via `PMS_ACCESS_LOG_FORMAT=json` with sensitive query-string keys already redacted. A new zero-dependency `backend/internal/metrics` package exposes Prometheus-format counters for `pms_http_requests_total`, `pms_http_request_duration_seconds`, `pms_scheduler_runs_total`, `pms_scheduler_last_run_timestamp_seconds`, and `pms_attachment_relocations_total`, served at `GET /metrics` and optionally gated by `PMS_METRICS_TOKEN` (Bearer). Scheduler ticks emit `ran`/`skipped`/`error` outcomes; the access-log middleware hands request observations to the registry via `SetAccessObserver`.)*
- ☑ Optional: direct Google Calendar sync — explicitly deferred in the architecture spec; not planned for v1.
