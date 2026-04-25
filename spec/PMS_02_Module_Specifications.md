# PMS Module Specifications

## How to Use This Document
This document describes each PMS module in implementation-ready form for a single AI coding agent. Each section includes:
- purpose
- functional requirements
- business rules
- suggested backend APIs
- suggested database entities
- frontend screens
- test focus

Use `PMS_01_Architecture_and_Global_Spec.md` for global rules and shared assumptions.

## 1. Global Platform Module

### Purpose
Provide the shared platform capabilities required by all business modules: authentication, authorization, property management, user management, logging, and dashboarding.

### Functional Requirements
- Users must log in using email and password.
- All API requests except login and health checks must be authenticated.
- A user may have access to multiple properties.
- Access is restricted by property and module.
- Super admin can see and manage all data.
- Owners can create their own properties.
- Super admin can create properties on behalf of users.
- Property managers and read-only users only access assigned properties and modules.
- Backend/API audit logging must be enabled in v1.

### Business Rules
- Property visibility is never global unless the user is super admin.
- Frontend navigation must hide modules the user cannot access, but the backend must still enforce access.
- Property context must be explicit in all module requests.

### Suggested API Endpoints

#### Auth
- `POST /api/auth/login`
- `POST /api/auth/logout`
- `GET /api/auth/me`

#### Users and Permissions
- `GET /api/users`
- `POST /api/users`
- `GET /api/users/{id}`
- `PATCH /api/users/{id}`
- `POST /api/users/{id}/property-permissions`
- `DELETE /api/users/{id}/property-permissions/{permissionId}`

#### Properties
- `GET /api/properties`
- `POST /api/properties`
- `GET /api/properties/{id}`
- `PATCH /api/properties/{id}`
- `GET /api/properties/{id}/settings`
- `PATCH /api/properties/{id}/settings`

#### Dashboard
- `GET /api/dashboard/summary?property_id=...`

### Suggested Database Entities
- `users`
- `roles`
- `property_user_permissions`
- `properties`
- `property_profiles`
- `property_secrets`
- `api_audit_logs`
- `auth_sessions`

### Frontend Screens
- login page
- users administration page
- property list
- property create/edit form
- permissions matrix page
- dashboard page with per-property widgets

### Test Focus
- authentication and session lifecycle
- role enforcement
- property scoping
- module-level authorization
- audit log creation for write endpoints

## 2. Occupancy and ICS Sync Module

### Purpose
Import property occupancy from configurable ICS sources, store raw and normalized data, display occupancy in UI, and expose it through an authenticated JSON endpoint for automation.

### Functional Requirements
- Each property has a configurable ICS URL.
- The system fetches ICS hourly.
- A manual sync trigger must be available.
- Raw ICS events must be stored for traceability.
- Normalized occupancies must be generated from raw events.
- Occupancies must be shown in:
  - month calendar view
  - list/table view
- An authenticated API endpoint must expose occupancies as JSON for n8n and similar automation.
- The design must support future source types such as Airbnb and direct bookings.

### Business Rules
- Sync must update existing occupancies when upstream ICS data changes.
- Source identity should rely on stable UID or best available event fingerprint.
- Occupancy is the primary shared record used by Nuki, messages, and optionally invoices.
- JSON endpoint access must use a token separate from the normal browser session if intended for automation use.

### Normalization Rules
- Store raw source data unchanged where possible.
- Map source event dates into property timezone.
- Normalize start and end into occupancy date/time values.
- Track sync status such as `active`, `updated`, `cancelled`, or `deleted_from_source` if source changes imply removal.

### Suggested API Endpoints
- `GET /api/properties/{id}/occupancies`
- `GET /api/properties/{id}/occupancies/calendar?month=YYYY-MM`
- `POST /api/properties/{id}/occupancy-sync/run`
- `GET /api/properties/{id}/occupancy-sync/runs`
- `PATCH /api/properties/{id}/occupancy-source`
- `GET /api/properties/{id}/occupancy-export?token=...`

### Suggested JSON Export Fields
- occupancy id
- property id
- property name
- source type
- external event uid
- stay start
- stay end
- status
- raw summary
- last synced at

### Suggested Database Entities
- `occupancy_sources`
- `occupancy_raw_events`
- `occupancies`
- `occupancy_sync_runs`
- `occupancy_api_tokens`

### Frontend Screens
- occupancy calendar page
- occupancy list page with filtering by month and status
- occupancy source settings panel
- sync status/history panel

### Test Focus
- ICS parsing
- duplicate prevention
- change detection
- manual and scheduled sync behavior
- JSON token authorization
- timezone correctness for displayed stays

## 3. Nuki Access Module

### Purpose
Create and manage Nuki access codes for stays based on occupancies and configured check-in/check-out times.

### Functional Requirements
- Each property stores Nuki integration credentials and one `authID`.
- Access codes can be generated from occupancies.
- Check-in and check-out times are configurable per property.
- The system should support automatic code creation after occupancy sync.
- The system must avoid duplicates if an occupancy is re-imported.
- Old codes must be cleaned up daily.
- Status lifecycle for PMS-managed Nuki access codes in v1 is:
  - `not_generated`
  - `generated`
  - `revoked`
