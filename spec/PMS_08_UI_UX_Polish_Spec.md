# PMS UI/UX Polish Spec

**Version:** 1.4 (2026-04-24)  
**Pair:** [`PMS_07_Design_Language.md`](PMS_07_Design_Language.md) (what) ↔ this file (how / where).  
**Audience:** AI developer agent. Every change below is mandatory unless marked *optional*. Where this spec and `PMS_07` disagree, `PMS_07` wins.

**Changelog:**
- **1.4 (2026-04-24)** — §12.2 *Hand-drawn illustrations* shipped. Seven `Illustration*` SFCs under `frontend/src/components/illustrations/`, `UiEmptyState` gains an `illustration` prop backed by a name → component registry, Login card renders `IllustrationKeys` in the hero, Dashboard / Invoices / Cleaning / the rest of the "Pick a property" prompts wired. UiKit showcase covers the inbox / no-results / error variants.
- **1.3 (2026-04-23)** — Chart.js migration step 3 complete. Hand-rolled SVG fallbacks for the monthly-trend and pacing-series charts deleted; `UiLineChart` is now the only renderer. `VITE_USE_CHARTS` flag and `frontend/src/utils/charts.ts` retired.
- **1.2 (2026-04-23)** — Chart.js 4.x via `vue-chartjs` 5.x selected for §12.1. `UiLineChart` wrapper landed behind `VITE_USE_CHARTS=1`. Monthly-trend and pacing-series charts wired through the wrapper; hand-rolled SVG kept as `v-else` fallback per migration step 2.
- **1.1 (2026-04-23)** — Promoted *New charting library* and *Hand-drawn illustrations* from §11 Non-goals into §12 Planned v1.1. Reconciled §7 (allowed chart dep) and §3.10 (Analytics may migrate off hand-rolled SVG).

---

## 0. Reference material

Screenshots of the current UI live in `spec/fe_spec/` (`dashboard.jpeg`, `occupancy.jpeg`, `finance.jpeg`, `analytics_outlook.jpeg`, `analytics_performance.jpeg`, `analytics_demand.jpeg`, `cleaning_log_salary.jpeg`, `invoices.jpeg`, `messages.jpeg`, `nuki_access.jpeg`, `property.jpeg`). Treat these as the **before** state. The issues below derive from them; fix all of them.

### Current-state issues (summary audit)

| # | Observed | Why it hurts |
|---|----------|--------------|
| 1 | Top-of-page module nav rendered as a row of default-blue underlined links, no active state, no icons, no mobile strategy. | Looks like debug HTML. Active page unclear. Collapses messily on phone. Fails link-vs-nav semantics. |
| 2 | `<h1>` and `<h2>` visually similar; card headers often just bold text on flat background. | No visual hierarchy → user re-reads page to find the section. |
| 3 | KPI tiles: grey background, thin border, tiny label, small number. Multiple tiles per row with no grouping. | Low-value "numbers in a row" look. Nothing stands out. Eye bounces. |
| 4 | Tables: dense rows, no zebra, no hover, numbers left-aligned in the same column as text, `-` for empty, browser-default fonts. | Impossible to scan money columns. Visual noise. Inconsistent empty-state. |
| 5 | Native `<input type="file">`, default date pickers of different widths, buttons of different heights inside the same toolbar. | Breaks visual rhythm, betrays "unfinished" feel. |
| 6 | Status communicated via tiny red text badges (`Loss`, `—`, `Yes`/`No`). | Small red text = accessibility risk. `Yes`/`No` cells waste space. |
| 7 | Toolbars and sections use plain background — page feels like one giant sheet with no grouping. | Nothing tells the eye where a section ends. |
| 8 | No evidence of mobile layout. Tables exceed viewport, topbar wraps to 3 lines. | Owner can't use the app on a phone while on the road. |
| 9 | Inconsistent colour usage: ad-hoc reds, greens, blues picked per screen. | No palette discipline → inaccessible + looks amateur. |
| 10 | No visible focus indicator; reliance on default browser ring. | Keyboard users drift; WCAG 2.4.7 fail. |
| 11 | Finance, Cleaning, Invoices all use similar layouts but each implements it differently. | Repeated code, divergent affordances. |
| 12 | Long text in cells (e.g., "Booking.com payout 5087259638") wraps awkwardly or overflows. | Tables look broken on medium widths. |

