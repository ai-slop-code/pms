package store

import (
	"context"
	"database/sql"
	"time"
)

// NukiGuestDailyEntry is the persisted "first guest unlock per (occupancy,
// calendar day)" row that powers the Analytics → Performance guest
// check-in heatmap (PMS_12 task 3). Mirrors CleaningDailyLog but is keyed
// by (property_id, occupancy_id, day_date) so multiple stays on the same
// calendar day each contribute their own first-entry timestamp.
type NukiGuestDailyEntry struct {
	ID                 int64
	PropertyID         int64
	OccupancyID        int64
	DayDate            string
	FirstEntryAt       time.Time
	NukiEventReference sql.NullString
	CreatedAt          time.Time
}

// UpsertNukiGuestDailyEntry inserts or refreshes the first-entry row for
// (property, occupancy, day). The reconciler is the only writer — it always
// passes the earliest unlock found in the latest fetch window, so the
// ON CONFLICT clause unconditionally replaces the timestamp and reference.
func (s *Store) UpsertNukiGuestDailyEntry(ctx context.Context, row *NukiGuestDailyEntry) error {
	now := time.Now().UTC().Format(time.RFC3339)
	first := row.FirstEntryAt.UTC().Format(time.RFC3339)
	var ref interface{}
	if row.NukiEventReference.Valid {
		ref = row.NukiEventReference.String
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO nuki_guest_daily_entries (property_id, occupancy_id, day_date, first_entry_at, nuki_event_reference, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(property_id, occupancy_id, day_date) DO UPDATE SET
			first_entry_at = excluded.first_entry_at,
			nuki_event_reference = excluded.nuki_event_reference`,
		row.PropertyID, row.OccupancyID, row.DayDate, first, ref, now)
	return err
}

// ListNukiGuestDailyEntriesInRange returns every (occupancy, day) first
// entry whose day_date is in [fromDate, toDate]. Both bounds are inclusive
// and use YYYY-MM-DD strings expressed in the property timezone (the same
// convention used everywhere else for day-keyed analytics tables).
//
// Closure-state filtering is done in SQL so the analytics layer never has
// to know about closure_state directly: closed nights are excluded;
// external_sale rows remain (PMS_14 §4 — externally-sold nights count as
// sold). Cancelled stays are excluded as well to mirror the rest of the
// analytics queries.
func (s *Store) ListNukiGuestDailyEntriesInRange(ctx context.Context, propertyID int64, fromDate, toDate string) ([]NukiGuestDailyEntry, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT g.id, g.property_id, g.occupancy_id, g.day_date, g.first_entry_at, g.nuki_event_reference, g.created_at
		FROM nuki_guest_daily_entries g
		JOIN occupancies o ON o.id = g.occupancy_id
		WHERE g.property_id = ?
		  AND g.day_date >= ? AND g.day_date <= ?
		  AND o.status NOT IN ('cancelled', 'deleted_from_source')
		  AND (o.closure_state IS NULL OR o.closure_state <> 'closed')
		  AND (o.stay_outcome IS NULL OR o.stay_outcome NOT IN ('cancelled_non_refundable', 'no_show'))
		ORDER BY g.first_entry_at ASC, g.id ASC`,
		propertyID, fromDate, toDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]NukiGuestDailyEntry, 0)
	for rows.Next() {
		var r NukiGuestDailyEntry
		var first, created string
		if err := rows.Scan(&r.ID, &r.PropertyID, &r.OccupancyID, &r.DayDate, &first, &r.NukiEventReference, &created); err != nil {
			return nil, err
		}
		r.FirstEntryAt, _ = time.Parse(time.RFC3339, first)
		r.CreatedAt, _ = time.Parse(time.RFC3339, created)
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListGeneratedNukiAccessCodesByExternalID returns generated codes (status
// = 'generated' or 'revoked' — i.e. the keypad code did exist on the lock
// at some point) keyed by their Nuki authID. The reconciler uses the map
// to resolve a Smartlock event's authID back to the owning occupancy.
//
// Revoked codes are intentionally included: we still want to credit unlocks
// that happened while the code was live to the original stay, even if the
// operator has since revoked the PIN.
func (s *Store) ListGeneratedNukiAccessCodesByExternalID(ctx context.Context, propertyID int64) (map[string]int64, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT external_nuki_id, occupancy_id
		FROM nuki_access_codes
		WHERE property_id = ? AND external_nuki_id IS NOT NULL AND external_nuki_id <> ''`,
		propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var ext string
		var occ int64
		if err := rows.Scan(&ext, &occ); err != nil {
			return nil, err
		}
		out[ext] = occ
	}
	return out, rows.Err()
}
