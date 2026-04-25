# PMS Frontend Cleanup Plan

**Version:** 1.24 (2026-04-25)  
**Status:** Phase 1 complete; Phase 2 §3.1 (view splits, all 8), §3.2 (shared API types), §3.3 (ESLint + Prettier), §3.4 (dialog replacement), §3.5 (view smoke specs, all 16), §3.6 (a11y — OccupancyView calendar grid, AnalyticsView heatmap keyboard nav, DOW/lead-time/LOS/cleaning bar-chart `sr-only` tables), §3.7 (strict TS tightening) all complete. Only §3.8 (HTML/CSS bar charts staying as-is unless interactivity needed) remains deferred by design.  
**Scope:** `frontend/src/` only. Backend + specs untouched.  
**Audience:** AI developer agent + any new human contributor.  
**Precondition:** PMS_08 v1.4 in place — all committed v1.1 UI work has shipped. This is the first structured cleanup pass focused on maintainability, not new features.

## 0. Goals & non-goals

**Goals.**
- Remove duplication that's accumulated while shipping 17 views.
- Kill known hacks (setTimeout-to-fight-blur, raw `rgba()` hexes, cross-view type copies).
- Close obvious a11y + type-safety gaps that the v1 spec mandates.
- Land the scaffolding (shared composables, shared types, design-token coverage) that makes future feature work smaller and safer.

**Non-goals for this pass.**
- No behaviour changes end-users could observe. Every fix must be a pure refactor or an a11y/typing bug fix; visual diffs should stay below the "noticeable" threshold.
- No new product features (the v1.1 scope is done).
- No rewriting of views larger than 400 lines. Splitting `AnalyticsView`, `FinanceView`, `MessagesView`, `InvoicesView` is intentionally deferred to Phase 2 — the risk/reward at this stage doesn't justify doing it mixed in with the housekeeping.

## 1. Audit findings (2026-04-24)

Full audit covered: 17 views, 19 UI primitives, 2 shell components, 1 chart wrapper, 3 stores, 1 composable, 1 HTTP module, 1 router, CSS + tokens. ~13.6 k LOC.

Counts by severity: **22 high**, **28 medium**, **17 low** — 67 findings total. The headline items:

| # | Category | Finding | Phase |
|---|----------|---------|-------|
| 1 | TS hygiene | Every view redeclares response types instead of importing from `api/*` | **2** |
| 2 | TS hygiene | `api/http.ts` spreads `HeadersInit` as `Record<string,string>` which silently drops `Headers` / tuple inputs | 1 |
| 3 | TS hygiene | Missing explicit return types on store + composable exports | 2 |
| 4 | SFC quality | Four files > 600 LOC (Analytics 1 477, Finance 920, Messages 809, Invoices 772) | **2** |
| 5 | SFC quality | 200 ms `setTimeout` "fight the blur" hack in MessagesView combobox | 1 |
| 6 | SFC quality | `shallowRef` candidates for large wholesale-replaced API payloads | 2 |
| 7 | Duplication | `eur()`, `shiftMonth`/`monthKey`, `fmtDay`/`fmtDateTime` helpers copy-pasted across 5+ views | 1 |
| 8 | Duplication | "Pick a property" empty-state boilerplate in 7 views | 2 (composable; individual refactors phase 2) |
| 9 | Duplication | The fetch-loading-error-toast triad appears 40+ times | 2 |
| 10 | Duplication | `box-shadow: 0 0 0 3px rgba(37,99,235,0.2)` focus ring duplicated in 8 files | 1 |
| 11 | Hacks | `rgba(15,23,42,0.4)` scrim duplicated in 2 overlay components (spec forbids raw hex) | 1 |
| 12 | Hacks | `UiLineChart` hex fallback array drifted out of sync with `--viz-*` tokens (viz-5 maps to red instead of cyan) | 1 |
| 13 | Hacks | `rgb(...)` interpolation in AnalyticsView heatmap + legend (spec: use `color-mix` + tokens) | 2 |
| 14 | A11y | Custom combobox in MessagesView lacks `role="combobox"`, `aria-expanded`, arrow-key nav | 2 |
| 15 | A11y | AppSidebar `aria-hidden` logic inverted (`false` when hidden, `undefined` when open) | 1 |
| 16 | A11y | Calendar cells, seasonality heatmap cells, DoW bars — no accessible data fallback | 2 |
| 17 | Testing | Zero spec files for views; `stores/property.ts` has no spec | 1 (property.spec) + 2 (views) |
| 18 | Tooling | No ESLint, Prettier, `lint`, or `type-check` npm script | 1 (scripts) + 2 (ESLint config) |
| 19 | Tooling | `overrides.esbuild: ^0.25.0` no longer needed — vite@6 already ships a clean version | 1 |
| 20 | Tooling | `vue-chartjs` imported nowhere; only `chart.js` is consumed directly | 1 |
| 21 | Tooling | No `engines` field, no `.nvmrc` | 1 |
| 22 | Tooling | `tsconfig.json` missing `noUncheckedIndexedAccess` (would catch real regexp index bugs) | 2 |

The full 67-item list lives in the PR description; this document tracks the execution plan.

## 2. Phase 1 — Safe, high-leverage fixes (this pass)

Ordered roughly by risk (ascending) + leverage (descending). Each item is a stand-alone edit.

### 2.1 Tooling & scripts