---

## 1. Global shell

### 1.1 New component tree

```
App.vue
└── ShellView.vue   ← re-skinned
    ├── AppTopbar.vue         (NEW — slim, global context only)
    ├── AppSidebar.vue        (NEW — primary nav, collapsible/drawer)
    ├── <router-view/>        inside <main id="main-content">
    └── ToastStack.vue        (NEW — fixed top-right)
```

### 1.2 Topbar — `frontend/src/components/shell/AppTopbar.vue`

- Height `56 px` desktop, `52 px` mobile. Sticky. `background: var(--color-surface)`, bottom hairline.
- Left: `<button>` hamburger (mobile only, `aria-controls="app-sidebar"`) + brand lockup "PMS" (logo optional).
- Right: property picker (current `<select>` restyled as our UI select), user email button → dropdown menu with "Profile", "Logout".
- **No module links here.** The existing text-link row is deleted.

### 1.3 Sidebar — `frontend/src/components/shell/AppSidebar.vue`

- Desktop (`≥ 1024 px`): persistent, `width: 240 px`, `position: sticky; top: 56px; height: calc(100vh - 56px)`, scrollable.
- Mobile/tablet (`< 1024 px`): `position: fixed; inset: 0 auto 0 0; width: 280px;` drawer with backdrop (`rgba(15,23,42,0.4)`). Toggled by the hamburger, closed on route change, Esc, or backdrop click.
- Items (order, with Lucide icons):
  - **Operations:** Dashboard (`LayoutDashboard`), Occupancy (`Calendar`), Nuki Access (`KeyRound`), Cleaning (`Sparkles`), Messages (`MessageSquare`).
  - **Money:** Finance (`Wallet`), Booking Payouts (`Receipt`), Invoices (`FileText`).
  - **Insights:** Analytics (`BarChart3`).
  - **Admin:** Properties (`Building2`), Users (`Users`).
- Group labels (`--font-xs`, `--color-text-subtle`, uppercase, `letter-spacing: 0.04em`, `margin: var(--space-4) var(--space-3) var(--space-1)`).
- Item: `<router-link>` → rendered as `<a>` with `min-height: 40 px`, icon + label, `border-radius: var(--radius-md)`, `margin: 0 var(--space-2)`. Active state per §8.2 of `PMS_07`.
- Items are **only** rendered when the user has the corresponding permission (existing `permissions` store).
- On mobile the sidebar includes the property picker and logout at the bottom (the topbar is minimal there).

### 1.4 Routing / layout updates

- Add "Skip to main content" `<a href="#main-content" class="skip-link">` as the first child of `<body>` / `App.vue`. Visible only on focus.
- Wrap router outlet in `<main id="main-content" tabindex="-1">`.
- Remove hash-only links from the existing topbar.

---

## 2. Shared UI primitives — `frontend/src/components/ui/`

Build these first. Every module page refactored afterwards must use them; raw HTML form controls, raw `<table>`, raw buttons are no longer allowed in module views.

