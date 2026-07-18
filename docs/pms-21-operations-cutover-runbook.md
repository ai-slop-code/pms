# PMS 21 Operations Cutover Runbook

Status: operator instructions for the future PMS 21 production switch. This does not approve production apply, feature-gate enablement, or destructive cleanup.

## Preconditions

- Record the exact old binary/version currently running.
- Record the exact new PMS 21 binary/version/commit to deploy.
- Record the matching frontend build/version for the new backend.
- Confirm the database backup location and restore procedure.
- Schedule a maintenance window or reduced-traffic period.
- Confirm no other migration, import, sync, invoice, message, cleaning, or Nuki job is running.
- Confirm Nuki, Booking.com ICS sync, Google cleaning calendar, finance imports, invoice generation, and message jobs are paused or safe to run during migration.
- Confirm latest migrations included in the new binary. Current latest local migration number is `000035`; add later migrations only through the normal migration path.
- Confirm `PMS21_RAW_BLOCKS_DUAL_WRITE`, `PMS21_OCCUPANCY_EXPORT_DISABLED`, and `PMS21_OCCUPANCY_LEGACY_WRITE_DISABLED` are set as intended.
- Confirm OpenAPI and frontend assets correspond to the deployed backend version.

## Backup Steps

- Stop or pause the old app/jobs as required by the maintenance plan.
- Take a database backup before applying migrations or running PMS 21 apply.
- Record backup path, timestamp, database size/checksum if practical, old binary version, and new binary version.
- Verify the backup can be opened or restored in a safe environment if practical.

## Dry-Run And Audit Steps

Run the read-only PMS 21 dry-run/audit command against a production backup or production database opened in a safe read-only operational mode:

```bash
cd backend
```

Review the saved artifact for:

- Auto-confirmable named-stay candidates.
- Review-required rows.
- Severe conflicts.
- Unmapped Nuki, cleaning, finance, invoice, message, dashboard, or export dependencies.
- Destructive-cleanup blockers.

Stop if severe conflict counts are non-zero and no owner-approved override exists. Do not fabricate the production audit artifact.

## Apply Steps

Current command state: Stage 2 apply is not implemented. Stop here until an explicit apply command exists, idempotency tests pass, and the owner approves production execution.

Required future command shape before use:

```bash
cd backend
```

Required future apply checks:

- Run additive database migrations required by the new binary before apply.
- Save the apply output artifact with created, updated, skipped, conflict, and review-required counts.
- Run apply a second time or run an idempotency check; the second run must report no duplicate creation and no unintended business-data mutation.
- Stop if Nuki PINs, external Nuki IDs, invoices, finance links, Google event IDs, or message history would be lost.

## Deployment Steps

- Deploy the new PMS 21 backend/frontend only after successful migration apply and verification.
- Resume jobs in a safe order, starting with read-only/listing checks before write-heavy syncs where possible.
- Run Booking.com ICS sync and verify raw blocks and source-link warnings behave as expected.
- Run or trigger Nuki sync and verify named-stay-primary code generation/listing without legacy occupancy dependency.
- Run cleaning reconciliation for a narrow date range and verify named-stay/raw-block ownership fields.

## Verification Steps

- Verify representative Booking.com payout/reservation-derived historical rows became confirmed named stays.
- Verify non-payout/non-reservation stay-like rows are review-required, not silently confirmed.
- Verify `named_stay_nights` exists and drives analytics results.
- Verify calendar KPI matches backend analytics semantics for sold nights.
- Verify frontend can create, edit, cancel, archive, reactivate, and toggle cleaning for named stays.
- Verify Nuki upcoming stays, active codes, dashboard widget, and guest daily entries do not require new legacy occupancy rows.
- Verify invoices, finance mappings, messages, cleaning calendar, and dashboard rows reference named stays where expected.
- Verify deprecated legacy endpoints either still work as compatibility or are intentionally disabled according to the release plan.

## Rollback Points

- Before migrations/apply: restore old binary and continue with the original database.
- After additive migrations but before apply: old binary may continue if additive tables/columns are compatible; keep additive objects in place unless a tested restore is chosen.
- After apply but before new binary traffic: prefer restoring the pre-apply backup if rollback is required.
- After new binary traffic: rollback requires an owner decision. The new version may write named-stay-primary data; restoring the old database backup can lose post-cutover writes.

Collapsed cutovers roll back by deploying the prior version. Do not attempt to flip undocumented per-area flags.

## Post-Cutover Monitoring

- Monitor Nuki generation errors.
- Monitor source-link conflicts and source-deleted warnings.
- Monitor cleaning reconciliation errors.
- Monitor finance import/rematch errors.
- Monitor invoice creation/regeneration.
- Monitor message generation.
- Monitor analytics/dashboard mismatches.
- Review all `needs_review` named stays created by migration.
- Keep database backups from before and after cutover.

## Legacy Cleanup Eligibility

- Do not drop legacy occupancy columns/tables/routes/token storage immediately after cutover.
- Cleanup becomes eligible only after the PMS 21 version runs successfully for an agreed release window and no rollback to the old binary is expected.
- Before destructive cleanup, run a cleanup readiness audit proving no required production behavior still depends on legacy-only data.
- Cleanup instructions must list exact objects to remove, backup requirements, restore implications, and tests to run after removal.
