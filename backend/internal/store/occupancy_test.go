package store

import (
	"context"
	"database/sql"
	"pms/backend/internal/testutil"
	"strings"
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

func TestListUpcomingStaysForNuki_UsesNamedStaysAndHidesRawBlocks(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u, err := st.CreateUser(ctx, "owner-nuki@test.local", "hash", "owner")
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProperty(ctx, u.ID, "P1", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	runID, err := st.StartOccupancySyncRun(ctx, p.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 2)
	end := start.AddDate(0, 0, 1)
	occ := &Occupancy{
		PropertyID:       p.ID,
		SourceType:       "booking_ics",
		SourceEventUID:   "deleted-with-code",
		StartAt:          start,
		EndAt:            end,
		Status:           "deleted_from_source",
		RawSummary:       sql.NullString{String: "CLOSED - Not available", Valid: true},
		GuestDisplayName: sql.NullString{String: "Alexander", Valid: true},
		ContentHash:      "h1",
	}
	if err := st.UpsertOccupancy(ctx, occ, runID); err != nil {
		t.Fatal(err)
	}
	saved, err := st.GetOccupancyBySourceEventUID(ctx, p.ID, "deleted-with-code")
	if err != nil || saved == nil {
		t.Fatalf("occupancy err=%v nil=%v", err, saved == nil)
	}
	nowText := time.Now().UTC().Format(time.RFC3339)
	res, err := st.DB.ExecContext(ctx, `
		INSERT INTO named_stays (property_id, display_name, stay_type, check_in_date, check_out_date, status, cleaning_required, source_channel, source_reference, review_status, nuki_generation_status, created_at, updated_at)
		VALUES (?, 'Alexander', 'booking_com', ?, ?, 'active', 1, 'booking_ics', 'deleted-with-code', 'confirmed', 'generated', ?, ?)`,
		p.ID, start.Format("2006-01-02"), end.Format("2006-01-02"), nowText, nowText)
	if err != nil {
		t.Fatal(err)
	}
	stayID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx, `
		INSERT INTO occupancy_stay_migration_map (old_occupancy_id, property_id, named_stay_id, migration_kind, notes, created_at)
		VALUES (?, ?, ?, 'named_stay', 'test_fixture', ?)`, saved.ID, p.ID, stayID, nowText); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertNukiCode(ctx, &NukiAccessCode{
		PropertyID:       p.ID,
		OccupancyID:      saved.ID,
		NamedStayID:      sql.NullInt64{Int64: stayID, Valid: true},
		CodeLabel:        "Booking-Alexander",
		ExternalNukiID:   sql.NullString{String: "ext-alexander", Valid: true},
		ValidFrom:        start.Add(12 * time.Hour),
		ValidUntil:       end.Add(7 * time.Hour),
		Status:           "generated",
		AccessCodeMasked: sql.NullString{String: "******", Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertOccupancy(ctx, &Occupancy{
		PropertyID:     p.ID,
		SourceType:     "booking_ics",
		SourceEventUID: "split-active-same-night",
		StartAt:        start,
		EndAt:          end,
		Status:         "active",
		RawSummary:     sql.NullString{String: "CLOSED - Not available", Valid: true},
		ContentHash:    "h2",
	}, runID); err != nil {
		t.Fatal(err)
	}

	rows, err := st.ListUpcomingStaysForNuki(ctx, p.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d want 1", len(rows))
	}
	if rows[0].StayID != stayID || rows[0].OccupancyID != saved.ID || rows[0].OccupancyStatus != "active" {
		t.Fatalf("unexpected row: %+v", rows[0])
	}
	if !rows[0].GeneratedStatus.Valid || rows[0].GeneratedStatus.String != "generated" {
		t.Fatalf("generated status=%#v", rows[0].GeneratedStatus)
	}
}

func TestCloseOccupancyNight_SplitsMultiNightStayAndClosesOnlySelectedNight(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u, err := st.CreateUser(ctx, "owner-close-night@test.local", "hash", "owner")
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProperty(ctx, u.ID, "P1", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	runID, err := st.StartOccupancySyncRun(ctx, p.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	occ := &Occupancy{
		PropertyID:     p.ID,
		SourceType:     "booking_ics",
		SourceEventUID: "august-block",
		StartAt:        time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC),
		EndAt:          time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC),
		Status:         "active",
		RawSummary:     sql.NullString{String: "CLOSED - Not available", Valid: true},
		ContentHash:    "h1",
	}
	if err := st.UpsertOccupancy(ctx, occ, runID); err != nil {
		t.Fatal(err)
	}
	saved, err := st.GetOccupancyBySourceEventUID(ctx, p.ID, "august-block")
	if err != nil || saved == nil {
		t.Fatalf("occupancy err=%v nil=%v", err, saved == nil)
	}
	if err := st.CloseOccupancyNight(ctx, p.ID, saved.ID, u.ID, "2026-08-10", "maintenance", "maintenance"); err != nil {
		t.Fatal(err)
	}
	rows, err := st.ListOccupancies(ctx, p.ID, "", time.UTC, nil, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	statusByUID := map[string]Occupancy{}
	for _, row := range rows {
		statusByUID[row.SourceEventUID] = row
	}
	// PMS_19 model: the aggregate stays active as the unnamed-block filler; only
	// the selected night gets a closed representation. No before/after rows.
	if got := statusByUID["august-block"].Status; got != "active" && got != "updated" {
		t.Fatalf("aggregate status=%s want active", got)
	}
	if _, ok := statusByUID["manual_split:august-block:before:20260807"]; ok {
		t.Fatal("did not expect a before split row in the new model")
	}
	closed := statusByUID["manual_split:august-block:closed:20260810"]
	if closed.Status != "active" || !closed.ClosureState.Valid || closed.ClosureState.String != ClosureStateClosed {
		t.Fatalf("closed row=%+v", closed)
	}
	if closed.StartAt.Format(time.RFC3339) != "2026-08-10T00:00:00Z" || closed.EndAt.Format(time.RFC3339) != "2026-08-11T00:00:00Z" {
		t.Fatalf("closed window=%s..%s", closed.StartAt, closed.EndAt)
	}
	for _, d := range []string{"2026-08-07", "2026-08-08", "2026-08-09", "2026-08-10"} {
		n, err := st.ActiveOccupancyNightCount(ctx, p.ID, d)
		if err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("night %s active count=%d want 1", d, n)
		}
	}
	hasManual, err := st.HasManualSplitForSourceEventUID(ctx, p.ID, "august-block")
	if err != nil {
		t.Fatal(err)
	}
	if !hasManual {
		t.Fatal("expected manual split marker")
	}
}

func TestSplitOccupancyIntoNights_CreatesManualNightRows(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u, err := st.CreateUser(ctx, "owner-split-night@test.local", "hash", "owner")
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProperty(ctx, u.ID, "P1", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	runID, err := st.StartOccupancySyncRun(ctx, p.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	occ := &Occupancy{
		PropertyID:     p.ID,
		SourceType:     "booking_ics",
		SourceEventUID: "july-merged",
		StartAt:        time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC),
		EndAt:          time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		Status:         "active",
		RawSummary:     sql.NullString{String: "CLOSED - Not available", Valid: true},
		ContentHash:    "h1",
	}
	if err := st.UpsertOccupancy(ctx, occ, runID); err != nil {
		t.Fatal(err)
	}
	saved, err := st.GetOccupancyBySourceEventUID(ctx, p.ID, "july-merged")
	if err != nil || saved == nil {
		t.Fatalf("occupancy err=%v nil=%v", err, saved == nil)
	}
	if err := st.SplitOccupancyIntoNights(ctx, p.ID, saved.ID); err != nil {
		t.Fatal(err)
	}
	rows, err := st.ListOccupancies(ctx, p.ID, "", time.UTC, nil, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	activeManual := 0
	for _, row := range rows {
		if strings.HasPrefix(row.SourceEventUID, "manual_split:july-merged:night:") && row.Status == "active" {
			activeManual++
			if row.EndAt.Sub(row.StartAt) != 24*time.Hour {
				t.Fatalf("manual duration=%s", row.EndAt.Sub(row.StartAt))
			}
		}
	}
	if activeManual != 2 {
		t.Fatalf("active manual nights=%d want 2", activeManual)
	}
	// Each night has exactly one active representation (the night row wins over
	// the aggregate filler).
	for _, d := range []string{"2026-07-30", "2026-07-31"} {
		n, err := st.ActiveOccupancyNightCount(ctx, p.ID, d)
		if err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("night %s active count=%d want 1", d, n)
		}
	}
}

func TestSplitOccupancyIntoNightRange_SplitsOnlySelectedNights(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u, err := st.CreateUser(ctx, "owner-split-range@test.local", "hash", "owner")
	if err != nil {
		t.Fatal(err)
	}
	p, err := st.CreateProperty(ctx, u.ID, "P1", "UTC", "en")
	if err != nil {
		t.Fatal(err)
	}
	runID, err := st.StartOccupancySyncRun(ctx, p.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	occ := &Occupancy{
		PropertyID:     p.ID,
		SourceType:     "booking_ics",
		SourceEventUID: "august-reservation",
		StartAt:        time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC),
		EndAt:          time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC),
		Status:         "active",
		RawSummary:     sql.NullString{String: "Reservation", Valid: true},
		ContentHash:    "h1",
	}
	if err := st.UpsertOccupancy(ctx, occ, runID); err != nil {
		t.Fatal(err)
	}
	saved, err := st.GetOccupancyBySourceEventUID(ctx, p.ID, "august-reservation")
	if err != nil || saved == nil {
		t.Fatalf("occupancy err=%v nil=%v", err, saved == nil)
	}
	if err := st.SplitOccupancyIntoNightRange(ctx, p.ID, saved.ID, "2026-08-10", "2026-08-11"); err != nil {
		t.Fatal(err)
	}
	rows, err := st.ListOccupancies(ctx, p.ID, "", time.UTC, nil, 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	byUID := map[string]Occupancy{}
	for _, row := range rows {
		byUID[row.SourceEventUID] = row
	}
	// New model: aggregate stays active as filler; no before row is created.
	if got := byUID["august-reservation"].Status; got != "active" && got != "updated" {
		t.Fatalf("aggregate status=%s want active", got)
	}
	if _, ok := byUID["manual_split:august-reservation:before:20260807"]; ok {
		t.Fatal("did not expect a before split row in the new model")
	}
	night := byUID["manual_split:august-reservation:night:20260810"]
	if night.Status != "active" || night.StartAt.Format(time.RFC3339) != "2026-08-10T00:00:00Z" || night.EndAt.Format(time.RFC3339) != "2026-08-11T00:00:00Z" {
		t.Fatalf("night row=%+v", night)
	}
	for uid := range byUID {
		if strings.HasPrefix(uid, "manual_split:august-reservation:night:") && uid != "manual_split:august-reservation:night:20260810" {
			t.Fatalf("unexpected split night row %s", uid)
		}
	}
	// The filler covers 8/7-8/9 and the night row covers 8/10, one each.
	for _, d := range []string{"2026-08-07", "2026-08-08", "2026-08-09", "2026-08-10"} {
		n, err := st.ActiveOccupancyNightCount(ctx, p.ID, d)
		if err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("night %s active count=%d want 1", d, n)
		}
	}
}