| Component        | Props (abridged)                                                | Notes |
| ---------------- | ---------------------------------------------------------------- | ----- |
| `UiButton`       | `variant='primary'\|'secondary'\|'ghost'\|'danger'`, `size='sm'\|'md'\|'lg'`, `loading`, `iconLeft`, `iconRight`, `block`. | Renders `<button type="button">` unless `as="a"`. Handles `aria-busy`. |
| `UiIconButton`   | `variant`, `size`, `label` (required, sets `aria-label`).        | Square. Icon-only. |
| `UiInput`        | `v-model`, `label`, `type`, `error`, `help`, `required`, `prefix`, `suffix`. | Label above, help / error below. Generates `id` + `aria-describedby`. |
| `UiSelect`       | Same API as `UiInput`, slot for `<option>`s.                     | Styled native select. |
| `UiDateInput`    | `v-model`, `label`.                                              | Wraps native `type="date"`. |
| `UiFileInput`    | `v-model`, `label`, `accept`, `filename`.                        | Hides native input, shows "Choose file" button + filename span. |
| `UiCard`         | slots: `header`, default, `footer`. Prop `tone='default'\|'sunken'`. | §8.6 of `PMS_07`. |
| `UiSection`      | `title`, `description`, slots: default, `actions`.               | `<h2>` + lede + right-aligned actions. |
| `UiKpiCard`      | `label`, `value`, `trend?` `{ delta, direction: 'up'\|'down'\|'flat', label }`, `tone?='default'\|'success'\|'warning'\|'danger'`. | Semantic trend arrow + text. |
| `UiTable`        | `columns`, `rows`, `rowKey`, `stickyHeader`, `zebra`, `empty`, `loading`. Slot per column. | Wraps `<div class="table-wrap">`. Sorting optional. |
| `UiBadge`        | `tone='neutral'\|'success'\|'warning'\|'danger'\|'info'`, `dot?`. | §8.11. |
| `UiToolbar`      | slots default + `trailing`.                                      | Flex wrap, toolbar shell. |
| `UiDialog`       | `open`, `title`, `size='sm'\|'md'\|'lg'`, slots default + `footer`. | Teleport to body, focus trap, Esc + backdrop close (unless `persistent`). |
| `UiInlineBanner` | `tone`, `title`, `icon?`. slot default.                          | Section-level alerts. |
| `UiEmptyState`   | `title`, `description`, slot `actions`.                          | Used in every empty surface. |
| `UiSkeleton`     | `variant='text'\|'rect'\|'kpi'\|'row'`, `count`.                 | Shimmer disabled under `prefers-reduced-motion`. |
| `UiTabs`         | `modelValue`, `tabs: {key,label,badge?}[]`.                      | ARIA tablist. Keyboard arrow nav. |
| `UiTag`          | `tone`.                                                          | Smaller than badge, square-ish (radius-sm). Used in chip filters. |
| `UiToast` (+ `useToast()`) | `push({ tone, title, message, timeout? })`.            | Live region. |

Put a Storybook-lite demo at `/ui-kit` (dev-only route) showing every component × every variant for manual QA. Not shipped in production bundle (guard behind `import.meta.env.DEV`).

### 2.1 Tokens file

Create `frontend/src/assets/tokens.css` with every token defined in `PMS_07` §2–§6. Import once from `main.ts`. Remove any hard-coded hex, px-size `font:`, or inline `style="background:#…"` from existing components as those components are refactored. (Use a repo-wide search for `#[0-9a-fA-F]{3,6}` during review.)

### 2.2 Base stylesheet

Create `frontend/src/assets/base.css`:

- Reset: `box-sizing: border-box`, `margin: 0`, `min-height: 100svh` on `html, body`, `font-family: var(--font-family-sans)`, `background: var(--color-bg)`, `color: var(--color-text)`.
- Focus ring: apply default `:focus-visible` ring globally (`outline: 2px solid var(--color-focus); outline-offset: 2px; border-radius: inherit;`).
- `.skip-link { position: absolute; left: -9999px; }` + `:focus { left: 8px; top: 8px; z-index: 100; }`.
- `@media (prefers-reduced-motion: reduce) { *, ::before, ::after { transition-duration: 0ms !important; animation-duration: 0ms !important; } }`.

---

## 3. Page-by-page remediation

Each section lists the **target layout**, the **minimum set of changes**, and the **mobile rules**. All pages share: new shell, `UiCard` / `UiTable` / `UiKpiCard` where applicable, no raw HTML controls, empty / loading / error states per §8.14 of `PMS_07`.

### 3.1 Dashboard (`views/DashboardView.vue`)

**Target layout (desktop):**

```
[PageHeader: "Dashboard" + selected property chip]
[Row 1 — hero KPIs (4 UiKpiCard, kpi-lg variant)]
  - Occupancy (this month)       with trend vs last month
  - Gross revenue (this month)   with trend
  - Net cashflow (this month)    tone=success/danger by sign
  - Upcoming check-ins (7 days)  neutral

[Row 2 — two cards, 2-col grid]
  - UiCard "Today at a glance": list (check-ins, check-outs, cleanings due) with dot badges.
  - UiCard "Alerts": freshness warnings (stale ICS, unmatched payouts, pin-reveal pending).

[Row 3 — two cards]
  - UiCard "Next 14 days" (mini calendar strip, colour-coded by load).
  - UiCard "Recent activity" (last 5 events: payout imported, cleaning logged, invoice sent).
```

**Changes:**

- Replace the existing tiny mixed-purpose cards with `UiKpiCard` row + two columns of task-oriented cards.
- Move "Recent activity" into a single card with timestamped items (`--font-sm` muted prefix, title bold).
- Every clickable row deep-links into the relevant module.

