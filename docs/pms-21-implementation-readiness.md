# PMS 21 Implementation Readiness

Status: active final-cleanup readiness companion to
`spec/PMS_21_Legacy_Occupancy_Removal_Spec.md`. The staged migration plan is
historical. Repository JSON artifacts record a 2026-07-19 production data
audit, apply, and idempotency run, but they do not prove cleanup readiness or
authorize Release A, Release B, or destructive Release C/D. This document must
not be read as production approval.

## Historical Pre-Cleanup Agent Handoff

Pass date: 2026-07-18.

Committed snapshot: `3605fcf feat(occupancy): implement PMS 21 remediation`.

What changed in the current remediation pass:

- Added frontend PMS 21 cleaning defaults for create/promote flows: `booking_com` and `external` default cleaning on; `maintenance` and `personal_use` default cleaning off.
- Added calendar day-detail lifecycle controls for named stays: edit display name, date range, stay type, cleaning required, cancel, archive, and reactivate through PMS 21 `/stays/{stayId}` endpoints.
- Displayed Nuki error details and raw-source warning details already present in calendar DTOs.
- Updated readiness and main plan docs to reflect the owner decision to use deployment/version rollback instead of adding the wider runtime gate set.
- Added `docs/pms-21-operations-cutover-runbook.md` with production preconditions, dry-run/audit steps, apply stop point, deployment, verification, rollback, monitoring, and cleanup eligibility.
- Updated `frontend/src/api/types/README.md` to distinguish concrete generated API contract types from route-only inventory and hand-authored UI/domain adapters.
- Committed all previously dirty PMS 21 code, docs, specs, migrations, generated types, and frontend/backend changes in the snapshot above so the worktree was clean after commit.
- Added a shared Stage 2 classifier and guarded `--apply --confirm-apply` path with explicit `--allow-review-required`, idempotent mapping, integration relinking, conflict refusal, and preservation tests.
- Added migration `000036_nuki_named_stay_primary`, preserving Nuki code/event/guest-entry IDs and values while allowing named-stay-primary rows without `occupancy_id`.
- Added migration `000037_finance_evidence_confirms_named_stays`, which confirms payout/statement-backed stays that were initially classified before finance links were backfilled.
- Added named-stay update/status Nuki reconciliation, raw source-link union health recomputation, backend calendar sold semantics, PMS 21 cleaning DTO identities, and strict `named_stay_nights` analytics.
- Added complete backend route inventory in OpenAPI and a router/OpenAPI coverage regression test. Touched PMS 21 contracts have concrete schemas; entries marked `x-contract-status: route-only` identify remaining contract work.

Verification run in that pass:

- `git diff --check`
- `npm run test -- OccupancyView.spec.ts` from `frontend/`
- `npm run type-check` from `frontend/`

This handoff predates the production JSON artifacts now in the repository. Its
"no production audit/apply" statements are preserved here only as historical
context and are superseded by the evidence inventory below.

Current cleanup blockers:

- No reviewed production audit Markdown approval is recorded.
- The eight unmapped cleaning rows, one external-sale conflict, and 50
  `needs_review` stays do not have complete row-level resolution evidence in
  the repository.
- No cleanup-readiness audit proves zero runtime/data/caller dependencies.
- No accepted Release A or Release B production observation window is
  recorded.
- No cleanup-backup restore/migrate/application-start drill is recorded.
- No digest-pinned destructive cleanup approval or execution artifact exists.
- Production Nuki values/external IDs, Google IDs/ownership, finance, invoice
  files, analytics, and message preservation checks remain mandatory.

## Stage 0 Decisions

- Source of truth: [ADR-002](adr/ADR-002-raw-booking-blocks-and-named-stays.md).
- Cleaning ownership: [ADR-003](adr/ADR-003-cleaning-event-ownership.md).
- Stay type semantics: [ADR-004](adr/ADR-004-stay-type-reporting-semantics.md).
- Compatibility and export retirement: [ADR-005](adr/ADR-005-occupancy-compatibility-window.md).
- Finance import behavior: [ADR-006](adr/ADR-006-finance-import-named-stay-behavior.md).
- Final model and canonical ownership: [ADR-007](adr/ADR-007-final-occupancy-model-and-canonical-stay-ownership.md), which supersedes ADR-005 and ADR-006 without rewriting them.

## Historical Initial-Cutover Policy