- Generation/sync failures must be represented via `error_message` while keeping the lifecycle status model above.
- UI must show:
  - generated codes
  - historical codes
  - occupancy linkage
  - valid time window
  - status and sync errors

### Business Rules
- Code validity start = occupancy arrival date at configured check-in time.
- Code validity end = occupancy departure date at configured check-out time.
- If occupancy dates change after code creation, the existing code must be updated or revoked/recreated safely.
- Access code history must remain visible even after revocation or expiration.
- API failures must produce retryable error states.
- `not_generated` means no currently usable Nuki code is linked for that occupancy.
- `generated` means a usable Nuki code is linked and managed.
- `revoked` means a previously linked/generated code was revoked or deleted and remains visible as history.

### Status Model (Business Decision)
- `not_generated`
- `generated`
- `revoked`
- Error details live in `error_message` and sync/event logs, not as a separate lifecycle status.

### Suggested API Endpoints
- `GET /api/properties/{id}/nuki/codes`
- `POST /api/properties/{id}/nuki/codes/generate`
- `POST /api/properties/{id}/nuki/codes/{codeId}/revoke`
- `POST /api/properties/{id}/nuki/sync/run`
- `GET /api/properties/{id}/nuki/runs`

### Suggested Database Entities
- `nuki_access_codes`
- `nuki_sync_runs`
- `nuki_event_logs`

### Frontend Screens
- generated/current codes table
- historical codes table
- Nuki integration settings panel
- code details drawer or dialog
- error/retry actions

### Test Focus
- code validity window calculation
- duplicate protection
- occupancy update reconciliation
- cleanup job
- failed integration retries

## 4. Cleaning Log and Salary Module

### Purpose
Use Nuki entry activity to derive cleaning events, calculate cleaner salary, show monthly analytics, and render an arrival-time heat map.

### Functional Requirements
- Cleaner activity is sourced only from Nuki.
- Each property stores one cleaner-linked Nuki `authID` in v1.
- For each day, only the first valid entry counts.
- Later entries on the same day are fully ignored.
- Cleaner fee and washing fee are configurable.
- Fee values can change over time with effective dates.
- Monthly salary must be calculated automatically.
- Monthly salary can be manually adjusted, for example with a bonus.
- The UI must show on one page:
  - counted cleaning days
  - monthly salary
  - raw first-entry timestamps
  - fee history
  - manual adjustments
  - heatmap by hour bucket
  - filters by property, month, and year

### Business Rules
- Cleaning count per month = number of unique days with first valid entry.
- Base salary per cleaning day = `cleaning_fee + washing_fee`.
- Monthly base salary = counted days x day rate applicable at event date.
- Final monthly salary = base salary + sum of monthly adjustments.
- Fee changes apply immediately from the configured effective timestamp/date.
- Cleaning entries must be tied to property timezone.

### Heatmap Rules
- `09:05` counts into `09:00-10:00`
- `08:59` counts into `08:00-09:00`
- only first entry of a day contributes

### Suggested API Endpoints
- `GET /api/properties/{id}/cleaning/logs?month=YYYY-MM`
- `GET /api/properties/{id}/cleaning/summary?month=YYYY-MM`
- `GET /api/properties/{id}/cleaning/heatmap?month=YYYY-MM`
- `GET /api/properties/{id}/cleaning/fees`
- `POST /api/properties/{id}/cleaning/fees`
- `GET /api/properties/{id}/cleaning/adjustments?month=YYYY-MM`
- `POST /api/properties/{id}/cleaning/adjustments`

### Suggested Database Entities
- `cleaner_profiles`
- `cleaner_fee_history`
- `cleaning_daily_logs`
- `cleaning_monthly_summaries`
- `cleaning_salary_adjustments`

### Frontend Screens
- single cleaning analytics page
- monthly summary cards
- daily log table
- fee history panel
- adjustment modal/form
- heatmap chart

> **Yearly cleaning stats moved to Analytics (§9).** The "Yearly Cleaning Stats" card is removed from the cleaning page; the same 12-bar-per-year chart now lives in the Performance tab of the Analytics module and is gated on `analytics:read`. The `GET /api/properties/{id}/cleaning/yearly-stats` endpoint is deprecated — keep it responding during migration, then delete once no UI calls it.

### Test Focus
- one-entry-per-day counting
- fee history application over time
- bonus/adjustment calculation
- month and year filtering
- heatmap bucketing

## 5. Finance Module

### Purpose
Provide a financial overview for each property, including income, expenses, recurring expenses, category breakdowns, attachments, and cleaner salary margin visibility.

### Functional Requirements
- Transactions support:
  - date
  - direction: incoming or outgoing
  - amount
  - category
  - note optional
  - attachment optional
- Currency is EUR only.
- Income usually comes from bookings but can also be entered manually.
- Show totals:
  - total incoming
  - total outgoing
  - monthly incoming
  - monthly outgoing
  - total income for the property
  - category breakdown