**Mobile:** KPIs stack 2-col on `≥ sm`, 1-col below. All secondary cards stack 1-col.

### 3.2 Properties list (`views/PropertiesView.vue`) and detail (`views/PropertyDetailView.vue`)

- List page: `UiTable` with columns Name / City / Timezone / Active / Actions. Right-aligned primary button "Add property" in PageHeader.
- Detail page: `UiTabs` along the top — **General**, **Access & ICS**, **Invoice defaults**, **Messages**, **Danger zone**. Each tab renders a `UiCard` with a `UiSection` inside.
- The "Week starts on" radio group lives under *General → Localisation* next to timezone and default language.
- Danger zone (deactivation / secret rotation) uses `tone="danger"` card with a top accent and requires typed confirmation dialog.

### 3.3 Occupancy (`views/OccupancyView.vue`)

- PageHeader with right-aligned "Sync now" `primary` button + last-sync timestamp badge.
- Toolbar: month picker, property filter (if multi-property view), view switch (Calendar / List) as a `UiTabs`.
- Calendar grid (existing SVG) moves inside a `UiCard`; legend below with semantic colour chips.
- The list view becomes a `UiTable` (Check-in / Check-out / Guest / Source / Status badge). Status uses `UiBadge` instead of text.

**Mobile:** calendar becomes week-at-a-time with horizontal swipe; list view becomes stacked cards per day.

### 3.4 Nuki Access (`views/NukiView.vue`)

- Split into two `UiSection`s: **Device status** and **Pin codes**.
- Lock status: large `UiBadge` (Online/Offline/Stale) + last-seen timestamp.
- Pin reveal: replace the current inline reveal with a `UiDialog` triggered by a `secondary` button; the dialog shows the pin, a countdown, and a "Copy" button. The dialog is `persistent` (explicit close) and logs the reveal on open.
- Keypad-code list becomes `UiTable` with Name / Window / Status / Actions.

### 3.5 Cleaning (`views/CleaningView.vue`)

- Two `UiSection`s: **Cleaning log** and **Salary**.
- Log: `UiTable` with Date / Guests / Cleaner / Duration / Amount / Linked stay. Summary KPI row above (Entries, Total hours, Total cost).
- Salary: `UiKpiCard` row (This month, YTD, Last payout) + `UiTable` for the breakdown.
- "Yearly Cleaning Stats" card — already moved to Analytics per §9; remove any remnants.

### 3.6 Finance (`views/FinanceView.vue`) — **largest rework**

Current: one huge stacked page. Target: a **tabbed** finance workspace.

- `PageHeader` title **Finance**, lede "Ledger, recurring rules, and monthly close.".
- `UiTabs`: **Overview**, **Transactions**, **Recurring rules**, **Categories**, **Monthly breakdown**.
- **Overview tab:** two rows of `UiKpiCard`:
  1. This month: Incoming, Outgoing, Balance (tone by sign), Property income, Cleaner expense, Cleaner margin %.
  2. YTD: Total incoming, Total outgoing, Net.
  Each KPI card shows a tiny `vs previous month` trend with arrow.
- **Transactions tab:** toolbar with month picker, direction filter, category filter, "Import CSV" `secondary` button (opens dialog) + "Add transaction" `primary` button (opens dialog). Table with sticky header, right-aligned amount column, status badges for `Stay mapped`, attachment icon. Long notes truncate with tooltip.
- **Recurring rules / Categories:** identical `UiTable` pattern + `UiDialog` forms.
- **Monthly breakdown:** two-card row (table + donut). Donut colours from `--viz-*`, never from semantic tokens.

Mobile: tabs become a horizontal scrollable strip; tables become stacked cards for KPIs and the monthly breakdown; transactions table keeps `overflow-x: auto` inside `.table-wrap`.

### 3.7 Booking Payouts (`views/BookingPayoutsView.vue`)

- PageHeader + toolbar (date range, source filter).
- Two KPIs (total gross, matched rate %).
- `UiTable` (Payout date / Reference / Gross / Stay link / Matched). Unmatched rows get `tone="warning"` row background (`--warning-weak`).

### 3.8 Invoices (`views/InvoicesView.vue`)

