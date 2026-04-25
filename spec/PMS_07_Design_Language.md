# PMS Design Language

**Version:** 1.0  
**Status:** Canonical. All visual / interaction decisions must trace back to this document. When something is missing, extend this file first, then implement.  
**Audience:** AI developer agents and humans making UI changes.

> **Principle.** *Modern, calm, professional.* We are a property-management back-office used daily by an owner who needs to answer money / occupancy / maintenance questions quickly. No flashy gradients, no hero illustrations, no animations that don't carry information. The UI should disappear; the data and actions should stand out. Think "Linear-meets-Stripe-Dashboard", not "SaaS landing page".

---

## 1. Design principles

1. **Low cognitive load.** One primary action per screen. Secondary actions muted. Nothing blinks. No more than two levels of heading visible above the fold.
2. **Content first.** Chrome (backgrounds, borders, shadows) is the quietest thing on screen. Numbers, names, and status labels are the loudest.
3. **Predictable structure.** The same shell (topbar + sidebar + page), the same card, the same table, the same toolbar — everywhere. Learn once, use everywhere.
4. **Responsive by default.** Every page must be usable on a 375 px phone. Tables degrade to cards, sidebars collapse to a drawer, toolbars stack.
5. **Accessible by default.** WCAG 2.1 AA contrast, visible focus, keyboard-reachable everything, semantic HTML, no colour-only state.
6. **Honest feedback.** Every action has a loading, success, empty, and error state. No silent failures.
7. **Boring typography.** One typeface, five sizes, two weights. Tabular figures for money and counts.

---

## 2. Colour system

All colours are defined as CSS custom properties in `frontend/src/assets/tokens.css` and consumed via `var(--token)`. Hard-coded hex values outside the token file are a lint-level error.

### 2.1 Neutral ramp (backgrounds, borders, text)

| Token             | Hex       | Role                                                          |
| ----------------- | --------- | ------------------------------------------------------------- |
| `--color-bg`      | `#F8FAFC` | Page background (slate-50).                                   |
| `--color-surface` | `#FFFFFF` | Card, table, dialog surface.                                  |
| `--color-sunken`  | `#F1F5F9` | Subtle wells (inside-card sections, code blocks, toolbar).    |
| `--color-border`  | `#E2E8F0` | Default hairline (cards, tables, inputs at rest).             |
| `--color-border-strong` | `#CBD5E1` | Dividers that must read at a glance.                    |
| `--color-text`    | `#0F172A` | Primary text (slate-900).                                     |
| `--color-text-muted` | `#475569` | Secondary text (labels, captions). **Min size 13 px.**     |
| `--color-text-subtle` | `#64748B` | Tertiary text (placeholders, helper). **Min size 13 px.** |
| `--color-text-disabled` | `#94A3B8` | Disabled labels on white.                               |

### 2.2 Brand / interactive

| Token             | Hex       | Role                                                  |
| ----------------- | --------- | ----------------------------------------------------- |
| `--color-primary` | `#2563EB` | Primary action. Links.                                |
| `--color-primary-hover` | `#1D4ED8` | Hover / pressed.                                |
| `--color-primary-weak` | `#EFF6FF` | Primary-tinted background (selected row, active tab bg). |
| `--color-focus`   | `#3B82F6` | Focus ring outer glow (with 2 px offset).             |

### 2.3 Status / semantic

All semantic colours come in a **3-stop ramp**: `fg` (text/icon), `bg` (filled surface), `weak` (tinted background).

| Role      | `*-fg`    | `*-bg`    | `*-weak`  | Used for                                     |
| --------- | --------- | --------- | --------- | -------------------------------------------- |
| `success` | `#047857` | `#059669` | `#ECFDF5` | Positive money, active, confirmed, paid.     |
| `warning` | `#B45309` | `#D97706` | `#FFFBEB` | Estimated/partial, warn freshness, orphan.   |
| `danger`  | `#B91C1C` | `#DC2626` | `#FEF2F2` | Loss, overdue, stale, cancelled, destructive.|
| `info`    | `#1D4ED8` | `#2563EB` | `#EFF6FF` | Neutral informational tags.                  |

