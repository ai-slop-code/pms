# PMS v1.1 — Implementation Plan

## Scope
Incremental release on top of v1.0. Tracks all work items targeted for the
v1.1 milestone. Each task lists the user-facing outcome, the surfaces that
must change (backend, frontend, schema, spec), and acceptance criteria.

---

## Tasks

### 1. Invoice generation — language, supplier address, property VAT ID  ✅

> **Implementation note (2026-04-29):** Most of the backend, schema and PDF
> plumbing was already in place from an earlier iteration:
>
> - The `invoices.language` column, the `language` field on the request /
>   response DTOs, the SK/EN label dictionary inside `invoicepdf`, and
>   locale-aware money formatting all existed.
> - `vat_id`, `ico`, `dic` already live on `property_profiles` (not on
>   `properties`), and the property settings API already exposes them in
>   `propertySettingsProfileDTO`.
> - `defaultInvoiceSupplier` already snapshots the property address as a
>   fallback when the profile billing address is empty.
>
> The only real gap was the **frontend property settings form**, which did
> not expose the tax/billing fields, so a user could not actually configure
> a VAT ID. That gap is now closed.

**User-facing outcome**

- Operator can choose the invoice language (Slovak or English) when
  generating or regenerating an invoice PDF. The chosen language is
  persisted on the invoice so re-rendered versions stay consistent.
- The "Dodávateľ" / "Supplier" block on the rendered PDF shows the property
  address configured on the property, in addition to the existing supplier
  identity fields.
- A property can have its own VAT ID configured. The VAT ID is shown in the
  supplier block of the invoice PDF when present.

**Backend** (already in place — no changes required)

- `vat_id`, `ico`, `dic`, `billing_address`, `city`, `postal_code`,
  `country` exist on `property_profiles` and round-trip through the
  `/api/properties/{id}/settings` endpoints (see
  `propertySettingsProfileDTO` and `Store.UpdatePropertyProfile`).
- The supplier snapshot stored on each invoice (`supplier_snapshot_json`)
  already captures all of the above. `defaultInvoiceSupplier` falls back
  to the property's own `address_line1` / `city` / `postal_code` /
  `country` when the profile billing address is blank — so the invoice
  shows the property address whenever the user hasn't entered a separate
  billing address.
- `invoices.language` is persisted, validated against `{sk, en}` and
  defaults to `sk` for legacy rows. `language` is accepted on POST /
  PATCH and re-used on regeneration.

**Frontend** (this release)

- `PropertyDetailView` profile tab now has a **Billing & tax** section
  with inputs for `legal_owner_name`, `billing_name`, `billing_address`,
  `billing` city / postal code / country, `IČO`, `DIČ`, `VAT ID (IČ DPH)`.
  The form posts the new keys via the existing `/settings` PATCH.
- The invoice list shows the document language as a small badge on each
  row so it is obvious whether a SK or EN PDF was generated.
- Invoice editor already had a language selector defaulting to the
  property's `default_language`; no changes needed.

**`invoicepdf` package** (already in place — no changes required)

- Language-aware label dictionary covers headings, party titles, detail
  rows, info card titles, payment-note copy and the footer.
- Money formatting switches the decimal separator (`,` for SK, `.` for
  EN). The supplier card prints `ICO`, `DIC` and `VAT ID` lines whenever
  set on the snapshot; the address is rendered from the snapshot fields.

**Spec updates**

- `spec/openapi.yaml` does not currently enumerate the property profile
  fields or the invoice schema in detail, so no diff there. The
  description above is the canonical reference for v1.1.

**Acceptance criteria**

- Generating a new invoice with `language=en` produces a PDF with English
  labels and English-formatted dates and amounts.
- Regenerating the same invoice without specifying a language re-uses the
  language stored at creation time.
- The supplier block on every newly generated invoice shows the property's
  address lines and (when set) VAT ID.
- Invoices issued before the v1.1 deploy continue to render in Slovak with
  the previously snapshotted supplier data; no historical PDF is altered.
- Backend unit tests cover: language default resolution, snapshot of
  property address + VAT ID, label localisation in `invoicepdf`.

