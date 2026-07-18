package store

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

// PMS_19 §13 acceptance tests for the reconciliation core.

func recTestProperty(t *testing.T) (*Store, int64) {
	t.Helper()
	st := testStore(t)
	ctx := context.Background()
	u, err := st.CreateUser(ctx, "rec-owner@test.local", "hash", "owner")
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProperty(ctx, u.ID, "Rec", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	return st, p.ID
}

func dt(s string) time.Time {
	v, _ := time.Parse("2006-01-02", s)
	return time.Date(v.Year(), v.Month(), v.Day(), 0, 0, 0, 0, time.UTC)
}

func block(uid, start, end string) DesiredBlock {
	return DesiredBlock{UID: uid, Start: dt(start), End: dt(end), Summary: "CLOSED - Not available", ContentHash: uid + start + end}
}

func nightCount(t *testing.T, st *Store, pid int64, d string) int {
	t.Helper()
	n, err := st.ActiveOccupancyNightCount(context.Background(), pid, d)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// §13.1 / §13.6: a multi-night block covers each night exactly once, and a
// repeat sync is idempotent.
func TestReconcile_MultiNightAndIdempotent(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()
	now := dt("2026-07-01")
	blk := block("uid-a@booking.com", "2026-07-09", "2026-07-12")
	for i := 0; i < 2; i++ {
		if err := st.ReconcileBookingICSSync(ctx, pid, "booking_ics", []DesiredBlock{blk}, now, &SyncCounters{}); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range []string{"2026-07-09", "2026-07-10", "2026-07-11"} {
		if got := nightCount(t, st, pid, d); got != 1 {
			t.Fatalf("night %s count=%d want 1", d, got)
		}
	}
}

func TestReconcile_RawBlocksDualWriteDisabledByDefault(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()
	if err := st.ReconcileBookingICSSync(ctx, pid, "booking_ics", []DesiredBlock{block("raw-off@booking.com", "2026-07-09", "2026-07-12")}, dt("2026-07-01"), &SyncCounters{}); err != nil {
		t.Fatal(err)
	}
	var got int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM raw_booking_blocks WHERE property_id = ?`, pid).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Fatalf("raw blocks written with gate disabled: got %d want 0", got)
	}
}

func TestReconcile_RawBlocksDualWriteUpsertsAndRebuildsNights(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()
	now := dt("2026-07-01")
	counters := &SyncCounters{RawBlocksDualWrite: true}
	uid := "raw-upsert@booking.com"
	if err := st.ReconcileBookingICSSync(ctx, pid, "booking_ics", []DesiredBlock{block(uid, "2026-07-09", "2026-07-12")}, now, counters); err != nil {
		t.Fatal(err)
	}
	if counters.RawBlocksInserted != 1 || counters.RawBlocksUpdated != 0 || counters.RawBlocksUnchanged != 0 {
		t.Fatalf("initial counters inserted/updated/unchanged=%d/%d/%d want 1/0/0", counters.RawBlocksInserted, counters.RawBlocksUpdated, counters.RawBlocksUnchanged)
	}
	var blockID int64
	var activeNights int
	if err := st.DB.QueryRowContext(ctx, `SELECT id FROM raw_booking_blocks WHERE property_id = ? AND source_event_uid = ? AND status = 'active'`, pid, uid).Scan(&blockID); err != nil {
		t.Fatal(err)
	}
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM raw_booking_block_nights WHERE raw_booking_block_id = ? AND active = 1`, blockID).Scan(&activeNights); err != nil {
		t.Fatal(err)
	}
	if activeNights != 3 {
		t.Fatalf("active raw nights=%d want 3", activeNights)
	}

	shrunk := block(uid, "2026-07-09", "2026-07-11")
	counters = &SyncCounters{RawBlocksDualWrite: true}
	if err := st.ReconcileBookingICSSync(ctx, pid, "booking_ics", []DesiredBlock{shrunk}, now, counters); err != nil {
		t.Fatal(err)
	}
	if counters.RawBlocksUpdated != 1 {
		t.Fatalf("updated counter=%d want 1", counters.RawBlocksUpdated)
	}
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM raw_booking_block_nights WHERE raw_booking_block_id = ? AND active = 1`, blockID).Scan(&activeNights); err != nil {
		t.Fatal(err)
	}
	if activeNights != 2 {
		t.Fatalf("active raw nights after shrink=%d want 2", activeNights)
	}
}

func TestReconcile_RawBlocksDualWriteMarksDisappearedDeleted(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()
	now := dt("2026-07-01")
	uid := "raw-gone@booking.com"
	if err := st.ReconcileBookingICSSync(ctx, pid, "booking_ics", []DesiredBlock{block(uid, "2026-07-31", "2026-08-01")}, now, &SyncCounters{RawBlocksDualWrite: true}); err != nil {
		t.Fatal(err)
	}
	counters := &SyncCounters{RawBlocksDualWrite: true}
	if err := st.ReconcileBookingICSSync(ctx, pid, "booking_ics", nil, now, counters); err != nil {
		t.Fatal(err)
	}
	if counters.RawBlocksDeletedFromSource != 1 {
		t.Fatalf("deleted raw blocks=%d want 1", counters.RawBlocksDeletedFromSource)
	}
	var status string
	var activeNights int
	if err := st.DB.QueryRowContext(ctx, `SELECT status FROM raw_booking_blocks WHERE property_id = ? AND source_event_uid = ?`, pid, uid).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != StatusDeletedFromSource {
		t.Fatalf("raw block status=%s want %s", status, StatusDeletedFromSource)
	}
	if err := st.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM raw_booking_block_nights n
		JOIN raw_booking_blocks b ON b.id = n.raw_booking_block_id
		WHERE b.property_id = ? AND b.source_event_uid = ? AND n.active = 1`, pid, uid).Scan(&activeNights); err != nil {
		t.Fatal(err)
	}
	if activeNights != 0 {
		t.Fatalf("active raw nights after disappearance=%d want 0", activeNights)
	}
}

// §13.2 / §13.10: a named one-night stay must not duplicate the aggregate.
func TestReconcile_NamedStayDoesNotDuplicateAggregate(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()
	now := dt("2026-07-01")
	uid := "3335c09bc362baaf67844f69ece8c3f4@booking.com"
	if err := st.ReconcileBookingICSSync(ctx, pid, "booking_ics", []DesiredBlock{block(uid, "2026-07-09", "2026-07-12")}, now, &SyncCounters{}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateNamedStay(ctx, pid, uid, "2026-07-11", "2026-07-12", "Koilpitchai", 1); err != nil {
		t.Fatal(err)
	}
	// Re-sync with the same block still present.
	if err := st.ReconcileBookingICSSync(ctx, pid, "booking_ics", []DesiredBlock{block(uid, "2026-07-09", "2026-07-12")}, now, &SyncCounters{}); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"2026-07-09", "2026-07-10", "2026-07-11"} {
		if got := nightCount(t, st, pid, d); got != 1 {
			t.Fatalf("night %s count=%d want 1", d, got)
		}
	}
	// July 11 must be owned by the named stay.
	dates, err := st.ListActiveOccupancyNightDates(ctx, pid, "2026-07-11", "2026-07-12")
	if err != nil {
		t.Fatal(err)
	}
	occID := dates["2026-07-11"]
	row, err := st.GetOccupancyByID(ctx, pid, occID)
	if err != nil {
		t.Fatal(err)
	}
	if !row.GuestDisplayName.Valid || row.GuestDisplayName.String != "Koilpitchai" {
		t.Fatalf("july 11 owner=%+v want named stay", row)
	}
}

func TestUpdateNamedStay_CanExpandIntoUnnamedBlockCoverage(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()
	now := dt("2026-07-01")
	uid := "expand@booking.com"
	if err := st.ReconcileBookingICSSync(ctx, pid, "booking_ics", []DesiredBlock{block(uid, "2026-07-09", "2026-07-12")}, now, &SyncCounters{}); err != nil {
		t.Fatal(err)
	}
	occID, err := st.CreateNamedStay(ctx, pid, uid, "2026-07-11", "2026-07-12", "One Night", 1)
	if err != nil {
		t.Fatal(err)
	}
	ci, co := "2026-07-10", "2026-07-12"
	if err := st.UpdateNamedStay(ctx, pid, occID, &ci, &co, nil); err != nil {
		t.Fatalf("expand named stay into unnamed filler: %v", err)
	}
	for _, d := range []string{"2026-07-09", "2026-07-10", "2026-07-11"} {
		if got := nightCount(t, st, pid, d); got != 1 {
			t.Fatalf("night %s count=%d want 1", d, got)
		}
	}
	dates, err := st.ListActiveOccupancyNightDates(ctx, pid, "2026-07-10", "2026-07-12")
	if err != nil {
		t.Fatal(err)
	}
	if dates["2026-07-10"] != occID || dates["2026-07-11"] != occID {
		t.Fatalf("expanded named stay did not own both nights: %+v want %d", dates, occID)
	}
}

func TestReconcile_DisappearedUIDUsesPropertyLocalToday(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u, err := st.CreateUser(ctx, "tz-owner@test.local", "hash", "owner")
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProperty(ctx, u.ID, "Pacific", "America/Los_Angeles", "en")
	if err != nil {
		t.Fatal(err)
	}
	pid := p.ID
	uid := "tz-cutoff@booking.com"
	if err := st.ReconcileBookingICSSync(ctx, pid, "booking_ics", []DesiredBlock{block(uid, "2026-07-11", "2026-07-12")}, dt("2026-07-01"), &SyncCounters{}); err != nil {
		t.Fatal(err)
	}
	// UTC date is July 13, but the property-local date is still July 12, so a
	// July 12 checkout is current/today and must be reconciled as disappeared.
	now := time.Date(2026, 7, 13, 1, 0, 0, 0, time.UTC)
	if err := st.ReconcileBookingICSSync(ctx, pid, "booking_ics", nil, now, &SyncCounters{}); err != nil {
		t.Fatal(err)
	}
	row, err := st.GetOccupancyBySourceEventUID(ctx, pid, uid)
	if err != nil {
		t.Fatal(err)
	}
	if row.Status != StatusDeletedFromSource {
		t.Fatalf("status=%s want %s using property-local cutoff", row.Status, StatusDeletedFromSource)
	}
}

// §13.3 / §13.8: a disappeared UID marks current/future rows deleted_from_source.
func TestReconcile_DisappearedUIDDeleted(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()
	now := dt("2026-07-01")
	uid := "old-july31@booking.com"
	if err := st.ReconcileBookingICSSync(ctx, pid, "booking_ics", []DesiredBlock{block(uid, "2026-07-31", "2026-08-01")}, now, &SyncCounters{}); err != nil {
		t.Fatal(err)
	}
	// Name it so it has a guest name.
	if _, err := st.CreateNamedStay(ctx, pid, uid, "2026-07-31", "2026-08-01", "Guest", 1); err != nil {
		t.Fatal(err)
	}
	// New feed without the UID (a different UID replaces it).
	if err := st.ReconcileBookingICSSync(ctx, pid, "booking_ics", []DesiredBlock{block("new@booking.com", "2026-08-07", "2026-08-08")}, now, &SyncCounters{}); err != nil {
		t.Fatal(err)
	}
	if got := nightCount(t, st, pid, "2026-07-31"); got != 0 {
		t.Fatalf("july 31 count=%d want 0 (disappeared)", got)
	}
	rows, err := st.ListOccupanciesForUpstreamUID(ctx, pid, uid)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if isActiveStatus(r.Status) {
			t.Fatalf("row %d still active after disappearance", r.ID)
		}
	}
}