### 2.4 Data-visualisation palette

For charts. **Never** use semantic colours for non-semantic series.

| Token              | Hex       | Role                 |
| ------------------ | --------- | -------------------- |
| `--viz-1`          | `#2563EB` | Series 1 (primary).  |
| `--viz-2`          | `#059669` | Series 2.            |
| `--viz-3`          | `#D97706` | Series 3.            |
| `--viz-4`          | `#7C3AED` | Series 4.            |
| `--viz-5`          | `#0891B2` | Series 5.            |
| `--viz-6`          | `#DB2777` | Series 6.            |
| `--viz-grid`       | `#E2E8F0` | Grid / axis lines.   |
| `--viz-axis-label` | `#64748B` | Tick labels.         |

Heatmap ramp (0 → 1): interpolate `#F1F5F9` → `#1E40AF`. Never green/red diverging unless the metric is signed.

### 2.5 Contrast rules

- Body text (`--color-text` on `--color-surface`): 16.1 : 1 ✓ AAA.
- Muted text on surface: must compute ≥ 4.5 : 1 when ≥ 13 px; the tokens above clear this.
- Status text (e.g. `success-fg`) on its `*-weak` background: must be ≥ 4.5 : 1; the ramp above clears this.
- **Never** use pure `#999` grey for any text.

---

## 3. Typography

One family: **Inter** (system fallback `-apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif`). Self-hosted in `frontend/src/assets/fonts/` — no Google Fonts CDN (privacy + reliability).

Monospace family for IDs, codes, timestamps in tables: **JetBrains Mono** (fallback `ui-monospace, SFMono-Regular, Menlo, monospace`).

### 3.1 Scale (modular, 1.125 ratio, 16 px base)

| Token         | Size / Line           | Weight | Usage                                    |
| ------------- | --------------------- | ------ | ---------------------------------------- |
| `--font-2xs`  | 11 / 14 px            | 500    | Micro labels (chart ticks, badges).      |
| `--font-xs`   | 12 / 16 px            | 500    | Table helper text, captions.             |
| `--font-sm`   | 13 / 18 px            | 500    | Labels, muted body.                      |
| `--font-md`   | 14 / 20 px            | 400    | **Default body.**                        |
| `--font-lg`   | 16 / 24 px            | 500    | Emphasised body, KPI supporting text.    |
| `--font-h4`   | 16 / 24 px            | 600    | Sub-section heading.                     |
| `--font-h3`   | 18 / 26 px            | 600    | Card title.                              |
| `--font-h2`   | 22 / 30 px            | 600    | Page section heading.                    |
| `--font-h1`   | 28 / 36 px            | 700    | Page title. One per page.                |
| `--font-kpi`  | 24 / 30 px            | 600    | Single KPI number. Tabular figures.      |
| `--font-kpi-lg` | 32 / 38 px          | 700    | Hero KPI (dashboard headline).           |

### 3.2 Rules

