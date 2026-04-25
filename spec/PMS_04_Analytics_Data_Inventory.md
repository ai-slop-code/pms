# PMS Analytics & Reporting Data Inventory

## Purpose

This document catalogs every analytical signal currently captured by the PMS database so a business analyst or property manager can scope a reporting dashboard against **real, already‑captured data**. Everything below is derivable from today's schema without new instrumentation, unless explicitly flagged as *not stored*.

**Schema source:** `backend/internal/migrate/*.up.sql` (SQLite). Store logic: `backend/internal/store/`. API aggregation: `backend/internal/api/`.

**Multi-tenancy:** Almost all business tables include `property_id`. Users link to properties via `properties.owner_user_id` and `property_user_permissions`.

---

## 1. Platform / governance

**Tables:** `users`, `auth_sessions`, `properties`, `property_profiles`, `property_user_permissions`, `api_audit_logs`

### Raw signals

- User roles (`super_admin`, `owner`, `property_manager`, `read_only`), active flags, `created_at` / `updated_at`
- Properties with timezone, default language, default currency, address, ownership, active flag
- Property profiles: legal/billing identity, tax IDs (`ico`, `dic`, `vat_id`), Wi-Fi, parking, default check-in/out times, cleaner Nuki ID
- Per-property module ACLs (`occupancy`, `nuki_access`, `cleaning_log`, `finance`, `invoices`, `messages`, …) at `read | write | admin`
- Session creation / expiry
- Audit log: `actor_user_id`, `action`, `entity_type`, `entity_id`, `outcome`, HTTP method / path, timestamp

### Metrics derivable

- Portfolio size, geo distribution, currency mix
- Profile completeness (% of properties with Wi-Fi / parking / VAT ID / contact phone)
- User counts by role, active vs inactive
- Properties per owner, manager workload
- Module access coverage (who can do what, where)
- Session activity volume; last active per user (via audit + sessions)
- Audit volume by action / path / outcome over time; admin-action heatmap

---

## 2. Occupancy (Booking.com ICS sync)

**Tables:** `occupancies`, `occupancy_raw_events`, `occupancy_sync_runs`, `occupancy_sources`, `occupancy_api_tokens`

### Raw signals per stay

- `start_at`, `end_at` (UTC), `status` (`active | updated | cancelled | deleted_from_source`)
- `source_type` (`booking_ics` or synthetic `booking_payout`), `source_event_uid`, `raw_summary`, `guest_display_name`
- `imported_at`, `last_synced_at`, `content_hash` (change detection), `last_sync_run_id`

### Sync run signals

- Per-run `started_at` / `finished_at`, `status`, `events_seen`, `occupancies_upserted`, `http_status`, `trigger` (`scheduled | manual`), `error_message`

### Metrics derivable

- **Occupancy / nights booked** per month / quarter / year, per property and portfolio-wide
- **Occupancy rate** (booked nights / available nights) — availability is implicit (calendar = all nights)
- **Average length of stay**, distribution
- **Lead time** (between `imported_at` and `start_at`)
- **Booking pace / pickup curve** (cumulative bookings by day before arrival)
- **Cancellation / modification rate** via status transitions and `content_hash` churn
- Peak vs off-peak seasonality
- Weekday vs weekend mix
- ICS feed reliability: success ratio, sync latency, error taxonomy, HTTP status distribution
- Token usage (B2B JSON export consumers via `last_used_at`)

> Not stored: room / listing-level price per night, ADR — the ICS feed doesn't carry rates. RevPAR / ADR become possible via Booking payouts (see §6).

---

## 3. Nuki smart-lock access

**Tables:** `nuki_access_codes`, `nuki_keypad_codes`, `nuki_event_logs`, `nuki_sync_runs`

### Raw signals

- Code lifecycle: `status` (`not_generated | generated | revoked`), `valid_from`, `valid_until`, `created_at`, `updated_at`, `revoked_at`, `error_message`
- Linkage `occupancy_id` ↔ stay
- Per-sync counters: processed / created / updated / revoked / failed
- Event log with type, message, JSON payload, timestamp
- Keypad mirror: enabled flag, `last_seen_at`, raw API payload

### Metrics derivable

