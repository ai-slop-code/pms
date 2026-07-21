package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrPMS21SchemaMissing          = errors.New("PMS 21 additive schema is not applied")
	ErrPMS21SevereConflicts        = errors.New("PMS 21 apply blocked by severe conflicts")
	ErrPMS21ReviewOverrideRequired = errors.New("PMS 21 apply has review-required stay candidates; explicit override is required")
)

type PMS21MigrationReport struct {
	Mode                  string                   `json:"mode"`
	ApplyImplemented      bool                     `json:"apply_implemented"`
	AllowReviewRequired   bool                     `json:"allow_review_required"`
	WouldCreate           PMS21CreateCounts        `json:"would_create"`
	WouldBackfillLinks    PMS21BackfillCounts      `json:"would_backfill_links"`
	Applied               PMS21ApplyCounts         `json:"applied"`
	Conflicts             PMS21ConflictCounts      `json:"conflicts"`
	Unmapped              PMS21UnmappedCounts      `json:"unmapped"`
	ReviewRequired        PMS21ReviewCounts        `json:"review_required"`
	ExistingNewModelRows  PMS21ExistingCounts      `json:"existing_new_model_rows"`
	Samples               map[string][]PMS21Sample `json:"samples"`
	IdempotentComparedRun bool                     `json:"idempotent_compared_with_prior_run"`
}

// Keep the old exported name source-compatible for callers of the dry-run API.
type PMS21MigrationDryRunReport = PMS21MigrationReport

type PMS21CreateCounts struct {
	RawBookingBlocks           int `json:"raw_booking_blocks"`
	RawBookingBlockNights      int `json:"raw_booking_block_nights"`
	NamedStays                 int `json:"named_stays"`
	NamedStayNights            int `json:"named_stay_nights"`
	StaySourceLinks            int `json:"stay_source_links"`
	PropertyAvailabilityBlocks int `json:"property_availability_blocks"`
	MigrationMapRows           int `json:"occupancy_stay_migration_map"`
	AutoConfirmedNamedStays    int `json:"auto_confirmed_named_stays"`
	ReviewRequiredNamedStays   int `json:"review_required_named_stays"`
}

type PMS21ApplyCounts struct {
	Created      PMS21CreateCounts   `json:"created"`
	UpdatedLinks PMS21BackfillCounts `json:"updated_links"`
	Skipped      PMS21CreateCounts   `json:"skipped"`
}

type PMS21BackfillCounts struct {
	NukiAccessCodes              int `json:"nuki_access_codes_named_stay_id"`
	NukiGuestDailyEntries        int `json:"nuki_guest_daily_entries_named_stay_id"`
	CleaningEventsNamed          int `json:"cleaning_events_named_stay_id"`
	CleaningEventsRaw            int `json:"cleaning_events_raw_booking_block_id"`
	FinanceBookings              int `json:"finance_bookings_named_stay_id"`
	Invoices                     int `json:"invoices_named_stay_id"`
	NamedStaysConfirmedByFinance int `json:"named_stays_confirmed_by_finance"`
}

type PMS21ConflictCounts struct {
	NamedStayOverlapPairs            int `json:"named_stay_overlap_pairs"`
	ExistingNamedStayNightCollisions int `json:"existing_named_stay_night_collisions"`
	NamedStayAvailabilityOverlaps    int `json:"named_stay_availability_overlaps"`
	RawBlockOverlapPairs             int `json:"raw_block_overlap_pairs"`
	ExternalSaleRows                 int `json:"external_sale_rows"`
	ActiveClosedRows                 int `json:"active_closed_rows"`
}

func (c PMS21ConflictCounts) Severe() int {
	return c.NamedStayOverlapPairs + c.ExistingNamedStayNightCollisions + c.NamedStayAvailabilityOverlaps
}

type PMS21UnmappedCounts struct {
	FinanceBookingsUnmatched              int `json:"finance_bookings_unmatched"`
	NukiCodesWithoutNamedLikeOccupancy    int `json:"nuki_codes_without_named_like_occupancy"`
	CleaningEventsWithoutMappingCandidate int `json:"cleaning_events_without_mapping_candidate"`
	InvoicesWithoutNamedLikeOccupancy     int `json:"invoices_without_named_like_occupancy"`
}

