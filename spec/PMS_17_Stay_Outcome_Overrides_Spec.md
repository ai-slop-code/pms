# PMS_17 - Stay Outcome Overrides: Non-Refundable Cancellation and No-Show

> Audience: product / property manager + implementing engineer.
> Scope: manual occupancy-level outcome labels for Booking.com stays where the apartment remains blocked by Booking.com, but no physical checkout cleaning should happen.
> Status: future feature specification only; no implementation in this change.

## 1. Problem Framing

Booking.com can leave a stay in the property ICS feed as occupied even when no guest will physically use the apartment. Two important cases are operationally different from a normal cancellation:

- The guest cancels a non-refundable stay. Booking.com still treats the stay as chargeable, keeps the blocked dates in the Booking calendar, and charges commission. PMS must keep the nights as occupied/sold and must not increase the cancellation rate, but the cleaner should not receive the checkout cleaning event because there was no guest turnover.
- The guest is a no-show. Booking.com does not charge commission in this case. PMS must remove the checkout cleaning event and must calculate Booking.com revenue/commission metrics using the no-show treatment instead of treating it as a normal commissioned stay.

Today PMS has three nearby concepts, none of which fully fits this requirement:

- `occupancies.status` has `active`, `updated`, `cancelled`, and `deleted_from_source`. Setting either new case to `cancelled` would remove the stay from occupancy metrics and would push it into cancellation metrics, which is wrong for non-refundable cancellations and likely wrong for no-shows.
- `occupancies.closure_state` has `closed` and `external_sale` from PMS_14. Reusing it would confuse operator-created blocked nights and externally sold nights with real Booking.com reservation outcomes.
- `finance_bookings.status` stores Booking statement statuses such as `OK`, `CANCELLED`, `NO_SHOW`, and `MODIFIED`. Statement analytics currently bucket `NO_SHOW` as `other`, outside active and cancellation-rate denominators. That is useful as a raw import rule, but it does not give the operator a way to remove cleaning from an ICS-backed occupancy.

This feature should introduce a separate manual stay-outcome override on occupancies and reconcile linked finance rows where needed.

## 2. Challenged Assumptions

The requested behavior is reasonable, but these points must be explicit before implementation:

- A non-refundable cancellation is not the same thing as `occupancies.status = 'cancelled'`. In PMS it should remain an active blocked stay with a special outcome label.
- A no-show has no physical occupancy, but PMS should still count it as occupied for the existing occupancy KPI because the dates were blocked and unavailable for resale. If PMS later adds separate "physical guest nights" metrics, no-show should be excluded there.
- Removing cleaning is not enough. If Nuki access codes and guest messages already exist, PMS must decide whether to revoke/suppress them too.
- Finance analytics currently read directly from `finance_bookings`. If the operator marks an occupancy in the Occupancy UI, analytics will not change unless the linked `finance_bookings` row is also overridden or analytics joins through `occupancies.finance_booking_id`.
- Booking.com statement status `CANCELLED` is not granular enough to distinguish refundable cancellation from non-refundable cancellation. The operator override is therefore authoritative when present.

## 3. Definitions

- **Stay outcome override**: a manual operator label on an occupancy that describes what happened operationally without changing the raw source status.
- **Normal stay**: an occupancy with no stay outcome override.
- **Cancelled: non-refundable**: a Booking.com reservation where the guest cancelled, Booking.com still charges/settles the reservation according to non-refundable rules, and the apartment remains blocked in the Booking.com calendar.
- **No-show**: a Booking.com reservation where the guest did not arrive and Booking.com does not charge commission. Revenue may be zero or may be a no-show fee, depending on the Booking statement/payout data.
- **Cleaning eligibility**: whether an occupancy checkout creates or keeps a PMS-managed Google Calendar cleaning event.
- **Financial lifecycle status**: the raw or canonical Booking status stored on `finance_bookings.status`, for example `OK`, `CANCELLED`, or `NO_SHOW`.

## 4. Product Decisions