Owner decision 2026-07-18: do not add the wider runtime gate set that was previously listed here. The deployment model is a version switch to the PMS 21 binary after safe migration and verification, with rollback by redeploying the prior version. Runtime flags exist only where code actually implements them and where they still protect safety-sensitive behavior:

- `PMS21_RAW_BLOCKS_DUAL_WRITE`: default off; controls additive raw-block dual-write during sync.
- `PMS21_OCCUPANCY_EXPORT_DISABLED`: default off; disables deprecated public occupancy export compatibility.
- `PMS21_OCCUPANCY_LEGACY_WRITE_DISABLED`: default off; must remain off until Nuki, finance, cleaning, analytics, messages, dashboard, and frontend lifecycle flows are safe without new legacy occupancy writes.

Collapsed cutovers use deployment rollback instead of per-area flags:

- Named-stay read model, calendar v2, date-scoped cleaning, Nuki named-stay reads, finance named-stay mapping, analytics named-stay reads, availability-block reads, and message named-stay reads ship as code in the PMS 21 version.
- Rollback means redeploying the prior backend/frontend version while preserving additive tables and columns for inspection or a later retry.
- A runtime flag is not required for each collapsed area because the owner chose a fast version cutover after verified migrations rather than long-term dual-mode operation.
- Operators must stop before deployment if the production audit or Stage 2 apply report has severe conflicts, unmapped integration rows, or unreviewed `needs_review` rows outside the approved threshold.

The policy above describes the initial additive migration. It is not the
active cleanup rollback model. After legacy writes stop, an older binary may
read stale legacy state and is not safe unless paired with its compatible
pre-release database backup and explicitly tested.

## Active Cleanup Release Policy

- Release A is non-destructive and stops creating every legacy dependency.
- Release B is non-destructive and removes runtime compatibility while legacy
  storage remains inert.
- Each release requires its own approved minimum 48-hour production window,
  representative workload, monitoring thresholds, incident restart rule, and
  named approver.
- Release C performs the destructive forward migration only after all data,
  runtime, API/caller, backup, and operational gates pass. Release D tooling
  retirement is combined with C after required diagnostics are captured.
- Cleanup audit, migration, verification, and API recreation use one approved
  immutable image digest, never a mutable tag as evidence.
- Existing-database server startup uses automatic migrations and explicitly
  leaves destructive migration `000039_legacy_occupancy_removal` pending. A
  brand-new database applies the complete final schema. The packaged
  `/app/pms21-cleanup` command is the only Release C execution path.
- No Release A/B window or Release C/D execution is recorded in this document.

Active command contract, with absolute paths and exact release identities:

```bash
/app/pms21-cleanup \
  --audit \
  --db /absolute/path/to/pms.db \
  --data-root /absolute/path/to/application-data \
  --image-digest 'sha256:<64_HEX_DIGEST>' \
  --commit '<BACKEND_COMMIT>' \
  --frontend-build '<FRONTEND_BUILD>' \
  --operator '<OPERATOR>' \
  --exception-register-reference '<APPROVED_EXCEPTION_REGISTER_REFERENCE>' \
  --approved-exceptions-reference '<APPROVED_EXCEPTIONS_REFERENCE>' \
  --analytics-parity-reference '<APPROVED_ANALYTICS_PARITY_REFERENCE>' \
  --remote-verification-reference '<APPROVED_REMOTE_VERIFICATION_REFERENCE>' \
  --caller-inventory-reference '<APPROVED_CALLER_INVENTORY_REFERENCE>' \
  > /absolute/restricted/path/PMS_21_cleanup_readiness_YYYY-MM-DD.json

/app/pms21-cleanup \
  --apply \
  --db /absolute/path/to/pms.db \
  --data-root /absolute/path/to/application-data \
  --image-digest 'sha256:<64_HEX_DIGEST>' \
  --commit '<BACKEND_COMMIT>' \
  --frontend-build '<FRONTEND_BUILD>' \
  --operator '<OPERATOR>' \
  --exception-register-reference '<APPROVED_EXCEPTION_REGISTER_REFERENCE>' \
  --approved-exceptions-reference '<APPROVED_EXCEPTIONS_REFERENCE>' \
  --analytics-parity-reference '<APPROVED_ANALYTICS_PARITY_REFERENCE>' \
  --remote-verification-reference '<APPROVED_REMOTE_VERIFICATION_REFERENCE>' \
  --caller-inventory-reference '<APPROVED_CALLER_INVENTORY_REFERENCE>' \
  --confirm-destructive-cleanup \
  --pre-report /absolute/restricted/path/PMS_21_cleanup_readiness_YYYY-MM-DD.json \
  > /absolute/restricted/path/PMS_21_cleanup_apply_YYYY-MM-DD.json
```