---

### 2. Occupancy — manually classify a Booking.com block as "closed" or "externally sold"  ✅

> **Implementation note (2026-05-08):** Backend (migration 000019, three
> closure endpoints, analytics/Nuki/cleaning predicates, JSON export
> categories), analytics aggregation (`BookableNightsInRange` wired into
> monthly/weekly/DOW occupancy and the 30/60/90 outlook KPIs), and
> frontend surfaces (stay-list actions, calendar visual states for closed
> and external-sale nights, closed-nights KPI, clickable calendar cells
> with a day-actions popup) are all in place. Spec `PMS_02` and `PMS_04`
> were updated to match. `openapi.yaml` was intentionally not extended:
> it does not yet cover the occupancy module, so closure-only coverage
> would be inconsistent with the rest of the file.

**Problem**

When an operator closes a property for a night inside the Booking.com
extranet, Booking publishes that block in the same iCalendar feed as a
regular reservation. PMS imports it and the occupancy module shows it
as a booked night. In reality, that block can mean one of two very
different things:

1. **The night is genuinely off the market** — owner stay, maintenance,
   soft-block to avoid sales. There is no guest and no payout. Counting
   it toward occupancy inflates the rate and distorts ADR / RevPAR.
2. **The night was sold through a different channel** — direct booking
   over the phone, repeat guest paying in cash, Airbnb / other OTA where
   PMS does not yet ingest payouts. The operator blocked the date on
   Booking only to prevent a double-booking. Such a night really *is*
   sold and *should* count toward occupancy and revenue, but today PMS
   has no way to capture the net income because there is no Booking
   reservation row and no statement line.

We cannot fix the upstream data: Booking's ICS output does not
distinguish either case from a reservation. The fix has to live inside
PMS — let the operator pick one of two labels for the imported block:
**Closed** or **Externally sold** (with a net amount).

**User-facing outcome**

- In the Occupancy module, an operator can mark any imported stay (or
  a date range) as either:
  - **Closed** — with a reason (free-text + optional category such as
    `owner_stay`, `maintenance`, `soft_block`, `other`). Closed nights
    are excluded from both the numerator and the denominator of the
    occupancy ratio.
  - **Externally sold** — with a **net amount** (in the property
    currency), an optional **channel** label (`airbnb`, `direct`,
    `walk_in`, `other`) and an optional free-text note. Externally-sold
    nights count as **sold** *and* as **available**, and the entered
    net amount feeds gross revenue, ADR and RevPAR.
- The calendar visually distinguishes the three states (regular
  Booking-paid, closed = neutral grey, externally sold = accent colour
  with a "€" / channel badge) and includes a small legend.
- Analytics (Performance and Demand tabs) recompute occupancy rate,
  ADR, RevPAR, and the seasonality / DOW heatmaps using the new
  definitions:
  - `nights sold` = active stays + externally-sold nights, excluding
    closed nights.
  - `available nights` = calendar nights − closed nights.
  - `gross revenue` for ADR/RevPAR includes the per-stay
    `external_net_amount` for externally-sold rows alongside matched
    Booking payouts.
- Either label can be removed at any time; the underlying ICS-derived
  stay row is preserved (we never delete imported data).
- Both labels are visible in audit log and survive ICS resyncs.

**Backend**

- New migration adds to `occupancies`:
  - `closure_state TEXT NULL` constrained to
    `('closed', 'external_sale')` or NULL.
  - `closure_reason TEXT`, `closure_category TEXT`,
    `closed_by_user_id INTEGER`, `closed_at TEXT`.
  - `external_net_amount NUMERIC NULL`, `external_currency TEXT NULL`,
    `external_channel TEXT NULL`. All three must be NULL unless
    `closure_state = 'external_sale'` (CHECK constraint). Amount is
    stored as the same numeric type used elsewhere for money in
    `occupancies` / payouts (cents-precision string or `REAL`,
    matching the existing convention — confirm at implementation
    time).