- Support recurring monthly expenses.
- Recurring expenses are generated when a month is opened.
- Recurring amounts can change over time.
- Changes affect only future months.
- Cleaner salary must appear as a linked monthly expense draft or entry.
- Display cleaner salary margin against total monthly property income.

### Business Rules
- Transaction categories determine reporting semantics, including whether an incoming transaction counts as property income.
- Recurring rule generation must be idempotent per property/month/rule version.
- Generated cleaner expense should stay linked to the monthly cleaning summary.
- Attachments must be stored on disk with metadata in DB.

### Category Model Recommendation
Seed baseline categories such as:
- booking_income
- utility_refund
- mortgage
- utilities
- internet
- cleaning_salary
- maintenance
- tax
- supplies
- other_income
- other_expense

Include a flag like `counts_toward_property_income`.

### Suggested API Endpoints
- `GET /api/properties/{id}/finance/transactions`
- `POST /api/properties/{id}/finance/transactions`
- `PATCH /api/properties/{id}/finance/transactions/{transactionId}`
- `DELETE /api/properties/{id}/finance/transactions/{transactionId}`
- `POST /api/properties/{id}/finance/months/{YYYY-MM}/open`
- `GET /api/properties/{id}/finance/summary?month=YYYY-MM`
- `GET /api/properties/{id}/finance/categories`
- `POST /api/properties/{id}/finance/categories`
- `GET /api/properties/{id}/finance/recurring-rules`
- `POST /api/properties/{id}/finance/recurring-rules`
- `PATCH /api/properties/{id}/finance/recurring-rules/{ruleId}`

### Suggested Database Entities
- `finance_categories`
- `finance_transactions`
- `finance_recurring_rules`
- `finance_month_states`

### Frontend Screens
- ledger table view
- transaction create/edit form
- monthly summary cards
- category breakdown widgets
- recurring expense management page
- attachment upload field

> **Yearly Overview moved to Analytics (§9).** The three "Yearly incoming / outgoing / net" tiles are removed from the finance page; the same figures now surface in the Performance tab of the Analytics module and are gated on `analytics:read`. The `yearly_incoming_cents` / `yearly_outgoing_cents` / `yearly_net_cents` fields on `GET /api/properties/{id}/finance/summary` are deprecated — keep returning them during migration for backwards compatibility, then drop once no UI consumes them.

### Test Focus
- monthly totals
- category breakdown calculations
- recurring generation idempotency
- future-only recurring updates
- cleaner margin calculation
- attachment metadata persistence

## 6. Invoice Module

### Purpose
Allow manual creation of PDF invoices for stays, in Slovak or English, with storage in DB and on disk.

### Functional Requirements
- Invoice creation is manual.
- One stay corresponds to one invoice.
- Invoice numbering is compliant with Slovak numbering expectations.
- Numbering is per property and per year.
- Invoice form supports:
  - language: Slovak or English
  - issue date
  - taxable supply date
  - due date
  - stay start and end
  - amount
  - supplier details
  - customer details
- Supplier details come from owner/business profile snapshots at generation time.
- Customer details are manually entered.
- Generated invoice must be marked as already paid via Booking.com.
- PDF must be stored on disk and represented in DB.
- Existing invoices must be downloadable.
- Invoice must be editable and regeneratable.
- Regeneration should preserve history through versioning.
- Branding support is required.

### Required Invoice Content

#### Supplier
- property/owner billing identity
- name
- address
- ICO
- DIC
- VAT ID if present, even if non-VAT mode does not calculate VAT

#### Customer
- name
- address
- city
- ZIP code
- company name
- VAT number

#### Stay and Billing
- invoice number / variable symbol
- stay period
- payable amount
- note that invoice is already paid and no payment is required because customer paid via Booking.com

### Business Rules
- Invoice snapshots must not change automatically when owner profile later changes.
- PDF regeneration should create a new file version while preserving prior metadata.
- The system must prevent duplicate invoice numbers within the same property-year.
- Because invoices are manual, occupancy linkage is optional but recommended.

### Suggested Invoice Number Strategy
Format suggestion:
- `<property_code>/<year>/<sequence>`

Example:
- `APT01/2026/0001`

The precise Slovak formatting can be adjusted, but uniqueness per property/year is mandatory.

### Suggested API Endpoints
- `GET /api/properties/{id}/invoices`
- `POST /api/properties/{id}/invoices`
- `GET /api/properties/{id}/invoices/{invoiceId}`
- `PATCH /api/properties/{id}/invoices/{invoiceId}`
- `POST /api/properties/{id}/invoices/{invoiceId}/regenerate`
- `GET /api/properties/{id}/invoices/{invoiceId}/download`
- `GET /api/properties/{id}/invoice-sequence/next-preview`

### Suggested Database Entities
- `invoice_sequences`
- `invoices`
- `invoice_files`

### Frontend Screens
- invoice list page
- invoice create/edit form
- invoice preview/download action
- invoice version history panel

### Test Focus
- numbering uniqueness
- correct language output
- correct stay period rendering
- PDF regeneration versioning
- file path persistence and download

## 7. Customer Message Templates Module

### Purpose
Generate property-specific multilingual check-in instruction messages with placeholders filled from property settings, occupancies, and Nuki access data, then copy them to clipboard.

