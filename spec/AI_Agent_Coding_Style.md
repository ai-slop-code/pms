# AI Agent Coding Style

Coding conventions for a Go backend + Vue 3 frontend project. Written for an
AI coding agent: every rule is prescriptive and the agent should produce
changes that match this document by default.

These conventions are distilled from a real working codebase and have proven
to keep diffs small, reviewable, and consistent across many features.

---

## 0. Operating principles for the agent

- **Read before you write.** Always read the surrounding file and one
  sibling file in the same package before editing, to match local patterns.
- **Make the minimum change** that satisfies the request. Do not refactor,
  rename, or "improve" code you weren't asked to touch.
- **Do not add docstrings, comments, or type annotations** to code you
  didn't change.
- **Do not invent abstractions** for one-time operations. Inline first;
  extract only when there are ≥ 2 real call sites.
- **Validate at boundaries only** — HTTP handlers, CLI flags, file I/O,
  external APIs. Do not re-validate inputs that are already trusted by the
  type system.
- **Tests live next to the code** they cover. Add at least one test for
  any new branch in business logic.

---

## 1. Repository layout

```
backend/
  cmd/<binary>/main.go            # entry points only; no business logic
  internal/
    api/                          # HTTP handlers + DTOs (one file per resource)
    store/                        # SQL access (one file per aggregate)
    <feature>/                    # pure feature packages, no HTTP awareness
    config/                       # env-var loader
    migrate/NNNNNN_<slug>.up.sql  # sequential, both up + down required
    testutil/                     # test helpers (OpenTestDB, fixtures, ...)
  go.mod
frontend/
  src/
    api/                          # typed HTTP client; all network calls here
    components/ui/                # reusable design-system primitives
    composables/                  # use*-prefixed shared logic
    router/
    stores/                       # one Pinia store per domain
    views/<Domain>/               # route-level views + co-located children
    assets/                       # global tokens, fonts
  package.json
deploy/                           # docker-compose, Dockerfiles, systemd, nginx
docs/                             # operator runbooks (deployment, backups, ...)
spec/                             # product + engineering specs
e2e/                              # Playwright tests
```

Rules:

- `cmd/<binary>` files contain `main()` and flag/env wiring only. Move
  every meaningful function to a package under `internal/`.
- `internal/api` may import `internal/store` and `internal/<feature>`.
  `internal/store` may not import `internal/api`. Feature packages may
  not import `internal/api` either — keep HTTP at the edge.
- One file per aggregate / resource. Do not split by layer-within-feature
  (no `models.go` + `service.go` + `repo.go` triplets).

---

## 2. Backend (Go)

### 2.1 Style

- Standard `gofmt` and `goimports`. CI enforces.
- Go version: pin a specific minor in `go.mod` (e.g. `go 1.25`). Bump
  deliberately, not opportunistically.
- Errors: lowercase, no trailing punctuation, no leading capital. Wrap
  with `fmt.Errorf("doing X: %w", err)` when adding context. Sentinel
  errors live next to the package that produces them
  (`store.ErrNotFound`, …). Do not use `errors.New` inside hot paths.
- Time: store and exchange UTC RFC3339 strings; convert to a user/tenant
  timezone (`time.LoadLocation(...)`) only at the rendering boundary.
- Money: integer cents (`int` or `int64`) on the wire and in the DB.
  Format only at render. Currency is a separate column.
- IDs: `int64` from auto-increment; never expose the raw DB id and a
  separate UUID unless there's a real external-reference reason.
- Logging: use the project's structured logger; do not `fmt.Println`.
  Log the *presence* of a secret (`"loaded master key"`), never the
  value.

### 2.2 HTTP handlers

- One handler per route, named `<verb><Resource>` (e.g. `createInvoice`,
  `listOccupancies`). Methods on `*Server`.
- Decode body via a small request struct defined next to the handler.
  Validate required fields explicitly; return `WriteError(w, 400, "...")`
  for client errors and `500` (with a generic message) for unexpected
  ones.
- All responses go through a shared `WriteJSON(w, status, payload)` /
  `WriteError(w, status, msg)`. Never write to `w` directly except when
  streaming a binary file.
- Audit every privileged action (`s.audit(r, actor, action, target_kind,
  target_id, outcome)`) **before** the long-running write so a crash still
  leaves a trail.
- Never trust the client for `property_id` / `tenant_id` / similar scope
  keys — derive them from the authenticated session and a "require
  module access" middleware helper.

### 2.3 Data access (`store/`)

- The store is a thin struct: `type Store struct { DB *sql.DB }`. No
  ORM. Use `database/sql` with `?` placeholders.
