package store

import (
	"context"
	"database/sql"
	"errors"
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
	ReviewActorUserID       sql.NullInt64  `json:"-"`
	ReviewedAt              sql.NullTime   `json:"-"`
	StayOutcome             sql.NullString `json:"-"`
	StayOutcomeReason       sql.NullString `json:"-"`
	StayOutcomeActorUserID  sql.NullInt64  `json:"-"`
	StayOutcomeMarkedAt     sql.NullTime   `json:"-"`
	NukiGenerationStatus    sql.NullString `json:"-"`
	NukiGenerationError     sql.NullString `json:"-"`
	NukiGenerationUpdatedAt sql.NullTime   `json:"-"`
	FirstKnownAt            time.Time      `json:"first_known_at"`
	CancellationEffectiveAt sql.NullTime   `json:"-"`
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
		       COALESCE(ns.review_resolution, ns.review_status), ns.manual_revenue_cents,
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
	if reviewStatus != "confirmed" && reviewStatus != "needs_review" {
		return nil, ErrNamedStayInvalidRange
	}
	var reviewResolution interface{}
	if reviewStatus == "confirmed" {
		reviewResolution = "confirmed"
	}
	nukiStatus := NukiGenerationNotApplicable

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
	firstKnownAt := nowStr
	if raw != nil {
		if importedAt := parseNamedStayTime(raw.importedAt); !importedAt.IsZero() && importedAt.Before(now) {
			firstKnownAt = raw.importedAt
		}
	}

	res, err := tx.ExecContext(ctx, `
		INSERT INTO named_stays (
			property_id, display_name, stay_type, check_in_date, check_out_date, status,
			cleaning_required, cleaning_override_reason, source_channel, source_reference,
			review_status, review_resolution, review_reason, nuki_generation_status, created_by_user_id, updated_by_user_id,
			first_known_at, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, 'active', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.PropertyID, displayName, stayType, ci.Format("2006-01-02"), co.Format("2006-01-02"), boolInt(cleaningRequired),
		nullableString(strings.TrimSpace(in.CleaningOverrideReason)), nullableString(strings.TrimSpace(in.SourceChannel)), nullableString(sourceReference),
		reviewStatus, reviewResolution, nullableString(strings.TrimSpace(in.ReviewReason)), nukiStatus,
		nullableInt64(in.CreatedByUserID), nullableInt64(in.CreatedByUserID), firstKnownAt, nowStr, nowStr)
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
	if ci.Format("2006-01-02") != current.CheckInDate || co.Format("2006-01-02") != current.CheckOutDate {
		if err := ensureNamedStayWithinActiveLinksTx(ctx, tx, propertyID, stayID, ci, co); err != nil {
			return nil, err
		}
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
	if _, err := tx.ExecContext(ctx, `
		UPDATE stay_source_links
		SET linked_check_in_date = ?, linked_check_out_date = ?, updated_at = ?
		WHERE property_id = ? AND named_stay_id = ? AND link_status <> 'manual_unlinked'`,
		ci.Format("2006-01-02"), co.Format("2006-01-02"), nowStr, propertyID, stayID); err != nil {
		return nil, err
	}
	if current.Status == NamedStayStatusActive {
		if err := replaceNamedStayNightsTx(ctx, tx, propertyID, stayID, nightsUTC(ci, co), true, nowStr); err != nil {
			return nil, err
		}
	}
	if _, err := recomputeSourceLinkHealthTx(ctx, tx, propertyID, nowStr); err != nil {
		return nil, err
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
	ci, co, err := parseNamedStayRange(current.CheckInDate, current.CheckOutDate)
	if err != nil {
		return nil, err
	}
	if status == NamedStayStatusActive {
		if err := namedStayRangeAvailableTx(ctx, tx, propertyID, stayID, ci, co); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE named_stays
		SET status = ?,
			cancellation_effective_at = CASE
				WHEN ? = 'cancelled' THEN COALESCE(cancellation_effective_at, ?)
				WHEN ? = 'active' AND status <> 'active' THEN NULL
				ELSE cancellation_effective_at
			END,
			updated_by_user_id = ?, updated_at = ?
		WHERE property_id = ? AND id = ?`, status, status, nowStr, status, nullableInt64(userID), nowStr, propertyID, stayID); err != nil {
		return nil, err
	}
	active := status == NamedStayStatusActive
	if err := replaceNamedStayNightsTx(ctx, tx, propertyID, stayID, nightsUTC(ci, co), active, nowStr); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetNamedStay(ctx, propertyID, stayID)
}

