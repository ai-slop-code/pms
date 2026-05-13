# PMS Coding Conventions

This document captures the coding-style conventions used across the PMS
codebase. It is intentionally short and prescriptive — the goal is that any
contributor (human or AI) can read it once and produce changes that match
the rest of the repository.

---

## 1. Commit messages

Every commit title must follow the format:

```
<TYPE-NN>: <imperative summary>
```

Rules:

- `TYPE` is one of:
  - `FEAT` — user-visible new capability
  - `FIX` — bug fix or regression
  - `REFACTOR` — internal restructuring with no behaviour change
  - `DOC` — documentation only
  - `CHORE` — tooling, build, deps, config
  - `TEST` — tests only
- `NN` is a zero-padded sequence number, scoped per release / feature
  branch. Numbers reset per branch; they only need to be unique within the
  set of commits being merged.
- Summary is in the **imperative mood**, present tense, no trailing period,
  ≤ 72 characters total (including the prefix).
- Use the body (separated by a blank line) for *why*, screenshots, links,
  or breaking-change notes when the title isn't enough.

### Examples

```
FEAT-01: SK/EN invoice language with localised PDF rendering
FEAT-02: property billing & tax fields (IČO, DIČ, VAT ID, billing address)
FIX-01:  use DATA_DIR (not PMS_INVOICE_DIR) in compose & systemd env
FIX-02:  drop Slovak leftovers from English invoice PDF
FIX-03:  refresh supplier snapshot from property profile on PDF regenerate
DOC-01:  add PMS v1.1 implementation plan
TEST-01: cover invoice supplier snapshot refresh on regenerate
CHORE-01: bump go to 1.26 in Dockerfile.backend
```

### Squashing

When multiple small commits address the same concern, prefer to squash them
into a single commit using one identifier rather than chaining several.
Example: ten small fixups during invoice work → one `FEAT-01: ...` commit.

---

## 2. Branches & releases

- Feature branches: `feature/<version>` (e.g. `feature/v1.1.0`) for the set
  of changes targeted at a release.
- Hotfix branches: `fix/<short-slug>` off the current release tag.
- Release tags: `vMAJOR.MINOR.PATCH` (e.g. `v1.1.0`).

Each release that ships behaviour changes gets a corresponding
`spec/PMS_<NN>_<version>_Implementation_Plan.md` file describing the work,
written *before* the work starts and updated as it lands.

---

## 3. Backend (Go)

### Layout

- `backend/cmd/<binary>/` — entry points only; no business logic.
- `backend/internal/api/` — HTTP handlers + DTOs. Naming: one file per
  resource (`invoice_handlers.go`, `cleaning_handlers.go`, …).
- `backend/internal/store/` — SQL access. One file per aggregate
  (`invoices.go`, `properties.go`).
- `backend/internal/<feature>/` — pure feature packages with no HTTP
  awareness (`invoicepdf`, `nuki`, `occupancy`, …).
- `backend/internal/migrate/NNNNNN_<slug>.{up,down}.sql` — sequentially
  numbered, both directions required.

### Style

- Standard `gofmt` / `goimports`. CI enforces.
- Errors: lowercase, no trailing punctuation. Wrap with `fmt.Errorf("...:
  %w", err)` when adding context. Domain errors live next to the package
  that produces them (`store.ErrInvoiceNotFound`, …).
- Time: store and exchange UTC RFC3339 strings; convert to property
  timezone (`time.LoadLocation(property.Timezone)`) only at the rendering
  boundary.
- Money: cents (`int`) on the wire and in the DB; format only at render.
- HTTP responses go through `WriteJSON` / `WriteError`. Never write to
  `w` directly from a handler unless streaming a binary file.
- Audit every privileged action via `s.audit(...)`. Log *before* a
  long-running write so a crash still leaves a trail.
- Snapshots (e.g. invoice supplier/customer JSON) are **frozen at create
  time**. Refreshing one requires an explicit user action (regenerate).