- Endpoints (Occupancy module):
  - `POST /api/properties/{id}/occupancies/{occupancyId}/close`
    `{ reason, category }`
  - `POST /api/properties/{id}/occupancies/{occupancyId}/external-sale`
    `{ net_amount, currency?, channel?, reason? }` —
    `net_amount` is required and must be `>= 0`; `currency` defaults
    to the property's default currency; `channel` is one of
    `{airbnb, direct, walk_in, other}`.
  - `POST /api/properties/{id}/occupancies/{occupancyId}/reopen`
    clears whichever label is set (closed *or* external sale) and
    nulls the associated columns.
- A stay can hold at most one of the two labels at a time; switching
  from one to the other goes through `reopen` first (or the handler
  performs an in-place transition and audits both events).
- Subsequent ICS resyncs **do not** touch any of the new columns. If
  the upstream event disappears (becomes `deleted_from_source`), the
  label is preserved on the historical row so analytics for past
  periods stays stable.
- Analytics SQL:
  - `analyticsBookableStatus` (replaces `analyticsActiveStatus`):
    `status IN ('active','updated') AND closure_state IS DISTINCT FROM 'closed'`.
  - `nights_sold` predicate adds: rows with
    `closure_state = 'external_sale'` are counted as sold even when
    they would otherwise be filtered out (they always have a real
    Booking-imported stay row, so the date range is valid).
  - `gross_revenue` aggregation adds a `COALESCE(external_net_amount, 0)`
    branch for externally-sold rows, so payout matching is unchanged
    for normal Booking stays. The amount is **prorated per night**
    (`external_net_amount / nights_in_stay`) when a stay only
    partially overlaps the analytics range — same proration rule
    already used for matched Booking payouts (BA §3.8).
  - `AvailableNightsInRange(stays, fromDate, toDate)` subtracts
    `closure_state = 'closed'` nights only.
- **Nuki code generation** (BA §3.4): the existing eligibility
  predicate (`status IN ('active','updated')`) gains
  `AND closure_state IS NULL` so neither `closed` nor `external_sale`
  rows produce a keypad code or guest message. Operators using a
  third-party channel issue codes through that channel's flow.
- **Cleaning trigger** (BA §3.6): the existing checkout-driven
  cleaning reconcile excludes `closure_state = 'closed'` rows but
  **keeps** `closure_state = 'external_sale'` rows (a real guest
  checks out, so a turnover clean is needed). No new code path —
  the change is a single predicate update in the cleaning trigger
  query.
- **iCal export** (BA §3.5):
  `/api/properties/{id}/occupancy-export` continues to emit closed
  *and* externally-sold nights as `STATUS:CONFIRMED`, tagged
  `CATEGORIES:PMS-CLOSURE` and `CATEGORIES:PMS-EXTERNAL-SALE`
  respectively. Amount, channel and reason are **never** included
  in the public feed.

**Frontend**

- Occupancy stay list / calendar: a single overflow menu per stay with
  three actions — **Mark closed**, **Mark as externally sold**,
  **Reopen** (the third only enabled when one of the labels is set).
- "Mark as externally sold" opens a small dialog with: net amount
  (required, currency-formatted), channel (select), note (textarea).
- Closed stays render in neutral grey with the closure category badge;
  externally-sold stays render in an accent colour with a "€ <amount>"
  chip and the channel label.
- Optional date-range tools (Phase 2 in the BA spec): same dialog can
  be opened from a calendar drag-selection to insert a synthetic
  closure or external-sale row when no upstream stay exists.
- Analytics tab labels its KPIs as **Occupancy rate (excl. closed)**
  with a tooltip explaining how externally-sold nights are folded in,
  and exposes a small chip with the count of externally-sold nights
  in the active range.

**Spec updates**

- Update `PMS_02_Module_Specifications.md` (Occupancy + Analytics
  modules) and `spec/openapi.yaml` to add the new fields and
  endpoints.
- Update `PMS_04_Analytics_Data_Inventory.md` to note that the
  canonical `nights_sold` metric subtracts closed nights and adds
  externally-sold nights, and that `gross_revenue` includes the
  `external_net_amount` column.

**Acceptance criteria**

- Marking an imported stay as closed *or* externally sold and
  re-running occupancy sync preserves the label and (for external
  sales) the entered amount.
