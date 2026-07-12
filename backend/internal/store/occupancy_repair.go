package store

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// PMS_19 §11 repair path for databases that already contain duplicate active
// rows. The plan is deterministic (§7.6) and never hard-deletes rows.

type RepairNightResolution struct {
	LocalNight  string  `json:"local_night"`
	WinnerOccID int64   `json:"winner_occupancy_id"`
	WinnerUID   string  `json:"winner_upstream_uid"`
	WinnerKind  string  `json:"winner_kind"`
	LoserOccIDs []int64 `json:"loser_occupancy_ids"`
	Reason      string  `json:"reason"`
}

// RepairRowAction records a whole-row disposition (supersede / deleted_from_source)
// plus the downstream artifacts that will be relinked or revoked, so the
// operator can review the dry-run before applying (§11).
type RepairRowAction struct {
	OccupancyID    int64  `json:"occupancy_id"`
	UpstreamUID    string `json:"upstream_uid"`
	Action         string `json:"action"`
	Reason         string `json:"reason"`
	GuestName      string `json:"guest_name,omitempty"`
	RevokeNuki     bool   `json:"revoke_nuki"`
	RemoveCleaning bool   `json:"remove_cleaning"`
}

type OccupancyRepairReport struct {
	PropertyID            int64                   `json:"property_id"`
	DryRun                bool                    `json:"dry_run"`
	NightsResolved        int                     `json:"nights_resolved"`
	DuplicatesResolved    int                     `json:"duplicates_resolved"`
	RowsDeletedFromSource int                     `json:"rows_deleted_from_source"`
	Resolutions           []RepairNightResolution `json:"resolutions"`
	RowActions            []RepairRowAction       `json:"row_actions"`
}

// OccupancyRepairPlan produces a dry-run report of how duplicate active nights
// would be resolved, mutating nothing.
func (s *Store) OccupancyRepairPlan(ctx context.Context, propertyID int64) (*OccupancyRepairReport, error) {
	return s.occupancyRepair(ctx, propertyID, true)
}

// OccupancyRepairApply applies the same deterministic resolution and returns
// the report of what changed.
func (s *Store) OccupancyRepairApply(ctx context.Context, propertyID int64) (*OccupancyRepairReport, error) {
	return s.occupancyRepair(ctx, propertyID, false)
}