- Each method takes `ctx context.Context` first.
- Return rich domain structs, not `map[string]interface{}`.
- Multi-statement work that must be atomic uses `BeginTx` →
  `defer rollback` → `Commit` → `tx = nil`. Always set `tx = nil` after
  commit so the deferred rollback is a no-op.
- Idempotent writes are preferred. Where the table has a uniqueness
  constraint, use `INSERT … ON CONFLICT DO UPDATE` or a guarded `UPDATE
  … WHERE …`.
- Snapshots (e.g. invoice supplier/customer JSON, audit payloads) are
  **frozen at create time**. Refreshing one requires an explicit user
  action (e.g. "regenerate").

### 2.4 Migrations

- One file per schema change, sequentially numbered:
  `internal/migrate/NNNNNN_<short_slug>.up.sql` plus a matching
  `.down.sql`.
- Never edit a shipped migration. Add a new one.
- Default ON DELETE policy: `CASCADE` for rows owned by the parent,
  `SET NULL` for cross-aggregate references.
- Money columns end in `_cents` and are `INTEGER NOT NULL`.
- Timestamps are `TEXT NOT NULL` storing RFC3339 in UTC. Defaults via
  `strftime('%Y-%m-%dT%H:%M:%SZ', 'now')`.
- Booleans are `INTEGER` (0/1).
- Add an index for any column used in a `WHERE` clause of a hot query.

### 2.5 Tests

- Standard `testing` package. Prefer table-driven tests for pure
  functions. Use plain sequential `t.Run` blocks for stateful flows.
- Test files live next to code (`finance_handlers.go` ↔
  `finance_handlers_test.go`).
- HTTP handlers are tested through `httptest` against a real
  `Server{Store: ..., DataDir: t.TempDir()}` — **no mocks of the store**.
  This catches SQL bugs the unit tests would miss.
- Use `testutil.OpenTestDB(t)` (or equivalent) to get a fresh in-memory
  or temp-file SQLite per test. The helper applies all migrations.
- `t.TempDir()` for any filesystem work; never write to the repo or
  `/tmp` directly.
- Failing assertions use `t.Fatalf("doing X: got %v want %v", got,
  want)` — message first, then values. Never `t.Errorf` followed by
  more code that depends on the assertion.

### 2.6 Configuration & secrets

- All runtime configuration is read from environment variables in
  `backend/internal/config/config.go`. Adding a knob means:
  1. add the env var with a sensible default,
  2. surface it in `deploy/docker-compose.yml` (and the systemd unit if
     used),
  3. mention it in the relevant runbook under `docs/deployment/`.
- Production refuses to boot without required secrets. Validate in
  `config.Load()` and return a fatal error listing every missing key.
- Secrets never appear in commits, logs, or audit entries.

---

## 3. Frontend (Vue 3 + TypeScript + Vite)

### 3.1 Style

- `<script setup lang="ts">` everywhere. **No Options API.**
- Strict TypeScript. No `any` outside clearly scoped escape hatches;
  prefer `unknown` and narrow.
- Components are `PascalCase.vue`. Composables are `useXxx.ts`. Stores
  are `useXxxStore.ts`.
- Imports use the `@/` alias for `src/`.
- File order inside `<script setup>`: imports → props/emits/defineModel →
  refs/computed → methods → lifecycle hooks → watchers.

### 3.2 Components

- Generic primitives in `components/ui/` (e.g. `UiCard`, `UiButton`,
  `UiInput`, `UiBadge`, `UiTable`). **Only these are imported by views**;
  no styling escape hatches in views.
- Domain components live next to the view that owns them. Promote to
  `components/` only when used by ≥ 2 views.
- Props: typed via `defineProps<{ ... }>()`. Emits: typed via
  `defineEmits<{ (e: 'update', value: X): void }>()`.
- Two-way bindings use `defineModel<T>()` rather than manual
  `props/emit` plumbing.
- Component CSS is `<style scoped>`. Global tokens live in
  `src/assets/`.

### 3.3 State & data

- One Pinia store per domain (`useFinanceStore`, `useAuthStore`, …).
  Stores own server data + cache invalidation; views are dumb.
- All network calls go through a single `api()` helper in
  `src/api/http.ts` that handles auth headers, base URL, and error
  shape. Per-domain modules (`src/api/finance.ts`) export typed wrappers.
- API responses are typed end-to-end. The wrapper module owns the
  request/response interfaces.
- Errors surface to the user via a shared toast/notification composable.
  In components: `e instanceof Error ? e.message : 'Failed to …'`.