- Performance tab: closing a stay reduces both numerator and
  denominator of occupancy and removes its (zero) revenue
  contribution; marking a stay externally sold leaves occupancy at
  the pre-fix value and increases ADR / RevPAR by the entered net
  amount divided by the corresponding nights / available nights.
  Reopening reverts both, in tests with a deterministic in-memory
  store.
- Heatmap and DOW occupancy use the same definition (closed
  excluded, external sales included).
- Switching a stay from closed → externally sold → reopened never
  loses the externally-entered amount until the explicit reopen
  step.
- Audit log records `occupancy_close`, `occupancy_mark_external_sale`,
  and `occupancy_reopen` actions with the actor, reason and (for
  external sales) the entered amount, currency and channel.
- A negative or non-numeric `net_amount` is rejected with HTTP 400
  and an i18n error message.
- Marking a stay closed suppresses Nuki code generation and the
  cleaning trigger for that stay; marking it externally sold
  suppresses Nuki only and **keeps** the cleaning trigger.
- The iCal export contains both labels with the
  `PMS-CLOSURE` / `PMS-EXTERNAL-SALE` category tags, and never
  leaks the entered amount, channel or reason.

---

### 3. Analytics — guest check-in time heatmap (from Nuki access log)  ✅

> **Implementation note (2026-04-29):** The original spec assumed
> `nuki_event_logs` carries unlock events with an `auth_id` column.
> The actual schema only stores operational lifecycle messages
> (`event_type`, `message`, `payload_json`) — there is no
> `auth_id` and no unlock log persisted in the DB. The implementation
> therefore mirrors the cleaning reconciler exactly:
>
> - A new `nuki_guest_daily_entries` table (migration 000020) keyed
>   by `(property_id, occupancy_id, day_date)` stores the earliest
>   guest unlock per stay per day.
> - A new `ReconcileGuestDailyEntries` service method live-fetches
>   the Smartlock log via `Client.ListSmartlockEvents`, partitions
>   guest events from cleaner events using
>   `cleanerAuthAliases`, and resolves each guest event to its
>   owning occupancy through `nuki_access_codes.external_nuki_id`.
> - A new scheduler job (`guest_reconcile`) reuses the existing
>   `cleaning_reconcile` interval and runs immediately after the
>   cleaning reconciler.
> - The new endpoint
>   `GET /api/properties/{id}/analytics/guest-checkin-heatmap`
>   reads the persisted rows, filters to the requested range, and
>   returns the 24-bucket histogram. Closed-stay rows are excluded
>   in SQL; externally-sold rows remain (PMS_14 §4 rule).

### 3. Analytics — guest check-in time heatmap (from Nuki access log)

**Problem**

The Cleaning module already derives a **time-of-day arrival heatmap**
for the cleaner from `nuki_event_logs` (one bucket per hour of the
day, first entry of the day only, displayed as a 0–23 bar chart). The
operator gets the same kind of insight for guests — at what time do
they actually walk in? — only by manually scrolling the Nuki access
log. Knowing the guest arrival distribution helps decide check-in
window copy, late-arrival messaging, and whether the cleaner's
default schedule still fits the real handover window.

The data is already there: every guest is issued a per-stay Nuki
keypad code (`nuki_keypad_codes` is keyed to an occupancy), and every
unlock event is captured in `nuki_event_logs` with an `auth_id`.
Filtering events whose `auth_id` is a guest code (i.e. **not** the
configured cleaner auth id and any aliases) yields the guest entries.

**User-facing outcome**

- Analytics gains a **Guest check-in time** card (Performance tab,
  next to or under the existing seasonality/DOW visuals) that shows a
  24-bucket bar chart of the hour-of-day at which guests first
  entered the apartment.
- Default range follows the rest of the Analytics tab (selected
  month, with the existing range picker honoured).
- Hover/tooltip shows the bucket count and a small example list
  ("e.g. 14:xx — 7 check-ins") consistent with the cleaning heatmap
  styling.
