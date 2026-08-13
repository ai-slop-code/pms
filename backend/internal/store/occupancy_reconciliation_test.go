package store

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestSyncCountersHasNoRawBlocksDualWriteFlag(t *testing.T) {
	if _, ok := reflect.TypeOf(SyncCounters{}).FieldByName("RawBlocksDualWrite"); ok {
		t.Fatal("RawBlocksDualWrite flag still exists")
	}
}

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

func dt(value string) time.Time {
	parsed, _ := time.Parse("2006-01-02", value)
	return time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.UTC)
}

func block(uid, start, end string) DesiredBlock {
	return DesiredBlock{
		UID:         uid,
		Start:       dt(start),
		End:         dt(end),
		Summary:     "CLOSED - Not available",
		ContentHash: uid + start + end,
	}
}

func TestReconcileBookingICSSyncUnconditionallyWritesRawBlocksAndNights(t *testing.T) {
	st, propertyID := recTestProperty(t)
	ctx := context.Background()
	counters := &SyncCounters{}
	uid := "raw-upsert@booking.com"

	if err := st.ReconcileBookingICSSync(ctx, propertyID, UpstreamSourceBookingICS,
		[]DesiredBlock{block(uid, "2026-07-09", "2026-07-12")}, dt("2026-07-01"), counters); err != nil {
		t.Fatal(err)
	}
	if counters.RawBlocksInserted != 1 {
		t.Fatalf("raw blocks inserted=%d want 1", counters.RawBlocksInserted)
	}
	var blockID int64
	if err := st.DB.QueryRowContext(ctx, `SELECT id FROM raw_booking_blocks WHERE property_id = ? AND source_event_uid = ?`, propertyID, uid).Scan(&blockID); err != nil {
		t.Fatal(err)
	}
	var nights int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM raw_booking_block_nights WHERE raw_booking_block_id = ? AND active = 1`, blockID).Scan(&nights); err != nil {
		t.Fatal(err)
	}
	if nights != 3 {
		t.Fatalf("active raw nights=%d want 3", nights)
	}
}

func TestReconcileBookingICSSyncMarksDisappearedRawBlockAndSourceLink(t *testing.T) {
	st, propertyID := recTestProperty(t)
	ctx := context.Background()
	now := dt("2026-07-01")
	uid := "raw-gone@booking.com"
	if err := st.ReconcileBookingICSSync(ctx, propertyID, UpstreamSourceBookingICS,
		[]DesiredBlock{block(uid, "2026-07-31", "2026-08-01")}, now, &SyncCounters{}); err != nil {
		t.Fatal(err)
	}
	counters := &SyncCounters{}
	if err := st.ReconcileBookingICSSync(ctx, propertyID, UpstreamSourceBookingICS, nil, now, counters); err != nil {
		t.Fatal(err)
	}
	if counters.RawBlocksDeletedFromSource != 1 {
		t.Fatalf("raw blocks deleted=%d want 1", counters.RawBlocksDeletedFromSource)
	}
	var status string
	var activeNights int
	if err := st.DB.QueryRowContext(ctx, `SELECT status FROM raw_booking_blocks WHERE property_id = ? AND source_event_uid = ?`, propertyID, uid).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != StatusDeletedFromSource {
		t.Fatalf("raw block status=%q want %q", status, StatusDeletedFromSource)
	}
	if err := st.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM raw_booking_block_nights n
		JOIN raw_booking_blocks b ON b.id = n.raw_booking_block_id
		WHERE b.property_id = ? AND b.source_event_uid = ? AND n.active = 1`, propertyID, uid).Scan(&activeNights); err != nil {
		t.Fatal(err)
	}
	if activeNights != 0 {
		t.Fatalf("active raw nights=%d want 0", activeNights)
	}
}