### Tests

- Use the standard `testing` package; prefer table-driven tests.
- Place tests next to the code (`*_test.go`).
- HTTP handlers are tested through `httptest` against a real
  `Server{Store: ..., DataDir: t.TempDir()}` — no mocks of the store.
- `t.TempDir()` for any filesystem work; never write to the repo.

---

## 4. Frontend (Vue 3 + TS)

### Layout

- `frontend/src/views/<Domain>/` — route-level views. Files are
  `PascalCase.vue`. Co-locate sub-components in the same folder.
- `frontend/src/components/ui/` — generic, reusable design-system
  primitives (`UiCard`, `UiButton`, `UiInput`, `UiBadge`, …). Only these
  are imported by views; no styling escape hatches.
- `frontend/src/api/` — typed HTTP client. All network calls go through
  `api()` from `@/api/http`.
- `frontend/src/stores/` — Pinia stores; one per domain.
- `frontend/src/composables/` — reusable composables (`useCurrentProperty`,
  …). Prefix with `use`.

### Style

- `<script setup lang="ts">` everywhere. No Options API.
- Strict typing. No `any` outside of clearly scoped escape hatches; prefer
  `unknown` and narrow.
- Forms: `ref<FormState>(...)` with `defineModel` for two-way bindings.
  Reset functions live next to the form definition.
- API errors: `e instanceof Error ? e.message : 'Failed to ...'`.
- No inline styles or magic colour values — use CSS custom properties
  (`var(--color-...)`, `var(--space-...)`).
- Component CSS is `<style scoped>`. Global tokens live in
  `frontend/src/assets/`.

### Tests

- `vitest` + `@vue/test-utils`, files named `*.spec.ts` next to the unit
  under test.
- Mock `@/api/http` with a small router helper (see
  `PropertyDetailView.spec.ts`) — no real network in unit tests.

---

## 5. Database & migrations

- SQLite via `modernc.org/sqlite` (CGO off). Pure-Go driver.
- Every schema change is a new `up` + `down` migration. Never edit a
  shipped migration; add a new one.
- Soft deletes are the exception, not the rule. Default is `ON DELETE
  CASCADE` for owned rows, `ON DELETE SET NULL` for cross-aggregate
  references.
- Money columns end in `_cents` and are `INTEGER`. Currency is a separate
  column.
- Timestamps are `TEXT NOT NULL` storing RFC3339 in UTC.

---

## 6. Spec & documentation

- All product / engineering specs live under `spec/PMS_NN_<topic>.md`.
  `NN` is monotonically increasing across the lifetime of the project so
  cross-references are stable.
- Implementation plans for releases are dedicated documents
  (`PMS_NN_v<version>_Implementation_Plan.md`) and double as the
  changelog source.
- This file (`PMS_13_Coding_Conventions.md`) is the canonical reference
  for coding style. Update it when a convention changes; do not duplicate
  the rules into module specs.

---

## 7. Configuration & secrets

- All runtime configuration is read from environment variables in
  `backend/internal/config/config.go`. Adding a knob means:
  1. add the env var to `config.go` with a sensible default,
  2. surface it in `deploy/docker-compose.yml` and
     `deploy/systemd/pms-server.service`,
  3. mention it in the relevant runbook under `docs/deployment/`.
- Secrets never appear in commits, logs, or audit entries. Logging the
  *presence* of a secret is fine; logging the value is not.
- Production refuses to boot without `PMS_MASTER_KEY`,
  `SESSION_SECRET`, and metrics auth — see `config.Load()`.

---

## 8. Definition of done

A change is ready to merge when:

1. Code compiles (`go build ./...`, frontend `vite build`) and tests pass
   (`go test ./...`, `vitest run`).
2. New behaviour has at least one unit or integration test.
3. Spec docs that describe the changed area are updated in the same PR.
4. Commit titles follow §1.
5. The diff doesn't include unrelated reformatting or "drive-by" cleanup.
