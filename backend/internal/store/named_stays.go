package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	StayTypeBookingCom  = "booking_com"
	StayTypeExternal    = "external"
	StayTypeMaintenance = "maintenance"
	StayTypePersonalUse = "personal_use"

	NamedStayStatusActive    = "active"
	NamedStayStatusCancelled = "cancelled"
	NamedStayStatusArchived  = "archived"

	NukiGenerationNotApplicable = "not_applicable"
	NukiGenerationPending       = "pending"
	NukiGenerationGenerated     = "generated"
	NukiGenerationError         = "error"
)

var ErrNamedStayInvalidType = errors.New("invalid stay type")

type NamedStay struct {
	ID                      int64          `json:"id"`
	PropertyID              int64          `json:"property_id"`
	DisplayName             string         `json:"display_name"`
	StayType                string         `json:"stay_type"`
	CheckInDate             string         `json:"check_in_date"`
	CheckOutDate            string         `json:"check_out_date"`
	Status                  string         `json:"status"`
	CleaningRequired        bool           `json:"cleaning_required"`
	CleaningOverrideReason  sql.NullString `json:"-"`
	SourceChannel           sql.NullString `json:"-"`
	SourceReference         sql.NullString `json:"-"`
	ManualRevenueCents      sql.NullInt64  `json:"-"`
	ManualRevenueCurrency   sql.NullString `json:"-"`
	ManualRevenueNote       sql.NullString `json:"-"`
	ReviewStatus            sql.NullString `json:"-"`
	ReviewReason            sql.NullString `json:"-"`
	NukiGenerationStatus    sql.NullString `json:"-"`
	NukiGenerationError     sql.NullString `json:"-"`
	NukiGenerationUpdatedAt sql.NullTime   `json:"-"`
	LegacyOccupancyID       sql.NullInt64  `json:"legacy_occupancy_id,omitempty"`
	CreatedAt               time.Time      `json:"created_at"`
	UpdatedAt               time.Time      `json:"updated_at"`
}

type NamedStayCreateInput struct {
	PropertyID             int64
	DisplayName            string
	StayType               string
	CheckInDate            string
	CheckOutDate           string
	CleaningRequired       *bool
	CleaningOverrideReason string
	SourceChannel          string
	SourceReference        string
	ReviewStatus           string
	ReviewReason           string
	CreatedByUserID        int64
	RawBookingBlockID      int64
	RequireWithinRawBlock  bool
}

type NamedStayUpdateInput struct {
	DisplayName            *string
	StayType               *string
	CheckInDate            *string
	CheckOutDate           *string
	CleaningRequired       *bool
	CleaningOverrideReason *string
	ManualRevenueCents     *int64
	ManualRevenueCurrency  *string
	ManualRevenueNote      *string
	UpdatedByUserID        int64
}

type NamedStayFinanceCandidate struct {
	ID                 int64
	DisplayName        string
	StayType           string
	CheckInDate        string
	CheckOutDate       string
	Status             string
	ReviewStatus       sql.NullString
	ManualRevenueCents sql.NullInt64
	HasFinanceData     bool
}

func (s *Store) ListNamedStayFinanceCandidates(ctx context.Context, propertyID int64, month string, limit, offset int) ([]NamedStayFinanceCandidate, error) {
	query := `
		SELECT ns.id, ns.display_name, ns.stay_type, ns.check_in_date, ns.check_out_date, ns.status,
		       ns.review_status, ns.manual_revenue_cents,
		       CASE WHEN EXISTS (SELECT 1 FROM finance_bookings fb WHERE fb.property_id = ns.property_id AND fb.named_stay_id = ns.id) THEN 1 ELSE 0 END
		FROM named_stays ns
		WHERE ns.property_id = ? AND ns.status = 'active' AND ns.stay_type IN ('booking_com', 'external')`
	args := []interface{}{propertyID}
	if strings.TrimSpace(month) != "" {
		query += ` AND substr(ns.check_in_date, 1, 7) = ?`
		args = append(args, strings.TrimSpace(month))
	}
	query += ` ORDER BY ns.check_in_date DESC, ns.id DESC`
	if limit > 0 {
		if offset < 0 {
			offset = 0
		}
		query += ` LIMIT ? OFFSET ?`
		args = append(args, limit, offset)
	}
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]NamedStayFinanceCandidate, 0)
	for rows.Next() {
		var row NamedStayFinanceCandidate
		var has int
		if err := rows.Scan(&row.ID, &row.DisplayName, &row.StayType, &row.CheckInDate, &row.CheckOutDate, &row.Status, &row.ReviewStatus, &row.ManualRevenueCents, &has); err != nil {
			return nil, err
		}
		row.HasFinanceData = has != 0
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) PromoteRawBookingBlockToNamedStay(ctx context.Context, propertyID, rawBlockID int64, in NamedStayCreateInput) (*NamedStay, error) {
	in.PropertyID = propertyID
	in.RawBookingBlockID = rawBlockID
	in.RequireWithinRawBlock = true
	if strings.TrimSpace(in.StayType) == "" {
		in.StayType = StayTypeBookingCom
	}
	if strings.TrimSpace(in.SourceChannel) == "" {
		in.SourceChannel = UpstreamSourceBookingICS
	}
	return s.CreateNamedStayRecord(ctx, in)
}