- PageHeader + "New invoice" `primary`.
- Filter chips (All / Draft / Sent / Paid / Overdue).
- `UiTable` with status badge column.
- Row click → `UiDialog` preview with download / resend buttons.

### 3.9 Messages (`views/MessagesView.vue`)

- Two-pane: left `UiCard` list of templates (searchable), right editor pane. On mobile collapses to single pane with back-button pattern.
- Editor uses sticky footer bar with "Cancel" + "Save" buttons; unsaved-changes banner on top.

### 3.10 Analytics (`views/AnalyticsView.vue`)

Already polished recently — apply the new primitives:

- Replace inline `style=""` blocks with scoped classes using tokens.
- Tabs → `UiTabs`.
- Glossary → `UiCard tone="sunken"` with disclosure toggle (already there; just reskin).
- KPI rows → `UiKpiCard`.
- Gap / orphan / unsold tables → `UiTable` with sticky header and stacked fallback on mobile.
- **Charts: Chart.js 4.x via `vue-chartjs` 5.x** rendered through `frontend/src/components/charts/UiLineChart.vue`. Lazy-imported so the library sits in its own route chunk. The `--viz-*` token palette plus the wrapper's `aria-label` + `sr-only` data-table fallback are mandatory — do not bypass the wrapper. Bar-style charts (net-per-stay, yearly cleaning, DOW occupancy) stay as HTML/CSS bars — simpler and cheaper; migrate only if interactivity is needed.

### 3.11 Users (`views/UsersView.vue` + detail)

- List: `UiTable` with role badges.
- Detail: tabs for **Profile**, **Properties & permissions**, **Sessions** (future). Permissions grid rebuilt as a responsive matrix using `UiTable` — columns are modules, rows are properties, cells contain level chips (`read`, `write`, `admin`).

### 3.12 Login (`views/LoginView.vue`)

- Centred card, 400 px max width, product name above, inputs via `UiInput`, primary button full-width size `lg`, error banner inline.
- Mobile: same card, `padding-inline: var(--space-4)`.

---

## 4. Accessibility remediation (repo-wide)

The following are concrete lint-style acceptance criteria. Each must hold in the merged UI.

| # | Check                                                                                     | How to verify |
|---|-------------------------------------------------------------------------------------------|---------------|
| A1 | Every page has exactly one `<h1>`; heading levels don't skip.                            | `grep -R "<h1" frontend/src/views` manual scan + axe audit. |
| A2 | Every `<button>` has a visible label or `aria-label`.                                    | axe DevTools. |
| A3 | Every `<img>` has `alt` (decorative → `alt=""`).                                         | axe DevTools. |
| A4 | Every form control is linked to its label (`<label for>` or wrapping).                   | axe DevTools. |
| A5 | All tables have `<caption>` or `aria-label` describing their purpose.                    | manual. |
| A6 | Colour is never the only signal (status, trends, validation include icon or text).       | screenshot diff. |
| A7 | `:focus-visible` outline ≥ 2 px on every interactive element, contrast ≥ 3 : 1 against its background. | manual. |
| A8 | Keyboard-only flow: login → dashboard → every module → nested dialog → close → return focus. | manual pass. |
| A9 | Contrast audit: body ≥ 4.5 : 1, large text ≥ 3 : 1.                                      | axe / contrast-ratio tool. |
| A10 | `prefers-reduced-motion` disables every transition > 0 ms except opacity fade ≤ 120 ms. | DevTools emulate. |
| A11 | Modal dialogs trap focus, return focus on close, close on Esc.                          | manual. |
| A12 | Toasts / banners use appropriate `aria-live` (polite for success, assertive for error). | manual. |
| A13 | Sidebar drawer on mobile: backdrop inert, `aria-modal="true"` on drawer, focus trap.    | manual. |
| A14 | Route changes announce via a visually hidden `aria-live="polite"` region that reads the new `<h1>`. | manual. |
| A15 | Language: `<html lang>` set by the user's preferred language.                           | manual. |

Add the free **`@axe-core/cli`** as a dev dependency and wire `npm run a11y` to run it against `http://localhost:5173`. Failing rules block merge.

---

## 5. Responsive rules (concrete)