- [x] Add `"type-check": "vue-tsc --noEmit"` and `"lint:style": "stylelint 'src/**/*.{vue,css}' --aei"` *(stylelint deferred — ESLint/Stylelint config is its own decision in Phase 2; keep the type-check script)* to `frontend/package.json`.
- [x] Add `"engines": { "node": ">=20" }` and a root `.nvmrc` (`20`).
- [x] Drop the `overrides.esbuild` block — vite 6 already resolves `esbuild@0.25.12`, so the override no longer changes anything.
- [x] Remove `vue-chartjs` from dependencies — the chart wrapper uses the raw `Chart` constructor; `vue-chartjs` is dead weight.

### 2.2 Design tokens

- [x] Add `--focus-ring` + `--focus-ring-offset` to `assets/tokens.css`. Derive from `--color-focus` via `color-mix`.
- [x] Add `--color-scrim` token (matches existing `rgba(15,23,42,0.4)`).
- [x] Backfill `--viz-7` and `--viz-8` — `UiLineChart` already assumes 8 but `tokens.css` declares only 6.
- [x] Replace every `box-shadow: 0 0 0 3px rgba(37,99,235,0.2)` with `box-shadow: var(--focus-ring)` — 8 files.
- [x] Replace every `background: rgba(15,23,42,0.4)` with `background: var(--color-scrim)` — 2 files.
- [x] Replace the remaining `rgba(37,99,235, …)` tints in views (selection highlights, list row active backgrounds — 5 spots) with `color-mix(in srgb, var(--color-primary) N%, transparent)`.

### 2.3 Shared utilities

- [x] `utils/format.ts` gains `formatEuros(cents, { signed? })` and `formatPercent(ratio, { digits? })`. Both respect the `EM_DASH` convention.
- [x] New `utils/month.ts` with `shiftMonth(key, delta)` and `monthKey(date)`. Five views stop redeclaring them.
- [x] New `utils/async.ts` with `sleep(ms)` (currently inlined in NukiView).
- [x] New `composables/useCurrentProperty.ts` — returns `{ pid, currentProperty, propertyStore }`, removes the same 3-line block from every property-scoped view.
- [x] New `composables/useCopyFeedback.ts` — one place for "copy text + flash 'copied' flag for 1.5 s". Replaces ad-hoc `setTimeout` flags in Messages / Occupancy / Nuki.
- [x] New `composables/useDocumentTitle.ts` — tiny `watchEffect` wrapper, replaces the one direct `document.title = …` assignment in `ShellView`.

### 2.4 Typing fixes

- [x] `api/http.ts`: narrow `headers` handling — accept `HeadersInit` properly instead of casting to `Record<string,string>`. Makes the error the test already guards against impossible.
- [x] New `api/types/index.ts` directory (empty-ish — just re-exports) to *start* the migration to shared types. Full migration is Phase 2; this is the landing pad.

### 2.5 A11y quick wins

- [x] Fix inverted `aria-hidden` in `AppSidebar` (was `'false'` while hidden).
- [x] Kill the `setTimeout(() => showOccList = false, 200)` hack in `MessagesView` — replace with `@mousedown.prevent` on dropdown items so the input never loses focus in the first place.
- [x] `UiDialog` backdrop: add `role="presentation"` to document the mouse-only affordance (keyboard users already have `Escape`).

### 2.6 Chart wrapper cleanup

- [x] `UiLineChart` hex fallback array: realign with `tokens.css` (viz-1..6 + neutrals). Drop the trailing two entries that nothing consumes.

### 2.7 Test coverage

- [x] New `stores/property.spec.ts` — covers `loadStored` / `fetchList` / localStorage sync / auto-reselect after the current property disappears.
- [x] Extend `utils/format.spec.ts` with `formatEuros`, `formatPercent`, and the previously-uncovered `formatEmpty` non-finite branch.
- [x] Add specs for the new composables where deterministic (`useCopyFeedback`, `useDocumentTitle`; `useCurrentProperty` is just a Pinia thin wrapper and covered by `property.spec`).

### 2.8 Dead weight

- [x] Remove the unused `vue-chartjs` import path from `package.json`.
- [x] Remove the retired `overrides.esbuild` block.

## 3. Phase 2 — Deferred work (tracked separately)

These items are high-value but too invasive to land in the same PR as the polish pass. They need their own scope + review.

### 3.1 View splits

| View | LOC | Proposed children |
|------|----:|-------------------|
| `AnalyticsView.vue` | 1 477 → 369 | `AnalyticsOutlookTab`, `AnalyticsPerformanceTab`, `AnalyticsDemandTab`, `AnalyticsGlossary` + shared `helpers.ts` ✅ (landed in 1.23) |
| `FinanceView.vue` | 920 → 429 | `FinanceOverviewTab`, `FinanceTransactionsTab`, `FinanceRecurringTab`, `FinanceCategoriesTab`, `FinanceBreakdownTab` + shared `helpers.ts` ✅ (landed in 1.23) |
| `MessagesView.vue` | 809 → 784 | `OccupancyCombobox` (ARIA 1.2), `TemplateEditor`, `CleaningMessageCard` + shared `helpers.ts` ✅ (landed in 1.23) |
| `InvoicesView.vue` | 772 → 388 | `InvoiceList`, `InvoiceEditorForm`, `InvoiceFilesTable` + shared `format.ts` ✅ (landed in 1.23) |
| `OccupancyView.vue` | 700 → 261 | `OccupancyCalendar`, `OccupancyStayList`, `OccupancySyncPanel` + shared `status.ts` ✅ (landed in 1.23) |
| `NukiView.vue` | 617 → 372 | `NukiUpcomingStays`, `NukiCodeTable`, `NukiRunsTimeline` (+ `NukiPinRevealDialog`, `status.ts`) ✅ (landed in 1.22) |
| `DashboardView.vue` | 646 → 174 | `DashboardHeroKpis`, `DashboardAlertsCard`, `DashboardUpcomingStays`, `DashboardNukiCodes`, `DashboardRecentInvoices`, `DashboardQuickActions` ✅ (landed in 1.21) |
| `CleaningView.vue` | 568 → 418 | `CleaningHeatmap`, `CleaningLogsTable`, `CleaningFeeHistory` ✅ (landed in 1.20) |