- Add a new occupancy-level stay outcome concept instead of overloading `status` or `closure_state`.
- `cancelled_non_refundable` and `no_show` must suppress checkout cleaning events.
- `cancelled_non_refundable` must continue to count nights as occupied/sold and available.
- `cancelled_non_refundable` must be excluded from cancellation-rate numerator and denominator.
- `cancelled_non_refundable` must keep Booking.com commission and revenue exactly as imported from Booking.com payout/statement data.
- `no_show` must use the imported Booking.com financial data as source of truth. In the expected case Booking.com provides zero commission, so PMS shows zero commission; if Booking.com imports a non-zero commission, PMS trusts and surfaces it.
- `no_show` must be excluded from cancellation-rate numerator and denominator, matching the existing statement-ingestion decision that `NO_SHOW` is an `other` lifecycle outcome.
- Reopening the stay clears the stay outcome override and lets existing source status/closure labels drive behavior again.

## 5. Answered Product Questions

These answers are part of the implementation contract:

1. Should `no_show` count in occupancy rate as a sold/blocked night, or should it be excluded from occupied nights because no guest physically stayed?

Answer: count it as occupied for the existing PMS occupancy KPI.

2. If a no-show has a charged no-show fee, should that fee count in gross revenue, ADR, and RevPAR?

Answer: yes. Follow the imported Booking.com payout/statement data.

3. Should PMS force commission to zero for `no_show`, or trust the Booking.com imported `commission_cents` value?

Answer: trust imported data. Do not silently rewrite imported commission values.

4. When marking a stay as `cancelled_non_refundable` or `no_show`, should PMS automatically revoke Nuki guest access codes and suppress future guest messages?

Answer: yes. If the code was already used, keep the historical Nuki entry logs unchanged.

5. Should these labels be allowed on `closure_state = 'closed'` or `external_sale` rows?

Answer: no. A row can be either a closure/external sale or a Booking.com stay outcome, not both.

## 6. Functional Requirements

- The Occupancy calendar and list must expose actions to mark an eligible stay as `Cancelled: non-refundable` or `No-show`.
- The action must be available only for Booking.com-backed occupancy rows in active downstream status: `active` or `updated`.
- The action must be disabled for rows already labelled `closure_state = 'closed'` or `closure_state = 'external_sale'`.
- The action should be allowed for past, current, and future stays because the operator may discover the outcome after the fact.
- The operator may enter an optional reason/note.
- PMS must audit who applied the outcome, when, and which previous outcome was replaced.
- PMS must provide a `Reopen / clear outcome` action.
- Clearing the outcome must not alter raw ICS data, raw Booking statement data, imported payout rows, invoices, or finance transactions.
- Re-running occupancy sync must not clear the outcome override.
- Re-importing a Booking statement or payout must not clear the outcome override.

## 7. Backend Model

Add stay-outcome fields to `occupancies` rather than changing `status` or `closure_state`:

- `stay_outcome` text nullable, allowed values `cancelled_non_refundable`, `no_show`.
- `stay_outcome_reason` text nullable.
- `stay_outcome_marked_by_user_id` integer nullable, FK to `users.id`.
- `stay_outcome_marked_at` text nullable.

Optional but recommended if analytics continues to read `finance_bookings` without joining occupancies:

- Add `outcome_override` text nullable to `finance_bookings`, populated from the linked occupancy outcome for Booking.com rows.
- Add `outcome_override_marked_at` text nullable.

The implementation must keep raw imported source fields intact:

- Do not rewrite `occupancies.status` to `cancelled` for either new outcome.
- Do not rewrite `finance_bookings.status` from `CANCELLED` or `NO_SHOW` to `OK`.
- Do not overwrite imported `commission_cents`, `amount_cents`, `net_cents`, or statement raw JSON.

## 8. API Requirements

Suggested endpoints:

- `POST /api/properties/{id}/occupancies/{occupancyId}/outcome/cancelled-non-refundable`
- `POST /api/properties/{id}/occupancies/{occupancyId}/outcome/no-show`
- `POST /api/properties/{id}/occupancies/{occupancyId}/outcome/clear`

Request body for mark endpoints:

```json
{
  "reason": "Guest cancelled but Booking.com kept the reservation non-refundable"
}
```

Response body:

```json
{
  "ok": true
}
```

Validation rules:

- Require occupancy write permission.
- Require the occupancy to belong to the requested property.
- Reject `closure_state = 'closed'` and `closure_state = 'external_sale'` with `409 Conflict`.
- Reject non-active occupancy statuses unless the implementation explicitly supports correcting already-cancelled historical rows.
- Limit reason length to the same length as closure reasons unless a shared note policy exists.

## 9. Cleaning Calendar Rules