- All page containers use `padding-inline: clamp(16px, 4vw, 32px)` and `max-width: 1280px` (tables-heavy pages: `1440px`).
- KPI grid: `grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));`.
- Toolbars: `display: flex; flex-wrap: wrap; gap: var(--space-3);`.
- Tables:
  - Default: wrap in `.table-wrap { overflow-x: auto; }`; sticky header works even while scrolling horizontally.
  - When `data-stack="true"` attribute is present on the `<UiTable>`, the CSS (`@media (max-width: 639.98px)`) flips rows into stacked `<dl>` cards. Use on all **summary / KPI-style** tables (e.g., cleaning breakdown, monthly categories). Keep the default for **ledger / detail** tables (transactions, booking payouts).
- Charts (SVG): set `viewBox`, `preserveAspectRatio="xMinYMid meet"`, width `100%`, `min-height: 160px`.
- Dialogs: full-viewport on `< 640 px` (`inset: 0; border-radius: 0;`); centred otherwise.
- Forms: single-column below `--bp-md`; two-column with `grid-template-columns: 1fr 1fr; gap: var(--space-4);` from `--bp-md` upward.

---

## 6. Content / copy review

Per module, replace:

| Existing                                                | Replacement                                                                                |
| ------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| "No file chosen" (browser default)                      | "No file selected" inside `UiFileInput`.                                                   |
| Empty cell values `-` or blank                          | `—` (em dash) via a shared `formatEmpty()` util.                                            |
| `Yes` / `No` in tables                                  | `UiBadge tone="success">Yes</UiBadge>` / `tone="neutral">No</UiBadge>` or replace with a check icon + `aria-label`. |
| Raw ISO dates in table cells                            | Locale short date via `Intl.DateTimeFormat`; ISO kept in `title=""` attribute.              |
| All-caps button text                                    | Sentence case.                                                                              |
| "Loss" text appended to negative numbers                | Keep the text — but additionally tone the KPI card `danger` and prefix the number with a minus sign; never rely on colour alone. |

---

## 7. Performance & bundle

- Self-host Inter & JetBrains Mono in `frontend/src/assets/fonts/` with `font-display: swap`; preload the two weights actually used (400, 600) via `<link rel="preload" as="font">` injected by Vite.
- Icons tree-shaken from `lucide-vue-next` (import-per-icon, never `import * from`).
- Keep the current code-splitting per route (Vite defaults).
- Target (v1): initial JS ≤ 120 kB gzipped after the refactor; CSS ≤ 20 kB gzipped.
- Target (v1.1 with charts + illustrations): initial JS ≤ 180 kB gzipped, lazy-loaded only on Analytics / Dashboard / empty-state routes; CSS ≤ 24 kB gzipped. Any chart library must be lazy-imported inside the route that needs it — never in the root bundle.

---

## 8. Implementation order (mandatory sequencing)

1. **Tokens + base.css + fonts + icon set.** Land first; no visual regression expected (tokens only, no component swaps).
2. **UI primitives** (`frontend/src/components/ui/*`) with dev-only `/ui-kit` demo page.
3. **Shell: topbar + sidebar + skip link + landmarks.** Regression-test routing + auth.
4. **Refactor module views** in this order (money/insights first since they were rated worst):
   1. Finance
   2. Booking Payouts
   3. Invoices
   4. Analytics (low-touch)
   5. Dashboard
   6. Occupancy
   7. Cleaning
   8. Nuki
   9. Messages
   10. Properties list + detail
   11. Users list + detail
   12. Login
5. **Accessibility pass** (axe, keyboard pass, focus audit, reduced-motion verification).
6. **Mobile pass** (375 × 667 → 414 × 896 → 768 × 1024): every page must be operable, no horizontal page scroll, tables either scroll in their wrapper or stack.
7. **Copy pass** per §6 above.

Each step ships independently. After step 2 the app looks mostly the same; the visible transformation starts at step 4.

---

## 9. Testing

- **Unit:** Vitest tests for every `Ui*` primitive covering prop variants and ARIA output (e.g., `UiButton` with `loading` exposes `aria-busy`, `UiDialog` traps focus).
- **Integration:** reuse current route tests; add smoke tests that each module route renders without console errors under the new shell.
- **Visual:** per page, add a Playwright screenshot test at the three breakpoints `375`, `768`, `1280`. Snapshots committed to `frontend/tests/visual/__screenshots__/`. (Playwright was a pending hardening follow-up anyway.)
- **Accessibility:** `npm run a11y` (axe CLI) must report zero violations on every top-level route.

---

