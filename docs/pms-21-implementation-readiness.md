# PMS 21 Implementation Readiness

Status: active readiness companion to `spec/PMS_21_Raw_Booking_Blocks_Named_Stays_Migration_Plan.md`. Local Stage 0 through Stage 11 implementation artifacts exist where referenced by the main plan, but production audit, Stage 2 apply/backfill, production gate enablement, and destructive cleanup remain blocked. This document must not be read as production approval.

## Stage 0 Decisions

- Source of truth: [ADR-002](adr/ADR-002-raw-booking-blocks-and-named-stays.md).
- Cleaning ownership: [ADR-003](adr/ADR-003-cleaning-event-ownership.md).
- Stay type semantics: [ADR-004](adr/ADR-004-stay-type-reporting-semantics.md).
- Compatibility and export retirement: [ADR-005](adr/ADR-005-occupancy-compatibility-window.md).
- Finance import behavior: [ADR-006](adr/ADR-006-finance-import-named-stay-behavior.md).

## Rollout Policy

Owner decision 2026-07-18: do not add the wider runtime gate set that was previously listed here. The deployment model is a version switch to the PMS 21 binary after safe migration and verification, with rollback by redeploying the prior version. Runtime flags exist only where code actually implements them and where they still protect safety-sensitive behavior:

- `PMS21_RAW_BLOCKS_DUAL_WRITE`: default off; controls additive raw-block dual-write during sync.
- `PMS21_OCCUPANCY_EXPORT_DISABLED`: default off; disables deprecated public occupancy export compatibility.
- `PMS21_OCCUPANCY_LEGACY_WRITE_DISABLED`: default off; must remain off until Nuki, finance, cleaning, analytics, messages, dashboard, and frontend lifecycle flows are safe without new legacy occupancy writes.

Collapsed cutovers use deployment rollback instead of per-area flags:

- Named-stay read model, calendar v2, date-scoped cleaning, Nuki named-stay reads, finance named-stay mapping, analytics named-stay reads, availability-block reads, and message named-stay reads ship as code in the PMS 21 version.
- Rollback means redeploying the prior backend/frontend version while preserving additive tables and columns for inspection or a later retry.
- A runtime flag is not required for each collapsed area because the owner chose a fast version cutover after verified migrations rather than long-term dual-mode operation.
- Operators must stop before deployment if the production audit or Stage 2 apply report has severe conflicts, unmapped integration rows, or unreviewed `needs_review` rows outside the approved threshold.

## Required Production Audit Artifact

Before any production backfill/cutover runs, save a read-only audit report with the counts and risk classes listed in `spec/PMS_21_Raw_Booking_Blocks_Named_Stays_Migration_Plan.md` under Production Data Audit. The expected reviewed artifact name is `docs/audits/PMS_21_production_data_audit_YYYY-MM-DD.md` or an equivalent operational artifact path recorded in the cutover notes.

Current repository state: no production audit artifact is present. Do not fabricate one. The owner will run the production audit.

Dry-run command template:

```bash
cd backend
```

Stage 2 apply command state: apply mode is not implemented in the current CLI. Stop before production apply until the command exists with explicit apply and confirmation flags, idempotency tests pass, and the output artifact format includes created, updated, skipped, conflict, and review-required counts.

The report must include:

- Occupancy classifications and overlap risks.
- Raw-block, named-stay, synthetic-finance, and availability-block candidates.
- Nuki, cleaning, finance, invoice, message, dashboard, and export consumers that still depend on `occupancy_id`.
- Ambiguous external sale, closure, mapping, and unmapped records.
- Disabled/no-URL Booking.com source properties.

## Stage 1 Constraint

Schema changes are additive only:

- Do not remove old columns or tables.
- Do not disable legacy write paths.
- Do not enable downstream read gates.
- Preserve legacy closure/off-market availability until replacement behavior is verified.

## Rollback Expectations

- Additive schema can remain in place during rollback.
- Dual-write can be disabled by flag while retaining new rows for inspection.
- Cleaning rollback preserves Google event IDs and desired hashes.
- Nuki rollback preserves generated PINs, external Nuki IDs, valid windows, revocation history, and legacy occupancy links.
- Finance rollback preserves both `occupancy_id` and `named_stay_id` links.
- Analytics rollback switches reads back to legacy tables and logs the active read model.
- Collapsed PMS 21 cutovers roll back by deploying the prior version, not by flipping undocumented per-area flags.

## Release Checklist

- Confirm the exact old binary/version currently running.
- Confirm the exact new PMS 21 binary/version/commit and matching frontend assets.
- Confirm the latest additive migrations included in the new binary.
- Take and verify a database backup before migrations or backfill.
- Pause or confirm safe operation for Nuki, Booking.com ICS sync, Google cleaning calendar, finance imports, invoice generation, and message jobs.
- Run the production dry-run/audit command and save the artifact.
- Stop if severe conflicts are non-zero and no owner-approved override exists.
- Run Stage 2 apply only after it exists with explicit confirmation flags and idempotency tests.
- Verify Nuki, cleaning, finance, invoices, messages, analytics, dashboard, and frontend lifecycle behavior before resuming normal traffic/jobs.
- Do not run destructive cleanup until the PMS 21 version has operated successfully for an agreed release window and rollback to the old binary is no longer expected.