- Codes generated vs revoked over time; success vs failure ratio per sync run
- **Time-to-generate** (occupancy import → first `generated`)
- Active codes overlapping any date (live access)
- Failure recurrence and error categorization
- Keypad inventory size, churn (`last_seen_at` drift), orphaned / expired codes
- Operational SLA on Nuki integration (sync frequency, last successful sync per property)

---

## 4. Cleaning operations

**Tables:** `cleaner_fee_history`, `cleaning_daily_logs`, `cleaning_monthly_summaries`, `cleaning_salary_adjustments` (+ `property_profiles.cleaner_nuki_auth_id`)

### Raw signals

- Per day: `day_date`, `first_entry_at` (Nuki entry time), `counted_for_salary`, `nuki_event_reference`
- Fee tiers with `effective_from` (`cleaning_fee_amount_cents` + `washing_fee_amount_cents` per day)
- Monthly cache: `counted_days`, `base_salary_cents`, `adjustments_total_cents`, `final_salary_cents`, `computed_at`
- Adjustments: amount (signed), reason text, who, when

### Metrics derivable

- **Cleaner labor cost** per month / year / property; YoY trend
- **Days worked** per month; days worked per booked checkout (productivity)
- **Cost per cleaning / cost per stay / cost per night**
- **Time-of-day arrival heatmap** (already exposed via `/cleaning/heatmap`)
- Attendance regularity (gaps, weekends worked)
- Adjustment frequency, distribution of reasons (text mining), bonus vs penalty share
- Fee schedule history (rate increases over time)

---

## 5. Finance ledger

**Tables:** `finance_transactions`, `finance_categories`, `finance_recurring_rules`, `finance_month_states`

### Raw signals

- Per transaction: `transaction_date`, `direction` (`incoming | outgoing`), `amount_cents`, `category_id`, `note`, `source_type` (`manual | booking_payout | recurring_rule | cleaning_salary`), `source_reference_id`, `is_auto_generated`, `attachment_path`, timestamps
- Categories: `code`, `title`, `direction`, `counts_toward_property_income` flag
- Recurring rules: amount, direction, monthly schedule, validity window
- "Month opened" markers (when recurring transactions were instantiated)

### Metrics derivable (much already exposed via `/finance/summary`)

- **P&L per month / year / property**: incoming, outgoing, net
- **Revenue mix by category** (booking income vs other)
- **Expense mix by category** (cleaning, maintenance, utilities, etc.)
- **Property income** (only categories flagged `counts_toward_property_income`)
- **Cleaner margin** = cleaner expense / property income (already computed)
- Manual vs automated transaction share
- Recurring rule coverage and adoption
- Month-close discipline (which months opened on time)
- Cashflow timing distribution (transaction_date vs created_at lag)

---

## 6. Booking.com payouts (commission & fee analytics)

**Table:** `finance_booking_payouts` (with FKs to `finance_transactions` and `occupancies`)

### Raw signals per CSV row

- `reference_number`, `payout_id`, `row_type`, `check_in_date`, `check_out_date`, `guest_name`, `reservation_status`, `payment_status`, `currency`
- **Money columns:** `amount_cents` (gross from CSV), `commission_cents`, `payment_service_fee_cents`, `net_cents` (required), `payout_date`
- Linkage: `transaction_id` (auto-created ledger entry, **uses net only**), `occupancy_id` (matched stay), full `raw_row_json`

### Metrics derivable

- **Gross booking revenue** (sum of `amount_cents`) — closest thing to ADR data we currently have
- **Booking.com commission %** = commission / gross; portfolio-wide and per stay
- **Effective take-rate** = (commission + payment_service_fee) / gross
- **Net payout** time-series, payout cadence (`payout_date` distribution)
- **ADR** (gross / nights), **RevPAR** (gross / available nights) once joined to occupancy
- **Reservation status mix** (paid, no-show, cancelled, etc.)
- Payment service fee burden over time
- **Payout-to-stay matching quality**: % rows with `occupancy_id` set vs orphaned
- Repeat guest detection (by `guest_name` — noisy but possible)
- Currency mix on the platform side (ledger is EUR-only)