## 10. Acceptance checklist (to copy into the PR)

- [ ] `tokens.css` landed; no hex codes outside it (grep clean).
- [ ] `base.css` loaded from `main.ts`; skip-link + focus-visible global rule present.
- [ ] All `Ui*` primitives implemented with Vitest coverage.
- [ ] `/ui-kit` dev-only route renders every component × variant without console warnings.
- [ ] `AppTopbar` + `AppSidebar` replace the old text-link top nav; module permission gating preserved.
- [ ] Every module view lists in §3 is using primitives — no raw `<table>`, `<input>`, `<button>` left.
- [ ] One `<h1>` per page; `axe` clean.
- [ ] Mobile: 375 px walkthrough passes for every route with no horizontal page scroll.
- [ ] Keyboard-only walkthrough passes from login to every module and back.
- [ ] `prefers-reduced-motion` disables non-opacity transitions.
- [ ] Playwright visual snapshots committed for `375 / 768 / 1280`.
- [ ] `PMS_03_Implementation_Checklists.md` banner + Phase 7 row added ("UI/UX polish — Done" referencing this doc + `PMS_07`).

---

## 11. Non-goals (do not do these in v1)

- Full theming (dark mode, brand colour per property).
- Changing module functionality or API contracts.
- Internationalising UI strings (locale-aware *dates/numbers* only).
- Real-time notifications / websockets.

---

## 12. Planned v1.1 (in-scope for the next iteration)

The items below were formerly non-goals. They are now committed work, tracked separately from the v1 acceptance checklist. They may land in any order once v1 is green.

### 12.1 Charting library

**Motivation.** The hand-rolled SVG charts in Analytics and the `Next 14 days` strip on Dashboard are awkward to extend (tooltips, legends, crosshair, zoom, responsive tick thinning) and every new chart duplicates geometry code. Moving to a maintained library unlocks consistent tooltips, accessible legends, and saves ~1 file per chart.

**Requirements for the chosen library:**

1. Vue 3 + `<script setup>` + TypeScript types out of the box. First-party Vue bindings preferred; a thin wrapper is acceptable if the core is framework-agnostic.
2. Tree-shakeable — we import only the chart kinds we render. No global registration of every renderer.
3. MIT / Apache-2.0 / BSD licence. No commercial tier blocking export / interactions.
4. Supports: line, bar (stacked + grouped), area, donut/pie, heatmap, mini-sparkline. (Matches what Analytics and Dashboard currently hand-roll.)
5. Themeable via CSS variables — must honour `--viz-1…--viz-8`, `--color-text`, `--color-text-muted`, `--color-border` without forking the library's own theme JSON.
6. Responsive: re-renders or reflows on container resize; works with `ResizeObserver`.
7. Accessible defaults: keyboard-navigable legend, `aria-label` on the chart root, SSR-friendly fallback text. Screen-reader parity with our current `<title>` + `<desc>` pattern — either native in the library or easy to inject.
8. Respects `prefers-reduced-motion` (animation opt-out).
9. Gzipped footprint ≤ 60 kB for the features we use (measured after tree-shake + Vite prod build).
10. Lazy-imported inside `AnalyticsView.vue` / `DashboardView.vue`. Never in the shell/root bundle.

**Candidates to evaluate (no pre-commitment):** ECharts (via `vue-echarts`), Chart.js (via `vue-chartjs`), `@unovis/vue`, Apache `@visactor/vchart`. Spike each against the 10 requirements above, pick one, record the decision in `spec/PMS_05_Analytics_Module_Spec.md` and amend §3.10 + §7 here.

**Decision (2026-04-23): Chart.js 4.x + `vue-chartjs` 5.x.** MIT-licensed, Vue 3 + TS native, explicit controller registration keeps the gz footprint to **≈ 55 kB** (Line + Bar + Tooltip + Filler), responsive via `ResizeObserver`, `prefers-reduced-motion` honoured by setting `animation: false`, CSS-variable themeable. Wrapper: `frontend/src/components/charts/UiLineChart.vue`. Decision also recorded in `spec/PMS_05_Analytics_Module_Spec.md` → *Chart rendering*.