- An empty-state message appears when there is no Nuki data or no
  guest entries in the range, identical in tone to the cleaning
  heatmap empty-state.

**Rules (mirrors the cleaning heatmap, applied to guests)**

- One bucket per hour `0..23`, in the property timezone.
- For each `(stay, calendar day)` pair, only the **first guest
  unlock** counts. Subsequent same-day re-entries by the same guest
  do not double-count.
- An event qualifies as a guest entry when its `auth_id` matches a
  `nuki_keypad_codes` row whose `occupancy_id` resolves to a
  non-cancelled, non-closed stay. Events whose `auth_id` matches the
  property's `cleaner_nuki_auth_id` (or any cleaner alias) are
  excluded; so are events with no resolvable code (manual unlocks,
  master keys).
- Externally-sold and closed nights (see task 2) are treated the
  same way as today: closed-stay codes don't exist; externally-sold
  rows have a real stay row but no Booking-issued keypad code unless
  the operator generated one — so they only contribute when a code
  was actually used.
- Successful unlock events only (existing predicate used by the
  cleaning reconcile path).

**Backend**

- New store helper, mirroring `ListCleaningDailyLogsForMonth`:
  `ListGuestFirstEntriesInRange(ctx, propertyID, fromDate, toDate)`
  returns `[]struct{ StayID int64; DayDate string; FirstEntryAt time.Time }`.
  Implementation: join `nuki_event_logs` to `nuki_keypad_codes` on
  `auth_id`, filter to events inside `[fromDate, toDate)` in the
  property timezone, group by `(occupancy_id, day_date)`, take
  `MIN(event_at)`. The query lives in
  `backend/internal/store/nuki.go` (new file or alongside existing
  Nuki store helpers).
- New endpoint:
  `GET /api/properties/{id}/analytics/guest-checkin-heatmap?from=YYYY-MM-DD&to=YYYY-MM-DD`
  → `{ from, to, buckets: [{ hour, count }, ...24] }`. Permission:
  `nuki_access:read` **and** `analytics:read` (matches the existing
  Analytics endpoint authorisation pattern; if Analytics handlers
  use a single `analytics:read` gate, follow that convention).
- Response shape mirrors `cleaningHeatmapResponse` for frontend
  symmetry: `{ from, to, buckets: [{ hour: int, count: int }] }`,
  always 24 buckets, zero-filled.
- Range validation: `to` exclusive, max range 366 days, both
  required; default to the current month in property TZ when
  omitted (same defaulting logic as the existing analytics handler).

**Frontend**

- Reuse `CleaningHeatmap.vue` (or extract a shared
  `HourOfDayHeatmap.vue` primitive in the same `components/charts/`
  folder) so both consumers share the styling, accessibility
  attributes (`role="img"`, `<title>`/`<desc>`), and tooltip
  behaviour.
- Add a `GuestCheckinHeatmap` card to the Analytics → Performance
  view, fed from a new composable
  `useGuestCheckinHeatmap(propertyId, range)` that calls the new
  endpoint.
- Card title localised (sk/en): "Príchody hostí podľa hodiny" /
  "Guest check-in times". Subtitle clarifies the source: "Based on
  Nuki keypad unlocks, first entry per stay per day."
- Loading / empty / error states match the existing analytics
  cards.

**Spec updates**

- Update `PMS_02_Module_Specifications.md` (Analytics + Nuki
  modules) with the new endpoint and the cleaner/guest auth-id
  partition rule.
- Update `PMS_04_Analytics_Data_Inventory.md`: add **Guest check-in
  hour-of-day** to the Nuki section's derived metrics list and note
  that it shares the data path with the cleaning heatmap.
- Add the endpoint to `spec/openapi.yaml`.

**Acceptance criteria**

- With seeded `nuki_event_logs` containing two guest unlocks on the
  same day for the same stay, only the earlier event contributes to
  its hour bucket.
- Cleaner unlocks for the same period do not appear in the guest
  heatmap and vice versa (cross-checked by the existing cleaning
  heatmap test fixtures).
- Range defaults to the current month in property TZ when query
  parameters are omitted; explicit ranges spanning DST boundaries
  bucket correctly in the property timezone.