### 3.4 Forms

- `const form = ref<FormState>({ ... })` with a `resetForm()` function
  defined next to the form.
- Validate on submit, not on every keystroke (unless the user explicitly
  asks for live validation).
- Disable the submit button while the request is in flight; never let
  the user double-submit.

### 3.5 Styling

- No inline styles. No magic colour or spacing values.
- Use CSS custom properties: `var(--color-…)`, `var(--space-…)`,
  `var(--radius-…)`.
- Tokens are defined once in `src/assets/` and consumed everywhere.

### 3.6 Tests

- `vitest` + `@vue/test-utils`. Files are `*.spec.ts` next to the unit.
- Mock `@/api/http` with a small router helper — no real network in unit
  tests.
- Test the behaviour visible to the user (rendered text, emitted
  events), not the internal data shape.
- Playwright e2e tests live in `e2e/tests/` and run against a real
  backend started by `e2e/scripts/start-backend.sh`.

---

## 4. Database (SQLite)

- Driver: `modernc.org/sqlite` (pure Go, **CGO off**). Keeps cross-compile
  trivial and the binary statically linkable.
- Single-file DB stored under `data/`. WAL mode enabled at startup.
- Backups via the `pms-backup` / equivalent tool that snapshots the file
  while holding a SQLite backup-API handle (no `cp` of a live DB).
- Schema lives in `internal/migrate/`. See §2.4.

---

## 5. Commit messages & branches

Use **Conventional Commits**:

```
<type>(<scope>): <imperative summary>

<optional body explaining why>
```

- Types: `feat`, `fix`, `refactor`, `docs`, `chore`, `test`, `perf`,
  `ci`, `build`.
- Scope is the package or feature area (`finance`, `auth`, `ui`, `db`).
  Omit when truly cross-cutting.
- Summary is ≤ 72 chars, imperative mood, no trailing period.
- Body explains *why*, links issues, notes breaking changes.

Branches:

- `feat/<short-slug>` for features.
- `fix/<short-slug>` for bugs.
- `chore/<short-slug>` for tooling.
- One PR per branch. Squash-merge into `main`. Delete the branch on
  merge.

Releases tagged `vMAJOR.MINOR.PATCH` (SemVer).

---

## 6. Pull requests

A PR is ready to merge when:

1. Code compiles: `go build ./...` and `npm run build` (frontend).
2. Tests pass: `go test ./...` and `npm run test` (or `vitest run`).
3. Linters pass: `go vet ./...` and `npm run lint`.
4. New behaviour has at least one unit or integration test.
5. Spec docs in the affected area are updated in the same PR.
6. The diff has no unrelated reformatting or "drive-by" cleanup.

PR description template:

```markdown
## Problem
<what was broken / missing, including reproduction steps>

## Changes
- <bulleted list of meaningful edits, by file or by component>

## Validation
- go build ✅
- go test ✅
- manual: <screenshot / steps>

## Follow-ups
- <issues deliberately not fixed in this PR>
```

---

## 7. Documentation

- Product + engineering specs under `spec/`, one topic per file. Use
  descriptive names (`Auth.md`, `BillingFlow.md`) — no opaque numeric
  prefixes unless explicit ordering matters.
- Operator runbooks (backups, deploy, incident response) under `docs/`.
- A `README.md` at the repo root explains how to run the project end to
  end in under 5 minutes.
- A `Makefile` (or `justfile`) provides one-line entry points for
  every common task: `make dev`, `make test`, `make build`,
  `make migrate`, `make seed`.

---

## 8. Tooling baseline

- Go: latest stable minor, `gofmt`, `goimports`, `go vet`,
  `staticcheck` in CI.
- Frontend: Vite + TS, ESLint + Prettier (pre-configured to disagree as
  little as possible), Vitest for unit, Playwright for e2e.
- CI runs on every push: build, test, lint, type-check. No merge to
  `main` if CI is red.
- Pre-commit (optional but recommended): `gofmt`, `eslint --fix`,
  `prettier --write` on staged files.

---

## 9. Things the agent should refuse / ask about

- Deleting files, dropping tables, `git push --force`, `git reset
  --hard`, amending published commits.
- Bypassing safety checks: `--no-verify`, `--force` flags on shared
  infrastructure, disabling tests instead of fixing them.
- Adding a new dependency without a one-line justification and a check
  for an existing equivalent in the codebase.
- Generating long-lived secrets or hardcoding credentials anywhere.

For everything else: take local, reversible action freely (edit files,
run tests). Ask only when the action is destructive or affects shared
systems.