// §13.7: shrinking the source range deletes the out-of-range named night.
func TestReconcile_ShrinkDeletesNamedNight(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()
	now := dt("2026-07-01")
	uid := "abc@booking.com"
	if err := st.ReconcileBookingICSSync(ctx, pid, "booking_ics", []DesiredBlock{block(uid, "2026-07-09", "2026-07-12")}, now, &SyncCounters{}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateNamedStay(ctx, pid, uid, "2026-07-11", "2026-07-12", "Late", 1); err != nil {
		t.Fatal(err)
	}
	// Block shrinks to 07-09..07-11 (July 11 night gone).
	if err := st.ReconcileBookingICSSync(ctx, pid, "booking_ics", []DesiredBlock{block(uid, "2026-07-09", "2026-07-11")}, now, &SyncCounters{}); err != nil {
		t.Fatal(err)
	}
	if got := nightCount(t, st, pid, "2026-07-11"); got != 0 {
		t.Fatalf("july 11 count=%d want 0 after shrink", got)
	}
	if got := nightCount(t, st, pid, "2026-07-09"); got != 1 {
		t.Fatalf("july 9 count=%d want 1", got)
	}
}

// §13.13: the partial unique index rejects a second active night.
func TestOccupancyNights_UniqueConstraint(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()
	tx, err := st.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO occupancies (property_id, source_type, source_event_uid, start_at, end_at, status, content_hash, imported_at, last_synced_at) VALUES (?, 'booking_ics','a','2026-07-09T00:00:00Z','2026-07-10T00:00:00Z','active','h','2026-07-01T00:00:00Z','2026-07-01T00:00:00Z')`, pid)
	if err != nil {
		t.Fatal(err)
	}
	id1, _ := res.LastInsertId()
	res2, err := tx.ExecContext(ctx, `INSERT INTO occupancies (property_id, source_type, source_event_uid, start_at, end_at, status, content_hash, imported_at, last_synced_at) VALUES (?, 'booking_ics','b','2026-07-09T00:00:00Z','2026-07-10T00:00:00Z','active','h','2026-07-01T00:00:00Z','2026-07-01T00:00:00Z')`, pid)
	if err != nil {
		t.Fatal(err)
	}
	id2, _ := res2.LastInsertId()
	if err := insertOccupancyNightTx(ctx, tx, pid, id1, "2026-07-09", "booking_ics", "a", true); err != nil {
		t.Fatal(err)
	}
	err = insertOccupancyNightTx(ctx, tx, pid, id2, "2026-07-09", "booking_ics", "b", true)
	if err != ErrOccupancyNightConflict {
		t.Fatalf("want ErrOccupancyNightConflict, got %v", err)
	}
}

// §7.4: naming a range outside the block is rejected.
func TestCreateNamedStay_RejectsOutsideBlock(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()
	now := dt("2026-07-01")
	uid := "b@booking.com"
	if err := st.ReconcileBookingICSSync(ctx, pid, "booking_ics", []DesiredBlock{block(uid, "2026-07-09", "2026-07-12")}, now, &SyncCounters{}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateNamedStay(ctx, pid, uid, "2026-07-11", "2026-07-14", "X", 1); err != ErrNamedStayOutsideBlock {
		t.Fatalf("want ErrNamedStayOutsideBlock, got %v", err)
	}
}

// §13.4: July 30 present, older duplicate under a disappeared UID is deleted.
func TestReconcile_July30PresentOlderDuplicateDeleted(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()
	now := dt("2026-07-01")
	if err := st.ReconcileBookingICSSync(ctx, pid, "booking_ics", []DesiredBlock{block("older@booking.com", "2026-07-30", "2026-07-31")}, now, &SyncCounters{}); err != nil {
		t.Fatal(err)
	}
	if err := st.ReconcileBookingICSSync(ctx, pid, "booking_ics", []DesiredBlock{block("be776189886f75a948567e0d2bbe80b9@booking.com", "2026-07-30", "2026-07-31")}, now, &SyncCounters{}); err != nil {
		t.Fatal(err)
	}
	if got := nightCount(t, st, pid, "2026-07-30"); got != 1 {
		t.Fatalf("july 30 active count=%d want exactly 1", got)
	}
	rows, err := st.ListOccupanciesForUpstreamUID(ctx, pid, "older@booking.com")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if isActiveStatus(r.Status) {
			t.Fatalf("older UID row %d still active", r.ID)
		}
	}
}

// §13.9: pre-existing overlapping active rows are resolved to one by repair.
func TestOccupancyRepair_ResolvesCapacityOneDuplicates(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()
	runID, err := st.StartOccupancySyncRun(ctx, pid, "test")
	if err != nil {
		t.Fatal(err)
	}
	a := &Occupancy{PropertyID: pid, SourceType: "booking_ics", SourceEventUID: "dup-a", StartAt: dt("2026-07-11"), EndAt: dt("2026-07-12"), Status: "active", ContentHash: "a"}
	b := &Occupancy{PropertyID: pid, SourceType: "booking_ics", SourceEventUID: "dup-b", StartAt: dt("2026-07-11"), EndAt: dt("2026-07-12"), Status: "active", ContentHash: "b", GuestDisplayName: sql.NullString{String: "Named", Valid: true}}
	if err := st.UpsertOccupancy(ctx, a, runID); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertOccupancy(ctx, b, runID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx, `UPDATE occupancies SET representation_kind='named_stay' WHERE source_event_uid='dup-b'`); err != nil {
		t.Fatal(err)
	}
	aRow, err := st.GetOccupancyBySourceEventUID(ctx, pid, "dup-a")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := st.OccupancyRepairPlan(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	if plan.NightsResolved != 1 || plan.DuplicatesResolved != 1 {
		t.Fatalf("plan nights=%d dups=%d want 1/1", plan.NightsResolved, plan.DuplicatesResolved)
	}
	if _, err := st.OccupancyRepairApply(ctx, pid); err != nil {
		t.Fatal(err)
	}
	if got := nightCount(t, st, pid, "2026-07-11"); got != 1 {
		t.Fatalf("after repair july 11 active count=%d want 1", got)
	}
	dates, err := st.ListActiveOccupancyNightDates(ctx, pid, "2026-07-11", "2026-07-12")
	if err != nil {
		t.Fatal(err)
	}
	winner, err := st.GetOccupancyByID(ctx, pid, dates["2026-07-11"])
	if err != nil {
		t.Fatal(err)
	}
	if winner.SourceEventUID != "dup-b" {
		t.Fatalf("winner=%s want dup-b (named)", winner.SourceEventUID)
	}
	loser, err := st.GetOccupancyByID(ctx, pid, aRow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !loser.SupersededAt.Valid {
		t.Fatalf("loser dup-a not superseded")
	}
}

// §11: repair marks today/future rows deleted_from_source when their upstream
// UID is absent from the latest successful raw snapshot, and enriches the row
// action report. Rows still present in the snapshot are untouched.
func TestOccupancyRepair_SnapshotDisappearance(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()
	now := time.Now().UTC()
	future := now.AddDate(0, 0, 10).Format("2006-01-02")
	futureEnd := now.AddDate(0, 0, 11).Format("2006-01-02")

	runID, err := st.StartOccupancySyncRun(ctx, pid, "test")
	if err != nil {
		t.Fatal(err)
	}
	// Present UID: kept. Gone UID: named future row that disappeared.
	keep := &Occupancy{PropertyID: pid, SourceType: "booking_ics", SourceEventUID: "keep@b", StartAt: dt(future), EndAt: dt(futureEnd), Status: "active", ContentHash: "k"}
	gone := &Occupancy{PropertyID: pid, SourceType: "booking_ics", SourceEventUID: "gone@b", StartAt: dt(future), EndAt: dt(futureEnd), Status: "active", ContentHash: "g", GuestDisplayName: sql.NullString{String: "Ghost", Valid: true}}
	if err := st.UpsertOccupancy(ctx, keep, runID); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertOccupancy(ctx, gone, runID); err != nil {
		t.Fatal(err)
	}
	// Snapshot only contains keep@b, then a successful run.
	if err := st.InsertOccupancyRawEvent(ctx, pid, runID, "keep@b", "RAW", "CLOSED", dt(future).Format(time.RFC3339), dt(futureEnd).Format(time.RFC3339), 0, "", "k"); err != nil {
		t.Fatal(err)
	}
	if err := st.FinishOccupancySyncRun(ctx, runID, "success", nil, nil, 1, 1); err != nil {
		t.Fatal(err)
	}

	plan, err := st.OccupancyRepairPlan(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	if plan.RowsDeletedFromSource != 1 {
		t.Fatalf("plan RowsDeletedFromSource=%d want 1", plan.RowsDeletedFromSource)
	}
	if len(plan.RowActions) != 1 || plan.RowActions[0].UpstreamUID != "gone@b" || !plan.RowActions[0].RevokeNuki {
		t.Fatalf("unexpected row actions: %+v", plan.RowActions)
	}

	if _, err := st.OccupancyRepairApply(ctx, pid); err != nil {
		t.Fatal(err)
	}
	goneRow, err := st.GetOccupancyBySourceEventUID(ctx, pid, "gone@b")
	if err != nil {
		t.Fatal(err)
	}
	if goneRow.Status != StatusDeletedFromSource {
		t.Fatalf("gone row status=%s want %s", goneRow.Status, StatusDeletedFromSource)
	}
	keepRow, err := st.GetOccupancyBySourceEventUID(ctx, pid, "keep@b")
	if err != nil {
		t.Fatal(err)
	}
	if keepRow.Status == StatusDeletedFromSource {
		t.Fatalf("keep row wrongly deleted")
	}
}

// §10.1: creating a named stay over a legacy split row's night relinks the
// existing generated Nuki code (same PIN) when the window matches.
func TestCreateNamedStay_RelinksLegacyNukiCode(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()
	now := dt("2026-07-01")
	uid := "relink@booking.com"
	if err := st.ReconcileBookingICSSync(ctx, pid, "booking_ics", []DesiredBlock{block(uid, "2026-07-11", "2026-07-12")}, now, &SyncCounters{}); err != nil {
		t.Fatal(err)
	}
	// Simulate a legacy generated night row with a generated Nuki code for the
	// same night, then supersede it via reconcile when the named stay is made.
	runID, _ := st.StartOccupancySyncRun(ctx, pid, "t")
	legacy := &Occupancy{PropertyID: pid, SourceType: "booking_ics", SourceEventUID: uid + "#night-20260711", StartAt: dt("2026-07-11"), EndAt: dt("2026-07-12"), Status: "active", ContentHash: "leg", GuestDisplayName: sql.NullString{String: "Old", Valid: true}}
	if err := st.UpsertOccupancy(ctx, legacy, runID); err != nil {
		t.Fatal(err)
	}
	legacyRow, err := st.GetOccupancyBySourceEventUID(ctx, pid, uid+"#night-20260711")
	if err != nil {
		t.Fatal(err)
	}
	// Classify as a legacy generated night (as the backfill would).
	if _, err := st.DB.ExecContext(ctx, `UPDATE occupancies SET representation_kind='legacy_generated_night' WHERE id=?`, legacyRow.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertNukiCode(ctx, &NukiAccessCode{
		PropertyID:  pid,
		OccupancyID: legacyRow.ID,
		CodeLabel:   "Booking-Old",
		ValidFrom:   dt("2026-07-11").Add(14 * time.Hour),
		ValidUntil:  dt("2026-07-12").Add(10 * time.Hour),
		Status:      "generated",
	}); err != nil {
		t.Fatal(err)
	}
	occID, err := st.CreateNamedStay(ctx, pid, uid, "2026-07-11", "2026-07-12", "New Guest", 1)
	if err != nil {
		t.Fatal(err)
	}
	code, err := st.GetNukiCodeByOccupancyID(ctx, pid, occID)
	if err != nil {
		t.Fatal(err)
	}
	if code == nil {
		t.Fatal("expected Nuki code relinked to the named stay")
	}
	if code.CodeLabel != "Booking-Old" {
		t.Fatalf("relinked code label=%q want original", code.CodeLabel)
	}
	// The legacy row must no longer own the code.
	old, err := st.GetNukiCodeByOccupancyID(ctx, pid, legacyRow.ID)
	if err != nil {
		t.Fatal(err)
	}
	if old != nil {
		t.Fatal("legacy row still owns the code after relink")
	}
}

// §10.4: naming the whole block moves the finance mapping to the named stay;
// a sub-range name leaves it on the aggregate.
func TestCreateNamedStay_MovesFinanceMappingWhenWholeBlock(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()
	now := dt("2026-07-01")
	uid := "fin@booking.com"
	if err := st.ReconcileBookingICSSync(ctx, pid, "booking_ics", []DesiredBlock{block(uid, "2026-07-09", "2026-07-11")}, now, &SyncCounters{}); err != nil {
		t.Fatal(err)
	}
	agg, err := st.GetOccupancyBySourceEventUID(ctx, pid, uid)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a linked finance booking on the aggregate.
	res, err := st.DB.ExecContext(ctx, `INSERT INTO finance_bookings (property_id, reference_number, occupancy_id, net_cents, payout_date, created_at, updated_at) VALUES (?, 'REF1', ?, 10000, ?, ?, ?)`, pid, agg.ID, now.Format("2006-01-02"), now.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		t.Fatal(err)
	}
	bid, _ := res.LastInsertId()
	if _, err := st.DB.ExecContext(ctx, `UPDATE occupancies SET finance_booking_id=? WHERE id=?`, bid, agg.ID); err != nil {
		t.Fatal(err)
	}
	occID, err := st.CreateNamedStay(ctx, pid, uid, "2026-07-09", "2026-07-11", "Whole", 1)
	if err != nil {
		t.Fatal(err)
	}
	named, err := st.GetOccupancyByID(ctx, pid, occID)
	if err != nil {
		t.Fatal(err)
	}
	if !named.FinanceBookingID.Valid || named.FinanceBookingID.Int64 != bid {
		t.Fatalf("finance mapping not moved to named stay: %+v", named.FinanceBookingID)
	}
	aggAfter, err := st.GetOccupancyByID(ctx, pid, agg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if aggAfter.FinanceBookingID.Valid {
		t.Fatal("aggregate still holds finance mapping after whole-block naming")
	}
}