### Functional Requirements
- Messages are generic, not guest-personalized.
- Supported languages in v1:
  - English
  - Slovak
  - German
  - Ukrainian
  - Hungarian
- Templates are editable in the UI.
- Templates are property-specific.
- Only check-in messages are required in v1.
- Generated message rows are tied to occupancies.
- Message generation must inject:
  - stay dates
  - property name
  - address
  - Wi-Fi details
  - parking details
  - contact phone
  - Nuki access code
  - check-in time
  - check-out time
- UI must provide one-click copy-to-clipboard actions per language.

### Business Rules
- If no Nuki code exists yet, the generated message must clearly show a missing-code state or disable copy until code is available.
- Template rendering should be previewable before copying.
- Templates should support placeholders and validation against unsupported variables.

### Suggested Placeholder Set
- `{{property_name}}`
- `{{property_address}}`
- `{{stay_start}}`
- `{{stay_end}}`
- `{{check_in_time}}`
- `{{check_out_time}}`
- `{{nuki_code}}`
- `{{wifi_name}}`
- `{{wifi_password}}`
- `{{parking_info}}`
- `{{contact_phone}}`

### Suggested API Endpoints
- `GET /api/properties/{id}/message-templates`
- `POST /api/properties/{id}/message-templates`
- `PATCH /api/properties/{id}/message-templates/{templateId}`
- `GET /api/properties/{id}/messages/generate?occupancy_id=...`

### Suggested Database Entities
- `message_templates`
- `message_template_versions` optional

### Frontend Screens
- message generation table keyed by occupancy
- copy buttons for each supported language
- template editor form
- preview dialog

### Test Focus
- placeholder replacement
- missing-code behavior
- property-specific template isolation
- copy payload correctness by language

## 8. Dashboard Module

### Purpose
Surface useful cross-module operational and financial summaries on a single property dashboard.

### Functional Requirements
- Show upcoming stays
- Show active Nuki codes
- Show latest sync status
- Show current month cleaning count and salary
- Show current month income/outgoing totals
- Show invoice counts or latest generated invoices

### Business Rules
- Dashboard widgets should be permission-aware.
- Widgets should degrade gracefully if a module is not configured for a property.

### Suggested API Endpoints
- `GET /api/properties/{id}/dashboard`

### Frontend Screens
- per-property dashboard landing page

### Test Focus
- permission-aware widget visibility
- partial data availability
- month aggregation correctness

## 9. Analytics Module

### Purpose
Provide the property owner with a **strategic business-intelligence layer** sitting on top of occupancy, finance-payouts, and cleaning data. This module is read-only and complements — does not replace — the operational Dashboard (§8). The source of truth for the business rules, metric definitions, disclaimers, and tab layout is [`PMS_05_Analytics_Module_Spec.md`](PMS_05_Analytics_Module_Spec.md); this section is the **implementation contract** that an AI developer agent must follow to build it.

### Functional Requirements
- Single `/analytics` page with three tabs: **Outlook** (forward-looking, default), **Performance** (retrospective), **Demand & Pricing**.
- Persistent **freshness bar** across all tabs: last ICS sync timestamp, last payout CSV import date, count of unmatched payouts.
- Period selector on Performance and Demand & Pricing (current month / rolling 12m / calendar year / custom range), with a YoY toggle.
- All revenue-annotated widgets must render a visible "revenue data through: <date>" disclaimer and a yellow/red staleness banner at 45-day / 75-day thresholds.
- Forward-revenue tiles must visually split **confirmed** (payout matched) from **estimated** (trailing-12m ADR × unmatched nights) portions.
- Returning-guest panel must display the "fuzzy name match — review for accuracy" caption and link to a paginated drill-down.
- Degrades gracefully when history is insufficient: YoY hidden for windows with <13 months of data, pace curves skip missing LY comparisons, heatmaps show "not enough data" tiles rather than zeros.

### Business Rules
- **Scope is single-property** (locked). The property is resolved from auth context; the `property_id` query parameter is accepted for future-proofing but must be validated against the caller's permissions.
- **Cancelled stays** (`occupancies.status IN ('cancelled','deleted_from_source')`) are excluded from occupancy, ADR, RevPAR, and revenue totals; they count only in cancellation metrics.
- **Revenue metrics cohort by arrival date** (`check_in_date` on payout, or `start_at` on occupancy), not by payout date. A March payout for a February stay belongs to February.
- **Stays without a matched payout** contribute to occupancy but not to actual-revenue metrics. Forward-revenue estimation is the only exception.
- **YoY** anchors on calendar period: July 2026 vs July 2025.
- **Pace vs LY** uses the "as-of offset" technique: reconstruct the set of bookings whose `imported_at ≤ D` for each historical day `D`.
- **Returning-guest detection** uses `normalize(guest_name) = lowercase + NFD unicode strip + trim + collapse spaces`, rejects names <6 normalized characters, and is a descriptive statistic only — it must never influence revenue or occupancy numbers.
- **All endpoints** require `analytics` module read permission on the property (see §7 of `PMS_01`). Permission string: `analytics`, level `read`.
- **Endpoints are read-only**; no DB writes, no new side effects. Safe to cache HTTP response body with `Cache-Control: private, max-age=60` *only if* the global `Cache-Control: no-store` default from `WriteJSON` is explicitly overridden per-handler.

