# New Features Backlog

A running list of small follow-up features and improvements that are out of
scope for the current PR but worth picking up later.

## Surface orphan booking-payout count as a health/metric signal

**Context.** PR #39 fixed a class of bug where a `finance_bookings` row
existed without its matching `finance_transactions` row, silently making
the affected payouts invisible to Monthly Incoming and property-income
totals. The bug was only discovered because a user noticed €0 monthly
income despite visible payouts. We currently rely on manual SQL
(`SELECT … WHERE transaction_id IS NULL`) or a one-off CLI run to detect
recurrence.

**Proposal.** Expose the orphan count so any future regression is caught
automatically without manual inspection.

**Acceptance criteria.**

- A Prometheus gauge, e.g. `pms_finance_booking_payout_orphans_total{property_id="…"}`,
  computed from
  `SELECT COUNT(*) FROM finance_bookings WHERE transaction_id IS NULL AND net_cents != 0`
  (zero-net rows are intentionally excluded — see
  `UpsertBookingFinanceTransaction` in `finance_bookings_merge.go`).
- The same number is also returned in the `/healthz` (or admin status)
  JSON payload as `finance.orphan_booking_payouts`, so operators without
  Prometheus can still see it at a glance.
- An alert rule example is added to `deploy/monitoring/prometheus-rules.yml`
  firing when the gauge stays > 0 for more than 15 minutes.
- Test: a unit test seeds an orphan booking and asserts the
  metric/handler returns the expected non-zero value.

**Non-goals.**

- Auto-repairing on detection. The repair tool (`pms-finance-repair`)
  remains the human-driven remediation path.
- Surfacing orphan counts for other tables (recurring rules, etc.) —
  separate follow-up if needed.