### 3.2 Shared API types

Every view declares its own `interface Invoice`, `interface FinanceTransaction`, etc. Target: one source of truth under `src/api/types/*.ts`, reused across views + stores + `api/http.ts`. Phase 1 lands the folder + an index; Phase 2 migrates the interfaces.

### 3.3 ESLint + Prettier config

Choose rule set (`@vue/eslint-config-typescript` + `eslint-plugin-vue` `strongly-recommended` tier), add `lint` + `lint:fix` scripts, wire into CI. Needs a call on Prettier line-length + semicolon style before landing.

### 3.4 Replace remaining `window.confirm` / `window.prompt` usage

Seven call sites (Nuki, Occupancy, Finance, Messages, UserDetail). `UiDialog` already exists — just not wired in. Each needs bespoke UX design for the confirmation copy, so the replacement work is not mechanical.

### 3.5 View smoke specs

Add a baseline `mount(View)` spec per view with `api` mocked. Stops the "1 line change breaks a distant view" class of regressions. Deferred because writing them is slow and best done *after* the view splits land.

### 3.6 Accessibility pass — custom widgets *(complete — see changelog 1.24)*

- Calendar cells (`OccupancyView`): `role="grid"` on container, `role="row"` per week, `role="gridcell"` + `aria-label` with date + occupied/check-in counts. ~✅~
- Seasonality heatmap (`AnalyticsView`): `role="grid"` + per-cell `role="gridcell"` with roving `tabindex` + Arrow/Home/End/PageUp/PageDown keyboard nav. ~✅~
- DoW + lead-time + LOS + yearly-cleaning bar charts (`AnalyticsView`): bars marked `aria-hidden` with a sibling `sr-only <table>` carrying the same data. ~✅~
- MessagesView combobox: full ARIA 1.2 combobox pattern (delivered with the v1.23 split).

### 3.7 Strict TS tightening *(complete — see changelog 1.19)*

Enable `noUncheckedIndexedAccess`, `verbatimModuleSyntax`. Fix the fallout (expect 30–50 sites of `array[idx]!` / `match[1]!`).

### 3.8 Chart rendering — remaining HTML/CSS bars

Only if interactivity is ever needed. Bar-style charts (net-per-stay, yearly cleaning, DoW occupancy) stay as HTML/CSS by design (PMS_05 §*Chart rendering*).

## 4. Changelog for this cleanup pass

- **1.24 (2026-04-25)** — Phase 2 §3.6 **complete**: accessibility pass for the three remaining custom widgets.
  - `OccupancyCalendar.vue`: calendar restructured into `role="grid"` → weekly `role="row"` wrappers → `role="gridcell"` with `aria-label="<iso-date>, N occupied nights, M check-ins"`. Weekday headers become `role="columnheader"`. Leading/trailing blank cells get `role="presentation"` so screen readers announce a clean 7-column grid per week. CSS switched from one flat `display: grid` to per-row grids (identical visual output; enables the row semantics without adding `display: contents` hacks).
  - `AnalyticsPerformanceTab.vue` seasonality heatmap: wrapped in `role="grid"` with per-year `role="row"` + `role="rowheader"` and per-week `role="gridcell"` cells. Each cell has `aria-label="<year> week <w>: <pct> occupancy"`. Roving tabindex keeps exactly one cell focusable; a grid-level `keydown` handler moves focus on Arrow/Home/End/PageUp/PageDown and updates focus to the target cell via `data-heat-row` / `data-heat-col` selectors. Focus ring added via `.heat-cell:focus { outline: 2px solid var(--color-primary) }`.
  - DoW occupancy + yearly cleaning (PerformanceTab) + lead-time + length-of-stay (DemandTab) HTML/CSS bar charts: bars now sit inside `role="img"` containers with an `aria-label` summary, individual bars marked `aria-hidden="true"`, and a sibling `<table class="sr-only">` carries the same data with `<caption>`, `<th scope="col">` headers, and `<th scope="row">` row labels — mirroring the `UiLineChart` fallback pattern mandated by PMS_05 §*Accessibility*.
  - Running totals unchanged (no spec churn): **49 test files, 251 tests**. Verification: lint 0/0w, type-check clean, build green.