### Suggested API Endpoints
All routes are registered under the authenticated router group in `backend/internal/api/server.go`.
- `GET /api/properties/{id}/analytics/outlook` — forward KPIs (30/60/90 nights + revenue split confirmed/estimated), pacing series (next 90 days), unsold-nights list (next 14 days), new-bookings sparkline (last 7 days).
- `GET /api/properties/{id}/analytics/performance?from=YYYY-MM-DD&to=YYYY-MM-DD&yoy=true` — occupancy, ADR, RevPAR, gross, net, effective take-rate for the period and prior-year window; monthly trend (24 months); ISO-week × year seasonality heatmap; day-of-week occupancy; cancellation rate + lead-time histogram; net-per-stay stay list.
- `GET /api/properties/{id}/analytics/demand?from=YYYY-MM-DD&to=YYYY-MM-DD` — lead-time distribution, length-of-stay distribution, ADR-by-month/DOW/lead-bucket, gap-nights list, orphan-midweek list, returning-guests summary.
- `GET /api/properties/{id}/analytics/pace?window=YYYY-MM` — cumulative booking-pace curve for the arrival window, plus the same-named window a year earlier.
- `GET /api/properties/{id}/analytics/returning-guests?limit=50&offset=0` — paginated drill-down `{ name, stay_count, first_stay, last_stay }`.
- `GET /api/properties/{id}/analytics/freshness` — `{ last_ics_sync_at, last_payout_date, unmatched_payouts_count, staleness_level: 'ok'|'warn'|'stale' }`.

Every list response echoes back the input filters and a `generated_at` RFC3339 timestamp.

### Display preferences
- The `properties.week_starts_on` column (migration `000016_property_week_starts_on`, default `'monday'`, permitted values `'monday'` | `'sunday'`) is surfaced on `propertyDTO` as `week_starts_on` and patched via `PATCH /api/properties/{id}`. The Analytics UI rotates day-of-week rows (Performance → DOW occupancy, Demand → ADR by DOW) so the first plotted bar matches this preference. Gap-night and orphan-midweek tables also localise their weekday context off the same value. The backend continues to emit raw dow indices `0..6` (0 = Sunday, JS/Go convention); rotation is a frontend concern.

### Polish pass (2026-04-24 — `spec/PMS_06_Analytics_Fix_Plan.md`)
- Outlook → unsold-nights are collapsed into date ranges (adjacent single-night gaps sharing the same next-guest merge into one row) and the "New bookings (7 days)" card is removed.
- Performance → Money → Occupancy → Cleaning visual hierarchy; operating cashflow tiles render before revenue KPIs; net-per-stay is a horizontal bar chart that hides stays whose gross/commission/fees are all zero; monthly trend suppresses leading zero-data months; seasonality heatmap adds an ISO-week-number axis strip and a 0–100 % colour legend.
- Demand → lead-time / LOS / cancellation / ADR-by-DOW render with friendly labels ("0–3 days", "4–5 nights", "Mon"), gap-night and orphan-midweek rows display the preceding-checkout date, the following-check-in date and all three weekdays, and the returning-guests card shows an inline Top-5 table before the "Show all" dialog.
- A collapsible **glossary** defines ADR, RevPAR, occupancy rate, effective take rate, confirmed vs estimated revenue, booking pace, gap night, orphan midweek, lead time and LOS.
- Date pickers on Performance and Demand reuse the Finance module's `.toolbar` + `.month-control` pattern (native `<input type="date">`); tab buttons retain an always-visible `#f1f5f9` background so inactive tabs remain discoverable.

### Suggested Database Entities
**No new tables in v1.** All metrics are live-computed from existing schema:
- `occupancies`, `occupancy_sync_runs` — nights, occupancy, lead time, cancellations, pace, gap nights.
- `finance_booking_payouts` — ADR, RevPAR, gross/net/commission/fees, returning-guest name source.
- `finance_transactions`, `cleaning_monthly_summaries` — net-per-stay cleaning allocation, cost-per-night roll-up.

**Optional v2 addition (flagged, do not build in v1):** `analytics_snapshots(property_id, metric_code, period_key, value_cents_or_ratio, computed_at)` — add only if p95 latency on any endpoint exceeds 500 ms on realistic data.

### Frontend Screens
- `AnalyticsView.vue` with three child components: `AnalyticsOutlookTab.vue`, `AnalyticsPerformanceTab.vue`, `AnalyticsDemandTab.vue`.
- `AnalyticsFreshnessBar.vue` rendered persistently above the tab nav.
- Shared chart primitives go in `frontend/src/components/charts/` — zero third-party chart deps; reuse the SVG-based bar/heatmap patterns already established in `CleaningView.vue`.
- Router entry `/analytics` gated on `hasModule('analytics', 'read')`.
- Left-nav entry "Analytics" appears only when the store reports analytics permission.