func (s *Store) UpdateNamedStayOutcome(ctx context.Context, propertyID, stayID, userID int64, outcome *string, reason string) (*NamedStay, error) {
	var value interface{}
	if outcome != nil {
		v := strings.TrimSpace(*outcome)
		if v != StayOutcomeCancelledNonRefundable && v != StayOutcomeNoShow {
			return nil, ErrNamedStayInvalidRange
		}
		value = v
	}
	now := time.Now().UTC().Format(time.RFC3339)
	reason = strings.TrimSpace(reason)
	var reasonValue, actorValue, markedAt interface{}
	if value != nil {
		reasonValue = nullableString(reason)
		actorValue = nullableInt64(userID)
		markedAt = now
	}
	res, err := s.DB.ExecContext(ctx, `
		UPDATE named_stays
		SET stay_outcome = ?, stay_outcome_reason = ?, stay_outcome_actor_user_id = ?,
			stay_outcome_marked_at = ?,
			updated_by_user_id = ?, updated_at = ?
		WHERE property_id = ? AND id = ?`, value, reasonValue, actorValue, markedAt,
		nullableInt64(userID), now, propertyID, stayID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, sql.ErrNoRows
	}
	return s.GetNamedStay(ctx, propertyID, stayID)
}

func (s *Store) UpdateNamedStayReview(ctx context.Context, propertyID, stayID, userID int64, reviewStatus, reason string) (*NamedStay, error) {
	reviewStatus = strings.TrimSpace(reviewStatus)
	if reviewStatus != "confirmed" && reviewStatus != "rejected" {
		return nil, ErrNamedStayInvalidRange
	}
	storedStatus := "confirmed"
	if reviewStatus == "rejected" {
		storedStatus = "needs_review"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.DB.ExecContext(ctx, `
		UPDATE named_stays
		SET review_status = ?, review_resolution = ?, review_reason = ?, review_actor_user_id = ?, reviewed_at = ?,
			updated_by_user_id = ?, updated_at = ?
		WHERE property_id = ? AND id = ?`, storedStatus, reviewStatus, nullableString(strings.TrimSpace(reason)),
		nullableInt64(userID), now, nullableInt64(userID), now, propertyID, stayID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, sql.ErrNoRows
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

func NamedStayNukiEligible(stayType, reviewStatus string) bool {
	return namedStayNukiEligible(stayType, reviewStatus)
}

const namedStaySelectSQL = `
	SELECT ns.id, ns.property_id, ns.display_name, ns.stay_type, ns.check_in_date, ns.check_out_date, ns.status,
	       ns.cleaning_required, ns.cleaning_override_reason, ns.source_channel, ns.source_reference,
	       ns.manual_revenue_cents, ns.manual_revenue_currency, ns.manual_revenue_note,
	       COALESCE(ns.review_resolution, ns.review_status),
	       ns.review_reason, ns.review_actor_user_id, ns.reviewed_at,
	       ns.stay_outcome, ns.stay_outcome_reason, ns.stay_outcome_actor_user_id, ns.stay_outcome_marked_at,
	       ns.nuki_generation_status, ns.nuki_generation_error, ns.nuki_generation_updated_at,
	       COALESCE(ns.first_known_at, ns.created_at), ns.cancellation_effective_at, ns.created_at, ns.updated_at
	FROM named_stays ns`

func scanNamedStays(rows *sql.Rows) ([]NamedStay, error) {
	defer rows.Close()
	var out []NamedStay
	for rows.Next() {
		var n NamedStay
		var cleaning int
		var firstKnown, created, updated string
		var reviewed, outcomeMarked, nukiUpdated, cancelled sql.NullString
		if err := rows.Scan(&n.ID, &n.PropertyID, &n.DisplayName, &n.StayType, &n.CheckInDate, &n.CheckOutDate, &n.Status,
			&cleaning, &n.CleaningOverrideReason, &n.SourceChannel, &n.SourceReference,
			&n.ManualRevenueCents, &n.ManualRevenueCurrency, &n.ManualRevenueNote,
			&n.ReviewStatus, &n.ReviewReason, &n.ReviewActorUserID, &reviewed,
			&n.StayOutcome, &n.StayOutcomeReason, &n.StayOutcomeActorUserID, &outcomeMarked,
			&n.NukiGenerationStatus, &n.NukiGenerationError, &nukiUpdated,
			&firstKnown, &cancelled, &created, &updated); err != nil {
			return nil, err
		}
		n.CleaningRequired = cleaning == 1
		n.CreatedAt, _ = time.Parse(time.RFC3339, created)
		n.UpdatedAt, _ = time.Parse(time.RFC3339, updated)
		n.FirstKnownAt = parseNamedStayTime(firstKnown)
		n.ReviewedAt = parseNamedStayNullTime(reviewed)
		n.StayOutcomeMarkedAt = parseNamedStayNullTime(outcomeMarked)
		n.CancellationEffectiveAt = parseNamedStayNullTime(cancelled)
		if nukiUpdated.Valid && nukiUpdated.String != "" {
			if parsed, err := time.Parse(time.RFC3339, nukiUpdated.String); err == nil {
				n.NukiGenerationUpdatedAt = sql.NullTime{Time: parsed, Valid: true}
			}
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func parseNamedStayNullTime(value sql.NullString) sql.NullTime {
	if !value.Valid || value.String == "" {
		return sql.NullTime{}
	}
	parsed, err := time.Parse(time.RFC3339, value.String)
	return sql.NullTime{Time: parsed, Valid: err == nil}
}

func parseNamedStayTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
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
	id             int64
	sourceType     string
	sourceEventUID string
	checkInDate    string
	checkOutDate   string
	rawSummary     sql.NullString
	importedAt     string
}

func getActiveRawBookingBlockTx(ctx context.Context, tx *sql.Tx, propertyID, rawBlockID int64) (*rawBookingBlockForStay, error) {
	var r rawBookingBlockForStay
	err := tx.QueryRowContext(ctx, `
		SELECT rb.id, rb.source_type, rb.source_event_uid, rb.check_in_date, rb.check_out_date, rb.raw_summary, rb.imported_at
		FROM raw_booking_blocks rb
		WHERE rb.property_id = ? AND rb.id = ? AND rb.status = 'active'`, propertyID, rawBlockID).
		Scan(&r.id, &r.sourceType, &r.sourceEventUID, &r.checkInDate, &r.checkOutDate, &r.rawSummary, &r.importedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUpstreamBlockNotFound
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
	startDate := ci.Format("2006-01-02")
	endDate := co.Format("2006-01-02")
	var blockCount int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM property_availability_blocks
		WHERE property_id = ? AND status = 'active' AND start_date < ? AND end_date > ?`, propertyID, endDate, startDate).Scan(&blockCount); err != nil {
		return err
	}
	if blockCount > 0 {
		return ErrNamedStayOverlap
	}
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
	hasLinks, covered, err := linkedRawNightUnionCoversTx(ctx, tx, propertyID, stayID, ci.Format("2006-01-02"), co.Format("2006-01-02"))
	if err != nil {
		return err
	}
	if hasLinks && !covered {
		return ErrNamedStayOutsideBlock
	}
	return nil
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