**Migration plan.**
1. Land the dependency behind a lazy dynamic import on Analytics only. ✅ *Done 2026-04-23.*
2. Port *one* chart first (suggested: monthly revenue line) and ship behind a `VITE_USE_CHARTS=1` flag. Keep the hand-rolled SVG as fallback. ✅ *Monthly trend + pacing series ported 2026-04-23.*
3. After one iteration of real use, port the remaining Analytics charts + the Dashboard 14-day strip. Delete the hand-rolled code in the same PR. ✅ *Done 2026-04-23.* SVG fallbacks + the `VITE_USE_CHARTS` flag deleted; `UiLineChart` is the sole line-chart renderer. Bar-style HTML/CSS charts retained intentionally (see §3.10). The "Dashboard 14-day strip" has no current implementation — add via `UiLineChart` when the widget ships.
4. Keep the `--viz-*` palette + `<title>`/`<desc>` wrapper mandatory; do not let the library bring its own palette.

**Acceptance.** Zero new axe violations on Analytics. Initial Analytics route bundle (gzipped) grows by ≤ 60 kB. `prefers-reduced-motion` disables chart animation. Keyboard users can reach and read every data point.

### 12.2 Hand-drawn illustrations / hero graphics

**Status:** ✅ Done 2026-04-24.

**Motivation.** Every `UiEmptyState`, the Login hero, and the onboarding surfaces currently show icon + text only. A small set of warm, hand-drawn illustrations (sketch line style, limited palette) gives the product personality and softens data-heavy screens without adding cognitive load.

**Style constraints.**

1. Monochrome line art using `currentColor`; a secondary accent layer using `var(--color-primary)` only. No full-colour illustrations that fight the token palette.
2. Delivered as inline SVG components under `frontend/src/components/illustrations/Illustration*.vue`. One component per illustration; default export is a `<svg>` with `role="img"` and an `aria-label` prop (defaults to empty → decorative, `aria-hidden="true"`).
3. Maximum footprint per illustration: 4 kB gzipped. Prefer geometric simplicity over raster detail.
4. Scalable: every illustration has `viewBox` + `preserveAspectRatio="xMidYMid meet"`. Consumers control size via CSS `width`/`max-width`.
5. Source: either commissioned originals, or a CC0 / MIT library (unDraw-style) recoloured to our palette. License recorded in `spec/PMS_08_Illustrations_Credits.md`.

**Placements (v1.1 scope).**

| Surface                                     | Illustration                            | Notes |
| ------------------------------------------- | --------------------------------------- | ----- |
| `UiEmptyState` — no data variant             | `IllustrationEmptyInbox`                | Default for tables/lists with zero rows. |
| `UiEmptyState` — no results / filtered out   | `IllustrationNoResults`                 | When filter yields nothing. |
| `UiEmptyState` — error / retry variant       | `IllustrationError`                     | Paired with a retry button. |
| Login card (above `<h1>`)                   | `IllustrationKeys` (keys + door)        | Max-width 160 px; hidden on viewports < 400 px height. |
| Dashboard — first-time user state           | `IllustrationDashboardWelcome`          | Shown only when zero properties configured. |
| Invoices — empty / "draft your first"        | `IllustrationInvoice`                   | |
| Cleaning — empty log                         | `IllustrationSparkles`                  | |

**API change.** `UiEmptyState` gains an optional `illustration?: 'inbox' \| 'no-results' \| 'error' \| string` prop. When set, the component renders the matching illustration in the existing `icon` slot position (slot wins if both provided). Consumers don't have to import the illustration component directly.

**Accessibility.** Illustrations are decorative by default (`aria-hidden="true"`). When an illustration carries meaning (e.g., the error state), set `aria-label` on the `<svg>` and omit `aria-hidden`. Never rely on the illustration alone to convey state — the existing `title` + `description` + `tone` are still mandatory.

**Motion.** No animation in v1.1. If animation is added later, it must obey `prefers-reduced-motion`.

**Acceptance.** All seven placements above render the new illustration at 375 / 768 / 1280. Added bundle weight ≤ 24 kB gzipped total across all illustrations. axe still clean. `UiEmptyState` existing Vitest suite updated to cover the `illustration` prop.

---

### 12.3 Explicitly still out of scope

- Dark mode / per-property brand theming.
- Changing module functionality or API contracts.
- Translating UI strings (dates/numbers remain the only locale-aware content).
- Real-time notifications / websockets.
- Playwright visual-snapshot suite and `@axe-core/cli` are follow-up hardening tasks tracked separately; not blocked by §12.