### Test Focus
- Cohorting: a March payout for a February stay lands in February revenue.
- Occupancy denominators: 28-day February and DST month boundaries compute correctly (stays stored in UTC; period arithmetic uses property timezone).
- Cancelled stays excluded from performance metrics, included in cancellation stats.
- Forward-revenue split: `confirmed + estimated` equals the total tile value.
- Pace-vs-LY "as-of" reconstruction produces monotonic cumulative curves.
- Returning-guest normalization handles diacritics (`Novák` == `novak`), collapses whitespace, rejects ≤5-character names.
- Gap-night detection returns **zero** for same-day checkout/check-in, **one** for a single empty night between two active stays.
- Freshness thresholds (45d / 75d) render the correct banner colour.
- YoY gracefully disables when <13 months of data exist.
- Permission enforcement: user without `analytics` permission receives 403 on every route.

### Implementation Plan for the AI Developer Agent

The plan is phased so each milestone ships a compilable, testable slice. Follow strictly in order — later phases depend on earlier ones. Each milestone has a definition-of-done; do not proceed until tests pass and `go vet ./... && go build ./...` is clean.

**Ground rules**
- No new Go dependencies. Use stdlib (`database/sql`, `time`, `sort`, `math`) only.
- No new npm dependencies. Charts are hand-rolled SVG or CSS-grid, matching the `CleaningView.vue` style.
- All SQL goes through `backend/internal/store/`. Never run SQL from handlers. Every new store method takes `ctx context.Context` first.
- Monetary fields stay in **cents** end-to-end; only format at the render layer (reuse the existing `eur()` helper on the frontend).
- Dates at period boundaries are computed in the **property timezone** (`properties.timezone`), then converted to UTC for the range predicate — the helper `parseMonthInPropertyTZ` in `cleaning_handlers.go` shows the pattern.
- All handlers route through `s.requirePropertyModuleAccess(w, r, permissions.Analytics, permissions.LevelRead)`; add `Analytics` to `backend/internal/permissions/permissions.go` if it does not already exist.
- Follow the JSON response style of existing handlers: struct-typed responses with explicit `json:"..."` tags, `WriteJSON` for success, `WriteError` for failures.
- The frontend must use the shared `api()` helper and must never embed `property_id` in query strings when the URL path already carries it.

**Milestone A0 — Permission & routing scaffold** *(half-day slice, unblocks everything)*
1. Add `Analytics permissions.Module = "analytics"` in `backend/internal/permissions/permissions.go` alongside the existing modules; extend any permissions test that enumerates all modules.
2. Register an empty handler group under `/api/properties/{id}/analytics` in `backend/internal/api/server.go`, returning 501 from a single placeholder route, to lock the URL shape.
3. Add `analytics` to the frontend permission catalogue (search `hasModule` / `LevelRead` enumerations in `frontend/src/stores/`), and add a disabled left-nav entry behind the permission.
- **Done when:** `go test ./...` green; frontend compiles; a logged-in user *with* analytics permission sees the nav entry, without permission does not.

**Milestone A1 — Freshness endpoint & bar** *(smallest end-to-end slice to prove the stack)*
1. `store.GetAnalyticsFreshness(ctx, propertyID)` returning `{ LastICSSyncAt *time.Time; LastPayoutDate *time.Time; UnmatchedPayoutsCount int }` — queries `MAX(completed_at)` from `occupancy_sync_runs` where status is a success value, `MAX(payout_date)` from `finance_booking_payouts`, and a count of `finance_booking_payouts` rows with `occupancy_id IS NULL`.
2. Handler `getAnalyticsFreshness` computes `staleness_level`: `ok` if `last_payout_date >= today − 45d`, `warn` between 45d and 75d, `stale` beyond 75d or if null.
3. `AnalyticsFreshnessBar.vue` fetches `/api/properties/{id}/analytics/freshness` on mount + watches `pid`, renders sync time, payout date, unmatched count, and a colour banner.
4. `AnalyticsView.vue` renders only the freshness bar and placeholder tabs.
- **Done when:** Go test asserts the 45/75-day colour thresholds; Vitest asserts `AnalyticsFreshnessBar` re-fetches on `pid` change and emits the correct staleness class.

**Milestone A2 — Outlook tab**
1. Store methods (all scoped to one property, all returning cent-precise integers):
   - `ListActiveOccupanciesInDateRange(ctx, pid, fromUTC, toUTC)` — used as the building block for nights-sold windows.
   - `SumPayoutGrossNetForStays(ctx, pid, fromArrivalDate, toArrivalDate)` — returns `(grossCents, netCents, matchedStayIDs []int64)`.
   - `TrailingADR(ctx, pid, asOf time.Time)` — trailing 12 months, denominator = matched nights, numerator = gross cents; returns 0 if <30 matched nights.
   - `ListUnsoldNightsWithContext(ctx, pid, fromUTC, toUTC)` — returns per-night rows with the IDs/labels of the stay before and after.
   - `NewBookingsByDay(ctx, pid, sinceUTC)` — groups `occupancies` by `DATE(imported_at)` over the window.
