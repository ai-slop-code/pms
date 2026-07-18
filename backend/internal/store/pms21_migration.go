package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type PMS21MigrationDryRunReport struct {
	Mode                  string                   `json:"mode"`
	ApplyImplemented      bool                     `json:"apply_implemented"`
	WouldCreate           PMS21CreateCounts        `json:"would_create"`
	WouldBackfillLinks    PMS21BackfillCounts      `json:"would_backfill_links"`
	Conflicts             PMS21ConflictCounts      `json:"conflicts"`
	Unmapped              PMS21UnmappedCounts      `json:"unmapped"`
	ReviewRequired        PMS21ReviewCounts        `json:"review_required"`
	ExistingNewModelRows  PMS21ExistingCounts      `json:"existing_new_model_rows"`
	Samples               map[string][]PMS21Sample `json:"samples"`
	IdempotentComparedRun bool                     `json:"idempotent_compared_with_prior_dry_run"`
}

type PMS21CreateCounts struct {
	RawBookingBlocks      int `json:"raw_booking_blocks"`
	RawBookingBlockNights int `json:"raw_booking_block_nights"`
	NamedStays            int `json:"named_stays"`
	NamedStayNights       int `json:"named_stay_nights"`
	StaySourceLinks       int `json:"stay_source_links"`
}

type PMS21BackfillCounts struct {
	NukiAccessCodes       int `json:"nuki_access_codes_named_stay_id"`
	NukiGuestDailyEntries int `json:"nuki_guest_daily_entries_named_stay_id"`
	CleaningEventsNamed   int `json:"cleaning_events_named_stay_id"`
	CleaningEventsRaw     int `json:"cleaning_events_raw_booking_block_id"`
	FinanceBookings       int `json:"finance_bookings_named_stay_id"`
	Invoices              int `json:"invoices_named_stay_id"`
}

type PMS21ConflictCounts struct {
	NamedStayOverlapPairs int `json:"named_stay_overlap_pairs"`
	RawBlockOverlapPairs  int `json:"raw_block_overlap_pairs"`
	ExternalSaleRows      int `json:"external_sale_rows"`
	ActiveClosedRows      int `json:"active_closed_rows"`
}

type PMS21UnmappedCounts struct {
	FinanceBookingsUnmatched              int `json:"finance_bookings_unmatched"`
	NukiCodesWithoutNamedLikeOccupancy    int `json:"nuki_codes_without_named_like_occupancy"`
	CleaningEventsWithoutMappingCandidate int `json:"cleaning_events_without_mapping_candidate"`
	InvoicesWithoutNamedLikeOccupancy     int `json:"invoices_without_named_like_occupancy"`
}

type PMS21ReviewCounts struct {
	FinanceSyntheticNamedStays int `json:"finance_synthetic_named_stays"`
	LegacyGeneratedNightRows   int `json:"legacy_generated_night_rows"`
}

type PMS21ExistingCounts struct {
	RawBookingBlocks           int `json:"raw_booking_blocks"`
	RawBookingBlockNights      int `json:"raw_booking_block_nights"`
	NamedStays                 int `json:"named_stays"`
	NamedStayNights            int `json:"named_stay_nights"`
	StaySourceLinks            int `json:"stay_source_links"`
	PropertyAvailabilityBlocks int `json:"property_availability_blocks"`
	MigrationMapRows           int `json:"occupancy_stay_migration_map"`
}