func (s *Store) occupancyRepair(ctx context.Context, propertyID int64, dryRun bool) (*OccupancyRepairReport, error) {
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	report := &OccupancyRepairReport{PropertyID: propertyID, DryRun: dryRun}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	loc := s.propertyLocationTx(ctx, tx, propertyID)

	// §11: mark today/future rows deleted_from_source when their upstream UID is
	// absent from the latest successful raw snapshot, or present but no longer
	// covers the row's nights. Skipped entirely when no successful snapshot
	// exists yet so a fresh DB is never wiped.
	snapshot, hasSnapshot, err := s.latestSuccessfulSnapshotTx(ctx, tx, propertyID)
	if err != nil {
		return nil, err
	}
	if hasSnapshot {
		if err := s.repairSnapshotDisappearance(ctx, tx, propertyID, snapshot, now, nowStr, loc, report, dryRun); err != nil {
			return nil, err
		}
	}

	rows, err := s.listActivePropertyRowsTx(ctx, tx, propertyID)
	if err != nil {
		return nil, err
	}
	winnerByNight, losersByNight, rowNights := assignNightWinners(rows)

	// Build the deterministic resolution report (§7.6).
	for night, winner := range winnerByNight {
		losers := losersByNight[night]
		if len(losers) == 0 {
			continue
		}
		res := RepairNightResolution{LocalNight: night, WinnerOccID: winner.ID, WinnerKind: winner.RepresentationKind.String, Reason: repairReason(winner)}
		if winner.UpstreamEventUID.Valid {
			res.WinnerUID = winner.UpstreamEventUID.String
		}
		res.LoserOccIDs = append(res.LoserOccIDs, losers...)
		report.Resolutions = append(report.Resolutions, res)
		report.NightsResolved++
		report.DuplicatesResolved += len(losers)
	}

	if dryRun {
		return report, nil
	}

	// Apply: rebuild coverage and supersede rows that lost every night.
	for i := range rows {
		if err := clearOccupancyNightsTx(ctx, tx, rows[i].ID); err != nil {
			return nil, err
		}
	}
	for i := range rows {
		r := &rows[i]
		nights := rowNights[r.ID]
		if len(nights) == 0 {
			if len(nightsUTC(r.StartAt, r.EndAt)) > 0 {
				if _, err := tx.ExecContext(ctx, `
					UPDATE occupancies SET superseded_at = COALESCE(superseded_at, ?), superseded_reason = COALESCE(superseded_reason, ?), last_synced_at = ?
					WHERE id = ?`, nowStr, SupersededDuplicateResolved, nowStr, r.ID); err != nil {
					return nil, err
				}
			}
			continue
		}
		if err := setOccupancyNightsTx(ctx, tx, propertyID, r.ID, nights, nullOrString(r.UpstreamSourceType), nullOrString(r.UpstreamEventUID), true); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return report, nil
}

// repairSnapshotDisappearance marks current/future active rows deleted_from_source
// when the latest successful snapshot no longer justifies them (§11).
func (s *Store) repairSnapshotDisappearance(ctx context.Context, tx *sql.Tx, propertyID int64, snapshot map[string]map[string]bool, now time.Time, nowStr string, loc *time.Location, report *OccupancyRepairReport, dryRun bool) error {
	rows, err := s.listActivePropertyRowsTx(ctx, tx, propertyID)
	if err != nil {
		return err
	}
	for i := range rows {
		r := &rows[i]
		if !r.UpstreamEventUID.Valid || strings.TrimSpace(r.UpstreamEventUID.String) == "" {
			continue // non-ICS row: not governed by the Booking snapshot.
		}
		if !checkoutTodayOrFutureInLocation(r, now, loc) {
			continue // historical rows are retained untouched.
		}
		uid := r.UpstreamEventUID.String
		covered, present := snapshot[uid]
		reason := ""
		switch {
		case !present:
			reason = SupersededSourceDeleted
		default:
			// UID present: delete only if none of the row's nights are still
			// covered by the latest source range.
			stillCovered := false
			for _, n := range nightsUTC(r.StartAt, r.EndAt) {
				if covered[n] {
					stillCovered = true
					break
				}
			}
			if !stillCovered {
				reason = SupersededSourceDeleted
			}
		}
		if reason == "" {
			continue
		}
		action := RepairRowAction{
			OccupancyID:    r.ID,
			UpstreamUID:    uid,
			Action:         StatusDeletedFromSource,
			Reason:         reason,
			RemoveCleaning: checkoutTodayOrFutureInLocation(r, now, loc),
		}
		if r.GuestDisplayName.Valid {
			action.GuestName = r.GuestDisplayName.String
			action.RevokeNuki = true // named/future rows may have a generated code to revoke.
		}
		report.RowActions = append(report.RowActions, action)
		report.RowsDeletedFromSource++
		if dryRun {
			continue
		}
		if err := s.markRowDeletedFromSourceTx(ctx, tx, r, nowStr, reason); err != nil {
			return err
		}
	}
	return nil
}

// latestSuccessfulSnapshotTx returns the covered-night set per upstream UID from
// the most recent successful sync run's raw snapshot. hasSnapshot is false when
// no successful run has been recorded for the property.
func (s *Store) latestSuccessfulSnapshotTx(ctx context.Context, tx *sql.Tx, propertyID int64) (map[string]map[string]bool, bool, error) {
	var runID int64
	err := tx.QueryRowContext(ctx, `
		SELECT id FROM occupancy_sync_runs
		WHERE property_id = ? AND status = 'success'
		ORDER BY id DESC LIMIT 1`, propertyID).Scan(&runID)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT source_event_uid, event_start, event_end FROM occupancy_raw_events
		WHERE property_id = ? AND sync_run_id = ?`, propertyID, runID)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := make(map[string]map[string]bool)
	for rows.Next() {
		var uid, startStr, endStr string
		if err := rows.Scan(&uid, &startStr, &endStr); err != nil {
			return nil, false, err
		}
		start, err1 := time.Parse(time.RFC3339, startStr)
		end, err2 := time.Parse(time.RFC3339, endStr)
		if err1 != nil || err2 != nil {
			continue
		}
		set := out[uid]
		if set == nil {
			set = make(map[string]bool)
			out[uid] = set
		}
		for _, n := range nightsUTC(start, end) {
			set[n] = true
		}
	}
	return out, true, rows.Err()
}

func repairReason(winner *Occupancy) string {
	switch {
	case winner.RepresentationKind.String == RepresentationNamedStay:
		return "kept_named_stay"
	case winner.ClosureState.Valid:
		return "kept_operator_label"
	case winner.RepresentationKind.String == RepresentationUnnamedBlock:
		return "kept_unnamed_block"
	default:
		return "capacity_one_duplicate"
	}
}

func repairBeats(a, b *Occupancy) bool {
	pa, pb := representationPriority(a), representationPriority(b)
	if pa != pb {
		return pa < pb
	}
	// §7.6 tie-break: newest source DTSTAMP wins when available.
	if c := compareDtstamp(a, b); c != 0 {
		return c > 0
	}
	if !a.LastSyncedAt.Equal(b.LastSyncedAt) {
		return a.LastSyncedAt.After(b.LastSyncedAt)
	}
	return a.ID < b.ID
}

// compareDtstamp returns +1 if a is newer, -1 if b is newer, 0 if incomparable.
func compareDtstamp(a, b *Occupancy) int {
	if !a.SourceDtstamp.Valid || !b.SourceDtstamp.Valid {
		return 0
	}
	ta, ea := time.Parse(time.RFC3339, a.SourceDtstamp.String)
	tb, eb := time.Parse(time.RFC3339, b.SourceDtstamp.String)
	if ea != nil || eb != nil || ta.Equal(tb) {
		return 0
	}
	if ta.After(tb) {
		return 1
	}
	return -1
}