- **1.23 (2026-04-25)** — Phase 2 §3.1 **remaining five view splits landed together**, completing the §3.1 scope:
  - `OccupancyView.vue` (700 → 261 LOC) → `occupancy/OccupancyCalendar.vue` (232), `OccupancyStayList.vue` (92), `OccupancySyncPanel.vue` (174), shared `status.ts` (43).
  - `InvoicesView.vue` (772 → 388 LOC) → `invoices/InvoiceList.vue` (128), `InvoiceEditorForm.vue` (261), `InvoiceFilesTable.vue` (39), shared `format.ts` (40). `InvoiceEditorForm` uses `defineModel('form')` so the parent binds with `v-model:form` and child can freely mutate the form state without the `vue/no-mutating-props` lint firing.
  - `MessagesView.vue` (809 → 784 LOC) → `messages/OccupancyCombobox.vue` (261 — full ARIA 1.2 pattern: `role`, `aria-expanded/controls/activedescendant`, ArrowUp/Down/Enter/Escape/Home/End, typeahead), `TemplateEditor.vue` (124), `CleaningMessageCard.vue` (78), shared `helpers.ts` (47). Parent keeps all API orchestration + tab state + the new-template inline form — splitting those would have meant prop-drilling every template-API call. The combobox extraction is the headline win: it replaces the old 200 ms blur-hack `setTimeout` with focus-safe `@mousedown.prevent` + full keyboard nav (§3.6 item resolved).
  - `FinanceView.vue` (920 → 429 LOC) → `finance/FinanceOverviewTab.vue` (55), `FinanceTransactionsTab.vue` (210), `FinanceRecurringTab.vue` (99), `FinanceCategoriesTab.vue` (94), `FinanceBreakdownTab.vue` (171), shared `helpers.ts` (75) for `displayDirection` / `displaySource` / `directionTone` / `VIZ_PALETTE` / `FinanceTab` / `TxForm` / `RecurringForm` / `CategoryForm`. Form state (`txForm`, `recurringForm`, `categoryForm`) lifted to `defineModel(...)` in each tab so the parent keeps ownership but the tab does the binding without prop mutation warnings. Edit-transaction `UiDialog` + payout import + all API calls kept in the parent.
  - `AnalyticsView.vue` (1 477 → 369 LOC) → `analytics/AnalyticsGlossary.vue` (48), `AnalyticsOutlookTab.vue` (209 — self-computes pacing + unsold range derivations), `AnalyticsPerformanceTab.vue` (473 — toolbar + money/occupancy/cleaning KPIs + monthly trend chart + seasonality heatmap + DOW bars + cancellation pills + yearly cleaning bars, self-computes `monthlyTrendVisible` / `netRowsFiltered` / `seasonalityGrid` / `dowRows`), `AnalyticsDemandTab.vue` (272 — lead/LOS/ADR-by-dim bars + gap/orphan tables + returning-guests card with Top 5), shared `helpers.ts` (109) for `eur` / `pct` / `freshnessTone` / `freshnessLabel` / `dowIndex` / `dowLabel` / `weekdayOfIso` / bucket labels / `heatCellColor` / `addDaysIso` / `todayIso` + `UnsoldRange` interface. Parent keeps `loadFreshness` / `loadOutlook` / `loadPerformance` / `loadDemand` / returning-guests pagination and lazy-loads each tab on first visit.
  - Running totals unchanged (no spec churn): **49 test files, 251 tests**. Verification: lint 0/0w, type-check clean, build green.
- **1.22 (2026-04-24)** — Phase 2 §3.1 **third view split landed**: `NukiView.vue` (617 → 372 LOC) decomposed into four children + one shared module under `src/views/nuki/`:
  - `NukiUpcomingStays.vue` (122 LOC) — upcoming-stays table with inline PIN-name input + Generate button. Props: `stays`, `pinNames`, `savingStayName`, `generatingOccupancyId`, `revealingCodeId`. Emits `update:pin-name`, `save-pin-name`, `generate`, `reveal`.
  - `NukiCodeTable.vue` (89 LOC) — enabled-codes table with Reveal/Delete actions. Props: `codes`, `revealingCodeId`. Emits `reveal`, `delete`.
  - `NukiRunsTimeline.vue` (73 LOC) — sync-history table + pagination toolbar. Props: `runs`, `page`, `hasMore`, `loading`. Emits `prev`, `next`.
  - `NukiPinRevealDialog.vue` (82 LOC) — PIN reveal modal with countdown + copy/close. Props: `reveal`, `secondsLeft`. Emits `copy`, `close`. Extracting the dialog was not in the plan but keeps all reveal-specific CSS colocated and drops ~80 LOC of markup + styles from the parent.
  - `status.ts` (30 LOC) — `displayStatus` / `statusTone` / `canGenerate` helpers + `NukiBadgeTone` type, imported by three of the children.
  - Parent NukiView keeps all reveal-timer machinery (`startRevealCountdown`, `closePinDialog`, `copyPinToClipboard`, `onBeforeUnmount`), the multi-step `loadAll` / `refreshAfterGenerate` / `syncCodesQuietly` orchestration, `revealPin` (touches multiple refs + toast + error), `generateForStay` (chained refresh + reveal), `deleteKeypadCode` (uses `useConfirm`), `saveStayNameForOccupancy`, runs pagination state, and the toolbar + banners. Children are pure presentational.
  - Running totals unchanged (no spec churn): **49 test files, 251 tests**. Verification: lint 0/14w, type-check clean, build green.