2. Handler `getAnalyticsOutlook` composes three sub-windows (30/60/90 days from today in property TZ) and a 90-day pacing series (cumulative nights-sold as of today).
3. `AnalyticsOutlookTab.vue`: three KPI tiles × two rows (nights then revenue), pacing chart (reuse SVG bar style), unsold-nights table, new-bookings sparkline.
4. Confirmed/estimated revenue split must be serialised as `{ confirmed_cents, estimated_cents, total_cents }` so the UI cannot mis-add them.
- **Done when:** Go tests cover (a) forward-revenue split invariant (`confirmed + estimated == total`), (b) empty-dataset case (all zeros, no nil deref), (c) ADR estimation falls back to 0 when <30 matched nights; Vitest asserts tile rendering and zero-state copy.

**Milestone A3 — Performance tab** *(also absorbs the yearly roll-ups migrated out of Finance and Cleaning — see `PMS_05` “Metrics migrated from other modules”.)*
1. Store: `ListMonthlyOccupancyAndADR(ctx, pid, fromMonth, toMonth)` → array of `{ month, nightsSold, availableNights, grossCents, netCents, commissionCents, paymentFeesCents, matchedNights }`.
2. Store: `ListWeeklyOccupancy(ctx, pid, fromYear, toYear)` → ISO-week × year matrix for the seasonality heatmap (cell value = occupancy rate).
3. Store: `ListDOWOccupancy(ctx, pid, fromUTC, toUTC)` → 7 rows with nights sold and available nights.
4. Store: `ListCancellationsInArrivalWindow(ctx, pid, fromUTC, toUTC)` → per-stay `{ startAt, cancelledAt, leadDays }`, where `cancelledAt` is the `last_synced_at` of the final status row that transitioned to `cancelled`/`deleted_from_source`.
5. Store: `ListNetPerStay(ctx, pid, fromUTC, toUTC)` → per-stay `{ startAt, endAt, grossCents, commissionCents, paymentFeeCents, cleaningAllocatedCents, netCents }`. Cleaning allocation reuses `cleaning_daily_logs` counted days × `cleaner_fee_history` effective on the checkout date.
6. Handler `getAnalyticsPerformance` fans out these reads in parallel with `errgroup`-style goroutines (or sequentially if simpler; profile later). Adds a prior-year block when `yoy=true`.
7. Store: `ListYearlyCleaningCounts(ctx, pid, year)` — the existing 12-bars-per-year cleaning count series. Implementation may call the existing cleaning-store method the old `/cleaning/yearly-stats` handler used; the new handler just re-exposes it under the analytics URL.
8. Store: `YearlyFinanceRollup(ctx, pid, year)` — returns `{ incomingCents, outgoingCents, netCents }`. Reuse the same SQL that powers the deprecated `yearly_*_cents` fields on `/finance/summary`; do not re-derive the math.
9. Handler extends the response with a top-level `{ yearly_cleaning: { year, series[12] }, yearly_finance: { year, incoming_cents, outgoing_cents, net_cents } }` block keyed off an optional `year=YYYY` query parameter (defaulting to current calendar year in the property timezone).
10. `AnalyticsPerformanceTab.vue` composed of: KPI grid, dual-axis monthly trend (two overlaid SVG polylines), seasonality heatmap, DOW heatmap, cancellation panel, net-per-stay table, **plus the migrated “Cleaning activity” 12-bar chart and the “Operating cashflow (year)” KPI trio**. Port the SVG bar markup verbatim from `CleaningView.vue` and the tile markup from `FinanceView.vue` so the visuals stay identical.
11. Frontend cleanup: remove the Yearly Cleaning Stats card from `CleaningView.vue` and the Yearly Overview tiles from `FinanceView.vue` (including unused refs, watchers, and CSS classes). Leave the deprecated backend endpoints/fields in place for now — they are deleted in A5.
- **Done when:** Go tests verify (a) cohort-by-arrival-date rule via a fixture with a March payout for a February stay, (b) cancelled stays excluded from occupancy but counted in cancellation rate, (c) effective take-rate equals `(commission+fees)/gross` exactly for a seeded month, (d) the analytics yearly cleaning series equals what the legacy `/cleaning/yearly-stats` handler returned for the same year, (e) the analytics yearly finance rollup equals the legacy `yearly_*_cents` summary fields for the same year. Vitest asserts table sort, YoY-disabled state, and that the migrated panels render in the Performance tab while their originals no longer render on the Cleaning / Finance pages.

