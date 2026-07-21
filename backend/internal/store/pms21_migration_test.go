package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"pms/backend/internal/testutil"
)

func insertPMS21LegacyOccupancy(t *testing.T, st *Store, propertyID int64, sourceType, uid, start, end, guest, representation, closure string) int64 {
	t.Helper()
	res, err := st.DB.Exec(`
		INSERT INTO occupancies (
			property_id, source_type, source_event_uid, start_at, end_at, status, raw_summary,
			guest_display_name, content_hash, imported_at, last_synced_at, representation_kind,
			closure_state, upstream_source_type, upstream_event_uid
		) VALUES (?, ?, ?, ?, ?, 'active', ?, NULLIF(?, ''), ?, '2026-01-01T00:00:00Z',
			'2026-01-01T00:00:00Z', NULLIF(?, ''), NULLIF(?, ''), ?, ?)`,
		propertyID, sourceType, uid, start, end, guest, guest, "hash:"+uid, representation, closure,
		nullableStringForTest(sourceType == "booking_ics", "booking_ics"), nullableStringForTest(sourceType == "booking_ics", uid))
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func nullableStringForTest(ok bool, value string) interface{} {
	if ok {
		return value
	}
	return nil
}

func TestApplyPMS21Migration_ClassifiesAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	st := &Store{DB: testutil.OpenTestDB(t)}
	propertyID := setupFinanceProperty(t, st)
	rawID := insertPMS21LegacyOccupancy(t, st, propertyID, "booking_ics", "raw-1", "2026-08-01T00:00:00Z", "2026-08-04T00:00:00Z", "", RepresentationUnnamedBlock, "")
	payoutID := insertPMS21LegacyOccupancy(t, st, propertyID, "booking_payout", "booking_payout:R-1", "2026-08-10T00:00:00Z", "2026-08-12T00:00:00Z", "Ada", RepresentationSyntheticFinance, "")
	closedID := insertPMS21LegacyOccupancy(t, st, propertyID, "manual", "closed-1", "2026-08-20T00:00:00Z", "2026-08-22T00:00:00Z", "", RepresentationManualClosure, ClosureStateClosed)

	plan, err := st.PlanPMS21Migration(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if plan.WouldCreate.RawBookingBlocks != 1 || plan.WouldCreate.NamedStays != 1 || plan.WouldCreate.PropertyAvailabilityBlocks != 1 {
		t.Fatalf("unexpected plan: %+v", plan.WouldCreate)
	}
	if plan.WouldCreate.AutoConfirmedNamedStays != 1 || plan.WouldCreate.ReviewRequiredNamedStays != 0 {
		t.Fatalf("unexpected review classification: %+v", plan.WouldCreate)
	}

	first, err := st.ApplyPMS21Migration(ctx, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if first.Applied.Created.RawBookingBlocks != 1 || first.Applied.Created.NamedStays != 1 || first.Applied.Created.PropertyAvailabilityBlocks != 1 {
		t.Fatalf("unexpected first apply: %+v", first.Applied)
	}
	var reviewStatus, migrationKind string
	if err := st.DB.QueryRow(`SELECT ns.review_status, m.migration_kind FROM occupancy_stay_migration_map m JOIN named_stays ns ON ns.id = m.named_stay_id WHERE m.old_occupancy_id = ?`, payoutID).Scan(&reviewStatus, &migrationKind); err != nil {
		t.Fatal(err)
	}
	if reviewStatus != "confirmed" || migrationKind != "synthetic_finance" {
		t.Fatalf("review=%q kind=%q", reviewStatus, migrationKind)
	}
	for _, id := range []int64{rawID, payoutID, closedID} {
		var n int
		if err := st.DB.QueryRow(`SELECT COUNT(*) FROM occupancies WHERE id = ?`, id).Scan(&n); err != nil || n != 1 {
			t.Fatalf("legacy occupancy %d was not preserved: count=%d err=%v", id, n, err)
		}
	}

	second, err := st.ApplyPMS21Migration(ctx, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if second.Applied.Created.RawBookingBlocks != 0 || second.Applied.Created.NamedStays != 0 || second.Applied.Created.PropertyAvailabilityBlocks != 0 || second.Applied.Created.MigrationMapRows != 0 {
		t.Fatalf("second apply created data: %+v", second.Applied.Created)
	}
	var maps int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM occupancy_stay_migration_map`).Scan(&maps); err != nil || maps != 3 {
		t.Fatalf("map rows=%d err=%v", maps, err)
	}
}

func TestApplyPMS21Migration_ReviewRowsRequireExplicitOverride(t *testing.T) {
	ctx := context.Background()
	st := &Store{DB: testutil.OpenTestDB(t)}
	propertyID := setupFinanceProperty(t, st)
	legacyID := insertPMS21LegacyOccupancy(t, st, propertyID, "booking_ics", "legacy-guest", "2026-09-01T00:00:00Z", "2026-09-03T00:00:00Z", "Legacy Guest", RepresentationNamedStay, "")

	plan, err := st.PlanPMS21Migration(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if plan.WouldCreate.AutoConfirmedNamedStays != 0 || plan.WouldCreate.ReviewRequiredNamedStays != 1 {
		t.Fatalf("ICS guest was not review-required: %+v", plan.WouldCreate)
	}
	if _, err := st.ApplyPMS21Migration(ctx, 10, false); !errors.Is(err, ErrPMS21ReviewOverrideRequired) {
		t.Fatalf("apply error=%v", err)
	}
	var namedCount int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM named_stays`).Scan(&namedCount); err != nil || namedCount != 0 {
		t.Fatalf("named stays changed before override: count=%d err=%v", namedCount, err)
	}
	if _, err := st.ApplyPMS21Migration(ctx, 10, true); err != nil {
		t.Fatal(err)
	}
	var reviewStatus, reason string
	if err := st.DB.QueryRow(`SELECT ns.review_status, ns.review_reason FROM named_stays ns JOIN occupancy_stay_migration_map m ON m.named_stay_id = ns.id WHERE m.old_occupancy_id = ?`, legacyID).Scan(&reviewStatus, &reason); err != nil {
		t.Fatal(err)
	}
	if reviewStatus != "needs_review" || reason != "legacy_non_reservation_stay" {
		t.Fatalf("review=%q reason=%q", reviewStatus, reason)
	}
}

func TestApplyPMS21Migration_RefusesNamedStayOverlap(t *testing.T) {
	ctx := context.Background()
	st := &Store{DB: testutil.OpenTestDB(t)}
	propertyID := setupFinanceProperty(t, st)
	insertPMS21LegacyOccupancy(t, st, propertyID, "booking_payout", "payout-1", "2026-10-01T00:00:00Z", "2026-10-04T00:00:00Z", "One", RepresentationSyntheticFinance, "")
	insertPMS21LegacyOccupancy(t, st, propertyID, "booking_statement", "statement-2", "2026-10-03T00:00:00Z", "2026-10-05T00:00:00Z", "Two", RepresentationSyntheticFinance, "")

	report, err := st.ApplyPMS21Migration(ctx, 10, false)
	if !errors.Is(err, ErrPMS21SevereConflicts) {
		t.Fatalf("apply error=%v", err)
	}
	if report == nil || report.Conflicts.NamedStayOverlapPairs != 1 {
		t.Fatalf("report=%+v", report)
	}
	var n int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM named_stays`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("named stays changed despite conflict: count=%d err=%v", n, err)
	}
}

func TestApplyPMS21Migration_BackfillsUniqueIntegrationLinks(t *testing.T) {
	ctx := context.Background()
	st := &Store{DB: testutil.OpenTestDB(t)}
	propertyID := setupFinanceProperty(t, st)
	legacyID := insertPMS21LegacyOccupancy(t, st, propertyID, "booking_payout", "booking_payout:R-2", "2026-11-01T00:00:00Z", "2026-11-03T00:00:00Z", "Grace", RepresentationSyntheticFinance, "")
	now := "2026-01-01T00:00:00Z"
	if _, err := st.DB.Exec(`INSERT INTO nuki_access_codes (property_id, occupancy_id, code_label, generated_pin_plain, external_nuki_id, valid_from, valid_until, status, created_at, updated_at) VALUES (?, ?, 'Grace', 'enc-pin', 'external-1', ?, ?, 'generated', ?, ?)`, propertyID, legacyID, now, now, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`INSERT INTO nuki_guest_daily_entries (property_id, occupancy_id, day_date, first_entry_at, nuki_event_reference, created_at) VALUES (?, ?, '2026-11-01', ?, 'event-1', ?)`, propertyID, legacyID, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`INSERT INTO cleaning_calendar_events (property_id, occupancy_id, cleaning_kind, google_calendar_id, google_event_id, cleaning_date, starts_at, ends_at, title, status, created_at, updated_at) VALUES (?, ?, 'named_stay', 'calendar', 'google-1', '2026-11-03', ?, ?, 'Clean', 'synced', ?, ?)`, propertyID, legacyID, now, now, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`INSERT INTO finance_bookings (property_id, reference_number, check_in_date, check_out_date, guest_name, net_cents, payout_date, occupancy_id, created_at, updated_at, source_channel, has_payout_data) VALUES (?, 'R-2', '2026-11-01', '2026-11-03', 'Grace', 10000, '2026-11-04', ?, ?, ?, 'booking_com', 1)`, propertyID, legacyID, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`INSERT INTO invoices (property_id, occupancy_id, invoice_number, sequence_year, sequence_value, language, issue_date, taxable_supply_date, due_date, stay_start_date, stay_end_date, supplier_snapshot_json, customer_snapshot_json, amount_total_cents, currency, payment_note, created_at, updated_at) VALUES (?, ?, '2026-1', 2026, 1, 'en', '2026-11-03', '2026-11-03', '2026-11-03', '2026-11-01', '2026-11-03', '{}', '{}', 10000, 'EUR', '', ?, ?)`, propertyID, legacyID, now, now); err != nil {
		t.Fatal(err)
	}

	report, err := st.ApplyPMS21Migration(ctx, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if report.Applied.UpdatedLinks.NukiAccessCodes != 1 || report.Applied.UpdatedLinks.NukiGuestDailyEntries != 1 || report.Applied.UpdatedLinks.CleaningEventsNamed != 1 || report.Applied.UpdatedLinks.FinanceBookings != 1 || report.Applied.UpdatedLinks.Invoices != 1 {
		t.Fatalf("link counts: %+v", report.Applied.UpdatedLinks)
	}
	var pin, externalID, googleID string
	var linked int64
	if err := st.DB.QueryRow(`SELECT generated_pin_plain, external_nuki_id, named_stay_id FROM nuki_access_codes WHERE occupancy_id = ?`, legacyID).Scan(&pin, &externalID, &linked); err != nil {
		t.Fatal(err)
	}
	if pin != "enc-pin" || externalID != "external-1" || linked == 0 {
		t.Fatalf("Nuki data changed: pin=%q external=%q stay=%d", pin, externalID, linked)
	}
	if err := st.DB.QueryRow(`SELECT google_event_id FROM cleaning_calendar_events WHERE occupancy_id = ?`, legacyID).Scan(&googleID); err != nil || googleID != "google-1" {
		t.Fatalf("Google event changed: id=%q err=%v", googleID, err)
	}
}

func TestApplyPMS21Migration_FinanceEvidenceConfirmsLegacyICSStay(t *testing.T) {
	ctx := context.Background()
	st := &Store{DB: testutil.OpenTestDB(t)}
	propertyID := setupFinanceProperty(t, st)
	legacyID := insertPMS21LegacyOccupancy(t, st, propertyID, "booking_ics", "finance-backed@booking.com", "2026-11-10T00:00:00Z", "2026-11-11T00:00:00Z", "Finance Guest", RepresentationNamedStay, "")
	now := "2026-01-01T00:00:00Z"
	if _, err := st.DB.Exec(`INSERT INTO finance_bookings (property_id, reference_number, check_in_date, check_out_date, guest_name, net_cents, payout_date, occupancy_id, created_at, updated_at, source_channel, has_payout_data, has_statement_data) VALUES (?, 'FIN-EVIDENCE', '2026-11-10', '2026-11-11', 'Finance Guest', 10000, '2026-11-12', ?, ?, ?, 'booking_com', 1, 1)`, propertyID, legacyID, now, now); err != nil {
		t.Fatal(err)
	}

	plan, err := st.PlanPMS21Migration(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if plan.WouldCreate.AutoConfirmedNamedStays != 1 || plan.WouldCreate.ReviewRequiredNamedStays != 0 {
		t.Fatalf("finance-backed classification: %+v", plan.WouldCreate)
	}

	first, err := st.ApplyPMS21Migration(ctx, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	var stayID int64
	var reviewStatus string
	if err := st.DB.QueryRow(`SELECT ns.id, ns.review_status FROM named_stays ns JOIN occupancy_stay_migration_map osm ON osm.named_stay_id = ns.id WHERE osm.old_occupancy_id = ?`, legacyID).Scan(&stayID, &reviewStatus); err != nil {
		t.Fatal(err)
	}
	if reviewStatus != "confirmed" {
		t.Fatalf("review status=%q want confirmed", reviewStatus)
	}
	var linkedStayID int64
	if err := st.DB.QueryRow(`SELECT named_stay_id FROM finance_bookings WHERE reference_number = 'FIN-EVIDENCE'`).Scan(&linkedStayID); err != nil {
		t.Fatal(err)
	}
	if linkedStayID != stayID || first.Applied.UpdatedLinks.FinanceBookings != 1 {
		t.Fatalf("finance link=%d stay=%d counts=%+v", linkedStayID, stayID, first.Applied.UpdatedLinks)
	}

	if _, err := st.DB.Exec(`UPDATE named_stays SET review_status = 'needs_review', review_reason = 'legacy_non_reservation_stay', nuki_generation_status = 'not_applicable' WHERE id = ?`, stayID); err != nil {
		t.Fatal(err)
	}
	repair, err := st.ApplyPMS21Migration(ctx, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if repair.Applied.UpdatedLinks.FinanceBookings != 0 || repair.Applied.UpdatedLinks.NamedStaysConfirmedByFinance != 1 {
		t.Fatalf("finance evidence repair counts: %+v", repair.Applied.UpdatedLinks)
	}
	var nukiStatus string
	if err := st.DB.QueryRow(`SELECT review_status, nuki_generation_status FROM named_stays WHERE id = ?`, stayID).Scan(&reviewStatus, &nukiStatus); err != nil {
		t.Fatal(err)
	}
	if reviewStatus != "confirmed" || nukiStatus != "pending" {
		t.Fatalf("repaired status=%q nuki=%q", reviewStatus, nukiStatus)
	}

	third, err := st.ApplyPMS21Migration(ctx, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if third.Applied.UpdatedLinks.FinanceBookings != 0 || third.Applied.UpdatedLinks.NamedStaysConfirmedByFinance != 0 {
		t.Fatalf("third apply was not idempotent: %+v", third.Applied.UpdatedLinks)
	}
}

func TestPMS21NukiPendingEligibilityMatchesSyncSelection(t *testing.T) {
	eligible := pms21Classification{StayType: StayTypeBookingCom, ReviewStatus: "confirmed", CheckOutDate: "2099-01-02"}
	if !pms21NukiPendingEligible(NamedStayStatusActive, eligible, sql.NullString{}) {
		t.Fatal("future confirmed Booking.com stay was not pending-eligible")
	}
	for name, tc := range map[string]struct {
		status  string
		class   pms21Classification
		outcome sql.NullString
	}{
		"past":                        {status: NamedStayStatusActive, class: pms21Classification{StayType: StayTypeBookingCom, ReviewStatus: "confirmed", CheckOutDate: "2020-01-02"}},
		"needs review":                {status: NamedStayStatusActive, class: pms21Classification{StayType: StayTypeBookingCom, ReviewStatus: "needs_review", CheckOutDate: "2099-01-02"}},
		"cancelled":                   {status: NamedStayStatusCancelled, class: eligible},
		"no show":                     {status: NamedStayStatusActive, class: eligible, outcome: sql.NullString{String: StayOutcomeNoShow, Valid: true}},
		"non-refundable cancellation": {status: NamedStayStatusActive, class: eligible, outcome: sql.NullString{String: StayOutcomeCancelledNonRefundable, Valid: true}},
	} {
		t.Run(name, func(t *testing.T) {
			if pms21NukiPendingEligible(tc.status, tc.class, tc.outcome) {
				t.Fatal("unexpected pending eligibility")
			}
		})
	}
}

func TestApplyPMS21Migration_BookingICSCivilDatesDoNotShiftWestOfUTC(t *testing.T) {
	ctx := context.Background()
	st := &Store{DB: testutil.OpenTestDB(t)}
	propertyID := setupFinanceProperty(t, st)
	if _, err := st.DB.Exec(`UPDATE properties SET timezone = 'America/Los_Angeles' WHERE id = ?`, propertyID); err != nil {
		t.Fatal(err)
	}
	insertPMS21LegacyOccupancy(t, st, propertyID, "booking_ics", "west-1", "2026-08-01T00:00:00Z", "2026-08-04T00:00:00Z", "", RepresentationUnnamedBlock, "")

	if _, err := st.ApplyPMS21Migration(ctx, 10, false); err != nil {
		t.Fatal(err)
	}
	var checkIn, checkOut string
	if err := st.DB.QueryRow(`SELECT check_in_date, check_out_date FROM raw_booking_blocks WHERE source_event_uid = 'west-1'`).Scan(&checkIn, &checkOut); err != nil {
		t.Fatal(err)
	}
	if checkIn != "2026-08-01" || checkOut != "2026-08-04" {
		t.Fatalf("ICS dates shifted: %s to %s", checkIn, checkOut)
	}

	instant := pms21LegacyRow{SourceType: "booking_payout", StartAt: "2026-08-10T00:00:00Z", EndAt: "2026-08-12T00:00:00Z", PropertyTimezone: "America/Los_Angeles"}
	ci, co, err := legacyPropertyDates(instant)
	if err != nil {
		t.Fatal(err)
	}
	if ci != "2026-08-09" || co != "2026-08-11" {
		t.Fatalf("instant dates were not property-local: %s to %s", ci, co)
	}
	generated := pms21LegacyRow{
		SourceType:         UpstreamSourceBookingICS,
		StartAt:            "2026-08-20T00:00:00Z",
		EndAt:              "2026-08-22T00:00:00Z",
		PropertyTimezone:   "America/Los_Angeles",
		RepresentationKind: sql.NullString{String: RepresentationLegacyGeneratedNight, Valid: true},
	}
	ci, co, err = legacyPropertyDates(generated)
	if err != nil {
		t.Fatal(err)
	}
	if ci != "2026-08-19" || co != "2026-08-21" {
		t.Fatalf("generated instant dates were not property-local: %s to %s", ci, co)
	}
}

func TestApplyPMS21Migration_ManualNamedStayCivilDatesDoNotShiftWestOfUTC(t *testing.T) {
	ctx := context.Background()
	st := &Store{DB: testutil.OpenTestDB(t)}
	propertyID := setupFinanceProperty(t, st)
	if _, err := st.DB.Exec(`UPDATE properties SET timezone = 'America/Los_Angeles' WHERE id = ?`, propertyID); err != nil {
		t.Fatal(err)
	}
	legacyID := insertPMS21LegacyOccupancy(t, st, propertyID, "manual", "manual-west-1", "2026-08-10T00:00:00Z", "2026-08-12T00:00:00Z", "Manual Guest", RepresentationNamedStay, "")

	if _, err := st.ApplyPMS21Migration(ctx, 10, true); err != nil {
		t.Fatal(err)
	}
	var checkIn, checkOut string
	if err := st.DB.QueryRow(`
		SELECT ns.check_in_date, ns.check_out_date
		FROM occupancy_stay_migration_map m JOIN named_stays ns ON ns.id = m.named_stay_id
		WHERE m.old_occupancy_id = ?`, legacyID).Scan(&checkIn, &checkOut); err != nil {
		t.Fatal(err)
	}
	if checkIn != "2026-08-10" || checkOut != "2026-08-12" {
		t.Fatalf("manual civil dates shifted: %s to %s", checkIn, checkOut)
	}
}

func TestApplyPMS21Migration_ManualUnnamedBlockRemainsUnmapped(t *testing.T) {
	ctx := context.Background()
	st := &Store{DB: testutil.OpenTestDB(t)}
	propertyID := setupFinanceProperty(t, st)
	legacyID := insertPMS21LegacyOccupancy(t, st, propertyID, "manual", "manual-unnamed-1", "2026-09-10T00:00:00Z", "2026-09-12T00:00:00Z", "", RepresentationUnnamedBlock, "")

	plan, err := st.PlanPMS21Migration(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if plan.WouldCreate.RawBookingBlocks != 0 || plan.WouldCreate.RawBookingBlockNights != 0 || plan.WouldCreate.MigrationMapRows != 1 {
		t.Fatalf("manual unnamed row was assigned raw ownership: %+v", plan.WouldCreate)
	}
	if _, err := st.ApplyPMS21Migration(ctx, 10, false); err != nil {
		t.Fatal(err)
	}
	var migrationKind string
	if err := st.DB.QueryRow(`SELECT migration_kind FROM occupancy_stay_migration_map WHERE old_occupancy_id = ?`, legacyID).Scan(&migrationKind); err != nil {
		t.Fatal(err)
	}
	if migrationKind != "unmapped" {
		t.Fatalf("migration kind=%q want unmapped", migrationKind)
	}
	var rawCount int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM raw_booking_blocks`).Scan(&rawCount); err != nil || rawCount != 0 {
		t.Fatalf("raw booking blocks=%d err=%v", rawCount, err)
	}
}

func TestApplyPMS21Migration_PlanCountsOnlyActiveCandidateNights(t *testing.T) {
	ctx := context.Background()
	st := &Store{DB: testutil.OpenTestDB(t)}
	propertyID := setupFinanceProperty(t, st)
	rawLegacyID := insertPMS21LegacyOccupancy(t, st, propertyID, "booking_ics", "inactive-raw-1", "2026-10-01T00:00:00Z", "2026-10-04T00:00:00Z", "", RepresentationUnnamedBlock, "")
	namedLegacyID := insertPMS21LegacyOccupancy(t, st, propertyID, "booking_payout", "cancelled-stay-1", "2026-10-10T00:00:00Z", "2026-10-13T00:00:00Z", "Cancelled Guest", RepresentationSyntheticFinance, "")
	if _, err := st.DB.Exec(`UPDATE occupancies SET status = 'deleted_from_source' WHERE id = ?`, rawLegacyID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`UPDATE occupancies SET status = 'cancelled' WHERE id = ?`, namedLegacyID); err != nil {
		t.Fatal(err)
	}
	now := "2026-01-01T00:00:00Z"
	if _, err := st.DB.Exec(`
		INSERT INTO finance_bookings (
			property_id, reference_number, check_in_date, check_out_date, guest_name, net_cents,
			payout_date, status, created_at, updated_at, source_channel, has_payout_data
		) VALUES (?, 'CANCELLED-FINANCE-1', '2026-10-20', '2026-10-23', 'Cancelled Finance Guest', 0,
			'2026-10-24', 'CANCELLED', ?, ?, 'booking_com', 1)`, propertyID, now, now); err != nil {
		t.Fatal(err)
	}

	plan, err := st.PlanPMS21Migration(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if plan.WouldCreate.RawBookingBlocks != 1 || plan.WouldCreate.NamedStays != 2 || plan.WouldCreate.RawBookingBlockNights != 0 || plan.WouldCreate.NamedStayNights != 0 {
		t.Fatalf("inactive candidate plan: %+v", plan.WouldCreate)
	}
	report, err := st.ApplyPMS21Migration(ctx, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if report.Applied.Created.RawBookingBlocks != plan.WouldCreate.RawBookingBlocks ||
		report.Applied.Created.NamedStays != plan.WouldCreate.NamedStays ||
		report.Applied.Created.RawBookingBlockNights != plan.WouldCreate.RawBookingBlockNights ||
		report.Applied.Created.NamedStayNights != plan.WouldCreate.NamedStayNights {
		t.Fatalf("plan=%+v apply=%+v", plan.WouldCreate, report.Applied.Created)
	}
}

func TestApplyPMS21Migration_PlanAccountsForExistingRawIdentity(t *testing.T) {
	ctx := context.Background()
	st := &Store{DB: testutil.OpenTestDB(t)}
	propertyID := setupFinanceProperty(t, st)
	insertPMS21LegacyOccupancy(t, st, propertyID, "booking_ics", "existing-raw-1", "2026-11-01T00:00:00Z", "2026-11-04T00:00:00Z", "", RepresentationUnnamedBlock, "")
	now := "2026-01-01T00:00:00Z"
	res, err := st.DB.Exec(`
		INSERT INTO raw_booking_blocks (
			property_id, source_type, source_event_uid, check_in_date, check_out_date, status,
			content_hash, imported_at, last_synced_at, created_at, updated_at
		) VALUES (?, 'booking_ics', 'existing-raw-1', '2026-11-01', '2026-11-04', 'active', 'existing-hash', ?, ?, ?, ?)`, propertyID, now, now, now, now)
	if err != nil {
		t.Fatal(err)
	}
	rawID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`INSERT INTO raw_booking_block_nights (property_id, raw_booking_block_id, local_night_date, active, created_at, updated_at) VALUES (?, ?, '2026-11-01', 1, ?, ?)`, propertyID, rawID, now, now); err != nil {
		t.Fatal(err)
	}

	plan, err := st.PlanPMS21Migration(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if plan.WouldCreate.RawBookingBlocks != 0 || plan.WouldCreate.RawBookingBlockNights != 2 || plan.WouldCreate.MigrationMapRows != 1 {
		t.Fatalf("existing identity plan: %+v", plan.WouldCreate)
	}
	report, err := st.ApplyPMS21Migration(ctx, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if report.Applied.Created.RawBookingBlocks != plan.WouldCreate.RawBookingBlocks || report.Applied.Created.RawBookingBlockNights != plan.WouldCreate.RawBookingBlockNights {
		t.Fatalf("plan=%+v apply=%+v", plan.WouldCreate, report.Applied.Created)
	}
}

func TestApplyPMS21Migration_CreatesAndLinksFinanceOnlyBookingIdempotently(t *testing.T) {
	ctx := context.Background()
	st := &Store{DB: testutil.OpenTestDB(t)}
	propertyID := setupFinanceProperty(t, st)
	now := "2026-01-01T00:00:00Z"
	res, err := st.DB.Exec(`
		INSERT INTO finance_bookings (
			property_id, reference_number, check_in_date, check_out_date, guest_name, net_cents,
			payout_date, created_at, updated_at, source_channel, has_payout_data
		) VALUES (?, 'FIN-ONLY-1', '2026-12-01', '2026-12-04', 'Finance Guest', 15000,
			'2026-12-05', ?, ?, 'booking_com', 1)`, propertyID, now, now)
	if err != nil {
		t.Fatal(err)
	}
	financeID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}

	plan, err := st.PlanPMS21Migration(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if plan.WouldCreate.AutoConfirmedNamedStays != 1 || plan.WouldBackfillLinks.FinanceBookings != 1 {
		t.Fatalf("finance-only row was not planned: create=%+v links=%+v", plan.WouldCreate, plan.WouldBackfillLinks)
	}
	first, err := st.ApplyPMS21Migration(ctx, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if first.Applied.Created.AutoConfirmedNamedStays != 1 || first.Applied.UpdatedLinks.FinanceBookings != 1 || first.Unmapped.FinanceBookingsUnmatched != 0 {
		t.Fatalf("finance-only apply=%+v unmapped=%+v", first.Applied, first.Unmapped)
	}
	var stayID int64
	var reviewStatus, sourceReference string
	if err := st.DB.QueryRow(`
		SELECT fb.named_stay_id, ns.review_status, ns.source_reference
		FROM finance_bookings fb JOIN named_stays ns ON ns.id = fb.named_stay_id
		WHERE fb.id = ?`, financeID).Scan(&stayID, &reviewStatus, &sourceReference); err != nil {
		t.Fatal(err)
	}
	if stayID == 0 || reviewStatus != "confirmed" || sourceReference != "FIN-ONLY-1" {
		t.Fatalf("stay=%d review=%q source=%q", stayID, reviewStatus, sourceReference)
	}

	second, err := st.ApplyPMS21Migration(ctx, 10, false)
	if err != nil {
		t.Fatal(err)
	}
	if second.Applied.Created.NamedStays != 0 || second.Applied.UpdatedLinks.FinanceBookings != 0 {
		t.Fatalf("second apply changed finance-only row: %+v", second.Applied)
	}
	var stays int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM named_stays`).Scan(&stays); err != nil || stays != 1 {
		t.Fatalf("named stays=%d err=%v", stays, err)
	}
}

func TestApplyPMS21Migration_FinanceOnlyUnverifiedSourceNeedsReview(t *testing.T) {
	ctx := context.Background()
	st := &Store{DB: testutil.OpenTestDB(t)}
	propertyID := setupFinanceProperty(t, st)
	now := "2026-01-01T00:00:00Z"
	if _, err := st.DB.Exec(`
		INSERT INTO finance_bookings (
			property_id, reference_number, check_in_date, check_out_date, guest_name, net_cents,
			payout_date, created_at, updated_at, source_channel, has_payout_data, has_statement_data
		) VALUES (?, 'UNVERIFIED-1', '2026-12-10', '2026-12-12', 'Review Guest', 0,
			'2026-12-13', ?, ?, 'booking_com', 0, 0)`, propertyID, now, now); err != nil {
		t.Fatal(err)
	}

	plan, err := st.PlanPMS21Migration(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if plan.WouldCreate.ReviewRequiredNamedStays != 1 || plan.ReviewRequired.FinanceBookingsNeedsReview != 1 {
		t.Fatalf("unverified finance classification: create=%+v review=%+v", plan.WouldCreate, plan.ReviewRequired)
	}
	if _, err := st.ApplyPMS21Migration(ctx, 10, false); !errors.Is(err, ErrPMS21ReviewOverrideRequired) {
		t.Fatalf("apply error=%v", err)
	}
	if _, err := st.ApplyPMS21Migration(ctx, 10, true); err != nil {
		t.Fatal(err)
	}
	var reviewStatus, reviewReason string
	if err := st.DB.QueryRow(`
		SELECT ns.review_status, ns.review_reason
		FROM finance_bookings fb JOIN named_stays ns ON ns.id = fb.named_stay_id
		WHERE fb.reference_number = 'UNVERIFIED-1'`).Scan(&reviewStatus, &reviewReason); err != nil {
		t.Fatal(err)
	}
	if reviewStatus != "needs_review" || reviewReason != "finance_source_unverified" {
		t.Fatalf("review=%q reason=%q", reviewStatus, reviewReason)
	}
}

func TestApplyPMS21Migration_RefusesCandidateOverlapWithExistingNamedStayNight(t *testing.T) {
	ctx := context.Background()
	st := &Store{DB: testutil.OpenTestDB(t)}
	propertyID := setupFinanceProperty(t, st)
	now := "2026-01-01T00:00:00Z"
	res, err := st.DB.Exec(`
		INSERT INTO named_stays (
			property_id, display_name, stay_type, check_in_date, check_out_date, status,
			cleaning_required, review_status, nuki_generation_status, created_at, updated_at
		) VALUES (?, 'Existing', 'booking_com', '2027-01-02', '2027-01-05', 'active', 1, 'confirmed', 'pending', ?, ?)`, propertyID, now, now)
	if err != nil {
		t.Fatal(err)
	}
	stayID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec(`INSERT INTO named_stay_nights (property_id, named_stay_id, local_night_date, active, created_at) VALUES (?, ?, '2027-01-03', 1, ?)`, propertyID, stayID, now); err != nil {
		t.Fatal(err)
	}
	insertPMS21LegacyOccupancy(t, st, propertyID, "booking_payout", "existing-overlap", "2027-01-03T00:00:00Z", "2027-01-06T00:00:00Z", "Candidate", RepresentationSyntheticFinance, "")

	report, err := st.ApplyPMS21Migration(ctx, 10, false)
	if !errors.Is(err, ErrPMS21SevereConflicts) {
		t.Fatalf("apply error=%v", err)
	}
	if report == nil || report.Conflicts.ExistingNamedStayNightCollisions != 1 {
		t.Fatalf("report=%+v", report)
	}
	var maps int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM occupancy_stay_migration_map`).Scan(&maps); err != nil || maps != 0 {
		t.Fatalf("preflight wrote mapping rows: count=%d err=%v", maps, err)
	}
}

func TestApplyPMS21Migration_RefusesNamedStayCandidateOverlapWithClosure(t *testing.T) {
	ctx := context.Background()
	st := &Store{DB: testutil.OpenTestDB(t)}
	propertyID := setupFinanceProperty(t, st)
	insertPMS21LegacyOccupancy(t, st, propertyID, "booking_payout", "stay-vs-closure", "2027-02-01T00:00:00Z", "2027-02-04T00:00:00Z", "Candidate", RepresentationSyntheticFinance, "")
	insertPMS21LegacyOccupancy(t, st, propertyID, "manual", "closure-overlap", "2027-02-03T00:00:00Z", "2027-02-05T00:00:00Z", "", RepresentationManualClosure, ClosureStateClosed)

	report, err := st.ApplyPMS21Migration(ctx, 10, false)
	if !errors.Is(err, ErrPMS21SevereConflicts) {
		t.Fatalf("apply error=%v", err)
	}
	if report == nil || report.Conflicts.NamedStayAvailabilityOverlaps != 1 {
		t.Fatalf("report=%+v", report)
	}
	var named, blocks int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM named_stays`).Scan(&named); err != nil {
		t.Fatal(err)
	}
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM property_availability_blocks`).Scan(&blocks); err != nil {
		t.Fatal(err)
	}
	if named != 0 || blocks != 0 {
		t.Fatalf("preflight wrote named=%d blocks=%d", named, blocks)
	}
}