- **1.21 (2026-04-24)** — Phase 2 §3.1 **second view split landed**: `DashboardView.vue` (646 → 174 LOC) decomposed into six widget components + three shared modules under `src/views/dashboard/`:
  - `DashboardHeroKpis.vue` (65 LOC) — hero KPI row; owns the `upcoming7DayCount` + `upcomingCount` derivations so the parent no longer knows about them. Takes `finance`, `cleaning`, `upcomingStays` props.
  - `DashboardAlertsCard.vue` (120 LOC) — alerts list + sync-status fallback in one card. Takes pre-computed `alerts` array + `syncStatus`.
  - `DashboardUpcomingStays.vue` (48 LOC), `DashboardNukiCodes.vue` (55 LOC), `DashboardRecentInvoices.vue` (45 LOC) — each is a dumb list card rendering its slice of `DashboardWidgets`.
  - `DashboardQuickActions.vue` (58 LOC) — bottom "Available areas" chip row. Takes the pre-filtered `actions` array.
  - `status.ts` (57 LOC) — `widgetTitle` / `displayStatus` / `statusTone` helpers + the `DashboardBadgeTone` type, imported by 4 of the children.
  - `alerts.ts` (7 LOC) — `DashboardAlert` interface. Lives in a `.ts` file because `verbatimModuleSyntax` + `<script setup>` makes exporting a type from a `.vue` file awkward.
  - `listRows.css` (72 LOC) — shared unscoped list-row styles, imported by the three list cards. Unscoped keeps the style budget flat; the `.dashboard-list-*` prefix avoids collisions.
  - Parent DashboardView keeps the `alerts` and `quickActions` computed (they need the full `summary` + auth/nav state, so lifting them would have meant prop-drilling) and the `load()` + `watch(pid, ...)` lifecycle. Template is now a 30-line flat layout instead of 310 lines of markup.
  - Running totals unchanged (no spec churn): **49 test files, 251 tests**. Verification: lint 0/14w, type-check clean, build green. Bundle: DashboardView chunk split out from main (now lazy-loadable per child).