- One `<h1>` per page. Page titles only.
- `<h2>` for major sections. Never more than ~5 per page.
- `<h3>` for cards / sub-sections within a section.
- **Never** use font size for emphasis inside body text — use weight or colour.
- Numbers rendered with `font-variant-numeric: tabular-nums` (enforced on `.num`, `.money`, table `td.num`, `<strong>` inside KPI cards).
- Money values: no thousands separators below 1 000 €; above, locale-default (the user's browser locale).
- Never all-caps except tiny labels `< 12 px` with `letter-spacing: 0.04em`.

---

## 4. Spacing scale

4 px base, powers of ~1.5. Always use tokens, never hard-coded `px` in components.

| Token        | Value   |
| ------------ | ------- |
| `--space-0`  | 0       |
| `--space-1`  | 4 px    |
| `--space-2`  | 8 px    |
| `--space-3`  | 12 px   |
| `--space-4`  | 16 px   |
| `--space-5`  | 24 px   |
| `--space-6`  | 32 px   |
| `--space-7`  | 48 px   |
| `--space-8`  | 64 px   |

**Vertical rhythm** between section headings and the first child: `--space-3`. Between two cards: `--space-5`. Between two sections: `--space-6`.

---

## 5. Corners, borders, elevation

| Token                | Value                                              | Usage                             |
| -------------------- | -------------------------------------------------- | --------------------------------- |
| `--radius-sm`        | 4 px                                               | Pills, table cells.               |
| `--radius-md`        | 8 px                                               | Inputs, buttons, tags.            |
| `--radius-lg`        | 12 px                                              | Cards, dialogs, toolbars.         |
| `--radius-xl`        | 16 px                                              | Hero/feature cards only.          |
| `--radius-full`      | 999 px                                             | Circular avatars, status dots.    |
| `--border-hairline`  | `1px solid var(--color-border)`                    | Default.                          |
| `--shadow-0`         | `none`                                             | Flat cards.                       |
| `--shadow-1`         | `0 1px 2px rgba(15, 23, 42, 0.04)`                 | Cards on coloured bg.             |
| `--shadow-2`         | `0 4px 12px rgba(15, 23, 42, 0.08)`                | Dropdowns, popovers, sticky bars. |
| `--shadow-3`         | `0 12px 32px rgba(15, 23, 42, 0.16)`               | Dialogs / modals.                 |

**Rule:** A page never mixes `--shadow-2` and `--shadow-3` on the same layer. Use shadow to signal *elevation*, not decoration.

---

## 6. Motion

- Standard easing: `cubic-bezier(0.2, 0, 0, 1)` ("emphasised out"). Token: `--ease-standard`.
- Durations: `--motion-1` 120 ms (hover, press), `--motion-2` 200 ms (panel slide, dialog fade), `--motion-3` 320 ms (route transition if used).
- `prefers-reduced-motion: reduce` → all transitions collapse to `0ms` except opacity. No exceptions.
- Never auto-animate data changes without a cue (e.g., KPI re-fetch). Only animate on user intent (click, drag, focus).

---

## 7. Iconography

- **Library:** [Lucide](https://lucide.dev) via `lucide-vue-next` (MIT, ~300 kB tree-shaken, one dep). All icons 1.5 px stroke, 20 px nominal.
- Inline SVG only; do not ship `<img>` icons.
- Icons must have either an accessible label (`aria-label`) or be marked `aria-hidden="true"` when paired with visible text.
- No emoji as UI icons (emoji are allowed in user-authored content such as message templates).

---

## 8. Components

### 8.1 Topbar

- Sticky, `height: 56 px` desktop / `52 px` mobile, `background: var(--color-surface)`, `border-bottom: var(--border-hairline)`.
- Contents (left → right): logo/brand, global property picker, user menu, logout.
- **Does not** contain module navigation — that lives in the sidebar (§8.2). The current topbar text-link row must be removed.
- On mobile (`< 768 px`): hamburger on the left toggles the sidebar drawer; the property picker collapses into the drawer.

### 8.2 Sidebar (primary navigation)

- `width: 240 px` desktop, fixed. Collapsed mini rail `64 px` (icon-only) when the user toggles it.
- `background: var(--color-surface)`, `border-right: var(--border-hairline)`.
- Items are **real `<a>` / `<router-link>`** with icon + label. Active item: `background: var(--color-primary-weak)`, `color: var(--color-primary)`, left accent `3px` in `--color-primary`. Hover: `background: var(--color-sunken)`.
- Groups (optional): "Operations" (Dashboard, Occupancy, Nuki, Cleaning, Messages), "Money" (Finance, Booking Payouts, Invoices), "Insights" (Analytics), "Admin" (Properties, Users).
- On mobile: the sidebar is a drawer (`position: fixed; inset: 0 auto 0 0;`) with `--shadow-3` and a scrim. Dismiss on backdrop click / Esc.

### 8.3 Page layout

```
┌─────────── Topbar (56px) ────────────┐
├─ Sidebar ──┬── Main content ──────────┤
│  (240px)   │  <PageHeader />          │
│            │  <Toolbar />?            │
│            │  <Section />…            │
└────────────┴──────────────────────────┘
```

Main content max-width: `1280 px`. Pages that are primarily tables (Finance transactions, Booking Payouts) may widen to `1440 px`; tables inside still horizontally scroll in their own wrapper rather than pushing page content.

### 8.4 PageHeader

- Row 1: `<h1>` page title, right-aligned primary CTA (if any).
- Row 2 (optional): one-line lede (14 px, `--color-text-muted`). Keep ≤ 120 chars.
- Row 3 (optional): breadcrumbs on detail pages.
- Bottom margin `--space-5`.

### 8.5 Toolbar

- Horizontal flex row, wraps on narrow widths.
- Background `var(--color-surface)`, `border: var(--border-hairline)`, `border-radius: var(--radius-lg)`, `padding: var(--space-3) var(--space-4)`.
- Contents: month/date inputs, filters (select / chips), a right-aligned primary action.
- On mobile: the toolbar becomes full-width, wraps to 2–3 rows; controls grow to `min-height: 44 px` for touch.

### 8.6 Card

- `background: var(--color-surface)`, `border: var(--border-hairline)`, `border-radius: var(--radius-lg)`, `padding: var(--space-4) var(--space-5)`, `box-shadow: var(--shadow-0)` (flat).
- Header (optional): `<h3>` + optional right-aligned meta / action. Separator below: `border-bottom: var(--border-hairline)` with `padding-bottom: var(--space-3)`.
- Cards never nest more than one level deep.

### 8.7 KPI card

- 160–240 px wide, grid `repeat(auto-fill, minmax(200px, 1fr))` with `gap: var(--space-3)`.
- Structure (top → bottom):
  1. Label (`--font-sm`, `--color-text-muted`, `text-transform: uppercase`, `letter-spacing: 0.04em`).
  2. Value (`--font-kpi` or `--font-kpi-lg`, tabular-nums).
  3. Optional trend / comparison line (`--font-xs`, coloured by direction with an arrow icon; **not colour-only** — the arrow carries the meaning).
- Never more than 6 KPI cards in one row of thought. If a section has more, split into two groups with `<h2>`.

### 8.8 Button

Three variants × three sizes. No more.

| Variant   | BG                      | Text / Border                 | Usage                           |
| --------- | ----------------------- | ----------------------------- | ------------------------------- |
| `primary` | `--color-primary`       | white                         | One per screen, main action.    |
| `secondary` | `--color-surface`     | `--color-border` / `--color-text` | Neutral actions.            |
| `ghost`   | transparent             | `--color-text-muted`          | Tertiary / toolbar / in-table.  |
| `danger`  | `--danger-bg`           | white                         | Destructive confirm only.       |

| Size | Height | Padding-x | Font        |
| ---- | ------ | --------- | ----------- |
| `sm` | 28 px  | 10 px     | `--font-sm` |
| `md` | 36 px  | 14 px     | `--font-md` |
| `lg` | 44 px  | 18 px     | `--font-md` |

- `border-radius: var(--radius-md)`.
- Min touch target on mobile: `44 × 44 px` (size `lg` or larger touch area via padding).
- Disabled: `opacity: 0.5; cursor: not-allowed;` and keep `aria-disabled="true"` on actual `<button>` elements.
- Focus: `outline: 2px solid var(--color-focus); outline-offset: 2px;`.

### 8.9 Form field

- Wrap in a `<label>` with block-level label text above the control (`--font-sm`, `--color-text-muted`, `margin-bottom: var(--space-1)`).
- Input: `height: 36 px` (desktop) / `44 px` (mobile), `border: 1px solid var(--color-border)`, `border-radius: var(--radius-md)`, `padding: 0 var(--space-3)`, `font: var(--font-md)`.
- Focus: `border-color: var(--color-primary); box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.2);`.
- Error: `border-color: var(--danger-fg);` plus a helper row below `(--font-xs, --danger-fg)`. Never rely on colour alone — include a text error message and an `aria-invalid="true"`.
- Native file inputs must be wrapped by a styled button + filename span. Never leave the browser-default "Choose file" control visible.
- Native `<select>` keeps the browser affordance but gets our border/radius/padding via `appearance: auto` (OK) — do not build a custom dropdown without a strong reason.

### 8.10 Table

- `border-collapse: separate; border-spacing: 0;` inside a `<div class="table-wrap">` that owns horizontal scroll (`overflow-x: auto`).
- `thead th`: `background: var(--color-sunken)`, `--font-sm`, `--color-text-muted`, uppercase, `letter-spacing: 0.04em`, `padding: var(--space-2) var(--space-3)`, `border-bottom: 1px solid var(--color-border-strong)`, sticky (`position: sticky; top: 0;`) when the table is taller than the viewport.
- `tbody td`: `padding: var(--space-3)`, `border-bottom: 1px solid var(--color-border)`.
- Zebra optional, off by default. Turn on (`tbody tr:nth-child(even) { background: var(--color-sunken); }`) for rows > 20.
- Row hover: `background: var(--color-primary-weak);` (cursor stays default unless the row is clickable).
- **Number columns** right-aligned, `.num` class (tabular-nums).
- **Status columns** use badges, not raw text.
- Empty cell value: em dash `—`, not `-`, not empty.
- Mobile (`< 640 px`): the `.table-wrap` keeps horizontal scroll for detail tables, but **summary tables** (per-property dashboards, monthly KPI rollups) convert to a **stacked list card** via `.table-stack` at this breakpoint: each row renders as a card with `grid-template-columns: auto 1fr` pairs of `<dt>` / `<dd>`.

### 8.11 Badges / status pills

- Shape: `border-radius: var(--radius-full)`, `padding: 2px 10px`, `--font-xs`, `font-weight: 600`.
- Colour: `*-weak` background, `*-fg` text. Success / warning / danger / info / neutral.
- A status pill **always** carries text; never icon-only. Optional leading 6 px dot in `*-bg`.

### 8.12 Dialog / modal

- `max-width: 640 px` (form dialogs) / `960 px` (data dialogs), centred, `--shadow-3`, `--radius-lg`, `padding: var(--space-5) var(--space-5)`.
- Title row: `<h2>` + close `×` (IconButton, `aria-label="Close"`).
- Body scrolls internally; footer (buttons) sticks to bottom.
- Backdrop: `rgba(15, 23, 42, 0.4)`, dismissible on Esc and backdrop click unless the dialog is destructive.
- Focus trap inside, focus returned to trigger on close.

### 8.13 Toast / inline banner

- **Inline banner** (preferred for validation / page-level warnings): full-width inside its section, `*-weak` bg, `*-fg` icon + text, `--radius-md`, `border-left: 4px solid var(--*-fg)`.
- **Toast** (preferred for async success / error): top-right stack, `--shadow-2`, auto-dismiss 5 s for success / 10 s for error, manual close always available, live-region `aria-live="polite"`. Errors must be dismiss-only + `aria-live="assertive"`.

### 8.14 Empty / loading / error states

Every async surface defines **four** states and the UI must show the matching one.

- **Loading:** skeletons that mirror the real layout (KPI block → 4 grey boxes, table → 6 grey rows). Never a centred spinner taking the full page unless > 2 s.
- **Empty:** short title ("No transactions yet"), one-line explanation, one primary CTA (if any). Uses the `EmptyState` component with optional icon.
- **Error:** red inline banner with the human-readable message (never the raw stack), plus a "Retry" `ghost` button.
- **Partial / stale:** warning banner at the top of the surface explaining what's missing — do not hide the data.

---

## 9. Accessibility contract

1. **Landmarks.** Every page has `<header>` (topbar), `<nav>` (sidebar + any in-page tab nav with `aria-label`), `<main>`, and optional `<aside>`. Include a "Skip to main content" link as the first focusable element (visually hidden until focused).
2. **Headings.** One `<h1>` per page. Don't skip levels. Tabs render `role="tablist"` / `role="tab"` / `role="tabpanel"`.
3. **Keyboard.** Every interactive element reachable with `Tab`. Tab panels switchable with `←`/`→`. Dialogs trap focus and restore it. Tables sortable with `Enter`/`Space` on header cells.
4. **Focus.** Visible ring on every focusable element. Never `outline: none` without a replacement.
5. **Colour.** Contrast ≥ 4.5 : 1 for body, ≥ 3 : 1 for large text + UI components. Status is never colour-only — always a glyph or a label.
6. **ARIA.** Prefer semantic HTML. Use ARIA only when semantic HTML is insufficient (custom tabs, toasts, dialogs). Never put `role="button"` on a `<div>` unless you also wire keyboard handlers.
7. **Forms.** Every input has a programmatic label. Error messages are associated via `aria-describedby`. Required fields marked with both `required` and a visible "Required" label (not just `*`).
8. **Language.** `<html lang="…">` reflects the actual UI language. Dates rendered with locale-aware formatting — no raw ISO strings in user-facing table cells (keep ISO in `title` for hover / parsing).
9. **Motion.** Honour `prefers-reduced-motion`.
10. **Touch.** 44 × 44 px minimum touch target; 8 px minimum gap between adjacent targets.

---

## 10. Responsive breakpoints

| Token       | Min-width | Behaviour                                               |
| ----------- | --------- | ------------------------------------------------------- |
| `--bp-sm`   | 480 px    | Small phone landscape. KPI grid 2 columns.              |
| `--bp-md`   | 768 px    | Tablet. Sidebar stays as drawer until `--bp-lg`.        |
| `--bp-lg`   | 1024 px   | Sidebar becomes persistent.                             |
| `--bp-xl`   | 1280 px   | Main content hits its max-width cap.                    |

Mobile-first. Base styles assume ≤ 479 px (single-column, stacked toolbars, drawer sidebar, tables as stacked cards for summary data).

---

## 11. Content & copy rules

- **Sentence case** everywhere ("Add transaction", not "Add Transaction"). Exception: brand names and product module names ("Booking Payouts" because it's the module name).
- **Verbs in buttons** ("Save changes", "Refresh data"), **nouns in headings** ("Transactions", not "View transactions").
- Money: always include the currency. Loss values are coloured `--danger-fg` and accompanied by a "Loss" badge — the colour alone is not the signal.
- Dates: `MMM d, yyyy` or locale short date; never `23/04/2026` alongside `2026-04-23` in the same view.
- Empty values render `—`.
- No exclamation marks in system copy. No emoji. No "oops".

---

## 12. Tokens file shape

`frontend/src/assets/tokens.css` exports all tokens under `:root {}`. A dark-mode `[data-theme="dark"] {}` block is prepared but **not enabled in v1** — tokens must be named by semantic role (`--color-text`, not `--slate-900`) so a future dark palette drops in without touching components.

```css
:root {
  /* Colour: neutrals */
  --color-bg: #f8fafc;
  --color-surface: #ffffff;
  /* … */

  /* Typography */
  --font-family-sans: "Inter", -apple-system, …;
  --font-md: 400 14px/20px var(--font-family-sans);
  /* … */

  /* Spacing */
  --space-1: 4px;
  /* … */

  /* Motion */
  --motion-1: 120ms;
  --ease-standard: cubic-bezier(0.2, 0, 0, 1);
}
```

Components import tokens indirectly via class / Vue scoped styles. Component files **never** redefine a token; they may compose tokens into local component aliases (e.g. `.btn-primary { background: var(--color-primary); }`).

---

## 13. Out of scope (v1)

- Dark mode (tokens ready, palette not defined).
- Theming per property (brand colour override).
- Internationalisation beyond date/number locale.
- Custom dropdowns, date-range pickers, or charting libraries. Stay with native controls + hand-rolled SVG.
- Micro-animations (confetti, Lottie, etc.) — explicitly forbidden.

---

## 14. Change-management

When a new pattern is needed:

1. Propose it in `PMS_07_Design_Language.md` with a rationale and a component sketch.
2. Add tokens / components to `tokens.css` and `frontend/src/components/ui/`.
3. Migrate at least one existing page to the new pattern in the same PR.
4. Update `PMS_08_UI_UX_Polish_Spec.md` if the change alters a module's page.