func (s *Store) CreateNamedStayRecord(ctx context.Context, in NamedStayCreateInput) (*NamedStay, error) {
	displayName := strings.TrimSpace(in.DisplayName)
	if displayName == "" {
		return nil, ErrNamedStayInvalidRange
	}
	stayType := strings.TrimSpace(in.StayType)
	if err := validateStayType(stayType); err != nil {
		return nil, err
	}
	ci, co, err := parseNamedStayRange(in.CheckInDate, in.CheckOutDate)
	if err != nil {
		return nil, err
	}
	cleaningRequired := defaultCleaningRequired(stayType)
	if in.CleaningRequired != nil {
		cleaningRequired = *in.CleaningRequired
	}
	reviewStatus := strings.TrimSpace(in.ReviewStatus)
	if reviewStatus == "" {
		reviewStatus = "confirmed"
	}
	nukiStatus := NukiGenerationNotApplicable
	if namedStayNukiEligible(stayType, reviewStatus) {
		nukiStatus = NukiGenerationPending
	}

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var raw *rawBookingBlockForStay
	sourceReference := strings.TrimSpace(in.SourceReference)
	if in.RawBookingBlockID > 0 || in.RequireWithinRawBlock {
		raw, err = getActiveRawBookingBlockTx(ctx, tx, in.PropertyID, in.RawBookingBlockID)
		if err != nil {
			return nil, err
		}
		if ci.Format("2006-01-02") < raw.checkInDate || co.Format("2006-01-02") > raw.checkOutDate {
			return nil, ErrNamedStayOutsideBlock
		}
		sourceReference = raw.sourceEventUID
	}
	if err := namedStayRangeAvailableTx(ctx, tx, in.PropertyID, 0, ci, co); err != nil {
		return nil, err
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO named_stays (
			property_id, display_name, stay_type, check_in_date, check_out_date, status,
			cleaning_required, cleaning_override_reason, source_channel, source_reference,
			review_status, review_reason, nuki_generation_status, created_by_user_id, updated_by_user_id,
			created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, 'active', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.PropertyID, displayName, stayType, ci.Format("2006-01-02"), co.Format("2006-01-02"), boolInt(cleaningRequired),
		nullableString(strings.TrimSpace(in.CleaningOverrideReason)), nullableString(strings.TrimSpace(in.SourceChannel)), nullableString(sourceReference),
		reviewStatus, nullableString(strings.TrimSpace(in.ReviewReason)), nukiStatus,
		nullableInt64(in.CreatedByUserID), nullableInt64(in.CreatedByUserID), nowStr, nowStr)
	if err != nil {
		return nil, err
	}
	stayID, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	if err := replaceNamedStayNightsTx(ctx, tx, in.PropertyID, stayID, nightsUTC(ci, co), true, nowStr); err != nil {
		return nil, err
	}
	if raw != nil {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO stay_source_links (
				property_id, named_stay_id, raw_booking_block_id, source_type, source_event_uid,
				linked_check_in_date, linked_check_out_date, link_status, created_at, updated_at
			)
			VALUES (?, ?, ?, ?, ?, ?, ?, 'active', ?, ?)`,
			in.PropertyID, stayID, raw.id, raw.sourceType, raw.sourceEventUID, ci.Format("2006-01-02"), co.Format("2006-01-02"), nowStr, nowStr); err != nil {
			return nil, err
		}
	}
	if !s.OccupancyLegacyWriteDisabled {
		legacyOccID, err := s.upsertLegacyOccupancyForNamedStayTx(ctx, tx, legacyNamedStayRow{
			ID:               stayID,
			PropertyID:       in.PropertyID,
			DisplayName:      displayName,
			StayType:         stayType,
			CheckInDate:      ci.Format("2006-01-02"),
			CheckOutDate:     co.Format("2006-01-02"),
			Status:           NamedStayStatusActive,
			CleaningRequired: cleaningRequired,
			Raw:              raw,
		}, now)
		if err != nil {
			return nil, err
		}
		if err := upsertOccupancyStayMigrationMapTx(ctx, tx, in.PropertyID, legacyOccID, stayID, nowStr); err != nil {
			return nil, err
		}
		if err := s.reconcileLegacyRawCoverageForNamedStayTx(ctx, tx, in.PropertyID, raw, now); err != nil {
			return nil, err
		}
		if raw != nil && raw.legacyOccupancyID.Valid && in.RequireWithinRawBlock && ci.Format("2006-01-02") == raw.checkInDate && co.Format("2006-01-02") == raw.checkOutDate {
			if _, err := s.MoveFinanceMappingTx(ctx, tx, in.PropertyID, raw.legacyOccupancyID.Int64, legacyOccID); err != nil {
				return nil, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetNamedStay(ctx, in.PropertyID, stayID)
}

func (s *Store) GetNamedStay(ctx context.Context, propertyID, stayID int64) (*NamedStay, error) {
	rows, err := s.DB.QueryContext(ctx, namedStaySelectSQL+` WHERE ns.property_id = ? AND ns.id = ?`, propertyID, stayID)
	if err != nil {
		return nil, err
	}
	stays, err := scanNamedStays(rows)
	if err != nil {
		return nil, err
	}
	if len(stays) == 0 {
		return nil, sql.ErrNoRows
	}
	return &stays[0], nil
}

func (s *Store) UpdateNamedStayRecord(ctx context.Context, propertyID, stayID int64, in NamedStayUpdateInput) (*NamedStay, error) {
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	current, err := getNamedStayTx(ctx, tx, propertyID, stayID)
	if err != nil {
		return nil, err
	}
	linkedRaw, err := getActiveLinkedRawForStayTx(ctx, tx, propertyID, stayID)
	if err != nil {
		return nil, err
	}
	displayName := current.DisplayName
	if in.DisplayName != nil {
		displayName = strings.TrimSpace(*in.DisplayName)
		if displayName == "" {
			return nil, ErrNamedStayInvalidRange
		}
	}
	stayType := current.StayType
	if in.StayType != nil {
		stayType = strings.TrimSpace(*in.StayType)
		if err := validateStayType(stayType); err != nil {
			return nil, err
		}
	}
	ciText := current.CheckInDate
	coText := current.CheckOutDate
	if in.CheckInDate != nil {
		ciText = strings.TrimSpace(*in.CheckInDate)
	}
	if in.CheckOutDate != nil {
		coText = strings.TrimSpace(*in.CheckOutDate)
	}
	ci, co, err := parseNamedStayRange(ciText, coText)
	if err != nil {
		return nil, err
	}
	if err := ensureNamedStayWithinActiveLinksTx(ctx, tx, propertyID, stayID, ci, co); err != nil {
		return nil, err
	}
	if current.Status == NamedStayStatusActive {
		if err := namedStayRangeAvailableTx(ctx, tx, propertyID, stayID, ci, co); err != nil {
			return nil, err
		}
	}
	cleaningRequired := current.CleaningRequired
	if in.CleaningRequired != nil {
		cleaningRequired = *in.CleaningRequired
	}
	var cleaningReason interface{}
	if in.CleaningOverrideReason != nil {
		cleaningReason = nullableString(strings.TrimSpace(*in.CleaningOverrideReason))
	} else {
		cleaningReason = nullNullableString(current.CleaningOverrideReason)
	}
	if in.ManualRevenueCents != nil && *in.ManualRevenueCents < 0 {
		return nil, ErrNamedStayInvalidRange
	}
	if in.ManualRevenueCents != nil {
		currency := current.ManualRevenueCurrency.String
		if in.ManualRevenueCurrency != nil {
			currency = strings.TrimSpace(*in.ManualRevenueCurrency)
		}
		if strings.TrimSpace(currency) == "" {
			return nil, ErrNamedStayInvalidRange
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE named_stays
		SET display_name = ?, stay_type = ?, check_in_date = ?, check_out_date = ?, cleaning_required = ?,
			cleaning_override_reason = ?, manual_revenue_cents = ?, manual_revenue_currency = ?, manual_revenue_note = ?,
			updated_by_user_id = ?, updated_at = ?
		WHERE property_id = ? AND id = ?`,
		displayName, stayType, ci.Format("2006-01-02"), co.Format("2006-01-02"), boolInt(cleaningRequired), cleaningReason,
		manualRevenueCentsArg(current.ManualRevenueCents, in.ManualRevenueCents), manualRevenueStringArg(current.ManualRevenueCurrency, in.ManualRevenueCurrency), manualRevenueStringArg(current.ManualRevenueNote, in.ManualRevenueNote),
		nullableInt64(in.UpdatedByUserID), nowStr, propertyID, stayID); err != nil {
		return nil, err
	}
	if linkedRaw != nil {
		if _, err := tx.ExecContext(ctx, `
			UPDATE stay_source_links
			SET linked_check_in_date = ?, linked_check_out_date = ?, updated_at = ?
			WHERE property_id = ? AND named_stay_id = ? AND link_status = 'active'`,
			ci.Format("2006-01-02"), co.Format("2006-01-02"), nowStr, propertyID, stayID); err != nil {
			return nil, err
		}
	}
	if current.Status == NamedStayStatusActive {
		if err := replaceNamedStayNightsTx(ctx, tx, propertyID, stayID, nightsUTC(ci, co), true, nowStr); err != nil {
			return nil, err
		}
	}
	if !s.OccupancyLegacyWriteDisabled {
		legacyOccID, err := s.upsertLegacyOccupancyForNamedStayTx(ctx, tx, legacyNamedStayRow{
			ID:               stayID,
			PropertyID:       propertyID,
			DisplayName:      displayName,
			StayType:         stayType,
			CheckInDate:      ci.Format("2006-01-02"),
			CheckOutDate:     co.Format("2006-01-02"),
			Status:           current.Status,
			CleaningRequired: cleaningRequired,
			Raw:              linkedRaw,
		}, now)
		if err != nil {
			return nil, err
		}
		if err := upsertOccupancyStayMigrationMapTx(ctx, tx, propertyID, legacyOccID, stayID, nowStr); err != nil {
			return nil, err
		}
		if err := s.reconcileLegacyRawCoverageForNamedStayTx(ctx, tx, propertyID, linkedRaw, now); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetNamedStay(ctx, propertyID, stayID)
}

func (s *Store) UpdateNamedStayStatus(ctx context.Context, propertyID, stayID int64, status string, userID int64) (*NamedStay, error) {
	status = strings.TrimSpace(status)
	if status != NamedStayStatusActive && status != NamedStayStatusCancelled && status != NamedStayStatusArchived {
		return nil, ErrNamedStayInvalidRange
	}
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	current, err := getNamedStayTx(ctx, tx, propertyID, stayID)
	if err != nil {
		return nil, err
	}
	linkedRaw, err := getActiveLinkedRawForStayTx(ctx, tx, propertyID, stayID)
	if err != nil {
		return nil, err
	}
	ci, co, err := parseNamedStayRange(current.CheckInDate, current.CheckOutDate)
	if err != nil {
		return nil, err
	}
	if status == NamedStayStatusActive {
		if err := namedStayRangeAvailableTx(ctx, tx, propertyID, stayID, ci, co); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE named_stays SET status = ?, updated_by_user_id = ?, updated_at = ? WHERE property_id = ? AND id = ?`, status, nullableInt64(userID), nowStr, propertyID, stayID); err != nil {
		return nil, err
	}
	active := status == NamedStayStatusActive
	if err := replaceNamedStayNightsTx(ctx, tx, propertyID, stayID, nightsUTC(ci, co), active, nowStr); err != nil {
		return nil, err
	}
	if !s.OccupancyLegacyWriteDisabled {
		legacyOccID, err := s.upsertLegacyOccupancyForNamedStayTx(ctx, tx, legacyNamedStayRow{
			ID:               stayID,
			PropertyID:       propertyID,
			DisplayName:      current.DisplayName,
			StayType:         current.StayType,
			CheckInDate:      current.CheckInDate,
			CheckOutDate:     current.CheckOutDate,
			Status:           status,
			CleaningRequired: current.CleaningRequired,
			Raw:              linkedRaw,
		}, now)
		if err != nil {
			return nil, err
		}
		if !active {
			if err := deactivateOccupancyNightsTx(ctx, tx, legacyOccID); err != nil {
				return nil, err
			}
		}
		if err := upsertOccupancyStayMigrationMapTx(ctx, tx, propertyID, legacyOccID, stayID, nowStr); err != nil {
			return nil, err
		}
		if err := s.reconcileLegacyRawCoverageForNamedStayTx(ctx, tx, propertyID, linkedRaw, now); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetNamedStay(ctx, propertyID, stayID)
}

func (s *Store) MarkNamedStayNukiGeneration(ctx context.Context, propertyID, stayID int64, status string, errText string) error {
	status = strings.TrimSpace(status)
	if status != NukiGenerationNotApplicable && status != NukiGenerationPending && status != NukiGenerationGenerated && status != NukiGenerationError {
		return ErrNamedStayInvalidRange
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.ExecContext(ctx, `
		UPDATE named_stays
		SET nuki_generation_status = ?, nuki_generation_error = ?, nuki_generation_updated_at = ?, updated_at = ?
		WHERE property_id = ? AND id = ?`, status, nullableString(strings.TrimSpace(errText)), now, now, propertyID, stayID)
	return err
}

func (s *Store) LinkNukiCodeToNamedStayByOccupancy(ctx context.Context, propertyID, occupancyID, stayID int64) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE nuki_access_codes SET named_stay_id = ? WHERE property_id = ? AND occupancy_id = ?`, stayID, propertyID, occupancyID)
	return err
}

func (s *Store) ResolveNamedStayIDForOccupancy(ctx context.Context, propertyID, occupancyID int64) (int64, error) {
	var stayID int64
	err := s.DB.QueryRowContext(ctx, `
		SELECT named_stay_id
		FROM occupancy_stay_migration_map
		WHERE property_id = ? AND old_occupancy_id = ? AND named_stay_id IS NOT NULL
		LIMIT 1`, propertyID, occupancyID).Scan(&stayID)
	if err != nil {
		return 0, err
	}
	return stayID, nil
}

func NamedStayNukiEligible(stayType, reviewStatus string) bool {
	return namedStayNukiEligible(stayType, reviewStatus)
}

const namedStaySelectSQL = `
	SELECT ns.id, ns.property_id, ns.display_name, ns.stay_type, ns.check_in_date, ns.check_out_date, ns.status,
	       ns.cleaning_required, ns.cleaning_override_reason, ns.source_channel, ns.source_reference,
	       ns.manual_revenue_cents, ns.manual_revenue_currency, ns.manual_revenue_note,
	       ns.review_status, ns.review_reason, ns.nuki_generation_status, ns.nuki_generation_error, ns.nuki_generation_updated_at,
	       ns.created_at, ns.updated_at,
	       osm.old_occupancy_id
	FROM named_stays ns
	LEFT JOIN occupancy_stay_migration_map osm ON osm.named_stay_id = ns.id AND osm.migration_kind = 'named_stay'`

func scanNamedStays(rows *sql.Rows) ([]NamedStay, error) {
	defer rows.Close()
	var out []NamedStay
	for rows.Next() {
		var n NamedStay
		var cleaning int
		var created, updated string
		var nukiUpdated sql.NullString
		if err := rows.Scan(&n.ID, &n.PropertyID, &n.DisplayName, &n.StayType, &n.CheckInDate, &n.CheckOutDate, &n.Status,
			&cleaning, &n.CleaningOverrideReason, &n.SourceChannel, &n.SourceReference,
			&n.ManualRevenueCents, &n.ManualRevenueCurrency, &n.ManualRevenueNote,
			&n.ReviewStatus, &n.ReviewReason, &n.NukiGenerationStatus, &n.NukiGenerationError, &nukiUpdated,
			&created, &updated, &n.LegacyOccupancyID); err != nil {
			return nil, err
		}
		n.CleaningRequired = cleaning == 1
		n.CreatedAt, _ = time.Parse(time.RFC3339, created)
		n.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		if nukiUpdated.Valid && nukiUpdated.String != "" {
			if parsed, err := time.Parse(time.RFC3339, nukiUpdated.String); err == nil {
				n.NukiGenerationUpdatedAt = sql.NullTime{Time: parsed, Valid: true}
			}
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func getNamedStayTx(ctx context.Context, tx *sql.Tx, propertyID, stayID int64) (*NamedStay, error) {
	rows, err := tx.QueryContext(ctx, namedStaySelectSQL+` WHERE ns.property_id = ? AND ns.id = ?`, propertyID, stayID)
	if err != nil {
		return nil, err
	}
	stays, err := scanNamedStays(rows)
	if err != nil {
		return nil, err
	}
	if len(stays) == 0 {
		return nil, sql.ErrNoRows
	}
	return &stays[0], nil
}

type rawBookingBlockForStay struct {
	id                int64
	sourceType        string
	sourceEventUID    string
	checkInDate       string
	checkOutDate      string
	rawSummary        sql.NullString
	legacyOccupancyID sql.NullInt64
}

func getActiveRawBookingBlockTx(ctx context.Context, tx *sql.Tx, propertyID, rawBlockID int64) (*rawBookingBlockForStay, error) {
	var r rawBookingBlockForStay
	err := tx.QueryRowContext(ctx, `
		SELECT rb.id, rb.source_type, rb.source_event_uid, rb.check_in_date, rb.check_out_date, rb.raw_summary, o.id
		FROM raw_booking_blocks rb
		LEFT JOIN occupancies o ON o.property_id = rb.property_id AND o.source_event_uid = rb.source_event_uid
		WHERE rb.property_id = ? AND rb.id = ? AND rb.status = 'active'`, propertyID, rawBlockID).
		Scan(&r.id, &r.sourceType, &r.sourceEventUID, &r.checkInDate, &r.checkOutDate, &r.rawSummary, &r.legacyOccupancyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUpstreamBlockNotFound
		}
		return nil, err
	}
	return &r, nil
}

func getActiveLinkedRawForStayTx(ctx context.Context, tx *sql.Tx, propertyID, stayID int64) (*rawBookingBlockForStay, error) {
	var r rawBookingBlockForStay
	err := tx.QueryRowContext(ctx, `
		SELECT rb.id, rb.source_type, rb.source_event_uid, rb.check_in_date, rb.check_out_date, rb.raw_summary, o.id
		FROM stay_source_links l
		JOIN raw_booking_blocks rb ON rb.id = l.raw_booking_block_id
		LEFT JOIN occupancies o ON o.property_id = rb.property_id AND o.source_event_uid = rb.source_event_uid
		WHERE l.property_id = ? AND l.named_stay_id = ? AND l.link_status = 'active' AND rb.status = 'active'
		ORDER BY l.id ASC LIMIT 1`, propertyID, stayID).
		Scan(&r.id, &r.sourceType, &r.sourceEventUID, &r.checkInDate, &r.checkOutDate, &r.rawSummary, &r.legacyOccupancyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

func parseNamedStayRange(checkIn, checkOut string) (time.Time, time.Time, error) {
	ci, err := parseOccupancySplitDate(strings.TrimSpace(checkIn))
	if err != nil {
		return time.Time{}, time.Time{}, ErrNamedStayInvalidRange
	}
	co, err := parseOccupancySplitDate(strings.TrimSpace(checkOut))
	if err != nil {
		return time.Time{}, time.Time{}, ErrNamedStayInvalidRange
	}
	if !co.After(ci) {
		return time.Time{}, time.Time{}, ErrNamedStayInvalidRange
	}
	return ci, co, nil
}

func validateStayType(stayType string) error {
	switch stayType {
	case StayTypeBookingCom, StayTypeExternal, StayTypeMaintenance, StayTypePersonalUse:
		return nil
	default:
		return ErrNamedStayInvalidType
	}
}

func defaultCleaningRequired(stayType string) bool {
	return stayType == StayTypeBookingCom || stayType == StayTypeExternal
}

func namedStayNukiEligible(stayType, reviewStatus string) bool {
	return reviewStatus == "confirmed" && (stayType == StayTypeBookingCom || stayType == StayTypeExternal)
}

func namedStayRangeAvailableTx(ctx context.Context, tx *sql.Tx, propertyID, stayID int64, ci, co time.Time) error {
	for _, night := range nightsUTC(ci, co) {
		var cnt int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM named_stay_nights
			WHERE property_id = ? AND local_night_date = ? AND active = 1 AND named_stay_id <> ?`, propertyID, night, stayID).Scan(&cnt); err != nil {
			return err
		}
		if cnt > 0 {
			return ErrNamedStayOverlap
		}
	}
	return nil
}

func replaceNamedStayNightsTx(ctx context.Context, tx *sql.Tx, propertyID, stayID int64, activeNights []string, active bool, nowStr string) error {
	if _, err := tx.ExecContext(ctx, `UPDATE named_stay_nights SET active = 0 WHERE property_id = ? AND named_stay_id = ?`, propertyID, stayID); err != nil {
		return err
	}
	if !active {
		return nil
	}
	for _, night := range activeNights {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO named_stay_nights (property_id, named_stay_id, local_night_date, active, created_at)
			VALUES (?, ?, ?, 1, ?)
			ON CONFLICT(property_id, named_stay_id, local_night_date) DO UPDATE SET active = 1`, propertyID, stayID, night, nowStr); err != nil {
			if strings.Contains(err.Error(), "uq_named_stay_nights_active_property_date") || strings.Contains(err.Error(), "UNIQUE constraint failed") {
				return ErrNamedStayOverlap
			}
			return err
		}
	}
	return nil
}

func ensureNamedStayWithinActiveLinksTx(ctx context.Context, tx *sql.Tx, propertyID, stayID int64, ci, co time.Time) error {
	var cnt int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM stay_source_links WHERE property_id = ? AND named_stay_id = ? AND link_status = 'active' AND raw_booking_block_id IS NOT NULL`, propertyID, stayID).Scan(&cnt); err != nil {
		return err
	}
	if cnt == 0 {
		return nil
	}
	var covering int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM stay_source_links l
		JOIN raw_booking_blocks rb ON rb.id = l.raw_booking_block_id
		WHERE l.property_id = ? AND l.named_stay_id = ? AND l.link_status = 'active' AND rb.status = 'active'
		  AND rb.check_in_date <= ? AND rb.check_out_date >= ?`, propertyID, stayID, ci.Format("2006-01-02"), co.Format("2006-01-02")).Scan(&covering); err != nil {
		return err
	}
	if covering == 0 {
		return ErrNamedStayOutsideBlock
	}
	return nil
}

type legacyNamedStayRow struct {
	ID               int64
	PropertyID       int64
	DisplayName      string
	StayType         string
	CheckInDate      string
	CheckOutDate     string
	Status           string
	CleaningRequired bool
	Raw              *rawBookingBlockForStay
}

func (s *Store) upsertLegacyOccupancyForNamedStayTx(ctx context.Context, tx *sql.Tx, row legacyNamedStayRow, now time.Time) (int64, error) {
	ci, co, err := parseNamedStayRange(row.CheckInDate, row.CheckOutDate)
	if err != nil {
		return 0, err
	}
	uid := fmt.Sprintf("named_stay:%d", row.ID)
	contentHash := fmt.Sprintf("named-stay:%d:%s:%s:%s:%s:%t", row.ID, row.DisplayName, row.StayType, row.CheckInDate, row.CheckOutDate, row.CleaningRequired)
	status := "active"
	if row.Status == NamedStayStatusCancelled {
		status = "cancelled"
	} else if row.Status == NamedStayStatusArchived {
		status = StatusDeletedFromSource
	}
	representationKind := RepresentationNamedStay
	var closureState interface{}
	if row.StayType == StayTypeMaintenance || row.StayType == StayTypePersonalUse {
		closureState = ClosureStateClosed
		representationKind = RepresentationManualClosure
	}
	var upstreamSource, upstreamUID interface{}
	var rawSummary interface{}
	if row.Raw != nil {
		upstreamSource = row.Raw.sourceType
		upstreamUID = row.Raw.sourceEventUID
		if row.Raw.rawSummary.Valid {
			rawSummary = row.Raw.rawSummary.String
		}
	}
	nights := nightsUTC(ci, co)
	nowStr := now.UTC().Format(time.RFC3339)
	res, err := tx.ExecContext(ctx, `
		INSERT INTO occupancies (
			property_id, source_type, source_event_uid, start_at, end_at, status,
			raw_summary, guest_display_name, content_hash, imported_at, last_synced_at,
			upstream_source_type, upstream_event_uid, representation_kind, representation_date,
			closure_state, cleaning_calendar_excluded
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(property_id, source_event_uid) DO UPDATE SET
			start_at = excluded.start_at,
			end_at = excluded.end_at,
			status = excluded.status,
			raw_summary = excluded.raw_summary,
			guest_display_name = excluded.guest_display_name,
			content_hash = excluded.content_hash,
			last_synced_at = excluded.last_synced_at,
			upstream_source_type = excluded.upstream_source_type,
			upstream_event_uid = excluded.upstream_event_uid,
			representation_kind = excluded.representation_kind,
			representation_date = excluded.representation_date,
			closure_state = excluded.closure_state,
			cleaning_calendar_excluded = excluded.cleaning_calendar_excluded,
			superseded_at = NULL,
			superseded_reason = NULL`,
		row.PropertyID, manualSplitSourceType, uid, ci.Format(time.RFC3339), co.Format(time.RFC3339), status,
		rawSummary, row.DisplayName, contentHash, nowStr, nowStr,
		upstreamSource, upstreamUID, representationKind, nullableRepresentationDate(nights), closureState, boolInt(!row.CleaningRequired))
	if err != nil {
		return 0, err
	}
	occID, _ := res.LastInsertId()
	if occID == 0 {
		if err := tx.QueryRowContext(ctx, `SELECT id FROM occupancies WHERE property_id = ? AND source_event_uid = ?`, row.PropertyID, uid).Scan(&occID); err != nil {
			return 0, err
		}
	}
	return occID, nil
}

func (s *Store) reconcileLegacyRawCoverageForNamedStayTx(ctx context.Context, tx *sql.Tx, propertyID int64, raw *rawBookingBlockForStay, now time.Time) error {
	if raw == nil || strings.TrimSpace(raw.sourceEventUID) == "" || strings.TrimSpace(raw.checkInDate) == "" || strings.TrimSpace(raw.checkOutDate) == "" {
		return nil
	}
	ci, co, err := parseNamedStayRange(raw.checkInDate, raw.checkOutDate)
	if err != nil {
		return err
	}
	loc := s.propertyLocationTx(ctx, tx, propertyID)
	return s.reconcileUpstreamCoverageTx(ctx, tx, propertyID, raw.sourceEventUID, nightsUTC(ci, co), now, loc, nil)
}

func upsertOccupancyStayMigrationMapTx(ctx context.Context, tx *sql.Tx, propertyID, legacyOccID, stayID int64, nowStr string) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO occupancy_stay_migration_map (old_occupancy_id, property_id, named_stay_id, migration_kind, notes, created_at)
		VALUES (?, ?, ?, 'named_stay', 'stage4_legacy_compat', ?)
		ON CONFLICT(old_occupancy_id) DO UPDATE SET
			property_id = excluded.property_id,
			named_stay_id = excluded.named_stay_id,
			migration_kind = excluded.migration_kind,
			notes = excluded.notes`, legacyOccID, propertyID, stayID, nowStr)
	return err
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func nullableInt64(v int64) interface{} {
	if v > 0 {
		return v
	}
	return nil
}

func manualRevenueCentsArg(current sql.NullInt64, next *int64) interface{} {
	if next == nil {
		return nullInt64Value(current)
	}
	if *next < 0 {
		return -1
	}
	return *next
}

func manualRevenueStringArg(current sql.NullString, next *string) interface{} {
	if next == nil {
		return nullStringValue(current)
	}
	return nullableString(strings.TrimSpace(*next))
}