- Frontend Analytics tab renders the new card with non-zero data
  for a property that has guest unlocks, and with the empty-state
  copy for a property that has none.
- Permission gate: a user without `nuki_access:read` (or whatever
  the chosen gate is) receives HTTP 403.
- Backend unit test covers the dedup-per-(stay,day) rule and the
  cleaner exclusion.

---

### 4. Finance — Booking.com Statement ingestion (accrual-basis merge)

> **Source spec:** [`spec/PMS_Statement_Ingestion_Spec.md`](PMS_Statement_Ingestion_Spec.md).
> All blocker questions in that document are answered; this task is the
> implementation contract.

**Problem**

Today the Finance module ingests only the **Payout Info** CSV from
Booking.com — a cash-basis view that excludes cancellations, hides the
booking-creation moment, and lacks `Persons`, `Booker name`,
`Commission %`, `Invoice number` and `Hotel id`. The PMS therefore
cannot compute lead time, cancellation rate, persons-mix or commission
trends from authoritative data.

Booking.com publishes a complementary **Statement** CSV (accrual /
commercial) that carries those fields and one row per reservation
regardless of payout status. Merging both files per reservation makes
the PMS lifecycle-aware (created → confirmed → cancelled / paid) and
unlocks the missing analytics, while keeping occupancy / ADR / RevPAR
on the existing cash basis.

**User-facing outcome**

- The Finance upload dialog accepts **either** file through a single
  upload control; file type is auto-detected by header signature
  (Q2). UTF-8 is preferred; cp1252 input is transparently
  re-encoded (Q3).
- The admin still picks the target property on each upload (Q1); on
  the first statement upload for a property the observed `Hotel id`
  is captured into `properties.booking_hotel_id` so future Phase 3
  auto-route is a one-line check (N4).
- Multi-hotel statement files are filtered to the selected
  property's `Hotel id`; the rest are reported as
  `row_count_skipped_other_hotel` in the import summary, with a
  warning banner (N1).
- Per Q15 / N6, the upload follows a **preview-then-commit** flow.
  N6 asks us to first verify what the existing payout flow does and
  match it; if today's payout flow is upload-and-commit, both
  flows are upgraded together to preview-then-commit so parity is
  preserved.
- Cancellations from the statement file are surfaced in Finance,
  the linked occupancy (when one exists from a prior payout) is
  flagged `cancelled` (never deleted) and is excluded entirely from
  occupancy / ADR / RevPAR (Q6, Q13).
- Re-uploading the same file is a silent no-op when row hashes match
  (Q7); a "this exact file was already uploaded on …" hint is
  shown when the file SHA-256 matches a previous import.

**Backend**

- **Migration** (single transactional file):
  - Rename `finance_booking_payouts` → `finance_bookings` (Q8).
  - Add canonical columns:
    `booked_on TIMESTAMPTZ`, `original_amount_cents INTEGER`,
    `commission_pct NUMERIC`, `persons INTEGER`, `rooms INTEGER`,
    `booker_name TEXT`, `guest_request TEXT`, `invoice_number TEXT`,
    `hotel_id TEXT`, `property_label TEXT`, `country TEXT`.
  - Add `source_channel TEXT NOT NULL DEFAULT 'booking_com'` (N3).
  - Add `has_payout_data BOOLEAN`, `has_statement_data BOOLEAN`
    (default false; OR-merged on import).
  - Add `raw_payout_row_json JSONB`, `raw_statement_row_json JSONB`
    (replacing any single `raw_*` column in place today).
  - Add `properties.booking_hotel_id TEXT NULL` (N4).
  - Add `occupancies.finance_booking_id` FK → `finance_bookings(id)`
    `ON DELETE SET NULL` (N12). Backfill from current implicit
    `(property_id, reference_number)` pairing.
  - Unique index `(property_id, source_channel, reference_number)` =
    canonical merge key.
  - Update GDPR delete path to also redact PII keys inside both
    `raw_*_json` blobs (`Guest name`, `Booker name`, `Guest request`,
    `Country`).