Cleaning eligibility must exclude both new outcomes:

```sql
status IN ('active', 'updated')
AND (closure_state IS NULL OR closure_state <> 'closed')
AND (stay_outcome IS NULL OR stay_outcome NOT IN ('cancelled_non_refundable', 'no_show'))
```

When an eligible stay is marked `cancelled_non_refundable` or `no_show`:

- The next cleaning-calendar reconciliation must delete/cancel the PMS-managed Google Calendar event for that stay's checkout.
- The local `cleaning_calendar_events` row should move to `status = 'removed'` using the existing removal lifecycle.
- The deletion must be idempotent.
- If Google deletion fails, PMS must keep the local event in `error` and expose retry exactly like other cleaning-calendar failures.

Same-day-arrival logic must also ignore these outcomes:

- A stay marked `cancelled_non_refundable` or `no_show` must not make the previous checkout event a same-day turnover.
- If a cleaning event already exists for the previous checkout, reconciliation should update the title from same-day turnover to no-guest using the existing PMS_15 title rules.
- Existing PMS_15 behavior that preserves the event time window when only same-day-arrival status changes should remain unless explicitly revisited.

## 10. Occupancy and Analytics Rules

Existing PMS occupancy analytics should treat outcomes as follows:

| Outcome | Existing occupancy KPI | Bookable denominator | Cancellation rate | Cleaning event |
|---|---:|---:|---:|---|
| none | Count if `status IN ('active','updated')` | Count | Raw status-driven | Yes |
| `cancelled_non_refundable` | Count as sold/occupied | Count | Exclude from numerator and denominator | No |
| `no_show` | Count as sold/occupied | Count | Exclude from numerator and denominator | No |

Do not count either outcome as `closure_state = 'closed'`. Closed nights remove both numerator and denominator; these outcomes should not do that by default because the Booking.com calendar remained blocked by a reservation.

Statement-derived cancellation-rate logic must change from raw-status-only to outcome-aware logic:

- `CANCELLED` with linked occupancy outcome `cancelled_non_refundable` is not a cancellation-rate numerator.
- `NO_SHOW` with linked occupancy outcome `no_show` remains outside numerator and denominator.
- Raw `CANCELLED` without an override remains a cancellation.
- Raw `OK` with `cancelled_non_refundable` should be treated as non-standard but occupied; this can happen if payout data arrives without statement status detail.

## 11. Finance Rules

### 11.1 Cancelled: Non-Refundable

For `cancelled_non_refundable`:

- Keep imported gross, net, commission, payment fee, payout date, and finance transaction values.
- Include the stay in revenue, ADR, RevPAR, and commission analytics as a financially materialized stay.
- Do not include the stay in cancellation-rate metrics.
- If a Booking statement row says `CANCELLED` with positive final amount and commission, the manual outcome explains why PMS treats it differently from a normal cancellation.

### 11.2 No-Show

For `no_show`:

- Keep imported rows immutable for audit.
- Commission analytics must use imported Booking.com commission data.
- If imported `commission_cents` is zero or null, effective take-rate for the no-show is zero.
- If imported `amount_cents` or `net_cents` is positive, count the revenue in gross revenue, ADR, and RevPAR according to the imported Booking.com data.
- If imported Booking data contains a non-zero commission for a no-show, surface it rather than silently dropping it. Booking.com import remains the source of truth.

Finance transaction rules:

- Do not auto-create manual adjustment transactions from the occupancy action.
- Imported payout/statement data remains the source of truth for money.
- If a finance row is linked via `occupancies.finance_booking_id`, the outcome override should be visible on the Booking payout/statement detail row.

## 12. Nuki and Guest Messaging Rules

The requested feature is about cleaning, but operationally the same labels mean no guest should arrive.

Recommended behavior:

- New Nuki code generation must skip occupancies with `stay_outcome IN ('cancelled_non_refundable', 'no_show')`.
- If a PMS-managed guest code already exists and is still valid, marking the outcome should revoke it on the next Nuki reconciliation.
- Guest instruction/message generation should hide these stays or show them in a disabled state with the outcome label.
- Historical Nuki logs and already-sent messages must not be deleted.

If automatic Nuki revocation is considered too risky, make it an explicit checkbox in the confirmation dialog. The default should still be checked.

## 13. Frontend Requirements

Occupancy UI:

- Add a separate `Outcome` column or badge next to the existing `Status` and `Label` columns.
- Show labels as `Cancelled: non-refundable` and `No-show`.
- Do not display these outcomes as `Closed` or `Externally sold`.
- Add actions for eligible rows: `Mark non-refundable cancellation`, `Mark no-show`, and `Clear outcome`.
- Use a confirmation dialog explaining that the action removes the checkout cleaning event but keeps the stay in Booking/occupancy history.
- After a successful action, reload the current calendar/list and show a success message.

Suggested badge tones:

- `cancelled_non_refundable`: warning.
- `no_show`: info or neutral.

Copy guidance:

- Non-refundable cancellation confirmation: `This keeps the nights counted as occupied and removes the checkout cleaning event. It will not count as a normal cancellation.`
- No-show confirmation: `This removes the checkout cleaning event and marks Booking.com commission handling as no-show. Revenue still comes from imported Booking.com files.`

Cleaning calendar UI:

- Removed cleaning events should show the same `removed` state used by other ineligible occupancies.
- If a user views the cleaning event history, show the removal reason as `stay outcome: cancelled_non_refundable` or `stay outcome: no_show`.

Finance UI:

- Linked Booking.com rows should show the occupancy outcome override.
- A no-show with non-zero commission should be visible because Booking.com import remains the source of truth even when it differs from the expected zero-commission case.

## 14. Reconciliation and Idempotence

- Occupancy sync upserts must preserve stay outcome columns, just as PMS_14 preserves closure columns.
- Finance import/merge must preserve manual outcome overrides.
- Cleaning reconciliation must be safe to rerun after every occupancy sync and after manual outcome changes.
- Marking the same outcome twice should be idempotent or return a clear no-op response.
- Switching from one outcome to another should be allowed only through an explicit replace action or a clear-then-mark flow. The audit log must capture the transition.

## 15. Edge Cases

- A guest cancels non-refundable before check-in and another guest books the same dates later: the operator should clear or replace the outcome when the original Booking.com event disappears or changes. PMS must not infer this automatically from ICS alone.
- A stay is marked no-show after the checkout date and the cleaning event already passed: PMS should still mark the local cleaning-calendar row removed for history, but Google deletion may be skipped or best-effort depending on current cleaning-calendar retention rules.
- A stay has a same-day arrival after it that is still valid: the outcome suppresses only the cleaning for the no-guest stay's own checkout. It must not suppress cleaning required by a previous real guest checkout.
- A multi-night stay is partially used, then abandoned: this feature is not a fit. Use normal occupancy and cleaning rules unless a future per-night outcome model is added.
- A statement marks `NO_SHOW` but the operator does not mark the occupancy: finance statement analytics may show it as `other`, but cleaning will still exist because the occupancy still looks active. This is acceptable for v1 unless automatic statement-to-occupancy outcome suggestions are added.

## 16. Test Focus

Backend tests:

- Marking `cancelled_non_refundable` preserves `occupancies.status = 'active'` and sets `stay_outcome`.
- Marking `no_show` preserves `occupancies.status = 'active'` and sets `stay_outcome`.
- Occupancy sync re-import does not clear `stay_outcome`.
- Cleaning reconciliation removes an existing Google event for `cancelled_non_refundable`.
- Cleaning reconciliation removes an existing Google event for `no_show`.
- Same-day-arrival lookup ignores both outcomes.
- Cancellation-rate statement analytics excludes a linked `cancelled_non_refundable` row from cancellation numerator.
- Commission analytics trusts imported no-show commission values.
- Closure/external-sale rows reject stay outcome marking.

Frontend tests:

- Occupancy list/calendar render outcome badges separately from closure labels.
- Outcome actions call the correct endpoints.
- Clear outcome restores normal action availability.
- Confirmation copy states the cleaning and analytics consequences.

## 17. Suggested Delivery Order

1. Add stay outcome persistence and API actions.
2. Wire Occupancy UI actions and badges.
3. Update cleaning-calendar eligibility and same-day-arrival filtering.
4. Update Nuki/message suppression if accepted.
5. Update statement/finance analytics to respect linked outcome overrides.
6. Add Finance UI visibility and no-show commission warning.

The feature is not complete until cleaning removal and finance/cancellation analytics both respect the outcome labels. Implementing only the Occupancy button would create a misleading UI state.
