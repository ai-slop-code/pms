package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// RecomputeSourceLinkHealth recalculates warning state without changing any
// named-stay business field. Coverage is the union of active linked raw nights.
func (s *Store) RecomputeSourceLinkHealth(ctx context.Context, propertyID int64) (int, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	conflicts, err := recomputeSourceLinkHealthTx(ctx, tx, propertyID, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return conflicts, nil
}

func recomputeSourceLinkHealthTx(ctx context.Context, tx *sql.Tx, propertyID int64, now string) (int, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT DISTINCT ns.id, ns.check_in_date, ns.check_out_date
		FROM named_stays ns
		JOIN stay_source_links l ON l.named_stay_id = ns.id AND l.property_id = ns.property_id
		WHERE ns.property_id = ? AND ns.status = 'active' AND l.link_status <> 'manual_unlinked'
		ORDER BY ns.id`, propertyID)
	if err != nil {
		return 0, err
	}
	type linkedStay struct {
		id       int64
		checkIn  string
		checkOut string
	}
	var stays []linkedStay
	for rows.Next() {
		var stay linkedStay
		if err := rows.Scan(&stay.id, &stay.checkIn, &stay.checkOut); err != nil {
			rows.Close()
			return 0, err
		}
		stays = append(stays, stay)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	conflicts := 0
	for _, stay := range stays {
		expected, err := dateNights(stay.checkIn, stay.checkOut)
		if err != nil {
			return conflicts, err
		}
		var activeBlocks, coveredNights, previousHealthy int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(DISTINCT rb.id),
			       COUNT(DISTINCT CASE WHEN rbn.active = 1 AND rbn.local_night_date >= ? AND rbn.local_night_date < ? THEN rbn.local_night_date END),
			       MAX(CASE WHEN l.link_status = 'active' THEN 1 ELSE 0 END)
			FROM stay_source_links l
			LEFT JOIN raw_booking_blocks rb ON rb.id = l.raw_booking_block_id AND rb.property_id = l.property_id AND rb.status = 'active'
			LEFT JOIN raw_booking_block_nights rbn ON rbn.raw_booking_block_id = rb.id AND rbn.property_id = l.property_id
			WHERE l.property_id = ? AND l.named_stay_id = ? AND l.link_status <> 'manual_unlinked'`,
			stay.checkIn, stay.checkOut, propertyID, stay.id).Scan(&activeBlocks, &coveredNights, &previousHealthy); err != nil {
			return conflicts, err
		}
		status := "active"
		var reason interface{}
		if activeBlocks == 0 || coveredNights == 0 {
			status = "source_deleted"
			reason = "raw_source_missing"
		} else if coveredNights != len(expected) {
			status = "conflict"
			reason = "raw_coverage_gap"
		}
		if status != "active" && previousHealthy == 1 {
			conflicts++
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE stay_source_links
			SET link_status = ?, conflict_reason = ?, updated_at = ?
			WHERE property_id = ? AND named_stay_id = ? AND link_status <> 'manual_unlinked'`,
			status, reason, now, propertyID, stay.id); err != nil {
			return conflicts, err
		}
	}
	return conflicts, nil
}

func linkedRawNightUnionCoversTx(ctx context.Context, tx *sql.Tx, propertyID, stayID int64, checkIn, checkOut string) (bool, bool, error) {
	var links int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM stay_source_links WHERE property_id = ? AND named_stay_id = ? AND link_status <> 'manual_unlinked'`, propertyID, stayID).Scan(&links); err != nil {
		return false, false, err
	}
	if links == 0 {
		return false, true, nil
	}
	expected, err := dateNights(checkIn, checkOut)
	if err != nil {
		return true, false, err
	}
	var covered int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT rbn.local_night_date)
		FROM stay_source_links l
		JOIN raw_booking_blocks rb ON rb.id = l.raw_booking_block_id AND rb.property_id = l.property_id AND rb.status = 'active'
		JOIN raw_booking_block_nights rbn ON rbn.raw_booking_block_id = rb.id AND rbn.property_id = l.property_id AND rbn.active = 1
		WHERE l.property_id = ? AND l.named_stay_id = ? AND l.link_status <> 'manual_unlinked'
		  AND rbn.local_night_date >= ? AND rbn.local_night_date < ?`, propertyID, stayID, checkIn, checkOut).Scan(&covered); err != nil {
		return true, false, fmt.Errorf("linked raw coverage: %w", err)
	}
	return true, covered == len(expected), nil
}