- **New tables**:
  - `finance_imports` — one row per upload with the columns listed
    in the source spec (`source_type`, `source_channel`, `hotel_id`,
    `invoice_number`, `period_start/end`, `uploaded_by`,
    `uploaded_at`, `file_sha256`, all `row_count_*` counters)
    (N5, Phase 1).
  - `finance_booking_merges` — merge audit log
    (`booking_id`, `import_id`, `source_type`, `changed_fields_json`,
    `occurred_at`).
- **Parser & merge service** (new package
  `backend/internal/finance/statements/`):
  - Header-signature detection: payout = `Payout date` + `Payout ID`;
    statement = `Booked on` + `Persons` + `Status`. Otherwise reject.
  - Encoding fallback: try UTF-8, fall back to cp1252 → UTF-8 on
    decode failure (Q3).
  - Currency guard: reject any row with `Currency != 'EUR'` with a
    structured error.
  - Sign normalisation: store `commission_cents` and
    `payment_service_fee_cents` as **positive** integers; payout
    parser flips sign on import.
  - Time zone: parse `Booked on` as `Europe/Bratislava`, store as
    UTC `TIMESTAMPTZ`.
  - Merge upsert per row, applying the precedence table from the
    source spec (statement wins for amount / commission / fee /
    dates / guest name; payout wins for `net_cents`; never overwrite
    non-null with NULL).
  - Idempotence: per-row stable hash over canonical fields per
    source; matching hashes count as `unchanged`. File SHA-256 is
    only a UI hint.
  - Multi-hotel filter (N1): rows whose `Hotel id` ≠ the selected
    property's captured `booking_hotel_id` are skipped and counted.
  - `status_bucket` derivation on read: `OK/ok` → `active`,
    `CANCELLED` → `cancelled`, anything else → `other` (Q16, N7).
  - Cancellation reconciliation: when a statement row flips an
    existing booking to `cancelled`, the linked `occupancies` row
    (via `finance_booking_id`) is marked cancelled, never deleted
    (Q6).
