# PMS Software Audit Report (2026-04-13)

## 1) Audit Scope and Method

This audit compares the current implementation against:

- `spec/README.md`
- `spec/PMS_01_Architecture_and_Global_Spec.md`
- `spec/PMS_02_Module_Specifications.md`
- `spec/PMS_03_Implementation_Checklists.md`

Audit method:

1. Spec-to-code traceability review (backend, frontend, migrations, tests)
2. Production-readiness review (security, reliability, architecture, operability)
3. Gap and risk rating by severity (Critical / High / Medium / Low)

---

## 2) Executive Assessment

### Overall maturity

- **Functional maturity:** strong progress in Foundations, Occupancy, Nuki, Cleaning, and Finance.
- **Production maturity:** **not yet commercial-grade** without hardening and missing modules.
- **Spec conformance:** partial; core v1 scope in architecture spec is not fully met (notably Invoices and Messages).

### Commercial readiness verdict

- **Current verdict: NOT READY for commercial launch**.
- Main blockers:
  - security hardening gaps
  - missing v1 business modules (Invoices, Message Templates)
  - operability/scalability shortcomings for multi-tenant production operation

---

## 3) What Is Implemented Well

- Solid modular backend structure with clear domain packages (`api`, `store`, `occupancy`, `nuki`, `migrate`).
- Role/property/module permission checks are implemented server-side (`backend/internal/api/property_access.go` + route handlers).
- Occupancy sync pipeline is robustly modeled (raw events + normalized events + sync runs).
- Nuki integration has meaningful lifecycle handling and reconciliation paths.
- Cleaning analytics and salary computation are present with fee history and adjustments.
- Finance module is substantial (transactions, categories, recurring rules, month-open generation, payout import/rematch/mapping).
- Automated backend tests exist for key modules (`internal/occupancy`, `internal/nuki`, `internal/store`, API tests).

---

## 4) Spec Compliance Snapshot

### Global architecture/spec alignment

- **Aligned:** Go + Vue + SQLite architecture, modular monolith, migrations in place.
- **Partially aligned:** service-layer separation is inconsistent (some handler-heavy business logic).
- **Not aligned yet:** full v1 scope requiring Invoices + Message Templates.

### Module status vs spec

- **Global Platform:** mostly implemented.
- **Occupancy/ICS:** implemented with good coverage.
- **Nuki:** implemented with substantial lifecycle support.
- **Cleaning:** implemented with monthly analytics and finance linkage.
- **Finance:** implemented and materially complete for v1 finance scope.
- **Invoices:** not implemented.
- **Customer Messages:** not implemented.
- **Dashboard:** present but still mostly partial widgets.

---

## 5) Findings (Prioritized)

## Critical

- **C1 — Incomplete v1 scope for commercial release**
  - **Evidence:** no invoice/message module routes, stores, migrations, or views matching `PMS_01` + `PMS_02` required v1 scope.
  - **Impact:** contract/spec non-fulfillment for stated product scope; revenue-impacting workflows unavailable.
  - **Recommendation:** explicitly re-baseline scope or deliver Invoices + Message Templates before launch.

## High

- **H1 — Session cookie configured insecure for production**
  - **Evidence:** `backend/internal/api/server.go` sets auth cookie with `Secure: false`.
  - **Impact:** session token exposure risk on non-TLS paths/misconfigurations; weak production security posture.
  - **Recommendation:** make `Secure` environment-driven (default true in production), add strict TLS deployment profile.

- **H2 — Sensitive automation token likely leaks through request logging**
  - **Evidence:** occupancy export uses query token (`/occupancy-export?token=...`) in `server.go`; global `chi` logger middleware logs request URLs in `backend/cmd/server/main.go`.
  - **Impact:** token exposure in logs/observability pipelines; unauthorized export risk if logs are accessed.
  - **Recommendation:** move automation auth token from query to header, or redact query logging globally.

- **H3 — Nuki PIN is stored and returned in plaintext**
  - **Evidence:** `generated_pin_plain` persisted in `backend/internal/store/nuki.go`; returned by `listNukiUpcomingStays` in `backend/internal/api/nuki_handlers.go`.
  - **Impact:** high sensitivity secret-at-rest exposure; broad read access may reveal door access codes.
  - **Recommendation:** avoid plaintext persistence, show PIN once on generation, encrypt at rest if temporary storage is unavoidable, restrict API exposure by role/action.

## Medium

- **M1 — Frontend module navigation is not permission-aware**
  - **Evidence:** `frontend/src/views/ShellView.vue` renders most module links unconditionally (except Users).
  - **Impact:** UX inconsistency and permission confusion; easier privilege probing.
  - **Recommendation:** load module permissions in shell and hide routes/links accordingly (backend checks remain authoritative).