type PMS21ReviewCounts struct {
	FinanceSyntheticNamedStays int `json:"finance_synthetic_named_stays"`
	FinanceBookingsNeedsReview int `json:"finance_bookings_needs_review"`
	LegacyGeneratedNightRows   int `json:"legacy_generated_night_rows"`
	NonReservationNamedStays   int `json:"non_reservation_named_stays"`
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

type pms21LegacyRow struct {
	ID                   int64
	PropertyID           int64
	PropertyTimezone     string
	SourceType           string
	SourceEventUID       string
	StartAt              string
	EndAt                string
	Status               string
	RawSummary           sql.NullString
	GuestDisplayName     sql.NullString
	ContentHash          string
	ImportedAt           string
	LastSyncedAt         string
	LastSyncRunID        sql.NullInt64
	ClosureState         sql.NullString
	ExternalRevenueCents sql.NullInt64
	ExternalCurrency     sql.NullString
	ExternalChannel      sql.NullString
	StayOutcome          sql.NullString
	CleaningExcluded     bool
	UpstreamSourceType   sql.NullString
	UpstreamEventUID     sql.NullString
	RepresentationKind   sql.NullString
	SourceDtstamp        sql.NullString
	HasFinanceEvidence   bool
}

type pms21Classification struct {
	Kind          string
	StayType      string
	ReviewStatus  string
	ReviewReason  string
	MigrationKind string
	CheckInDate   string
	CheckOutDate  string
	SourceType    string
	SourceUID     string
}

type pms21FinanceRow struct {
	ID                int64
	PropertyID        int64
	ReferenceNumber   string
	SourceChannel     string
	CheckInDate       sql.NullString
	CheckOutDate      sql.NullString
	GuestName         sql.NullString
	Status            sql.NullString
	ReservationStatus sql.NullString
	HasPayoutData     bool
	HasStatementData  bool
}

type pms21CandidateRange struct {
	PropertyID int64
	ID         int64
	Kind       string
	Start      string
	End        string
}

const (
	pms21KindRaw          = "raw_block"
	pms21KindNamed        = "named_stay"
	pms21KindAvailability = "availability_block"
	pms21KindUnmapped     = "unmapped"
)

// PlanPMS21Migration and ApplyPMS21Migration deliberately share load and
// classification functions so apply cannot drift from the reviewed dry run.
func (s *Store) PlanPMS21Migration(ctx context.Context, sampleLimit int) (*PMS21MigrationReport, error) {
	return s.planPMS21Migration(ctx, sampleLimit, false)
}

func (s *Store) planPMS21Migration(ctx context.Context, sampleLimit int, allowReview bool) (*PMS21MigrationReport, error) {
	if sampleLimit <= 0 {
		sampleLimit = 10
	}
	if err := ensurePMS21Schema(ctx, s.DB); err != nil {
		return nil, err
	}
	r := &PMS21MigrationReport{
		Mode: "dry_run", ApplyImplemented: true, AllowReviewRequired: allowReview,
		Samples: map[string][]PMS21Sample{}, IdempotentComparedRun: true,
	}
	if err := s.populatePMS21ExistingCounts(ctx, r); err != nil {
		return nil, err
	}
	if err := s.populatePMS21Diagnostics(ctx, r, sampleLimit); err != nil {
		return nil, err
	}
	rows, err := loadPMS21LegacyRows(ctx, s.DB)
	if err != nil {
		return nil, err
	}
	var namedCandidates, availabilityCandidates []pms21CandidateRange
	for _, row := range rows {
		class, err := classifyPMS21LegacyRow(row)
		if err != nil {
			return nil, fmt.Errorf("classify occupancy %d: %w", row.ID, err)
		}
		var mapped int
		if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM occupancy_stay_migration_map WHERE old_occupancy_id = ?`, row.ID).Scan(&mapped); err != nil {
			return nil, err
		}
		if mapped > 0 {
			continue
		}
		nights, err := dateNights(class.CheckInDate, class.CheckOutDate)
		if err != nil {
			return nil, fmt.Errorf("occupancy %d dates: %w", row.ID, err)
		}
		switch class.Kind {
		case pms21KindRaw:
			blocks, blockNights, err := plannedPMS21RawCreates(ctx, s.DB, row, class, nights)
			if err != nil {
				return nil, err
			}
			r.WouldCreate.RawBookingBlocks += blocks
			r.WouldCreate.RawBookingBlockNights += blockNights
			r.WouldCreate.MigrationMapRows++
			if err := addPMS21CandidateLinkCounts(ctx, s.DB, row.ID, class, &r.WouldBackfillLinks); err != nil {
				return nil, err
			}
		case pms21KindNamed:
			r.WouldCreate.NamedStays++
			if legacyStatusActive(row.Status) {
				r.WouldCreate.NamedStayNights += len(nights)
			}
			r.WouldCreate.MigrationMapRows++
			if class.ReviewStatus == "confirmed" {
				r.WouldCreate.AutoConfirmedNamedStays++
			} else {
				r.WouldCreate.ReviewRequiredNamedStays++
				r.ReviewRequired.NonReservationNamedStays++
			}
			if class.SourceUID != "" && class.SourceType == UpstreamSourceBookingICS {
				r.WouldCreate.StaySourceLinks++
			}
			if err := addPMS21CandidateLinkCounts(ctx, s.DB, row.ID, class, &r.WouldBackfillLinks); err != nil {
				return nil, err
			}
			if legacyStatusActive(row.Status) {
				namedCandidates = append(namedCandidates, pms21CandidateRange{PropertyID: row.PropertyID, ID: row.ID, Kind: "occupancy", Start: class.CheckInDate, End: class.CheckOutDate})
			}
		case pms21KindAvailability:
			r.WouldCreate.PropertyAvailabilityBlocks++
			r.WouldCreate.MigrationMapRows++
			if legacyStatusActive(row.Status) {
				availabilityCandidates = append(availabilityCandidates, pms21CandidateRange{PropertyID: row.PropertyID, ID: row.ID, Kind: "occupancy", Start: class.CheckInDate, End: class.CheckOutDate})
			}
		case pms21KindUnmapped:
			r.WouldCreate.MigrationMapRows++
		}
	}
	financeRows, err := loadPMS21FinanceRows(ctx, s.DB)
	if err != nil {
		return nil, err
	}
	for _, row := range financeRows {
		class := classifyPMS21FinanceRow(row)
		if class.Kind != pms21KindNamed {
			r.ReviewRequired.FinanceBookingsNeedsReview++
			continue
		}
		nights, err := dateNights(class.CheckInDate, class.CheckOutDate)
		if err != nil {
			return nil, fmt.Errorf("finance booking %d dates: %w", row.ID, err)
		}
		r.WouldCreate.NamedStays++
		if financeStatusActive(row) {
			r.WouldCreate.NamedStayNights += len(nights)
		}
		r.WouldBackfillLinks.FinanceBookings++
		if class.ReviewStatus == "confirmed" {
			r.WouldCreate.AutoConfirmedNamedStays++
		} else {
			r.WouldCreate.ReviewRequiredNamedStays++
			r.ReviewRequired.FinanceBookingsNeedsReview++
		}
		if financeStatusActive(row) {
			namedCandidates = append(namedCandidates, pms21CandidateRange{PropertyID: row.PropertyID, ID: row.ID, Kind: "finance", Start: class.CheckInDate, End: class.CheckOutDate})
		}
	}
	if err := s.populatePMS21CandidateConflicts(ctx, r, namedCandidates, availabilityCandidates); err != nil {
		return nil, err
	}
	return r, nil
}

func plannedPMS21RawCreates(ctx context.Context, db *sql.DB, row pms21LegacyRow, class pms21Classification, nights []string) (int, int, error) {
	var rawID int64
	err := db.QueryRowContext(ctx, `SELECT id FROM raw_booking_blocks WHERE property_id = ? AND source_type = ? AND source_event_uid = ?`, row.PropertyID, class.SourceType, class.SourceUID).Scan(&rawID)
	if errors.Is(err, sql.ErrNoRows) {
		if legacyStatusActive(row.Status) {
			return 1, len(nights), nil
		}
		return 1, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	if !legacyStatusActive(row.Status) {
		return 0, 0, nil
	}
	missing := 0
	for _, night := range nights {
		var exists int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM raw_booking_block_nights WHERE property_id = ? AND raw_booking_block_id = ? AND local_night_date = ?`, row.PropertyID, rawID, night).Scan(&exists); err != nil {
			return 0, 0, err
		}
		if exists == 0 {
			missing++
		}
	}
	return 0, missing, nil
}

func addPMS21CandidateLinkCounts(ctx context.Context, db *sql.DB, occupancyID int64, class pms21Classification, counts *PMS21BackfillCounts) error {
	items := []struct {
		query string
		dest  *int
	}{
		{`SELECT COUNT(*) FROM nuki_access_codes WHERE occupancy_id = ? AND named_stay_id IS NULL`, &counts.NukiAccessCodes},
		{`SELECT COUNT(*) FROM nuki_guest_daily_entries WHERE occupancy_id = ? AND named_stay_id IS NULL`, &counts.NukiGuestDailyEntries},
		{`SELECT COUNT(*) FROM finance_bookings WHERE occupancy_id = ? AND named_stay_id IS NULL`, &counts.FinanceBookings},
		{`SELECT COUNT(*) FROM invoices WHERE occupancy_id = ? AND named_stay_id IS NULL`, &counts.Invoices},
	}
	if class.Kind == pms21KindNamed {
		items = append(items, struct {
			query string
			dest  *int
		}{`SELECT COUNT(*) FROM cleaning_calendar_events WHERE occupancy_id = ? AND named_stay_id IS NULL`, &counts.CleaningEventsNamed})
	} else if class.Kind == pms21KindRaw {
		items = append(items, struct {
			query string
			dest  *int
		}{`SELECT COUNT(*) FROM cleaning_calendar_events WHERE occupancy_id = ? AND raw_booking_block_id IS NULL`, &counts.CleaningEventsRaw})
	}
	for _, item := range items {
		var n int
		if err := db.QueryRowContext(ctx, item.query, occupancyID).Scan(&n); err != nil {
			return err
		}
		*item.dest += n
	}
	return nil
}