> Important caveat: **only the net payout currently lands on the ledger**. Gross / commission / fees are stored on the payout table only, so any full P&L view should join `finance_booking_payouts`.

---

## 7. Invoices

**Tables:** `invoices`, `invoice_files`, `invoice_sequences`

### Raw signals

- `invoice_number`, `sequence_year`, `sequence_value`, `language` (`sk | en`)
- Dates: `issue_date`, `taxable_supply_date`, `due_date`, `stay_start_date`, `stay_end_date`
- `amount_total_cents`, `currency`, `payment_status`, `payment_note`, `version`
- Frozen JSON snapshots (`supplier_snapshot_json`, `customer_snapshot_json`) — contain VAT IDs, ICO / DIC, addresses
- Linkage: `occupancy_id`, `finance_booking_payout_id`
- PDF files: `version`, `file_size_bytes`, `created_at`

### Metrics derivable

- Invoice volume per month / quarter / year
- Total invoiced amount; invoiced vs payout-net reconciliation
- Average versions per invoice (regeneration frequency)
- Customer concentration (parsing snapshot JSON: top guests, top countries by VAT prefix)
- Stay-linked vs payout-linked vs standalone invoices
- Average `issue_date − stay_end_date` (billing speed)
- Language mix on issued invoices

> Caveat: VAT rate / net / tax split is **not stored in columns** — only the total. A VAT-detailed report would need either a schema addition or PDF parsing.

---

## 8. Messages / templates

**Table:** `message_templates` only (rendered messages are not persisted)

### Raw signals

- Language, type (`check_in`, `cleaning_staff`), title, body, active flag, `updated_at`

### Metrics derivable

- Template inventory by language and type
- Active vs inactive templates
- Edit frequency (via `updated_at`)

> **Not derivable today:** messages actually sent, opens, conversion. There is no outbound send log — every guest message is rendered ephemerally and copy-pasted to WhatsApp.

---

## Cross-cutting time dimensions

Every reporting axis below is supported by stored timestamps:

| Axis | Source |
|---|---|
| Stay arrival / departure date | `occupancies.start_at` / `end_at` |
| ICS feed reliability over time | `occupancy_sync_runs.started_at` / `finished_at` |
| Nuki integration health | `nuki_sync_runs`, `nuki_event_logs.created_at` |
| Code validity | `nuki_access_codes.valid_from` / `valid_until` |
| Cleaning attendance | `cleaning_daily_logs.day_date`, `first_entry_at` |
| Cashflow date | `finance_transactions.transaction_date` |
| Booking payout date | `finance_booking_payouts.payout_date` |
| Invoice issue / taxable supply / due dates | `invoices.*` |
| Audit timestamps | `api_audit_logs.created_at` |

---

## What's already aggregated and exposed via API

- **Dashboard** (`getDashboardSummary`): last sync state (occupancy + Nuki), upcoming 5 stays, active 5 Nuki codes, current-month cleaning summary (days + salary), current-month finance summary (in / out / net), 3 most recent invoices.
- **Finance summary** (`/finance/summary`): all-time + selected month / year totals, property income, cleaner expense, **cleaner margin %**, per-category breakdown.
- **Cleaning**: monthly summary, **24-hour entry heatmap**, yearly stats (counted days per month).
- **Booking payouts**: enriched list with linked stay window + linked invoice id.
- **Audit log**: filterable list (used as a compliance feed today, not yet aggregated).

---

## Known gaps to flag proactively

1. **No room rate / ADR data** in occupancy itself — ICS doesn't carry it. Gross revenue and ADR can only be reconstructed via `finance_booking_payouts.amount_cents`.
2. **Ledger only stores net payouts** for booking income — gross / commission / fees live on the payout table and need to be joined for a full P&L.
3. **No VAT breakdown column** on invoices — only `amount_total_cents`. Tax analytics require a schema extension.
4. **No outbound-message log** — we render but don't persist sends / opens.
5. **No competitor / market data** — internal performance only.
6. **Single-property ledger currency (EUR)** — multi-currency analytics aren't possible without FX capture.
7. **Cleaning attendance is reconciled on demand** — we keep `cleaning_daily_logs` (per day) but not raw Nuki entries beyond `first_entry_at`, so we can't easily reconstruct "all entries that day".