- **1.20 (2026-04-24)** — Phase 2 §3.1 **first view split landed**: `CleaningView.vue` (509 → 418 LOC) decomposed into three presentational children under `src/views/cleaning/`:
  - `CleaningHeatmap.vue` (75 LOC) — owns `nonZero` + `maxCount` derived state; takes `buckets: CleaningHeatBucket[]` prop. Moved all `.arrival-hbar*` styles with it.
  - `CleaningLogsTable.vue` (47 LOC) — dumb table; takes `logs: CleaningLogRow[]` prop. Moved `UiBadge` import with it (only consumer in CleaningView).
  - `CleaningFeeHistory.vue` (93 LOC) — owns the fee form state + local `eur()` helper; takes `fees` + `saving` props, emits `submit` with pre-computed payload (eur→cents + ISO date). Parent wires `@submit="addFee"` and keeps `savingFee` + API call.
  - Monthly adjustments section kept inline in the parent (not in the plan's proposed children and tightly coupled to the `month` ref).
  - Running totals unchanged (no spec churn): **49 test files, 251 tests**. Verification: lint 0/15w, type-check clean, build green. Bundle sizes for CleaningView chunk unchanged (Vite inlines template fragments either way).
- **1.19 (2026-04-24)** — Phase 2 §3.7 **complete**: enabled `noUncheckedIndexedAccess` + `verbatimModuleSyntax` in `frontend/tsconfig.json`.
  - Fixed ~60 sites of possibly-undefined indexed access across production + spec code. Mostly `arr[i].foo()` → `arr[i]?.foo()` for spec-side wrapper indexing; `arr[0].id` → `arr[0]?.id ?? fallback` for store/view code where a safe default exists; explicit `if (!x) return` guards in `UiDialog.vue` (focus-trap) and `UiTabs.vue` (keyboard nav).
  - New helper `parseMonthKey(key)` in `utils/month.ts` replaces 4 ad-hoc `str.split('-').map(Number)` tuple destructures in `OccupancyView.vue` — tuple destructuring under `noUncheckedIndexedAccess` yields `string | undefined`, and the shared helper sidesteps this while centralising malformed-input fallback.
  - Test-side router helper `handlers[match]()` → `handlers[match]!()` across 9 view spec files (match is guaranteed by the preceding `find()`).
  - `verbatimModuleSyntax` landed with **zero fallout** — all imports in the codebase were already in the correct `import type` / runtime split thanks to earlier §3.2 work and the ESLint `consistent-type-imports` rule from §3.3.
  - Running totals unchanged (no spec churn): **49 test files, 251 tests**. Verification: lint 0/15w, type-check clean, build green.
- **1.18 (2026-04-24)** — Phase 2 §3.2 **complete**: view DTO interfaces moved to shared modules under `src/api/types/`:
  - New files: `analytics.ts` (22 types), `bookingPayouts.ts` (2), `cleaning.ts` (7), `dashboard.ts` (7), `finance.ts` (5 + `FinanceDirection` union), `invoice.ts` (6), `messages.ts` (5 — re-exports `OccupancySummary` as `MessagesOccupancy`), `nuki.ts` (4), `occupancy.ts` (4), `users.ts` (2), plus `index.ts` barrel.
  - Rename conflicts resolved by domain-prefixing (e.g. `NukiKeypadCode` vs Cleaning's `CleaningNukiCodeRow`, `InvoiceOccupancyOption` vs `BookingPayoutOccupancyOption`) and aliased at import site where the view uses a shorter local name (e.g. `import type { NukiUpcomingStay as UpcomingStay }`).
  - Views migrated: UsersView, UserDetailView, CleaningView, DashboardView, BookingPayoutsView, NukiView, OccupancyView, MessagesView, InvoicesView, FinanceView, AnalyticsView. Deleted 70+ inline `interface` blocks — the only per-view-local interface kept is AnalyticsView's `UnsoldRange` (derived UI helper, not an API DTO).
  - AnalyticsView and InvoicesView imports trimmed to the types actually referenced directly; transitively-reached types (e.g. `PerformanceKPIs` via `PerformanceResponse`) intentionally not re-imported.
  - Running totals unchanged (no spec churn): **49 test files, 251 tests**. Verification: lint 0/0, type-check clean, build green. Bundle sizes unchanged (type-only imports are erased by the TypeScript compiler).
- **1.17 (2026-04-24)** — Phase 2 §3.5 view smoke specs **complete** — final batch of 6 landed together:
  - `NukiView.spec.ts` (3 tests): empty-state when no property; loads `/nuki/codes`, `/nuki/upcoming-stays`, `/nuki/runs` on mount and renders a stay row (date-based assertion — the stays table omits the guest name column); error banner when the initial load rejects. Mocks `useToast` + `useConfirm`.
  - `OccupancyView.spec.ts` (3 tests): empty-state when no property; mounts on the calendar tab and fires `/occupancies` for the active property; error banner on rejection. Mocks `useConfirm`. URL router covers the sync-tab triple (`/occupancy-sync/runs`, `/occupancy-api-tokens`, `/occupancy-source`) as fallbacks.
  - `InvoicesView.spec.ts` (3 tests): empty-state when no property; loads invoices on mount and renders an invoice row (`customer` object shape required to match the template’s `invoice.customer.company_name`); error banner when the initial load rejects.
  - `MessagesView.spec.ts` (3 tests): empty-state when no property; loads `/message-templates` on mount, switches to the Templates tab, renders a template title; error banner on rejection. Mocks `useConfirm`.
  - `FinanceView.spec.ts` (3 tests): empty-state when no property; loads the four parallel endpoints (categories/transactions/summary/recurring-rules) on mount; error banner on rejection. Mocks `useToast`, `useConfirm`, and `vue-router` (view uses `RouterLink`).
  - `AnalyticsView.spec.ts` (3 tests): **no** “Pick a property” prompt — the view simply guards its loaders, so the empty-state test asserts `apiMock` never hits `/analytics/*`; mount with an active property calls at least one `/analytics/` endpoint; error banner on rejection.
  - Running totals: **49 test files, 251 tests**. Verification: lint 0/0, type-check clean, build green.
- **1.16 (2026-04-24)** — Phase 2 §3.5 view smoke specs continued:
  - `DashboardView.spec.ts` (3 tests): “Select a property” prompt when no `currentId` (also asserts `apiMock` was never called); loads `/api/properties/:id/dashboard` for the active property and renders widget data (upcoming stay summary); error banner when the dashboard endpoint rejects. Seeds both property and auth stores because the view’s nav filter calls `auth.canAccessPropertyModule`.
  - Running totals: 43 test files, 233 tests. Verification: lint 0/0, type-check clean, build green.
- **1.15 (2026-04-24)** — Phase 2 §3.5 view smoke specs continued:
  - `CleaningView.spec.ts` (3 tests): “Pick a property” empty-state when no `currentId`; loads cleaning data on mount and renders log rows (asserts `apiMock` saw the seven `Promise.all` GETs for logs/summary/heatmap/fees/adjustments/settings/nuki codes); error banner when the initial load rejects. Introduces a URL-substring `apiRouter` with shaped fallbacks for every endpoint the view fires so tests only need to stub the ones they assert on.
  - Running totals: 42 test files, 230 tests. Verification: lint 0/0, type-check clean, build green.
- **1.14 (2026-04-24)** — Phase 2 §3.5 view smoke specs continued:
  - `PropertyDetailView.spec.ts` (3 tests): mounts with route `params.id = '9'`, loads property + settings in parallel (render asserts on the header + both `/api/properties/9` and `/api/properties/9/settings` GETs); error banner when the property fetch rejects; general-details PATCH submits the form and shows `General details saved.` (submits via `form.trigger('submit.prevent')` because the `UiButton` lives inside the `UiTabs` default-slot `v-if` and clicking the rendered button doesn’t propagate submit reliably through the slot). Introduces a URL+method `apiRouter` helper that matches the first handler whose `url` prefixes the call and whose method matches; the PATCH handler is listed before the GET so test assertions remain order-independent.
  - Running totals: 41 test files, 227 tests. Verification: lint 0/0, type-check clean, build green.
- **1.13 (2026-04-24)** — Phase 2 §3.5 view smoke specs continued, §3.2 audit closed:
  - `BookingPayoutsView.spec.ts` (3 tests): “Pick a property” empty-state when no `currentId`; loads payouts for the active property and renders reference numbers / guest names / amounts; error banner surfaces when the payouts endpoint rejects. Seeds `usePropertyStore.list` + `currentId` directly and routes API calls through an `apiRouter` helper that falls back to `{ payouts: [], occupancies: [] }` for the concurrent `loadOccupancyOptions()` lookahead (four months: m-2..m+1) so tests only need to stub the endpoints they actually assert on.
  - §3.2 shared API types — audited. The only cross-file name collision is `OccupancyOption`, which has **different** field sets in `InvoicesView.vue` (`summary`, `guest_display_name`, `has_payout_data`) vs `BookingPayoutsView.vue` (`source_event_uid`, `raw_summary`). These are intentionally distinct response projections, not duplication. No extraction warranted; marking §3.2 complete.
  - Running totals: 40 test files, 224 tests. Verification: lint 0/0, type-check clean, build green.
- **1.12 (2026-04-24)** — Phase 2 §3.5 view smoke specs continued:
  - `ShellView.spec.ts` (5 tests): scaffolding renders (`#main-content`, RouterView stub); topbar `@logout` emit triggers `auth.logout()` + `router.push('/login')`; mount with an already-signed-in user fetches `/api/properties`; `ensureAllowedCurrentRoute` watcher replaces to `/` when the active route declares `meta.module` the user cannot access; `/login` is exempt from the redirect. Mocks `vue-router`, shell children (AppTopbar/AppSidebar/ToastStack/ConfirmHost), `useDocumentTitle`, and the HTTP client. The route stub is a `reactive({ name, path, fullPath, meta })` mutated in place so the shell’s `watch([pid, …, () => route.fullPath])` actually fires; initial attempt used `ref` + object replacement and the watcher never re-ran.
  - Running totals: 39 test files, 221 tests. Verification: lint 0/0, type-check clean, build green.
- **1.11 (2026-04-24)** — Phase 2 §3.5 view smoke specs continued:
  - `UsersView.spec.ts` (5 tests): role labels + tones for all four roles, empty state, load-error banner, happy-path user creation (POST payload → success banner → list refresh → inputs cleared), and create-error banner.
  - `UserDetailView.spec.ts` (4 tests): mounts with route `params.id = '42'`; initial load hydrates user/perms + properties; load-error banner; DELETE flows through `useConfirm` — accepted path issues the DELETE and shows the success banner, dismissed path issues no DELETE. Mocks `useConfirm` at module level. Introduces an `apiRouter(handlers)` helper that dispatches by URL prefix, replacing fragile mockResolvedValueOnce call-order stacks. Installs an in-memory `localStorage` stub locally because the property store's `watch(currentId, …)` writes through `setItem` (jsdom's `localStorage` is read-only).
  - Running totals: 38 test files, 216 tests. Verification: lint 0/0, type-check clean, build green.
- **1.10 (2026-04-24)** — Phase 2 §3.5 view smoke specs expanded:
  - `LoginView.spec.ts` (5 tests): form render, successful login → default route redirect, `?redirect=` query honoured, error-banner surfacing from `Error` rejections, and generic fallback for non-`Error` rejections. Mocks `vue-router` `useRouter`/`useRoute` and the two API calls of `authStore.login` (POST `/api/auth/login` + GET `/api/users/:id` for permissions).
  - `PropertyFormView.spec.ts` (4 tests): default-prefilled form render, POST payload shape + navigation on success, error banner on API rejection, and cancel-without-API. `mockImplementation` dispatches on `opts.method` since both the create and the store’s follow-up `fetchList` hit `/api/properties`.
  - Running totals: 36 test files, 207 tests. Verification: lint 0/0, type-check clean, build green.
- **1.9 (2026-04-24)** — Phase 2 §3.5 view smoke specs kicked off with `PropertiesView.spec.ts`.
- **1.8 (2026-04-24)** — New composable `useTransientMessage(durationMs?)` wraps the `ref('') + setTimeout(→ '', 3000)` banner pattern. MessagesView's `success` banner now calls `showSuccess(…)` instead of mutating a ref and scheduling a reset; the composable cancels its own timer when a new message arrives, so back-to-back saves no longer flash-clear each other. OccupancyView's `copiedExport` was intentionally left alone (different semantics). 6 new unit tests.
- **1.7 (2026-04-24)** — AnalyticsView currency unification: dropped the inline `eur(cents)` (custom `€`-prefix + `toLocaleString`) in favour of the shared `formatEuros` from `@/utils/format` via the `const eur = (cents?) => formatEuros(cents ?? 0)` pattern already used by CleaningView, BookingPayoutsView, and DashboardView. Chart axis/tooltip formatters kept their in-place `Intl.NumberFormat` calls because they receive whole-euro values.
- **1.6 (2026-04-24)** — Phase 2 §3.3 shipped: ESLint 9 (flat config) + Prettier 3 + scripts.
  - New `eslint.config.js` layering `@eslint/js` recommended, `@typescript-eslint/recommended`, `eslint-plugin-vue` `flat/recommended`, and `eslint-config-prettier` (to suppress stylistic conflicts with Prettier).
  - **Regression guards**: `no-restricted-globals` + `no-restricted-syntax` make `confirm(…)`, `prompt(…)`, `alert(…)` and the `window.*` equivalents a lint error with pointers to `useConfirm` / `UiDialog` / `useToast`. A future drift back to native dialogs now fails CI before it lands.
  - New `.prettierrc.json` (no-semi, single-quote, 110 col, trailing-all) and `.prettierignore`.
  - Added scripts `lint`, `lint:fix`, `format`, `format:check`.
  - Minor cleanup forced by the first lint pass: dropped unused `computed` import from `BookingPayoutsView` and unused `vi` import from `UiDialog.spec.ts`.
  - `vue/attribute-hyphenation` and `vue/v-on-event-hyphenation` disabled: TypeScript-typed `defineProps<{ ariaLabel: string }>` in `UiLineChart` clashes with hyphenation; we leave attribute casing to authors and rely on the Vue compiler's normalisation for kebab/camel interop.
  - Final state: **lint 0 errors / 0 warnings**, type-check clean, **187/187 tests**, build green.
- **1.5 (2026-04-24)** — `useCurrentProperty` Tier 2: `ShellView` and `DashboardView` migrated. The misleading local `const props = usePropertyStore()` (shadowing Vue's component props concept) is gone; both views now use `const { pid, currentProperty, propertyStore } = useCurrentProperty()`. Watchers `() => props.currentId` → `pid`. All template `props.currentId` → `pid`. `npm run type-check` clean, **187/187 tests**, build green (ShellView +0.03 kB gz for the composable import).
- **1.4 (2026-04-24)** — `useCurrentProperty` composable adopted across 8 views (Tier 1 drop-ins).
  - Views: `MessagesView`, `BookingPayoutsView`, `OccupancyView`, `NukiView`, `FinanceView`, `CleaningView`, `InvoicesView`, `AnalyticsView`. Each drops `import { usePropertyStore }` + `const propertyStore = usePropertyStore()` + the inline `pid` (and, where present, `currentProperty`) computeds — replaced by `const { pid } = useCurrentProperty()` or `const { pid, currentProperty } = useCurrentProperty()`.
  - Deferred to a later pass: `ShellView`, `DashboardView` (both still use `props.currentId` in templates and watchers; require wider “props” → `pid`/`propertyStore` rename). `UserDetailView`, `PropertyFormView`, `PropertiesView` stay on `usePropertyStore()` because they only need `list` / `fetchList`, no `currentId` — adding the composable would be pure indirection.
  - `npm run type-check` clean, **187/187 tests**, build green. Bundle sizes unchanged (±0.01 kB gz on affected chunks).
- **1.3 (2026-04-24)** — Phase 2 §3.4 shipped: no more native `window.confirm` / `window.prompt` calls in the codebase.
  - New `composables/useConfirm.ts` + `components/ui/ConfirmHost.vue` singleton (mounted once in `ShellView`, same pattern as `ToastStack`). `confirm({ title, message, confirmLabel, cancelLabel, tone })` returns `Promise<boolean>`; concurrent calls resolve the previous promise as `false` before opening the new one so no promise leaks. Escape key / backdrop click also resolve as `false` via `update:open`.
  - Replaced **5 `confirm(…)`** sites (NukiView keypad delete, OccupancyView token revoke, UserDetailView permission remove, MessagesView template delete, FinanceView transaction delete) with styled danger dialogs.
  - Replaced the **2 `prompt(…)`** sites in FinanceView's “edit transaction” flow with a proper in-view `UiDialog` holding two labelled `UiInput`s, loading/disabled states and an inline error banner.
  - 4 new unit tests cover `useConfirm` resolve/reject/queue behaviour.
  - `npm run type-check` clean, **187/187 tests** (+4), build green. FinanceView chunk +0.34 kB gz (new dialog) — well under budget.
- **1.2 (2026-04-24)** — Phase 1 follow-up. The utilities landed in 1.1 are now actually consumed:
  - Deleted **5 copies** of the inline `eur()` helper (Dashboard/Finance/Invoices/Cleaning/BookingPayouts) — now one-line wrappers around `formatEuros`. AnalyticsView kept its inline `eur()` because it uses a hard-coded `€` prefix (locale-independent) that differs from the Intl format; conversion is scheduled with the AnalyticsView split in Phase 2.
  - Deleted **4 copies** of inline `monthKey` + `shiftMonth` (BookingPayouts/Occupancy/Finance/Cleaning) — now import from `utils/month.ts`. The util was realigned from UTC to local-time semantics to match what the views always did.
  - Deleted NukiView's local `sleep(ms)` helper in favour of `utils/async.ts`.
  - MessagesView copy flows (`copiedLang` and `cleaningCopied`) now use `useCopyFeedback(2000)` — kills two ad-hoc `setTimeout` sites while preserving the template's truthy checks.
  - `npm run type-check` clean, **183/183 tests** (+1), build succeeds. FinanceView bundle shrank 0.35 kB gz.
- **1.1 (2026-04-24)** — Phase 1 executed. `npm run type-check` clean, **182 tests passing (+29 vs. 153 baseline)**, production build green (index 118.79 kB / 46.24 kB gz — +0.09 kB gz vs. baseline, well under the 1 kB budget). Remaining raw `rgba()` are limited to `tokens.css` shadow definitions (canonical) and the green success tint inside `tokens.css`-derived `--success-bg`. No view-level raw hexes.
- **1.0 (2026-04-24)** — Initial cleanup plan. Phase 1 landed inline with this document.

## 5. Success criteria for Phase 1

- `npm run type-check` green. ✅
- `npm test` green, **+ ≥ 10 tests** target met: +29 (153 → 182). ✅
- `npm run build` output: index chunk does not regress by more than 1 kB gz. Measured +0.09 kB gz. ✅
- Zero new raw hex / rgb / rgba literals in `.vue` / `.css` outside `assets/tokens.css`. Enforced manually; automation deferred to Phase 2 via a stylelint rule. ✅
- No visual diffs on the four reference screens (`dashboard`, `occupancy`, `finance`, `analytics_performance`) beyond the focus-ring consolidation (which may be imperceptibly different due to `color-mix` rounding). Manual spot-check only at this stage.
