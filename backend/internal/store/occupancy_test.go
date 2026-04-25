package store

import (
	"context"
	"database/sql"
	"pms/backend/internal/testutil"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	return &Store{DB: testutil.OpenTestDB(t)}
}

func TestMarkOccupanciesDeletedFromSource_OnlyFuture(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u, err := st.CreateUser(ctx, "owner@test.local", "hash", "owner")
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProperty(ctx, u.ID, "P1", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	runID, err := st.StartOccupancySyncRun(ctx, p.ID, "manual")
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	past := &Occupancy{
		PropertyID:     p.ID,
		SourceType:     "booking_ics",
		SourceEventUID: "past-uid",
		StartAt:        now.AddDate(0, 0, -10),
		EndAt:          now.AddDate(0, 0, -8),
		Status:         "active",
		RawSummary:     sql.NullString{String: "past", Valid: true},
		ContentHash:    "h1",
	}
	future := &Occupancy{
		PropertyID:     p.ID,
		SourceType:     "booking_ics",
		SourceEventUID: "future-uid",
		StartAt:        now.AddDate(0, 0, 2),
		EndAt:          now.AddDate(0, 0, 4),
		Status:         "active",
		RawSummary:     sql.NullString{String: "future", Valid: true},
		ContentHash:    "h2",
	}
	if err := st.UpsertOccupancy(ctx, past, runID); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertOccupancy(ctx, future, runID); err != nil {
		t.Fatal(err)
	}

	// Simulate source removing both UIDs. Past occupancy should be retained.
	if err := st.MarkOccupanciesDeletedFromSource(ctx, p.ID, "booking_ics", nil); err != nil {
		t.Fatal(err)
	}

	items, err := st.ListOccupancies(ctx, p.ID, "", time.UTC, nil, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	statusByUID := map[string]string{}
	for _, it := range items {
		statusByUID[it.SourceEventUID] = it.Status
	}
	if got := statusByUID["past-uid"]; got != "active" {
		t.Fatalf("past uid status = %q, want active", got)
	}
	if got := statusByUID["future-uid"]; got != "deleted_from_source" {
		t.Fatalf("future uid status = %q, want deleted_from_source", got)
	}
}
