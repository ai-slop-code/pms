package store

import (
	"context"
	"testing"

	"pms/backend/internal/testutil"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	return &Store{DB: testutil.OpenTestDB(t)}
}

func TestOccupancySourceConfiguration(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	owner, err := st.CreateUser(ctx, "occupancy-source@test.local", "hash", "owner")
	if err != nil {
		t.Fatal(err)
	}
	property, err := st.CreateProperty(ctx, owner.ID, "Source", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}

	active := false
	sourceType := "booking_ics"
	if err := st.UpdateOccupancySource(ctx, property.ID, &active, &sourceType); err != nil {
		t.Fatal(err)
	}
	source, err := st.GetOccupancySource(ctx, property.ID)
	if err != nil {
		t.Fatal(err)
	}
	if source.Active || source.SourceType != sourceType {
		t.Fatalf("source=%+v", source)
	}
}

func TestOccupancyRawEventSnapshotUpsert(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	owner, err := st.CreateUser(ctx, "occupancy-raw@test.local", "hash", "owner")
	if err != nil {
		t.Fatal(err)
	}
	property, err := st.CreateProperty(ctx, owner.ID, "Raw", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	runID, err := st.StartOccupancySyncRun(ctx, property.ID, "test")
	if err != nil {
		t.Fatal(err)
	}

	if err := st.InsertOccupancyRawEventDetailed(ctx, property.ID, runID, "booking_ics", "uid-1", "raw-1", "first", "2026-08-01T00:00:00Z", "2026-08-02T00:00:00Z", 1, "CONFIRMED", "2026-07-01T00:00:00Z", "hash-1"); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertOccupancyRawEventDetailed(ctx, property.ID, runID, "booking_ics", "uid-1", "raw-2", "updated", "2026-08-01T00:00:00Z", "2026-08-03T00:00:00Z", 2, "CONFIRMED", "2026-07-02T00:00:00Z", "hash-2"); err != nil {
		t.Fatal(err)
	}

	var count, sequence int
	var raw, summary, hash string
	if err := st.DB.QueryRowContext(ctx, `
		SELECT COUNT(*), raw_component, summary, sequence_num, content_hash
		FROM occupancy_raw_events
		WHERE property_id = ? AND sync_run_id = ? AND source_type = 'booking_ics' AND source_event_uid = 'uid-1'`, property.ID, runID).
		Scan(&count, &raw, &summary, &sequence, &hash); err != nil {
		t.Fatal(err)
	}
	if count != 1 || raw != "raw-2" || summary != "updated" || sequence != 2 || hash != "hash-2" {
		t.Fatalf("count=%d raw=%q summary=%q sequence=%d hash=%q", count, raw, summary, sequence, hash)
	}
}