**Milestone A4 — Demand & Pricing tab + Pace endpoint**
1. Store: `ListLeadTimeBuckets(ctx, pid, fromUTC, toUTC)` — buckets 0–3 / 4–14 / 15–45 / 46–90 / 91+ on `DATE(start_at) − DATE(imported_at)`.
2. Store: `ListLengthOfStayBuckets(ctx, pid, fromUTC, toUTC)` — buckets 1 / 2 / 3 / 4–5 / 6–7 / 8–14 / 15+ nights.
3. Store: `ADRByDimension(ctx, pid, fromUTC, toUTC, dim)` where `dim` ∈ `{month, dow, lead_bucket}` — returns bucket → `{ grossCents, matchedNights }` and let the handler compute the ratio.
4. Store: `ListGapNights(ctx, pid, fromUTC, toUTC)` — single available nights sandwiched between two active stays; same-day checkout/check-in yields **no** gap.
5. Store: `ListOrphanMidweek(ctx, pid, fromUTC, toUTC)` — 1–2 consecutive Mon–Thu unsold nights with booked weekends on both sides.
6. Store: `PaceCurveForWindow(ctx, pid, windowStart, windowEnd)` — for each integer `T` in `[0, 180]`, count `occupancies` whose arrival falls in the window AND whose `imported_at <= windowStart - T days`. Return the same curve for `(windowStart - 1y, windowEnd - 1y)` for the LY overlay.
7. Store: `ListReturningGuests(ctx, pid, fromUTC, toUTC)` — groups active payout-matched stays by `normalize(guest_name)`, filters names ≥6 chars, returns count & rate; **and** a paginated flat list for the drill-down endpoint.
8. `AnalyticsDemandTab.vue` renders histograms, three ADR bar charts, gap/orphan tables, returning-guests summary with a "View drill-down" dialog that paginates via `/analytics/returning-guests`.
- **Done when:** Go tests verify (a) normalization rules (`Novák` → `novak`, rejects names <6 chars), (b) gap-night boundary cases (same-day = 0 gaps, single empty night = 1 gap), (c) pace curve is monotonic non-decreasing, (d) LY pace omits cleanly when <13 months of history exist.

**Milestone A5 — Polish, observability, spec close-out**
1. Add the `Analytics` module row to §7.5 permission matrix in `PMS_01_Architecture_and_Global_Spec.md` if missing.
2. Flip the Analytics item in `PMS_03_Implementation_Checklists.md` from ⬜ to ✅ with a short delivery summary referencing this section.
3. Retire the deprecated yearly surfaces: remove `GET /api/properties/{id}/cleaning/yearly-stats` (handler + store method + tests) and drop the `yearly_incoming_cents` / `yearly_outgoing_cents` / `yearly_net_cents` fields from the `/finance/summary` response once no frontend reads them. Update the existing finance and cleaning tests that asserted on these fields.
3. Observability: no special metric wiring required — the existing `pms_http_requests_total{method,status}` and `pms_http_request_duration_seconds{method}` counters from `backend/internal/metrics` already cover these routes. Optionally add a Grafana annotation "analytics live".
4. Performance guardrails: add `EXPLAIN QUERY PLAN` checks (as comments in store tests) for every query that scans > occupancies × 30 and confirm SQLite picks an index on `(property_id, start_at)`, `(property_id, imported_at)`, and `(property_id, payout_date)`. If not, add migration `000017_analytics_indexes.up.sql` / `.down.sql` creating exactly those composite indexes.
5. E2E smoke: a single Playwright scenario logging in as an owner, navigating to `/analytics`, asserting the freshness bar + Outlook KPI tiles render without console errors (deferred to the Playwright epic in §11 of `PMS_03`, not blocking for v1).

**Acceptance checklist for the whole module**
- [ ] `go test ./...` green with ≥80% coverage on new `store/analytics_*.go` files.
- [ ] `npm test` green; Vitest covers the freshness colour logic, the confirmed/estimated split, and the returning-guest drill-down pager.
- [ ] Every analytics endpoint rejects a user without `analytics:read` with 403, and a user on a non-owned property with 404/403 consistent with other modules.
- [ ] Yearly Cleaning Stats no longer render on the Cleaning page; Yearly Overview tiles no longer render on the Finance page; both now render inside the Performance tab and return the same numbers the legacy endpoints returned for the same year.
- [ ] The `GET /cleaning/yearly-stats` endpoint and the `yearly_*_cents` fields on `/finance/summary` are either deleted (preferred) or explicitly marked deprecated with an ADR note.
- [ ] No new Go or npm dependencies added (`go.mod` and `package.json` diffs only affect `require` blocks if strictly necessary — they shouldn't be).
- [ ] `data/pms.db` realistic dataset (1,000+ occupancies, 500+ payouts) loads each endpoint in <500 ms on the dev laptop; attach `EXPLAIN QUERY PLAN` notes if any query is slower.
- [ ] All revenue widgets show the "revenue data through: <date>" disclaimer; banner colour logic matches the 45/75-day rule.
- [ ] The Deferred-for-v2 `analytics_snapshots` table is **not** created.

## 10. Deferred Future Module: Direct Google Calendar Sync

### Status
This is explicitly out of scope for v1. Implement the occupancy JSON endpoint first and let n8n handle Google Calendar synchronization.

### Why
- lower complexity
- no OAuth product work in v1
- easier debugging
- keeps occupancy as the system of record

### If Implemented Later
Need:
- Google account authorization flow
- calendar mapping per property
- event reconciliation rules
- token refresh handling
- sync logs and retries

## Delivery Guidance for the AI Coding Agent
- Start from the global platform and occupancy module first.
- Treat occupancy as the central relation for automations.
- Build all integrations with explicit sync logs and statuses.
- Prefer deterministic, traceable workflows over hidden automation.
