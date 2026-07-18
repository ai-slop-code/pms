package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func stage4RawBlock(t *testing.T, st *Store, propertyID int64, uid, checkIn, checkOut string) int64 {
	t.Helper()
	ctx := context.Background()
	if err := st.ReconcileBookingICSSync(ctx, propertyID, UpstreamSourceBookingICS, []DesiredBlock{block(uid, checkIn, checkOut)}, dt("2026-07-01"), &SyncCounters{RawBlocksDualWrite: true}); err != nil {
		t.Fatal(err)
	}
	var blockID int64
	if err := st.DB.QueryRowContext(ctx, `SELECT id FROM raw_booking_blocks WHERE property_id = ? AND source_event_uid = ?`, propertyID, uid).Scan(&blockID); err != nil {
		t.Fatal(err)
	}
	return blockID
}

func TestNamedStayStage4_PromoteRawBlockPartialPreservesRawCoverage(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()
	rawID := stage4RawBlock(t, st, pid, "stage4-partial@booking.com", "2026-07-09", "2026-07-12")

	stay, err := st.PromoteRawBookingBlockToNamedStay(ctx, pid, rawID, NamedStayCreateInput{
		DisplayName:     "Guest One",
		StayType:        StayTypeBookingCom,
		CheckInDate:     "2026-07-10",
		CheckOutDate:    "2026-07-12",
		CreatedByUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stay.ID == 0 || !stay.LegacyOccupancyID.Valid {
		t.Fatalf("stay ids not populated: %#v", stay)
	}
	if !stay.CleaningRequired {
		t.Fatal("booking_com cleaning_required=false, want true")
	}
	if !stay.NukiGenerationStatus.Valid || stay.NukiGenerationStatus.String != NukiGenerationPending {
		t.Fatalf("nuki_generation_status=%v want pending", stay.NukiGenerationStatus)
	}

	var namedNights int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM named_stay_nights WHERE named_stay_id = ? AND active = 1`, stay.ID).Scan(&namedNights); err != nil {
		t.Fatal(err)
	}
	if namedNights != 2 {
		t.Fatalf("named nights=%d want 2", namedNights)
	}
	var rawNights int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM raw_booking_block_nights WHERE raw_booking_block_id = ? AND active = 1`, rawID).Scan(&rawNights); err != nil {
		t.Fatal(err)
	}
	if rawNights != 3 {
		t.Fatalf("raw nights=%d want 3", rawNights)
	}
	var linkStatus string
	if err := st.DB.QueryRowContext(ctx, `SELECT link_status FROM stay_source_links WHERE named_stay_id = ? AND raw_booking_block_id = ?`, stay.ID, rawID).Scan(&linkStatus); err != nil {
		t.Fatal(err)
	}
	if linkStatus != "active" {
		t.Fatalf("link_status=%s want active", linkStatus)
	}
	var mappedStayID int64
	if err := st.DB.QueryRowContext(ctx, `SELECT named_stay_id FROM occupancy_stay_migration_map WHERE old_occupancy_id = ?`, stay.LegacyOccupancyID.Int64).Scan(&mappedStayID); err != nil {
		t.Fatal(err)
	}
	if mappedStayID != stay.ID {
		t.Fatalf("mapped stay id=%d want %d", mappedStayID, stay.ID)
	}
	var legacyNamedNights int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM occupancy_nights WHERE occupancy_id = ? AND active = 1`, stay.LegacyOccupancyID.Int64).Scan(&legacyNamedNights); err != nil {
		t.Fatal(err)
	}
	if legacyNamedNights != 2 {
		t.Fatalf("legacy named occupancy nights=%d want 2", legacyNamedNights)
	}
	var legacyRawNights int
	if err := st.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM occupancy_nights n
		JOIN occupancies o ON o.id = n.occupancy_id
		WHERE o.property_id = ? AND o.source_event_uid = ? AND n.active = 1`, pid, "stage4-partial@booking.com").Scan(&legacyRawNights); err != nil {
		t.Fatal(err)
	}
	if legacyRawNights != 1 {
		t.Fatalf("legacy raw leftover nights=%d want 1", legacyRawNights)
	}

	updated, err := st.UpdateNamedStayRecord(ctx, pid, stay.ID, NamedStayUpdateInput{
		CheckInDate:  ptrString("2026-07-09"),
		CheckOutDate: ptrString("2026-07-11"),
	})
	if err != nil {
		t.Fatal(err)
	}
	var linkedCheckIn, linkedCheckOut string
	if err := st.DB.QueryRowContext(ctx, `SELECT linked_check_in_date, linked_check_out_date FROM stay_source_links WHERE named_stay_id = ?`, updated.ID).Scan(&linkedCheckIn, &linkedCheckOut); err != nil {
		t.Fatal(err)
	}
	if linkedCheckIn != "2026-07-09" || linkedCheckOut != "2026-07-11" {
		t.Fatalf("linked range=%s/%s want 2026-07-09/2026-07-11", linkedCheckIn, linkedCheckOut)
	}
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM occupancy_nights WHERE occupancy_id = ? AND active = 1`, updated.LegacyOccupancyID.Int64).Scan(&legacyNamedNights); err != nil {
		t.Fatal(err)
	}
	if legacyNamedNights != 2 {
		t.Fatalf("legacy named occupancy nights after update=%d want 2", legacyNamedNights)
	}
}

func ptrString(v string) *string { return &v }

func TestNamedStayStage4_OverlapAndStatusLifecycle(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()

	stay, err := st.CreateNamedStayRecord(ctx, NamedStayCreateInput{
		PropertyID:      pid,
		DisplayName:     "External Guest",
		StayType:        StayTypeExternal,
		CheckInDate:     "2026-08-01",
		CheckOutDate:    "2026-08-03",
		CreatedByUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = st.CreateNamedStayRecord(ctx, NamedStayCreateInput{
		PropertyID:      pid,
		DisplayName:     "Maintenance",
		StayType:        StayTypeMaintenance,
		CheckInDate:     "2026-08-02",
		CheckOutDate:    "2026-08-04",
		CreatedByUserID: 1,
	})
	if !errors.Is(err, ErrNamedStayOverlap) {
		t.Fatalf("overlap err=%v want %v", err, ErrNamedStayOverlap)
	}

	cancelled, err := st.UpdateNamedStayStatus(ctx, pid, stay.ID, NamedStayStatusCancelled, 1)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.Status != NamedStayStatusCancelled {
		t.Fatalf("status=%s want cancelled", cancelled.Status)
	}
	var activeNights int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM named_stay_nights WHERE named_stay_id = ? AND active = 1`, stay.ID).Scan(&activeNights); err != nil {
		t.Fatal(err)
	}
	if activeNights != 0 {
		t.Fatalf("active nights after cancel=%d want 0", activeNights)
	}

	second, err := st.CreateNamedStayRecord(ctx, NamedStayCreateInput{
		PropertyID:      pid,
		DisplayName:     "Maintenance",
		StayType:        StayTypeMaintenance,
		CheckInDate:     "2026-08-02",
		CheckOutDate:    "2026-08-04",
		CreatedByUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.CleaningRequired {
		t.Fatal("maintenance cleaning_required=true, want false")
	}
	_, err = st.UpdateNamedStayStatus(ctx, pid, stay.ID, NamedStayStatusActive, 1)
	if !errors.Is(err, ErrNamedStayOverlap) {
		t.Fatalf("reactivate err=%v want %v", err, ErrNamedStayOverlap)
	}
}

func TestNamedStayStage11_LegacyWriteDisabledDoesNotCreateDerivedOccupancy(t *testing.T) {
	st, pid := recTestProperty(t)
	st.OccupancyLegacyWriteDisabled = true
	ctx := context.Background()

	stay, err := st.CreateNamedStayRecord(ctx, NamedStayCreateInput{
		PropertyID:      pid,
		DisplayName:     "Stage Eleven Guest",
		StayType:        StayTypeExternal,
		CheckInDate:     "2026-08-10",
		CheckOutDate:    "2026-08-12",
		CreatedByUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stay.LegacyOccupancyID.Valid {
		t.Fatalf("legacy occupancy id=%d, want null", stay.LegacyOccupancyID.Int64)
	}
	var legacyRows int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM occupancies WHERE property_id = ? AND source_event_uid = ?`, pid, "named_stay:"+fmt.Sprint(stay.ID)).Scan(&legacyRows); err != nil {
		t.Fatal(err)
	}
	if legacyRows != 0 {
		t.Fatalf("derived legacy occupancies=%d want 0", legacyRows)
	}
	var mapRows int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM occupancy_stay_migration_map WHERE property_id = ? AND named_stay_id = ?`, pid, stay.ID).Scan(&mapRows); err != nil {
		t.Fatal(err)
	}
	if mapRows != 0 {
		t.Fatalf("legacy migration map rows=%d want 0", mapRows)
	}
}

func TestNamedStayStage4_NukiGenerationBadgeState(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()
	stay, err := st.CreateNamedStayRecord(ctx, NamedStayCreateInput{
		PropertyID:      pid,
		DisplayName:     "Guest With Nuki Error",
		StayType:        StayTypeBookingCom,
		CheckInDate:     "2026-09-01",
		CheckOutDate:    "2026-09-02",
		CreatedByUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.MarkNamedStayNukiGeneration(ctx, pid, stay.ID, NukiGenerationError, "nuki_credentials_not_configured"); err != nil {
		t.Fatal(err)
	}
	refreshed, err := st.GetNamedStay(ctx, pid, stay.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !refreshed.NukiGenerationStatus.Valid || refreshed.NukiGenerationStatus.String != NukiGenerationError {
		t.Fatalf("nuki status=%v want error", refreshed.NukiGenerationStatus)
	}
	if !refreshed.NukiGenerationError.Valid || refreshed.NukiGenerationError.String != "nuki_credentials_not_configured" {
		t.Fatalf("nuki error=%v", refreshed.NukiGenerationError)
	}
}