- **M2 — Non-transactional multi-write flows can create partial finance state**
  - **Evidence:** payout import loops create/update transactions and payout mappings without wrapping per-row atomic transaction in `backend/internal/api/finance_handlers.go`.
  - **Impact:** partial write scenarios (e.g., transaction created but payout row fails) reduce accounting consistency.
  - **Recommendation:** use per-row DB transaction boundary for create tx + payout + mapping.

- **M3 — Attachment handling deviates from architecture guidance and trust boundaries**
  - **Evidence:** stored path layout is `attachments/<property>/<timestamp_filename>` in `saveFinanceAttachment`; JSON API accepts direct `attachment_path` from client in `parseTransactionCreateRequest`.
  - **Impact:** weaker traceability and potential path spoofing metadata; diverges from intended `<property>/<transaction>/<filename>` pattern in architecture spec.
  - **Recommendation:** remove client-settable attachment path, persist only server-generated file refs, and align storage hierarchy with transaction ID.

- **M4 — Throughput and scalability bottlenecks**
  - **Evidence:** DB pinned to one open connection (`db.SetMaxOpenConns(1)` in `backend/internal/dbconn/dbconn.go`); some lookups fetch full lists then filter in memory (`GetFinanceTransactionByID`, `GetFinanceRecurringRuleByID` in `backend/internal/store/finance.go`).
  - **Impact:** performance degradation as tenants/data grow.
  - **Recommendation:** replace list-then-filter methods with targeted SQL queries; evaluate SQLite write/read contention strategy and migration plan.

- **M5 — Scheduler design not multi-instance safe**
  - **Evidence:** schedulers run in-process in `backend/cmd/server/main.go` with no leader election or distributed lock.
  - **Impact:** duplicate background work if multiple app instances run (future scaling/deployment risk).
  - **Recommendation:** add job lock table / lease / external scheduler for singleton job execution.

- **M6 — Dashboard remains partially placeholder vs module-level expectations**
  - **Evidence:** dashboard response includes placeholders for several widgets in `backend/internal/api/server.go`; frontend also notes minimal widgets (`frontend/src/views/DashboardView.vue`).
  - **Impact:** reduced operational value and incomplete cross-module visibility for production operations.
  - **Recommendation:** implement remaining widget data paths and permission-aware visibility.

## Low

- **L1 — API shape inconsistency and naming drift**
  - **Evidence:** dashboard endpoint differs from suggested module path conventions (`/api/dashboard/summary?property_id=...` vs per-property REST-style patterns in specs).
  - **Impact:** small maintainability/API ergonomics issue.
  - **Recommendation:** keep backward compatibility but add canonical per-property endpoint and deprecate gradually.

---

## 6) Architecture and Design Observations

- Domain model direction is mostly strong and consistent with future PostgreSQL migration goals.
- Business logic placement is mixed: some areas use service-driven orchestration (Occupancy/Nuki), while parts of Finance remain handler-centric.
- Good use of sync run entities and explicit statuses improves auditability.
- Security posture needs focused hardening pass before commercial use.

---

## 7) Test and Quality Posture

- Positive:
  - backend test coverage exists for major modules and key workflows
  - recent finance booking payout mapping/backfill tests are present
- Gaps:
  - no invoice tests (module absent)
  - no message template rendering tests (module absent)
  - no frontend automated test suite observed for critical UI flows
  - no explicit performance/load or security test harness

---

## 8) Recommended Remediation Plan

### Phase A (Immediate launch blockers)

1. Fix cookie security configuration for production.
2. Eliminate query-token logging risk on occupancy export.
3. Remove plaintext Nuki PIN persistence/exposure model.
4. Decide scope truthfully: either implement Invoices + Messages or formally mark them post-v1.

### Phase B (Commercial hardening)

1. Wrap financial import/rematch multi-write operations in DB transactions.
2. Make frontend navigation permission-aware.
3. Align attachment storage and metadata model to architecture spec.
4. Implement singleton-safe background scheduling strategy.

### Phase C (Scale and maintainability)

1. Remove full-list lookup anti-patterns in store layer.
2. Add performance profiling and query optimization.
3. Expand dashboard widgets and operational observability.
4. Add automated frontend tests for high-risk user flows.

---

## 9) Final Audit Conclusion

The software is **well progressed and technically promising**, with strong delivery in operational modules (Occupancy, Nuki, Cleaning, Finance).  
However, it is **not yet production-grade for commercial sale** under the current specification baseline due to:

- unresolved security hardening issues,
- missing v1 business modules (Invoices, Messages),
- and several medium-risk architecture/operability gaps.

With the remediation plan above, the project can move from “advanced internal beta” to “commercial-ready” in a structured way.