type PMS21Sample struct {
	PropertyID int64  `json:"property_id"`
	ID         int64  `json:"id,omitempty"`
	OtherID    int64  `json:"other_id,omitempty"`
	Start      string `json:"start,omitempty"`
	End        string `json:"end,omitempty"`
	OtherStart string `json:"other_start,omitempty"`
	OtherEnd   string `json:"other_end,omitempty"`
	Status     string `json:"status,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// PlanPMS21Migration is the read-only dry-run planner required before PMS_21
// old-data backfill. It deliberately does not mutate state; a later apply mode
// must reuse this classification logic rather than reimplementing it.
func (s *Store) PlanPMS21Migration(ctx context.Context, sampleLimit int) (*PMS21MigrationDryRunReport, error) {
	if sampleLimit <= 0 {
		sampleLimit = 10
	}
	r := &PMS21MigrationDryRunReport{
		Mode:                  "dry_run",
		ApplyImplemented:      false,
		Samples:               map[string][]PMS21Sample{},
		IdempotentComparedRun: true,
	}
	var err error
	if r.ExistingNewModelRows.RawBookingBlocks, err = countInt(ctx, s.DB, `SELECT COUNT(*) FROM raw_booking_blocks`); err != nil {
		return nil, err
	}
	if r.ExistingNewModelRows.RawBookingBlockNights, err = countInt(ctx, s.DB, `SELECT COUNT(*) FROM raw_booking_block_nights`); err != nil {
		return nil, err
	}
	if r.ExistingNewModelRows.NamedStays, err = countInt(ctx, s.DB, `SELECT COUNT(*) FROM named_stays`); err != nil {
		return nil, err
	}
	if r.ExistingNewModelRows.NamedStayNights, err = countInt(ctx, s.DB, `SELECT COUNT(*) FROM named_stay_nights`); err != nil {
		return nil, err
	}
	if r.ExistingNewModelRows.StaySourceLinks, err = countInt(ctx, s.DB, `SELECT COUNT(*) FROM stay_source_links`); err != nil {
		return nil, err
	}
	if r.ExistingNewModelRows.PropertyAvailabilityBlocks, err = countInt(ctx, s.DB, `SELECT COUNT(*) FROM property_availability_blocks`); err != nil {
		return nil, err
	}
	if r.ExistingNewModelRows.MigrationMapRows, err = countInt(ctx, s.DB, `SELECT COUNT(*) FROM occupancy_stay_migration_map`); err != nil {
		return nil, err
	}

	if r.WouldCreate.RawBookingBlocks, err = countInt(ctx, s.DB, rawCandidateSQL(`COUNT(*)`)); err != nil {
		return nil, err
	}
	if r.WouldCreate.NamedStays, err = countInt(ctx, s.DB, namedCandidateSQL(`COUNT(*)`)); err != nil {
		return nil, err
	}
	if r.WouldCreate.RawBookingBlockNights, err = countEstimatedNights(ctx, s.DB, rawCandidateSQL(`start_at, end_at`)); err != nil {
		return nil, err
	}
	if r.WouldCreate.NamedStayNights, err = countEstimatedNights(ctx, s.DB, namedCandidateSQL(`start_at, end_at`)); err != nil {
		return nil, err
	}
	// Initial backfill creates one source link for each Booking.com named-like
	// stay that has an upstream UID. Split/merge multi-link repair is a later
	// conflict-resolution concern.
	if r.WouldCreate.StaySourceLinks, err = countInt(ctx, s.DB, namedCandidateSQL(`COUNT(*)`)+` AND upstream_event_uid IS NOT NULL`); err != nil {
		return nil, err
	}

	if r.WouldBackfillLinks.NukiAccessCodes, err = countInt(ctx, s.DB, `
		SELECT COUNT(*)
		FROM nuki_access_codes nac
		JOIN occupancies o ON o.id = nac.occupancy_id
		WHERE nac.named_stay_id IS NULL AND `+namedPredicate("o")); err != nil {
		return nil, err
	}
	if r.WouldBackfillLinks.NukiGuestDailyEntries, err = countInt(ctx, s.DB, `
		SELECT COUNT(*)
		FROM nuki_guest_daily_entries nge
		JOIN occupancies o ON o.id = nge.occupancy_id
		WHERE nge.named_stay_id IS NULL AND `+namedPredicate("o")); err != nil {
		return nil, err
	}
	if r.WouldBackfillLinks.CleaningEventsNamed, err = countInt(ctx, s.DB, `
		SELECT COUNT(*)
		FROM cleaning_calendar_events c
		JOIN occupancies o ON o.id = c.occupancy_id
		WHERE c.named_stay_id IS NULL AND `+namedPredicate("o")); err != nil {
		return nil, err
	}
	if r.WouldBackfillLinks.CleaningEventsRaw, err = countInt(ctx, s.DB, `
		SELECT COUNT(*)
		FROM cleaning_calendar_events c
		WHERE c.raw_booking_block_id IS NULL
		  AND c.upstream_event_uid IS NOT NULL
		  AND c.cleaning_kind = 'provisional_block'`); err != nil {
		return nil, err
	}
	if r.WouldBackfillLinks.FinanceBookings, err = countInt(ctx, s.DB, `
		SELECT COUNT(*)
		FROM finance_bookings fb
		JOIN occupancies o ON o.id = fb.occupancy_id
		WHERE fb.named_stay_id IS NULL AND `+namedPredicate("o")); err != nil {
		return nil, err
	}
	if r.WouldBackfillLinks.Invoices, err = countInt(ctx, s.DB, `
		SELECT COUNT(*)
		FROM invoices i
		JOIN occupancies o ON o.id = i.occupancy_id
		WHERE i.named_stay_id IS NULL AND `+namedPredicate("o")); err != nil {
		return nil, err
	}

	if r.Conflicts.NamedStayOverlapPairs, err = countInt(ctx, s.DB, overlapPairsSQL(namedPredicate("a"), namedPredicate("b"))); err != nil {
		return nil, err
	}
	if r.Conflicts.RawBlockOverlapPairs, err = countInt(ctx, s.DB, overlapPairsSQL(rawPredicate("a"), rawPredicate("b"))); err != nil {
		return nil, err
	}
	if r.Conflicts.ExternalSaleRows, err = countInt(ctx, s.DB, `SELECT COUNT(*) FROM occupancies WHERE closure_state = 'external_sale'`); err != nil {
		return nil, err
	}
	if r.Conflicts.ActiveClosedRows, err = countInt(ctx, s.DB, `SELECT COUNT(*) FROM occupancies WHERE closure_state = 'closed' AND status IN ('active','updated')`); err != nil {
		return nil, err
	}

	if r.Unmapped.FinanceBookingsUnmatched, err = countInt(ctx, s.DB, `SELECT COUNT(*) FROM finance_bookings WHERE occupancy_id IS NULL AND named_stay_id IS NULL`); err != nil {
		return nil, err
	}
	if r.Unmapped.NukiCodesWithoutNamedLikeOccupancy, err = countInt(ctx, s.DB, `
		SELECT COUNT(*) FROM nuki_access_codes nac
		WHERE nac.named_stay_id IS NULL
		  AND NOT EXISTS (SELECT 1 FROM occupancies o WHERE o.id = nac.occupancy_id AND `+namedPredicate("o")+`)`); err != nil {
		return nil, err
	}
	if r.Unmapped.CleaningEventsWithoutMappingCandidate, err = countInt(ctx, s.DB, `
		SELECT COUNT(*) FROM cleaning_calendar_events c
		WHERE c.named_stay_id IS NULL AND c.raw_booking_block_id IS NULL
		  AND c.upstream_event_uid IS NULL
		  AND (c.occupancy_id IS NULL OR NOT EXISTS (SELECT 1 FROM occupancies o WHERE o.id = c.occupancy_id AND (`+namedPredicate("o")+` OR `+rawPredicate("o")+`)))`); err != nil {
		return nil, err
	}
	if r.Unmapped.InvoicesWithoutNamedLikeOccupancy, err = countInt(ctx, s.DB, `
		SELECT COUNT(*) FROM invoices i
		WHERE i.named_stay_id IS NULL
		  AND (i.occupancy_id IS NULL OR NOT EXISTS (SELECT 1 FROM occupancies o WHERE o.id = i.occupancy_id AND `+namedPredicate("o")+`))`); err != nil {
		return nil, err
	}

	if r.ReviewRequired.FinanceSyntheticNamedStays, err = countInt(ctx, s.DB, `
		SELECT COUNT(*) FROM occupancies
		WHERE source_type IN ('booking_payout', 'booking_statement')
		  AND COALESCE(representation_kind, '') = 'named_stay'`); err != nil {
		return nil, err
	}
	if r.ReviewRequired.LegacyGeneratedNightRows, err = countInt(ctx, s.DB, `SELECT COUNT(*) FROM occupancies WHERE COALESCE(representation_kind, '') = 'legacy_generated_night'`); err != nil {
		return nil, err
	}

	if r.Samples["named_stay_overlaps"], err = samplePairs(ctx, s.DB, overlapSamplesSQL(namedPredicate("a"), namedPredicate("b")), sampleLimit, "named_stay_overlap"); err != nil {
		return nil, err
	}
	if r.Samples["raw_block_overlaps"], err = samplePairs(ctx, s.DB, overlapSamplesSQL(rawPredicate("a"), rawPredicate("b")), sampleLimit, "raw_block_overlap"); err != nil {
		return nil, err
	}
	if r.Samples["unmatched_finance_bookings"], err = sampleRows(ctx, s.DB, `
		SELECT property_id, id, COALESCE(check_in_date, ''), COALESCE(check_out_date, ''), COALESCE(status, COALESCE(reservation_status, ''))
		FROM finance_bookings
		WHERE occupancy_id IS NULL AND named_stay_id IS NULL
		ORDER BY property_id, check_in_date, id
		LIMIT ?`, sampleLimit, "finance_unmatched"); err != nil {
		return nil, err
	}
	return r, nil
}

func rawCandidateSQL(selectExpr string) string {
	return `SELECT ` + selectExpr + ` FROM occupancies WHERE ` + rawPredicate("occupancies")
}

func namedCandidateSQL(selectExpr string) string {
	return `SELECT ` + selectExpr + ` FROM occupancies WHERE ` + namedPredicate("occupancies")
}

func rawPredicate(alias string) string {
	return fmt.Sprintf(`(COALESCE(%[1]s.representation_kind, '') = 'unnamed_block' OR (%[1]s.source_type = 'booking_ics' AND COALESCE(TRIM(%[1]s.guest_display_name), '') = ''))`, alias)
}

func namedPredicate(alias string) string {
	return fmt.Sprintf(`(COALESCE(%[1]s.representation_kind, '') = 'named_stay' OR COALESCE(TRIM(%[1]s.guest_display_name), '') <> '')`, alias)
}

func overlapPairsSQL(leftPredicate, rightPredicate string) string {
	return `SELECT COUNT(*)
		FROM occupancies a
		JOIN occupancies b ON a.property_id = b.property_id AND a.id < b.id
		WHERE a.status IN ('active','updated') AND b.status IN ('active','updated')
		  AND ` + leftPredicate + ` AND ` + rightPredicate + `
		  AND a.start_at < b.end_at AND b.start_at < a.end_at`
}

func overlapSamplesSQL(leftPredicate, rightPredicate string) string {
	return `SELECT a.property_id, a.id, b.id, a.start_at, a.end_at, b.start_at, b.end_at
		FROM occupancies a
		JOIN occupancies b ON a.property_id = b.property_id AND a.id < b.id
		WHERE a.status IN ('active','updated') AND b.status IN ('active','updated')
		  AND ` + leftPredicate + ` AND ` + rightPredicate + `
		  AND a.start_at < b.end_at AND b.start_at < a.end_at
		ORDER BY a.property_id, a.start_at, a.id
		LIMIT ?`
}

func countInt(ctx context.Context, db *sql.DB, query string, args ...interface{}) (int, error) {
	var n int
	if err := db.QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func countEstimatedNights(ctx context.Context, db *sql.DB, query string) (int, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	total := 0
	for rows.Next() {
		var start, end string
		if err := rows.Scan(&start, &end); err != nil {
			return 0, err
		}
		n, err := estimatedDateNights(start, end)
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, rows.Err()
}

func estimatedDateNights(start, end string) (int, error) {
	// Legacy rows store UTC RFC3339 instants. SQLite's date() mirrors the current
	// compatibility model closely enough for dry-run counts; apply mode must use
	// property-local dates before writing rows.
	startAt, err := time.Parse(time.RFC3339, start)
	if err != nil {
		return 0, err
	}
	endAt, err := time.Parse(time.RFC3339, end)
	if err != nil {
		return 0, err
	}
	sy, sm, sd := startAt.UTC().Date()
	ey, em, ed := endAt.UTC().Date()
	startDate := time.Date(sy, sm, sd, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(ey, em, ed, 0, 0, 0, 0, time.UTC)
	if !endDate.After(startDate) {
		return 0, nil
	}
	return int(endDate.Sub(startDate).Hours() / 24), nil
}

func samplePairs(ctx context.Context, db *sql.DB, query string, limit int, reason string) ([]PMS21Sample, error) {
	rows, err := db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PMS21Sample
	for rows.Next() {
		var s PMS21Sample
		if err := rows.Scan(&s.PropertyID, &s.ID, &s.OtherID, &s.Start, &s.End, &s.OtherStart, &s.OtherEnd); err != nil {
			return nil, err
		}
		s.Reason = reason
		out = append(out, s)
	}
	return out, rows.Err()
}

func sampleRows(ctx context.Context, db *sql.DB, query string, limit int, reason string) ([]PMS21Sample, error) {
	rows, err := db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PMS21Sample
	for rows.Next() {
		var s PMS21Sample
		if err := rows.Scan(&s.PropertyID, &s.ID, &s.Start, &s.End, &s.Status); err != nil {
			return nil, err
		}
		s.Reason = reason
		out = append(out, s)
	}
	return out, rows.Err()
}
