package store

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func seedNukiGuestEntryOccupancy(t *testing.T, st *Store, propertyID int64, runID int64, uid string, startDay int) int64 {
	t.Helper()
	ctx := context.Background()
	start := time.Date(2026, 4, startDay, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 2)
	occ := &Occupancy{
		PropertyID:     propertyID,
		SourceType:     "booking_ics",
		SourceEventUID: uid,
		StartAt:        start,
		EndAt:          end,
		Status:         "active",
		RawSummary:     sql.NullString{String: uid, Valid: true},
		ContentHash:    "h-" + uid,
	}
	if err := st.UpsertOccupancy(ctx, occ, runID); err != nil {
		t.Fatal(err)
	}
	row, err := st.GetOccupancyBySourceEventUID(ctx, propertyID, uid)
	if err != nil || row == nil {
		t.Fatalf("get after upsert (uid=%s): %v", uid, err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := st.DB.ExecContext(ctx, `
		INSERT INTO named_stays (property_id, display_name, stay_type, check_in_date, check_out_date, status, cleaning_required, source_channel, source_reference, review_status, nuki_generation_status, created_at, updated_at)
		VALUES (?, ?, 'booking_com', ?, ?, 'active', 1, 'booking_ics', ?, 'confirmed', 'pending', ?, ?)`,
		propertyID, "Guest "+uid, start.Format("2006-01-02"), end.Format("2006-01-02"), uid, now, now)
	if err != nil {
		t.Fatal(err)
	}
	stayID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx, `
		INSERT INTO occupancy_stay_migration_map (old_occupancy_id, property_id, named_stay_id, migration_kind, notes, created_at)
		VALUES (?, ?, ?, 'named_stay', 'test_fixture', ?)`, row.ID, propertyID, stayID, now); err != nil {
		t.Fatal(err)
	}
	return row.ID
}

func TestUpsertNukiGuestDailyEntry_DedupAndOverwrite(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	u, err := st.CreateUser(ctx, "owner-nuki-guest@test.local", "hash", "owner")
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProperty(ctx, u.ID, "P-guest", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	runID, err := st.StartOccupancySyncRun(ctx, p.ID, "manual")
	if err != nil {
		t.Fatal(err)
	}
	occID := seedNukiGuestEntryOccupancy(t, st, p.ID, runID, "uid-A", 10)

	// Insert two unlocks for the same (occupancy, day). The reconciler is
	// supposed to feed the earlier one; simulate it overwriting a stale
	// later value.
	later := time.Date(2026, 4, 10, 18, 30, 0, 0, time.UTC)
	earlier := time.Date(2026, 4, 10, 14, 5, 0, 0, time.UTC)
	if err := st.UpsertNukiGuestDailyEntry(ctx, &NukiGuestDailyEntry{
		PropertyID: p.ID, OccupancyID: occID, DayDate: "2026-04-10",
		FirstEntryAt:       later,
		NukiEventReference: sql.NullString{String: "evt-late", Valid: true},
	}); err != nil {
		t.Fatalf("upsert later: %v", err)
	}
	if err := st.UpsertNukiGuestDailyEntry(ctx, &NukiGuestDailyEntry{
		PropertyID: p.ID, OccupancyID: occID, DayDate: "2026-04-10",
		FirstEntryAt:       earlier,
		NukiEventReference: sql.NullString{String: "evt-early", Valid: true},
	}); err != nil {
		t.Fatalf("upsert earlier: %v", err)
	}
	rows, err := st.ListNukiGuestDailyEntriesInRange(ctx, p.ID, "2026-04-01", "2026-04-30")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d want 1 (dedup by (occ,day))", len(rows))
	}
	if !rows[0].FirstEntryAt.Equal(earlier) {
		t.Fatalf("FirstEntryAt=%v want %v (overwrite must apply)", rows[0].FirstEntryAt, earlier)
	}
	if !rows[0].NukiEventReference.Valid || rows[0].NukiEventReference.String != "evt-early" {
		t.Fatalf("nuki_event_reference=%v", rows[0].NukiEventReference)
	}
}

func TestListNukiGuestDailyEntriesInRange_FiltersClosedAndCancelled(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	u, err := st.CreateUser(ctx, "owner-nuki-filter@test.local", "hash", "owner")
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProperty(ctx, u.ID, "P-filter", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	runID, err := st.StartOccupancySyncRun(ctx, p.ID, "manual")
	if err != nil {
		t.Fatal(err)
	}
	activeID := seedNukiGuestEntryOccupancy(t, st, p.ID, runID, "uid-active", 10)
	closedID := seedNukiGuestEntryOccupancy(t, st, p.ID, runID, "uid-closed", 12)
	externalID := seedNukiGuestEntryOccupancy(t, st, p.ID, runID, "uid-external", 14)

	if err := st.CloseOccupancy(ctx, p.ID, closedID, u.ID, "owner stay", "owner_stay"); err != nil {
		t.Fatalf("close: %v", err)
	}
	closedStayID, err := st.ResolveNamedStayIDForOccupancy(ctx, p.ID, closedID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx, `UPDATE named_stays SET stay_type = 'maintenance' WHERE id = ?`, closedStayID); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkOccupancyExternalSale(ctx, p.ID, externalID, u.ID, 12000, "EUR", "direct", ""); err != nil {
		t.Fatalf("external_sale: %v", err)
	}
	externalStayID, err := st.ResolveNamedStayIDForOccupancy(ctx, p.ID, externalID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx, `UPDATE named_stays SET stay_type = 'external' WHERE id = ?`, externalStayID); err != nil {
		t.Fatal(err)
	}

	insert := func(occID int64, day string, hour int) {
		t.Helper()
		ts := time.Date(2026, 4, parseDayHelper(day), hour, 0, 0, 0, time.UTC)
		if err := st.UpsertNukiGuestDailyEntry(ctx, &NukiGuestDailyEntry{
			PropertyID: p.ID, OccupancyID: occID, DayDate: day,
			FirstEntryAt: ts,
		}); err != nil {
			t.Fatalf("upsert (%d,%s): %v", occID, day, err)
		}
	}
	insert(activeID, "2026-04-10", 14)
	insert(closedID, "2026-04-12", 15)
	insert(externalID, "2026-04-14", 16)

	rows, err := st.ListNukiGuestDailyEntriesInRange(ctx, p.ID, "2026-04-01", "2026-04-30")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d want 2 (closed excluded, active+external_sale kept)", len(rows))
	}
	got := map[int64]bool{}
	for _, r := range rows {
		got[r.OccupancyID] = true
	}
	if !got[activeID] || !got[externalID] {
		t.Fatalf("missing expected occupancy ids: %v", got)
	}
	if got[closedID] {
		t.Fatalf("closed occupancy must be excluded")
	}

	// Range narrowing: limit to 2026-04-13..2026-04-15 — only external row.
	rows, err = st.ListNukiGuestDailyEntriesInRange(ctx, p.ID, "2026-04-13", "2026-04-15")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].OccupancyID != externalID {
		t.Fatalf("range filter: rows=%d want 1 (external only)", len(rows))
	}
}

func parseDayHelper(s string) int {
	// "2026-04-10" -> 10
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return 1
	}
	return t.Day()
}