func (s *Store) ApplyPMS21Migration(ctx context.Context, sampleLimit int, allowReview bool) (*PMS21MigrationReport, error) {
	plan, err := s.planPMS21Migration(ctx, sampleLimit, allowReview)
	if err != nil {
		return nil, err
	}
	if plan.Conflicts.Severe() > 0 {
		return plan, fmt.Errorf("%w: named_stay_overlap_pairs=%d existing_named_stay_night_collisions=%d named_stay_availability_overlaps=%d", ErrPMS21SevereConflicts, plan.Conflicts.NamedStayOverlapPairs, plan.Conflicts.ExistingNamedStayNightCollisions, plan.Conflicts.NamedStayAvailabilityOverlaps)
	}
	if plan.ReviewRequired.FinanceBookingsNeedsReview > plan.WouldCreate.ReviewRequiredNamedStays {
		return plan, fmt.Errorf("%w: finance bookings lack valid stay dates", ErrPMS21ReviewOverrideRequired)
	}
	if plan.WouldCreate.ReviewRequiredNamedStays > 0 && !allowReview {
		return plan, fmt.Errorf("%w: candidates=%d", ErrPMS21ReviewOverrideRequired, plan.WouldCreate.ReviewRequiredNamedStays)
	}
	legacyRows, err := loadPMS21LegacyRows(ctx, s.DB)
	if err != nil {
		return nil, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	actual := PMS21ApplyCounts{}
	for _, row := range legacyRows {
		class, err := classifyPMS21LegacyRow(row)
		if err != nil {
			return nil, fmt.Errorf("classify occupancy %d: %w", row.ID, err)
		}
		if err := applyPMS21LegacyRow(ctx, tx, row, class, allowReview, &actual); err != nil {
			return nil, fmt.Errorf("apply occupancy %d: %w", row.ID, err)
		}
	}
	financeRows, err := loadPMS21FinanceRows(ctx, tx)
	if err != nil {
		return nil, err
	}
	for _, row := range financeRows {
		class := classifyPMS21FinanceRow(row)
		if class.Kind != pms21KindNamed {
			return nil, fmt.Errorf("finance booking %d: %w", row.ID, ErrPMS21ReviewOverrideRequired)
		}
		if err := applyPMS21FinanceRow(ctx, tx, row, class, allowReview, &actual); err != nil {
			return nil, fmt.Errorf("apply finance booking %d: %w", row.ID, err)
		}
	}
	if err := backfillPMS21IntegrationLinks(ctx, tx, &actual.UpdatedLinks); err != nil {
		return nil, err
	}
	if err := confirmPMS21NamedStaysWithFinanceEvidence(ctx, tx, &actual.UpdatedLinks); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	result, err := s.planPMS21Migration(ctx, sampleLimit, allowReview)
	if err != nil {
		return nil, err
	}
	result.Mode = "apply"
	result.Applied = actual
	return result, nil
}

func classifyPMS21LegacyRow(row pms21LegacyRow) (pms21Classification, error) {
	ci, co, err := legacyPropertyDates(row)
	if err != nil {
		return pms21Classification{}, err
	}
	class := pms21Classification{Kind: pms21KindUnmapped, MigrationKind: "unmapped", CheckInDate: ci, CheckOutDate: co}
	representation := strings.TrimSpace(row.RepresentationKind.String)
	closure := strings.TrimSpace(row.ClosureState.String)
	guest := strings.TrimSpace(row.GuestDisplayName.String)
	upstreamUID := strings.TrimSpace(row.UpstreamEventUID.String)
	if upstreamUID == "" {
		upstreamUID = strings.TrimSpace(row.SourceEventUID)
	}
	upstreamType := strings.TrimSpace(row.UpstreamSourceType.String)
	if upstreamType == "" && row.SourceType == UpstreamSourceBookingICS {
		upstreamType = UpstreamSourceBookingICS
	}
	isBookingICS := strings.TrimSpace(row.SourceType) == UpstreamSourceBookingICS

	if closure == ClosureStateClosed || representation == RepresentationManualClosure {
		class.Kind = pms21KindAvailability
		class.MigrationKind = "availability_block"
		return class, nil
	}
	if isBookingICS && (representation == RepresentationUnnamedBlock ||
		(representation == RepresentationLegacyGeneratedNight && guest == "" && upstreamUID != "") ||
		(guest == "" && representation != RepresentationNamedStay)) {
		class.Kind = pms21KindRaw
		class.MigrationKind = "raw_block"
		class.SourceType = UpstreamSourceBookingICS
		class.SourceUID = upstreamUID
		if class.SourceUID == "" {
			return pms21Classification{}, errors.New("raw block candidate has no source UID")
		}
		return class, nil
	}

	isFinanceReservation := row.SourceType == "booking_payout" || row.SourceType == "booking_statement"
	isNamedLike := representation == RepresentationNamedStay || representation == RepresentationSyntheticFinance ||
		representation == RepresentationExternalSale || guest != "" || closure == ClosureStateExternalSale || isFinanceReservation
	if !isNamedLike {
		return class, nil
	}
	class.Kind = pms21KindNamed
	class.MigrationKind = "named_stay"
	class.StayType = StayTypeExternal
	if row.SourceType == UpstreamSourceBookingICS || upstreamType == UpstreamSourceBookingICS || isFinanceReservation {
		class.StayType = StayTypeBookingCom
	}
	if closure == ClosureStateExternalSale || representation == RepresentationExternalSale {
		class.StayType = StayTypeExternal
	}
	class.ReviewStatus = "needs_review"
	class.ReviewReason = "legacy_non_reservation_stay"
	if isFinanceReservation || row.HasFinanceEvidence {
		class.ReviewStatus = "confirmed"
		class.ReviewReason = ""
	}
	if isFinanceReservation {
		class.MigrationKind = "synthetic_finance"
	}
	class.SourceType = upstreamType
	class.SourceUID = upstreamUID
	if isFinanceReservation {
		class.SourceType = row.SourceType
		class.SourceUID = row.SourceEventUID
	}
	return class, nil
}

func classifyPMS21FinanceRow(row pms21FinanceRow) pms21Classification {
	class := pms21Classification{
		Kind:          pms21KindUnmapped,
		MigrationKind: "synthetic_finance",
		ReviewStatus:  "needs_review",
		ReviewReason:  "finance_missing_stay_dates",
		SourceType:    strings.TrimSpace(row.SourceChannel),
		SourceUID:     strings.TrimSpace(row.ReferenceNumber),
		StayType:      StayTypeExternal,
	}
	ci := strings.TrimSpace(row.CheckInDate.String)
	co := strings.TrimSpace(row.CheckOutDate.String)
	if !row.CheckInDate.Valid || !row.CheckOutDate.Valid {
		return class
	}
	if _, _, err := parseNamedStayRange(ci, co); err != nil {
		return class
	}
	class.Kind = pms21KindNamed
	class.CheckInDate = ci
	class.CheckOutDate = co
	if strings.EqualFold(class.SourceType, "booking_com") {
		class.StayType = StayTypeBookingCom
	}
	if strings.EqualFold(class.SourceType, "booking_com") && (row.HasPayoutData || row.HasStatementData) {
		class.ReviewStatus = "confirmed"
		class.ReviewReason = ""
	} else {
		class.ReviewReason = "finance_source_unverified"
	}
	return class
}

func financeStatusActive(row pms21FinanceRow) bool {
	status := strings.ToUpper(strings.TrimSpace(row.Status.String))
	if status == "" {
		status = strings.ToUpper(strings.TrimSpace(row.ReservationStatus.String))
	}
	return status != "CANCELLED" && status != "CANCELLED_BY_GUEST" && status != "CANCELLED_BY_PARTNER"
}

func pms21NukiPendingEligible(status string, class pms21Classification, stayOutcome sql.NullString) bool {
	if status != NamedStayStatusActive || !namedStayNukiEligible(class.StayType, class.ReviewStatus) {
		return false
	}
	outcome := strings.TrimSpace(stayOutcome.String)
	if outcome == StayOutcomeCancelledNonRefundable || outcome == StayOutcomeNoShow {
		return false
	}
	return class.CheckOutDate >= time.Now().UTC().Format("2006-01-02")
}

func applyPMS21LegacyRow(ctx context.Context, tx *sql.Tx, row pms21LegacyRow, class pms21Classification, allowReview bool, counts *PMS21ApplyCounts) error {
	var existingKind string
	err := tx.QueryRowContext(ctx, `SELECT migration_kind FROM occupancy_stay_migration_map WHERE old_occupancy_id = ?`, row.ID).Scan(&existingKind)
	if err == nil {
		incrementPMS21Skipped(class, counts)
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if class.Kind == pms21KindNamed && class.ReviewStatus == "needs_review" && !allowReview {
		return ErrPMS21ReviewOverrideRequired
	}
	now := time.Now().UTC().Format(time.RFC3339)
	switch class.Kind {
	case pms21KindRaw:
		status := "deleted_from_source"
		if legacyStatusActive(row.Status) {
			status = "active"
		}
		var rawID int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM raw_booking_blocks WHERE property_id = ? AND source_type = ? AND source_event_uid = ?`, row.PropertyID, class.SourceType, class.SourceUID).Scan(&rawID)
		if errors.Is(err, sql.ErrNoRows) {
			res, err := tx.ExecContext(ctx, `
				INSERT INTO raw_booking_blocks (
					property_id, source_type, source_event_uid, check_in_date, check_out_date, status,
					raw_summary, content_hash, source_dtstamp, first_seen_sync_run_id, last_sync_run_id,
					imported_at, last_synced_at, deleted_from_source_at, created_at, updated_at
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				row.PropertyID, class.SourceType, class.SourceUID, class.CheckInDate, class.CheckOutDate, status,
				nullStringArg(row.RawSummary), row.ContentHash, nullStringArg(row.SourceDtstamp), nullInt64Arg(row.LastSyncRunID), nullInt64Arg(row.LastSyncRunID),
				row.ImportedAt, row.LastSyncedAt, deletedAtArg(status, row.LastSyncedAt), now, now)
			if err != nil {
				return err
			}
			rawID, err = res.LastInsertId()
			if err != nil {
				return err
			}
			counts.Created.RawBookingBlocks++
		} else if err != nil {
			return err
		} else {
			counts.Skipped.RawBookingBlocks++
		}
		if status == "active" {
			for _, night := range mustDateNights(class.CheckInDate, class.CheckOutDate) {
				res, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO raw_booking_block_nights (property_id, raw_booking_block_id, local_night_date, active, created_at, updated_at) VALUES (?, ?, ?, 1, ?, ?)`, row.PropertyID, rawID, night, now, now)
				if err != nil {
					return err
				}
				n, _ := res.RowsAffected()
				counts.Created.RawBookingBlockNights += int(n)
			}
		}
		if err := insertPMS21Map(ctx, tx, row, class.MigrationKind, rawID, 0, 0, now); err != nil {
			return err
		}
		counts.Created.MigrationMapRows++
	case pms21KindNamed:
		status := NamedStayStatusArchived
		if legacyStatusActive(row.Status) {
			status = NamedStayStatusActive
		} else if strings.EqualFold(row.Status, "cancelled") {
			status = NamedStayStatusCancelled
		}
		displayName := strings.TrimSpace(row.GuestDisplayName.String)
		if displayName == "" {
			displayName = strings.TrimSpace(row.RawSummary.String)
		}
		if displayName == "" {
			displayName = strings.TrimSpace(row.SourceEventUID)
		}
		cleaning := defaultCleaningRequired(class.StayType) && !row.CleaningExcluded
		nukiStatus := NukiGenerationNotApplicable
		if pms21NukiPendingEligible(status, class, row.StayOutcome) {
			nukiStatus = NukiGenerationPending
		}
		res, err := tx.ExecContext(ctx, `
			INSERT INTO named_stays (
				property_id, display_name, stay_type, check_in_date, check_out_date, status, cleaning_required,
				source_channel, source_reference, manual_revenue_cents, manual_revenue_currency,
				review_status, review_reason, stay_outcome, nuki_generation_status, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row.PropertyID, displayName, class.StayType, class.CheckInDate, class.CheckOutDate, status, boolInt(cleaning),
			nullableString(class.SourceType), nullableString(class.SourceUID), nullInt64Arg(row.ExternalRevenueCents), nullStringArg(row.ExternalCurrency),
			class.ReviewStatus, nullableString(class.ReviewReason), nullStringArg(row.StayOutcome), nukiStatus, now, now)
		if err != nil {
			return err
		}
		stayID, err := res.LastInsertId()
		if err != nil {
			return err
		}
		counts.Created.NamedStays++
		if class.ReviewStatus == "confirmed" {
			counts.Created.AutoConfirmedNamedStays++
		} else {
			counts.Created.ReviewRequiredNamedStays++
		}
		if status == NamedStayStatusActive {
			for _, night := range mustDateNights(class.CheckInDate, class.CheckOutDate) {
				if _, err := tx.ExecContext(ctx, `INSERT INTO named_stay_nights (property_id, named_stay_id, local_night_date, active, created_at) VALUES (?, ?, ?, 1, ?)`, row.PropertyID, stayID, night, now); err != nil {
					return err
				}
				counts.Created.NamedStayNights++
			}
		}
		if err := insertPMS21Map(ctx, tx, row, class.MigrationKind, 0, stayID, 0, now); err != nil {
			return err
		}
		counts.Created.MigrationMapRows++
		if class.SourceUID != "" && class.SourceType == UpstreamSourceBookingICS {
			var rawID sql.NullInt64
			if err := tx.QueryRowContext(ctx, `SELECT id FROM raw_booking_blocks WHERE property_id = ? AND source_event_uid = ? ORDER BY id LIMIT 1`, row.PropertyID, class.SourceUID).Scan(&rawID); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			linkStatus := "source_deleted"
			if rawID.Valid {
				linkStatus = "active"
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO stay_source_links (property_id, named_stay_id, raw_booking_block_id, source_type, source_event_uid, linked_check_in_date, linked_check_out_date, link_status, conflict_reason, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, row.PropertyID, stayID, nullInt64Arg(rawID), class.SourceType, class.SourceUID, class.CheckInDate, class.CheckOutDate, linkStatus, conflictReasonArg(linkStatus), now, now); err != nil {
				return err
			}
			counts.Created.StaySourceLinks++
		}
	case pms21KindAvailability:
		status := "archived"
		if legacyStatusActive(row.Status) {
			status = "active"
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO property_availability_blocks (property_id, block_type, start_date, end_date, reason, source_occupancy_id, status, created_at, updated_at) VALUES (?, 'closed', ?, ?, ?, ?, ?, ?, ?)`, row.PropertyID, class.CheckInDate, class.CheckOutDate, nullStringArg(row.RawSummary), row.ID, status, now, now)
		if err != nil {
			return err
		}
		blockID, err := res.LastInsertId()
		if err != nil {
			return err
		}
		counts.Created.PropertyAvailabilityBlocks++
		if err := insertPMS21Map(ctx, tx, row, class.MigrationKind, 0, 0, blockID, now); err != nil {
			return err
		}
		counts.Created.MigrationMapRows++
	case pms21KindUnmapped:
		if err := insertPMS21Map(ctx, tx, row, "unmapped", 0, 0, 0, now); err != nil {
			return err
		}
		counts.Created.MigrationMapRows++
	}
	return nil
}

func applyPMS21FinanceRow(ctx context.Context, tx *sql.Tx, row pms21FinanceRow, class pms21Classification, allowReview bool, counts *PMS21ApplyCounts) error {
	if class.ReviewStatus == "needs_review" && !allowReview {
		return ErrPMS21ReviewOverrideRequired
	}
	status := NamedStayStatusActive
	if !financeStatusActive(row) {
		status = NamedStayStatusCancelled
	}
	displayName := strings.TrimSpace(row.GuestName.String)
	if displayName == "" {
		displayName = strings.TrimSpace(row.ReferenceNumber)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	nukiStatus := NukiGenerationNotApplicable
	if pms21NukiPendingEligible(status, class, sql.NullString{}) {
		nukiStatus = NukiGenerationPending
	}
	res, err := tx.ExecContext(ctx, `
		INSERT INTO named_stays (
			property_id, display_name, stay_type, check_in_date, check_out_date, status, cleaning_required,
			source_channel, source_reference, review_status, review_reason, nuki_generation_status,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		row.PropertyID, displayName, class.StayType, class.CheckInDate, class.CheckOutDate, status,
		boolInt(defaultCleaningRequired(class.StayType)), nullableString(class.SourceType), nullableString(class.SourceUID),
		class.ReviewStatus, nullableString(class.ReviewReason), nukiStatus, now, now)
	if err != nil {
		return err
	}
	stayID, err := res.LastInsertId()
	if err != nil {
		return err
	}
	counts.Created.NamedStays++
	if class.ReviewStatus == "confirmed" {
		counts.Created.AutoConfirmedNamedStays++
	} else {
		counts.Created.ReviewRequiredNamedStays++
	}
	if status == NamedStayStatusActive {
		for _, night := range mustDateNights(class.CheckInDate, class.CheckOutDate) {
			if _, err := tx.ExecContext(ctx, `INSERT INTO named_stay_nights (property_id, named_stay_id, local_night_date, active, created_at) VALUES (?, ?, ?, 1, ?)`, row.PropertyID, stayID, night, now); err != nil {
				return err
			}
			counts.Created.NamedStayNights++
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE finance_bookings SET named_stay_id = ?, updated_at = ? WHERE id = ? AND occupancy_id IS NULL AND named_stay_id IS NULL`, stayID, now, row.ID)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return errors.New("finance booking was concurrently linked")
	}
	counts.UpdatedLinks.FinanceBookings++
	return nil
}

func backfillPMS21IntegrationLinks(ctx context.Context, tx *sql.Tx, counts *PMS21BackfillCounts) error {
	updates := []struct {
		query string
		count *int
	}{
		{`UPDATE nuki_access_codes SET named_stay_id = (SELECT named_stay_id FROM occupancy_stay_migration_map m WHERE m.old_occupancy_id = nuki_access_codes.occupancy_id AND m.property_id = nuki_access_codes.property_id) WHERE named_stay_id IS NULL AND EXISTS (SELECT 1 FROM occupancy_stay_migration_map m WHERE m.old_occupancy_id = nuki_access_codes.occupancy_id AND m.property_id = nuki_access_codes.property_id AND m.named_stay_id IS NOT NULL)`, &counts.NukiAccessCodes},
		{`UPDATE nuki_guest_daily_entries SET named_stay_id = (SELECT named_stay_id FROM occupancy_stay_migration_map m WHERE m.old_occupancy_id = nuki_guest_daily_entries.occupancy_id AND m.property_id = nuki_guest_daily_entries.property_id) WHERE named_stay_id IS NULL AND EXISTS (SELECT 1 FROM occupancy_stay_migration_map m WHERE m.old_occupancy_id = nuki_guest_daily_entries.occupancy_id AND m.property_id = nuki_guest_daily_entries.property_id AND m.named_stay_id IS NOT NULL)`, &counts.NukiGuestDailyEntries},
		{`UPDATE cleaning_calendar_events SET named_stay_id = (SELECT named_stay_id FROM occupancy_stay_migration_map m WHERE m.old_occupancy_id = cleaning_calendar_events.occupancy_id AND m.property_id = cleaning_calendar_events.property_id) WHERE named_stay_id IS NULL AND EXISTS (SELECT 1 FROM occupancy_stay_migration_map m WHERE m.old_occupancy_id = cleaning_calendar_events.occupancy_id AND m.property_id = cleaning_calendar_events.property_id AND m.named_stay_id IS NOT NULL)`, &counts.CleaningEventsNamed},
		{`UPDATE cleaning_calendar_events SET raw_booking_block_id = (SELECT raw_booking_block_id FROM occupancy_stay_migration_map m WHERE m.old_occupancy_id = cleaning_calendar_events.occupancy_id AND m.property_id = cleaning_calendar_events.property_id) WHERE raw_booking_block_id IS NULL AND EXISTS (SELECT 1 FROM occupancy_stay_migration_map m WHERE m.old_occupancy_id = cleaning_calendar_events.occupancy_id AND m.property_id = cleaning_calendar_events.property_id AND m.raw_booking_block_id IS NOT NULL)`, &counts.CleaningEventsRaw},
		{`UPDATE finance_bookings SET named_stay_id = (SELECT named_stay_id FROM occupancy_stay_migration_map m WHERE m.old_occupancy_id = finance_bookings.occupancy_id AND m.property_id = finance_bookings.property_id) WHERE named_stay_id IS NULL AND occupancy_id IS NOT NULL AND EXISTS (SELECT 1 FROM occupancy_stay_migration_map m WHERE m.old_occupancy_id = finance_bookings.occupancy_id AND m.property_id = finance_bookings.property_id AND m.named_stay_id IS NOT NULL)`, &counts.FinanceBookings},
		{`UPDATE invoices SET named_stay_id = (SELECT named_stay_id FROM occupancy_stay_migration_map m WHERE m.old_occupancy_id = invoices.occupancy_id AND m.property_id = invoices.property_id) WHERE named_stay_id IS NULL AND occupancy_id IS NOT NULL AND EXISTS (SELECT 1 FROM occupancy_stay_migration_map m WHERE m.old_occupancy_id = invoices.occupancy_id AND m.property_id = invoices.property_id AND m.named_stay_id IS NOT NULL)`, &counts.Invoices},
	}
	for _, update := range updates {
		res, err := tx.ExecContext(ctx, update.query)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		*update.count += int(n)
	}
	return nil
}

func confirmPMS21NamedStaysWithFinanceEvidence(ctx context.Context, tx *sql.Tx, counts *PMS21BackfillCounts) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE named_stays
		SET review_status = 'confirmed',
		    review_reason = NULL,
		    nuki_generation_status = CASE
		        WHEN status = 'active'
		         AND stay_type IN ('booking_com', 'external')
		         AND (stay_outcome IS NULL OR stay_outcome NOT IN ('cancelled_non_refundable', 'no_show'))
		         AND check_out_date >= ?
		         AND COALESCE(nuki_generation_status, 'not_applicable') = 'not_applicable'
		        THEN 'pending'
		        ELSE nuki_generation_status
		    END,
		    updated_at = ?
		WHERE review_status = 'needs_review'
		  AND review_reason = 'legacy_non_reservation_stay'
		  AND EXISTS (
		      SELECT 1
		      FROM finance_bookings fb
		      WHERE fb.named_stay_id = named_stays.id
		        AND fb.property_id = named_stays.property_id
		        AND lower(trim(COALESCE(fb.source_channel, ''))) = 'booking_com'
		        AND (fb.has_payout_data = 1 OR fb.has_statement_data = 1)
		        AND upper(trim(COALESCE(fb.status, fb.reservation_status, ''))) NOT IN
		            ('CANCELLED', 'CANCELLED_BY_GUEST', 'CANCELLED_BY_PARTNER')
		  )`, time.Now().UTC().Format("2006-01-02"), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	counts.NamedStaysConfirmedByFinance += int(n)
	return nil
}

func loadPMS21LegacyRows(ctx context.Context, q queryer) ([]pms21LegacyRow, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT o.id, o.property_id, COALESCE(p.timezone, 'UTC'), o.source_type, o.source_event_uid,
		       o.start_at, o.end_at, o.status, o.raw_summary, o.guest_display_name, o.content_hash,
		       o.imported_at, o.last_synced_at, o.last_sync_run_id, o.closure_state,
		       o.external_net_amount_cents, o.external_currency, o.external_channel, o.stay_outcome,
		       COALESCE(o.cleaning_calendar_excluded, 0), o.upstream_source_type, o.upstream_event_uid,
		       o.representation_kind, o.source_dtstamp,
		       EXISTS (
		           SELECT 1
		           FROM finance_bookings fb
		           WHERE fb.property_id = o.property_id
		             AND fb.occupancy_id = o.id
		             AND lower(trim(COALESCE(fb.source_channel, ''))) = 'booking_com'
		             AND (fb.has_payout_data = 1 OR fb.has_statement_data = 1)
		             AND upper(trim(COALESCE(fb.status, fb.reservation_status, ''))) NOT IN
		                 ('CANCELLED', 'CANCELLED_BY_GUEST', 'CANCELLED_BY_PARTNER')
		       )
		FROM occupancies o JOIN properties p ON p.id = o.property_id ORDER BY o.property_id, o.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pms21LegacyRow
	for rows.Next() {
		var row pms21LegacyRow
		var hasFinanceEvidence int
		if err := rows.Scan(&row.ID, &row.PropertyID, &row.PropertyTimezone, &row.SourceType, &row.SourceEventUID,
			&row.StartAt, &row.EndAt, &row.Status, &row.RawSummary, &row.GuestDisplayName, &row.ContentHash,
			&row.ImportedAt, &row.LastSyncedAt, &row.LastSyncRunID, &row.ClosureState,
			&row.ExternalRevenueCents, &row.ExternalCurrency, &row.ExternalChannel, &row.StayOutcome,
			&row.CleaningExcluded, &row.UpstreamSourceType, &row.UpstreamEventUID, &row.RepresentationKind, &row.SourceDtstamp,
			&hasFinanceEvidence); err != nil {
			return nil, err
		}
		row.HasFinanceEvidence = hasFinanceEvidence != 0
		out = append(out, row)
	}
	return out, rows.Err()
}

func loadPMS21FinanceRows(ctx context.Context, q queryer) ([]pms21FinanceRow, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT id, property_id, reference_number, source_channel, check_in_date, check_out_date,
		       guest_name, status, reservation_status, has_payout_data, has_statement_data
		FROM finance_bookings
		WHERE occupancy_id IS NULL AND named_stay_id IS NULL
		ORDER BY property_id, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pms21FinanceRow
	for rows.Next() {
		var row pms21FinanceRow
		var hasPayout, hasStatement int
		if err := rows.Scan(&row.ID, &row.PropertyID, &row.ReferenceNumber, &row.SourceChannel,
			&row.CheckInDate, &row.CheckOutDate, &row.GuestName, &row.Status, &row.ReservationStatus,
			&hasPayout, &hasStatement); err != nil {
			return nil, err
		}
		row.HasPayoutData = hasPayout != 0
		row.HasStatementData = hasStatement != 0
		out = append(out, row)
	}
	return out, rows.Err()
}

type queryer interface {
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
}

func ensurePMS21Schema(ctx context.Context, db *sql.DB) error {
	required := []string{"raw_booking_blocks", "raw_booking_block_nights", "named_stays", "named_stay_nights", "stay_source_links", "property_availability_blocks", "occupancy_stay_migration_map"}
	for _, table := range required {
		var found string
		if err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&found); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: missing table %s", ErrPMS21SchemaMissing, table)
			}
			return err
		}
	}
	return nil
}

func (s *Store) populatePMS21ExistingCounts(ctx context.Context, r *PMS21MigrationReport) error {
	items := []struct {
		query string
		dest  *int
	}{
		{`SELECT COUNT(*) FROM raw_booking_blocks`, &r.ExistingNewModelRows.RawBookingBlocks}, {`SELECT COUNT(*) FROM raw_booking_block_nights`, &r.ExistingNewModelRows.RawBookingBlockNights},
		{`SELECT COUNT(*) FROM named_stays`, &r.ExistingNewModelRows.NamedStays}, {`SELECT COUNT(*) FROM named_stay_nights`, &r.ExistingNewModelRows.NamedStayNights},
		{`SELECT COUNT(*) FROM stay_source_links`, &r.ExistingNewModelRows.StaySourceLinks}, {`SELECT COUNT(*) FROM property_availability_blocks`, &r.ExistingNewModelRows.PropertyAvailabilityBlocks},
		{`SELECT COUNT(*) FROM occupancy_stay_migration_map`, &r.ExistingNewModelRows.MigrationMapRows},
	}
	for _, item := range items {
		if err := s.DB.QueryRowContext(ctx, item.query).Scan(item.dest); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) populatePMS21Diagnostics(ctx context.Context, r *PMS21MigrationReport, sampleLimit int) error {
	var err error
	if r.Conflicts.RawBlockOverlapPairs, err = countInt(ctx, s.DB, overlapPairsSQL(rawPredicate("a"), rawPredicate("b"))); err != nil {
		return err
	}
	if r.Conflicts.ExternalSaleRows, err = countInt(ctx, s.DB, `SELECT COUNT(*) FROM occupancies WHERE closure_state = 'external_sale'`); err != nil {
		return err
	}
	if r.Conflicts.ActiveClosedRows, err = countInt(ctx, s.DB, `SELECT COUNT(*) FROM occupancies WHERE closure_state = 'closed' AND status IN ('active','updated')`); err != nil {
		return err
	}
	if r.Unmapped.FinanceBookingsUnmatched, err = countInt(ctx, s.DB, `SELECT COUNT(*) FROM finance_bookings WHERE occupancy_id IS NULL AND named_stay_id IS NULL`); err != nil {
		return err
	}
	if r.Unmapped.NukiCodesWithoutNamedLikeOccupancy, err = countInt(ctx, s.DB, `SELECT COUNT(*) FROM nuki_access_codes nac WHERE nac.named_stay_id IS NULL AND NOT EXISTS (SELECT 1 FROM occupancies o WHERE o.id = nac.occupancy_id AND `+namedPredicate("o")+`)`); err != nil {
		return err
	}
	if r.Unmapped.CleaningEventsWithoutMappingCandidate, err = countInt(ctx, s.DB, `SELECT COUNT(*) FROM cleaning_calendar_events c WHERE c.named_stay_id IS NULL AND c.raw_booking_block_id IS NULL AND c.upstream_event_uid IS NULL AND (c.occupancy_id IS NULL OR NOT EXISTS (SELECT 1 FROM occupancies o WHERE o.id = c.occupancy_id AND (`+namedPredicate("o")+` OR `+rawPredicate("o")+`)))`); err != nil {
		return err
	}
	if r.Unmapped.InvoicesWithoutNamedLikeOccupancy, err = countInt(ctx, s.DB, `SELECT COUNT(*) FROM invoices i WHERE i.named_stay_id IS NULL AND (i.occupancy_id IS NULL OR NOT EXISTS (SELECT 1 FROM occupancies o WHERE o.id = i.occupancy_id AND `+namedPredicate("o")+`))`); err != nil {
		return err
	}
	if r.ReviewRequired.FinanceSyntheticNamedStays, err = countInt(ctx, s.DB, `SELECT COUNT(*) FROM occupancies WHERE source_type IN ('booking_payout', 'booking_statement')`); err != nil {
		return err
	}
	if r.ReviewRequired.LegacyGeneratedNightRows, err = countInt(ctx, s.DB, `SELECT COUNT(*) FROM occupancies WHERE COALESCE(representation_kind, '') = 'legacy_generated_night'`); err != nil {
		return err
	}
	if r.WouldBackfillLinks.NukiAccessCodes, err = countInt(ctx, s.DB, pendingMapBackfillSQL("nuki_access_codes", "nac", "named_stay_id", "nac.occupancy_id")); err != nil {
		return err
	}
	if r.WouldBackfillLinks.NukiGuestDailyEntries, err = countInt(ctx, s.DB, pendingMapBackfillSQL("nuki_guest_daily_entries", "nge", "named_stay_id", "nge.occupancy_id")); err != nil {
		return err
	}
	if r.WouldBackfillLinks.CleaningEventsNamed, err = countInt(ctx, s.DB, pendingMapBackfillSQL("cleaning_calendar_events", "c", "named_stay_id", "c.occupancy_id")); err != nil {
		return err
	}
	if r.WouldBackfillLinks.CleaningEventsRaw, err = countInt(ctx, s.DB, pendingRawMapBackfillSQL()); err != nil {
		return err
	}
	if r.WouldBackfillLinks.FinanceBookings, err = countInt(ctx, s.DB, pendingMapBackfillSQL("finance_bookings", "fb", "named_stay_id", "fb.occupancy_id")); err != nil {
		return err
	}
	if r.WouldBackfillLinks.NamedStaysConfirmedByFinance, err = countInt(ctx, s.DB, `
		SELECT COUNT(*)
		FROM named_stays ns
		WHERE ns.review_status = 'needs_review'
		  AND ns.review_reason = 'legacy_non_reservation_stay'
		  AND EXISTS (
		      SELECT 1
		      FROM finance_bookings fb
		      WHERE fb.property_id = ns.property_id
		        AND lower(trim(COALESCE(fb.source_channel, ''))) = 'booking_com'
		        AND (fb.has_payout_data = 1 OR fb.has_statement_data = 1)
		        AND upper(trim(COALESCE(fb.status, fb.reservation_status, ''))) NOT IN
		            ('CANCELLED', 'CANCELLED_BY_GUEST', 'CANCELLED_BY_PARTNER')
		        AND (
		            fb.named_stay_id = ns.id
		            OR (
		                fb.named_stay_id IS NULL
		                AND fb.occupancy_id IS NOT NULL
		                AND EXISTS (
		                    SELECT 1
		                    FROM occupancy_stay_migration_map osm
		                    WHERE osm.property_id = fb.property_id
		                      AND osm.old_occupancy_id = fb.occupancy_id
		                      AND osm.named_stay_id = ns.id
		                )
		            )
		        )
		  )`); err != nil {
		return err
	}
	if r.WouldBackfillLinks.Invoices, err = countInt(ctx, s.DB, pendingMapBackfillSQL("invoices", "i", "named_stay_id", "i.occupancy_id")); err != nil {
		return err
	}
	if r.Samples["named_stay_overlaps"], err = samplePairs(ctx, s.DB, overlapSamplesSQL(namedPredicate("a"), namedPredicate("b")), sampleLimit, "named_stay_overlap"); err != nil {
		return err
	}
	if r.Samples["raw_block_overlaps"], err = samplePairs(ctx, s.DB, overlapSamplesSQL(rawPredicate("a"), rawPredicate("b")), sampleLimit, "raw_block_overlap"); err != nil {
		return err
	}
	if r.Samples["unmatched_finance_bookings"], err = sampleRows(ctx, s.DB, `SELECT property_id, id, COALESCE(check_in_date, ''), COALESCE(check_out_date, ''), COALESCE(status, COALESCE(reservation_status, '')) FROM finance_bookings WHERE occupancy_id IS NULL AND named_stay_id IS NULL ORDER BY property_id, check_in_date, id LIMIT ?`, sampleLimit, "finance_unmatched"); err != nil {
		return err
	}
	return nil
}

func (s *Store) populatePMS21CandidateConflicts(ctx context.Context, r *PMS21MigrationReport, named, availability []pms21CandidateRange) error {
	for i := 0; i < len(named); i++ {
		for j := i + 1; j < len(named); j++ {
			if named[i].PropertyID == named[j].PropertyID && pms21RangesOverlap(named[i], named[j]) {
				r.Conflicts.NamedStayOverlapPairs++
			}
		}
	}

	rows, err := s.DB.QueryContext(ctx, `
		SELECT property_id, named_stay_id, local_night_date
		FROM named_stay_nights
		WHERE active = 1`)
	if err != nil {
		return err
	}
	existingPairs := map[string]struct{}{}
	for rows.Next() {
		var propertyID, stayID int64
		var night string
		if err := rows.Scan(&propertyID, &stayID, &night); err != nil {
			rows.Close()
			return err
		}
		for _, candidate := range named {
			if candidate.PropertyID == propertyID && night >= candidate.Start && night < candidate.End {
				key := fmt.Sprintf("%s:%d:%d", candidate.Kind, candidate.ID, stayID)
				existingPairs[key] = struct{}{}
			}
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	r.Conflicts.ExistingNamedStayNightCollisions = len(existingPairs)

	rows, err = s.DB.QueryContext(ctx, `
		SELECT property_id, id, start_date, end_date
		FROM property_availability_blocks
		WHERE status = 'active'`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var block pms21CandidateRange
		block.Kind = "availability"
		if err := rows.Scan(&block.PropertyID, &block.ID, &block.Start, &block.End); err != nil {
			rows.Close()
			return err
		}
		availability = append(availability, block)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, candidate := range named {
		for _, block := range availability {
			if candidate.PropertyID == block.PropertyID && pms21RangesOverlap(candidate, block) {
				r.Conflicts.NamedStayAvailabilityOverlaps++
			}
		}
	}
	return nil
}

func pms21RangesOverlap(a, b pms21CandidateRange) bool {
	return a.Start < b.End && b.Start < a.End
}

func pendingMapBackfillSQL(table, alias, column, occupancyExpr string) string {
	return fmt.Sprintf(`SELECT COUNT(*) FROM %s %s JOIN occupancy_stay_migration_map m ON m.old_occupancy_id = %s AND m.property_id = %s.property_id WHERE %s.%s IS NULL AND m.named_stay_id IS NOT NULL`, table, alias, occupancyExpr, alias, alias, column)
}
func pendingRawMapBackfillSQL() string {
	return `SELECT COUNT(*) FROM cleaning_calendar_events c JOIN occupancy_stay_migration_map m ON m.old_occupancy_id = c.occupancy_id AND m.property_id = c.property_id WHERE c.raw_booking_block_id IS NULL AND m.raw_booking_block_id IS NOT NULL`
}

func rawPredicate(alias string) string {
	return fmt.Sprintf(`(%[1]s.source_type = 'booking_ics' AND (COALESCE(%[1]s.representation_kind, '') = 'unnamed_block' OR (COALESCE(%[1]s.representation_kind, '') = 'legacy_generated_night' AND COALESCE(TRIM(%[1]s.guest_display_name), '') = '') OR COALESCE(TRIM(%[1]s.guest_display_name), '') = ''))`, alias)
}
func namedPredicate(alias string) string {
	return fmt.Sprintf(`(COALESCE(%[1]s.representation_kind, '') IN ('named_stay','synthetic_finance','external_sale') OR COALESCE(TRIM(%[1]s.guest_display_name), '') <> '' OR %[1]s.source_type IN ('booking_payout','booking_statement') OR COALESCE(%[1]s.closure_state, '') = 'external_sale')`, alias)
}
func overlapPairsSQL(leftPredicate, rightPredicate string) string {
	return `SELECT COUNT(*) FROM occupancies a JOIN occupancies b ON a.property_id = b.property_id AND a.id < b.id WHERE a.status IN ('active','updated') AND b.status IN ('active','updated') AND ` + leftPredicate + ` AND ` + rightPredicate + ` AND a.start_at < b.end_at AND b.start_at < a.end_at`
}
func overlapSamplesSQL(leftPredicate, rightPredicate string) string {
	return `SELECT a.property_id, a.id, b.id, a.start_at, a.end_at, b.start_at, b.end_at FROM occupancies a JOIN occupancies b ON a.property_id = b.property_id AND a.id < b.id WHERE a.status IN ('active','updated') AND b.status IN ('active','updated') AND ` + leftPredicate + ` AND ` + rightPredicate + ` AND a.start_at < b.end_at AND b.start_at < a.end_at ORDER BY a.property_id, a.start_at, a.id LIMIT ?`
}

func insertPMS21Map(ctx context.Context, tx *sql.Tx, row pms21LegacyRow, kind string, rawID, stayID, availabilityID int64, now string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO occupancy_stay_migration_map (old_occupancy_id, property_id, raw_booking_block_id, named_stay_id, availability_block_id, migration_kind, notes, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, row.ID, row.PropertyID, nullableInt64(rawID), nullableInt64(stayID), nullableInt64(availabilityID), kind, "pms21_stage2_apply", now)
	return err
}
func incrementPMS21Skipped(class pms21Classification, counts *PMS21ApplyCounts) {
	switch class.Kind {
	case pms21KindRaw:
		counts.Skipped.RawBookingBlocks++
	case pms21KindNamed:
		counts.Skipped.NamedStays++
		if class.ReviewStatus == "confirmed" {
			counts.Skipped.AutoConfirmedNamedStays++
		} else {
			counts.Skipped.ReviewRequiredNamedStays++
		}
	case pms21KindAvailability:
		counts.Skipped.PropertyAvailabilityBlocks++
	default:
		counts.Skipped.MigrationMapRows++
	}
}
func legacyStatusActive(status string) bool { return status == "active" || status == "updated" }
func legacyPropertyDates(row pms21LegacyRow) (string, string, error) {
	startAt, err := time.Parse(time.RFC3339, row.StartAt)
	if err != nil {
		return "", "", err
	}
	endAt, err := time.Parse(time.RFC3339, row.EndAt)
	if err != nil {
		return "", "", err
	}
	ci, co := startAt.UTC().Format("2006-01-02"), endAt.UTC().Format("2006-01-02")
	representation := strings.TrimSpace(row.RepresentationKind.String)
	isCivilSentinel := row.SourceType == manualSplitSourceType ||
		(row.SourceType == UpstreamSourceBookingICS && representation != RepresentationLegacyGeneratedNight)
	if !isCivilSentinel {
		loc, err := time.LoadLocation(row.PropertyTimezone)
		if err != nil {
			return "", "", err
		}
		ci, co = startAt.In(loc).Format("2006-01-02"), endAt.In(loc).Format("2006-01-02")
	}
	if co <= ci {
		return "", "", ErrNamedStayInvalidRange
	}
	return ci, co, nil
}
func dateNights(start, end string) ([]string, error) {
	ci, co, err := parseNamedStayRange(start, end)
	if err != nil {
		return nil, err
	}
	return nightsUTC(ci, co), nil
}
func mustDateNights(start, end string) []string { nights, _ := dateNights(start, end); return nights }
func nullStringArg(v sql.NullString) interface{} {
	if v.Valid {
		return v.String
	}
	return nil
}
func nullInt64Arg(v sql.NullInt64) interface{} {
	if v.Valid {
		return v.Int64
	}
	return nil
}
func deletedAtArg(status, fallback string) interface{} {
	if status == "deleted_from_source" {
		return fallback
	}
	return nil
}
func conflictReasonArg(status string) interface{} {
	if status == "source_deleted" {
		return "raw_source_missing"
	}
	return nil
}

func countInt(ctx context.Context, db *sql.DB, query string, args ...interface{}) (int, error) {
	var n int
	if err := db.QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
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