All angle-bracket values are placeholders, not evidence that an artifact exists
or has been approved. `--data-root` must identify the absolute application data
root against which stored invoice file paths are checked.

## Existing Stage 2 Evidence

The repository contains these immutable historical JSON artifacts:

- `docs/audits/PMS_21_production_data_audit_2026-07-19.json`
- `docs/audits/PMS_21_production_apply_2026-07-19.json`
- `docs/audits/PMS_21_production_apply_idempotency_2026-07-19.json`

They record Stage 2 migration evidence, including an idempotent second apply.
They do not resolve the non-zero exceptions, prove final value parity, prove
runtime independence, satisfy Release A/B windows, or approve destructive
cleanup. A reviewed Markdown approval and restricted row-level exception
register remain required; do not fabricate them.

## Historical Stage 2 Command Templates

The `pms21-migration` binary referenced below has been retired and removed.
These templates are historical evidence only and are not runnable Release C
cleanup instructions.

Dry-run command template:

```bash
cd backend
go run ./cmd/pms21-migration --db /absolute/path/to/verified-production-backup.db --dry-run --sample-limit 25 > ../docs/audits/PMS_21_production_data_audit_YYYY-MM-DD.json
```

Reviewed audit notes were expected to use
`docs/audits/PMS_21_production_data_audit_YYYY-MM-DD.md` and reference the raw
JSON. No reviewed Markdown artifact is currently recorded.

Historical Stage 2 apply command:

```bash
cd backend
go run ./cmd/pms21-migration --db /absolute/path/to/production.db --apply --confirm-apply --sample-limit 25 --allow-review-required > ../docs/audits/PMS_21_production_apply_YYYY-MM-DD.json
```

Omit `--allow-review-required` only when the dry run reports zero review-required named-stay candidates. Without the flag, apply stops before writing if any such candidates exist. Never use the flag to confirm those rows: override-created rows remain `review_status = needs_review`.

Do not reuse these Stage 2 commands as final-cleanup commands. The active
digest-pinned Release A-D procedure is in the operations runbook.

The report must include:

- Occupancy classifications and overlap risks.
- Raw-block, named-stay, synthetic-finance, and availability-block candidates.
- Nuki, cleaning, finance, invoice, message, dashboard, and export consumers that still depend on `occupancy_id`.
- Ambiguous external sale, closure, mapping, and unmapped records.
- Disabled/no-URL Booking.com source properties.

## Historical Stage 1 Constraint

Schema changes are additive only:

- Do not remove old columns or tables.
- Do not disable legacy write paths.
- Do not enable downstream read gates.
- Preserve legacy closure/off-market availability until replacement behavior is verified.

## Cleanup Rollback Expectations

- Before destructive cleanup, rollback uses the tested compatible image/database
  pair; redeploying an old binary alone is not automatically safe after legacy
  writes stop.
- During a quiesced Release C failure before traffic resumes, restore the
  verified pre-cleanup database and compatible image.
- After traffic resumes on the cleaned schema, fix-forward is the default.
  Restoring the cleanup backup requires owner approval and acceptance of later
  write loss unless a separate forward-recovery plan exists.
- Historical down migrations are not an operational rollback.

## Cleanup Readiness Checklist

- Record the exact backend digest, commit, frontend build, schema version,
  database fingerprint, effective PMS 21 flags, container command, and operator.
- Resolve every non-zero exception through a reviewed row-level register.
- Complete and approve Release A and Release B independently, including each
  required production window and representative workload.
- Prove zero prohibited writes in A and zero legacy runtime access in B.
- Prove removed APIs/export have no callers, including cached frontends,
  automation, support tools, workers, cron jobs, and reports.
- Capture statistics/value parity and remote Google/Nuki orphan checks.
- Take a SQLite-consistent cleanup backup, verify checksum/integrity, and record
  a restore/migrate/application-start drill using the compatible image.
- Record free disk, temporary-copy budget, measured migration/lock duration,
  maintenance-window limit, rollback authority, and traffic-resume authority.
- Run destructive cleanup only after explicit go/no-go approval; preserve all
  required pre/post reports outside the repository when they contain sensitive
  production detail and commit only redacted checksum-linked summaries.