- **API**:
  - `POST /api/properties/{id}/finance/imports/preview`
    `multipart/form-data` → returns parsed diff
    (`inserts`, `updates`, `unchanged`, `skipped_other_hotel`,
    `rejected[]` with row index + reason); response includes a
    short-lived `preview_token` (server-side cached parsed result
    keyed by SHA-256, TTL 15 min).
  - `POST /api/properties/{id}/finance/imports/commit`
    `{ preview_token }` → applies the merge, writes `finance_imports`
    + `finance_booking_merges`, returns the import summary.
  - `GET /api/properties/{id}/finance/imports?limit&cursor` → audit
    list.
  - Permissions: `finance:write` for both endpoints (Q14, same role
    as today's payout upload).

**Frontend**

- Replace the two existing payout/statement upload affordances (or
  the single payout-only one) with a **single** "Upload Booking.com
  CSV" button on the Finance page. Drag-drop is **out of scope** for
  v1.1 (Q17 says CSV upload is the long-term solution; UX
  investment is justified but kept proportional).
- Preview dialog (N6) renders four lists from the preview response:
  inserts / updates (with field-level old → new), unchanged (count
  only), skipped (other hotel) and rejected. A primary "Commit"
  button calls the commit endpoint with the `preview_token`.
- Booking.com booker name (new PII field) is surfaced **only** on
  the booking detail view; list views and analytics omit it (N10).
- Finance imports history page (or table on the existing Finance
  page) reads `GET …/imports` and shows date, type, counts, and a
  link to the merge-audit details.
- An info banner appears on Analytics tabs whenever the active
  range pre-dates the first statement upload, explaining that
  statement-derived metrics are unavailable for that bucket
  ("no statement data — metric unavailable").

**Analytics (Phase 2 within this task; ships in v1.1 alongside
Phase 1)**

- **Cancellation rate** KPI + trend (Performance tab, Q11):
  - Trend: booking-cohort, `cancelled / (cancelled + active)`
    grouped by `booked_on` month.
  - Operational KPI: arrival-cohort, same fraction grouped by
    `arrival` month.
  - `other` excluded from numerator and denominator (N7).
- **Lead-time histogram** (Demand tab, N2 overrides Q11): single
  chart with two series — "precise (statement)" from
  `arrival - booked_on` (days), and "approximate (calendar)" from
  the existing ICS-derived series. Legend toggles each series.
- **Persons distribution** + **ADR by persons** (Demand tab):
  active stays only, `NULL` / `0` persons excluded.
- **Commission rate trend** (weighted = `Σ commission / Σ final`,
  active stays only) and **commission per stay** bar chart (mirrors
  "Net per stay") on the Performance tab.
- All statement-derived metrics render the explicit "no statement
  data" empty state for buckets where `has_statement_data = false`
  for every row (avoids misleading 0% cancellation on pre-rollout
  months).
- Freshness disclaimer extended to include the last statement date
  in addition to the last payout date.
- Occupancies remain payout-driven (N8): future-arrival statement
  rows do not synthesise occupancies; lead-time / cancellation /
  persons-mix queries read directly from `finance_bookings`.
- Cancellation timestamp (N9) is **not** derived in v1.1 — neither
  file carries it cleanly. A follow-up task will explore fuzzy
  pairing against the ICS feed.

**Spec updates**

- `spec/openapi.yaml`: add the new preview/commit/imports endpoints
  and the `finance_bookings` response schema (canonical columns +
  source flags). Mark the rename of `finance_booking_payouts` and
  any client-visible response shape changes.
- `PMS_02_Module_Specifications.md`: rewrite the Finance module
  section to describe the merged `finance_bookings` table, the
  upload pipeline (preview → commit), the merge precedence rules,
  and the new analytics metrics.
- `PMS_04_Analytics_Data_Inventory.md`: add lead time, cancellation
  rate (both cohorts), persons distribution, ADR-by-persons,
  weighted commission rate and commission-per-stay; flag each as
  statement-derived with the "no statement data" rule.
- `spec/PMS_Statement_Ingestion_Spec.md`:
  resolve N6 (record what today's payout flow does and the chosen
  parity outcome) and link to this task as the implementation
  vehicle.

**Acceptance criteria**

(Mirrors the source spec's acceptance list; restated here so this
plan is self-contained.)

- Either CSV uploads through the single Finance upload control;
  the file type is auto-detected from the header signature.
- After uploading both files for the same month in any order,
  every reservation that appears in both has `has_payout_data` AND
  `has_statement_data` set, with merged values per the precedence
  table and signs / timezone normalised.
- Multi-hotel statement files only ingest rows matching the
  selected property's `booking_hotel_id`; the remainder are
  reported as `row_count_skipped_other_hotel`.
- A statement row with `Status='CANCELLED'` flips the linked
  occupancy (when one exists) to `cancelled`, never deletes it,
  and excludes it from occupancy / ADR / RevPAR.
- `MODIFIED`, `NO_SHOW`, `REFUSED_BY_HOTEL` rows ingest, are
  bucketed `other`, and are excluded from cancellation-rate
  numerator and denominator.
- Re-uploading either file is a no-op when row hashes match;
  changes write a `finance_booking_merges` row and bump
  `row_count_updated`.
- New analytics (lead-time dual-series, persons mix,
  ADR-by-persons, commission rate, commission-per-stay,
  cancellation rate booking-cohort + arrival-cohort) match
  hand-computed expectations on the September sample files
  (`spec/statement_processing/September_*.csv`); pre-statement
  buckets render the explicit "no statement data" state.
- GDPR delete-on-request nulls canonical PII columns AND redacts
  PII fields inside both `raw_*_json` blobs.
- Currency != EUR rejects the row with a structured error in the
  preview rejection list.
- Backend unit tests cover: statement-then-payout and the reverse
  order, cancel-after-payout and payout-after-cancel, idempotent
  re-upload (byte-different but logically identical re-export),
  cp1252 fallback, multi-hotel filter, sign convention, and
  no-overwrite-with-NULL precedence.
- Frontend vitest covers: preview rendering of all four lists,
  commit flow with `preview_token`, the "no statement data" empty
  state, and booker-name visibility (detail-only).

---
