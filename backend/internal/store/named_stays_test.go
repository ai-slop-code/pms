package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func stage4RawBlock(t *testing.T, st *Store, propertyID int64, uid, checkIn, checkOut string) int64 {
	t.Helper()
	ctx := context.Background()
	if err := st.ReconcileBookingICSSync(ctx, propertyID, UpstreamSourceBookingICS, []DesiredBlock{block(uid, checkIn, checkOut)}, dt("2026-07-01"), &SyncCounters{}); err != nil {
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
	if stay.ID == 0 {
		t.Fatalf("stay id not populated: %#v", stay)
	}
	if !stay.CleaningRequired {
		t.Fatal("booking_com cleaning_required=false, want true")
	}
	if !stay.NukiGenerationStatus.Valid || stay.NukiGenerationStatus.String != NukiGenerationPending {
		t.Fatalf("nuki_generation_status=%v want pending", stay.NukiGenerationStatus)
	}
	wantFirstKnown := dt("2026-07-01")
	if !stay.FirstKnownAt.Equal(wantFirstKnown) {
		t.Fatalf("first_known_at=%s want raw imported_at %s", stay.FirstKnownAt, wantFirstKnown)
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
	var legacyTables int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_schema WHERE type = 'table' AND name = 'occupancies'`).Scan(&legacyTables); err != nil {
		t.Fatal(err)
	}
	if legacyTables != 0 {
		t.Fatalf("legacy occupancy tables=%d want 0", legacyTables)
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

func TestNamedStayCreateDoesNotCreateDerivedOccupancy(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()

	_, err := st.CreateNamedStayRecord(ctx, NamedStayCreateInput{
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
	var legacyTables int
	if err := st.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sqlite_schema
		WHERE type = 'table' AND name IN ('occupancies', 'occupancy_stay_migration_map')`).Scan(&legacyTables); err != nil {
		t.Fatal(err)
	}
	if legacyTables != 0 {
		t.Fatalf("legacy occupancy tables=%d want 0", legacyTables)
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

func TestSourceLinkHealth_DisappearShrinkAndRecover(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()
	uid := "source-health@booking.com"
	rawID := stage4RawBlock(t, st, pid, uid, "2026-07-20", "2026-07-23")
	stay, err := st.PromoteRawBookingBlockToNamedStay(ctx, pid, rawID, NamedStayCreateInput{
		DisplayName: "Source Owned Guest", StayType: StayTypeBookingCom,
		CheckInDate: "2026-07-20", CheckOutDate: "2026-07-23",
	})
	if err != nil {
		t.Fatal(err)
	}
	assertLink := func(wantStatus, wantReason string) {
		t.Helper()
		var status string
		var reason sql.NullString
		if err := st.DB.QueryRowContext(ctx, `SELECT link_status, conflict_reason FROM stay_source_links WHERE named_stay_id = ?`, stay.ID).Scan(&status, &reason); err != nil {
			t.Fatal(err)
		}
		if status != wantStatus || reason.String != wantReason || reason.Valid != (wantReason != "") {
			t.Fatalf("link status=%q reason=%#v want %q/%q", status, reason, wantStatus, wantReason)
		}
	}
	assertLink("active", "")

	counters := &SyncCounters{}
	if err := st.ReconcileBookingICSSync(ctx, pid, UpstreamSourceBookingICS, nil, dt("2026-07-19"), counters); err != nil {
		t.Fatal(err)
	}
	assertLink("source_deleted", "raw_source_missing")
	if counters.RawBlockConflicts != 1 {
		t.Fatalf("raw conflicts=%d want 1", counters.RawBlockConflicts)
	}
	unchanged, err := st.GetNamedStay(ctx, pid, stay.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.DisplayName != "Source Owned Guest" || unchanged.CheckInDate != "2026-07-20" || unchanged.CheckOutDate != "2026-07-23" || unchanged.Status != NamedStayStatusActive {
		t.Fatalf("sync mutated named stay: %+v", unchanged)
	}

	if err := st.ReconcileBookingICSSync(ctx, pid, UpstreamSourceBookingICS, []DesiredBlock{block(uid, "2026-07-20", "2026-07-22")}, dt("2026-07-19"), &SyncCounters{}); err != nil {
		t.Fatal(err)
	}
	assertLink("conflict", "raw_coverage_gap")
	if err := st.ReconcileBookingICSSync(ctx, pid, UpstreamSourceBookingICS, []DesiredBlock{block(uid, "2026-07-20", "2026-07-23")}, dt("2026-07-19"), &SyncCounters{}); err != nil {
		t.Fatal(err)
	}
	assertLink("active", "")
}

func TestNamedStayUpdate_AllowsUnrelatedEditsWithSourceWarnings(t *testing.T) {
	for _, tc := range []struct {
		name          string
		desiredBlocks []DesiredBlock
		wantStatus    string
		nullSourceID  bool
	}{
		{name: "source deleted", wantStatus: "source_deleted", nullSourceID: true},
		{name: "coverage conflict", desiredBlocks: []DesiredBlock{block("warning-edit", "2026-07-20", "2026-07-22")}, wantStatus: "conflict"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, pid := recTestProperty(t)
			ctx := context.Background()
			rawID := stage4RawBlock(t, st, pid, "warning-edit", "2026-07-20", "2026-07-23")
			stay, err := st.PromoteRawBookingBlockToNamedStay(ctx, pid, rawID, NamedStayCreateInput{
				DisplayName: "Original Guest", StayType: StayTypeBookingCom,
				CheckInDate: "2026-07-20", CheckOutDate: "2026-07-23",
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := st.ReconcileBookingICSSync(ctx, pid, UpstreamSourceBookingICS, tc.desiredBlocks, dt("2026-07-19"), &SyncCounters{}); err != nil {
				t.Fatal(err)
			}
			if tc.nullSourceID {
				if _, err := st.DB.ExecContext(ctx, `UPDATE stay_source_links SET raw_booking_block_id = NULL WHERE named_stay_id = ?`, stay.ID); err != nil {
					t.Fatal(err)
				}
			}

			displayName := "Edited Guest"
			stayType := StayTypeExternal
			cleaningRequired := false
			cleaningReason := "operator override"
			revenue := int64(12345)
			currency := "EUR"
			note := "manual adjustment"
			updated, err := st.UpdateNamedStayRecord(ctx, pid, stay.ID, NamedStayUpdateInput{
				DisplayName: &displayName, StayType: &stayType,
				CleaningRequired: &cleaningRequired, CleaningOverrideReason: &cleaningReason,
				ManualRevenueCents: &revenue, ManualRevenueCurrency: &currency, ManualRevenueNote: &note,
			})
			if err != nil {
				t.Fatal(err)
			}
			if updated.DisplayName != displayName || updated.StayType != stayType || updated.CleaningRequired ||
				updated.CleaningOverrideReason.String != cleaningReason ||
				updated.ManualRevenueCents.Int64 != revenue || updated.ManualRevenueCurrency.String != currency || updated.ManualRevenueNote.String != note {
				t.Fatalf("unrelated edits not persisted: %+v", updated)
			}
			var linkStatus string
			if err := st.DB.QueryRowContext(ctx, `SELECT link_status FROM stay_source_links WHERE named_stay_id = ?`, stay.ID).Scan(&linkStatus); err != nil {
				t.Fatal(err)
			}
			if linkStatus != tc.wantStatus {
				t.Fatalf("link status=%q want %q", linkStatus, tc.wantStatus)
			}
			_, err = st.UpdateNamedStayRecord(ctx, pid, stay.ID, NamedStayUpdateInput{CheckOutDate: ptrString("2026-07-24")})
			if !errors.Is(err, ErrNamedStayOutsideBlock) {
				t.Fatalf("date update with %s source warning error=%v", tc.wantStatus, err)
			}
		})
	}
}

func TestNamedStayUpdate_AllowsUnionOfAdjacentRawBlocks(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()
	if err := st.ReconcileBookingICSSync(ctx, pid, UpstreamSourceBookingICS, []DesiredBlock{
		block("union-a", "2026-08-10", "2026-08-12"), block("union-b", "2026-08-12", "2026-08-14"),
	}, dt("2026-07-01"), &SyncCounters{}); err != nil {
		t.Fatal(err)
	}
	var firstID, secondID int64
	if err := st.DB.QueryRowContext(ctx, `SELECT id FROM raw_booking_blocks WHERE property_id = ? AND source_event_uid = 'union-a'`, pid).Scan(&firstID); err != nil {
		t.Fatal(err)
	}
	if err := st.DB.QueryRowContext(ctx, `SELECT id FROM raw_booking_blocks WHERE property_id = ? AND source_event_uid = 'union-b'`, pid).Scan(&secondID); err != nil {
		t.Fatal(err)
	}
	stay, err := st.PromoteRawBookingBlockToNamedStay(ctx, pid, firstID, NamedStayCreateInput{
		DisplayName: "Union Guest", StayType: StayTypeBookingCom,
		CheckInDate: "2026-08-10", CheckOutDate: "2026-08-12",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := st.DB.ExecContext(ctx, `INSERT INTO stay_source_links (property_id, named_stay_id, raw_booking_block_id, source_type, source_event_uid, linked_check_in_date, linked_check_out_date, link_status, created_at, updated_at) VALUES (?, ?, ?, 'booking_ics', 'union-b', '2026-08-10', '2026-08-14', 'active', ?, ?)`, pid, stay.ID, secondID, now, now); err != nil {
		t.Fatal(err)
	}
	updated, err := st.UpdateNamedStayRecord(ctx, pid, stay.ID, NamedStayUpdateInput{
		CheckOutDate: ptrString("2026-08-14"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.CheckOutDate != "2026-08-14" {
		t.Fatalf("checkout=%s", updated.CheckOutDate)
	}
	var activeLinks int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM stay_source_links WHERE named_stay_id = ? AND link_status = 'active' AND conflict_reason IS NULL`, stay.ID).Scan(&activeLinks); err != nil || activeLinks != 2 {
		t.Fatalf("active links=%d err=%v", activeLinks, err)
	}
}

func TestNamedStayUpdate_RejectsGapInRawBlockUnion(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()
	if err := st.ReconcileBookingICSSync(ctx, pid, UpstreamSourceBookingICS, []DesiredBlock{
		block("gap-a", "2026-09-10", "2026-09-12"), block("gap-b", "2026-09-13", "2026-09-15"),
	}, dt("2026-07-01"), &SyncCounters{}); err != nil {
		t.Fatal(err)
	}
	var firstID, secondID int64
	if err := st.DB.QueryRowContext(ctx, `SELECT id FROM raw_booking_blocks WHERE property_id = ? AND source_event_uid = 'gap-a'`, pid).Scan(&firstID); err != nil {
		t.Fatal(err)
	}
	if err := st.DB.QueryRowContext(ctx, `SELECT id FROM raw_booking_blocks WHERE property_id = ? AND source_event_uid = 'gap-b'`, pid).Scan(&secondID); err != nil {
		t.Fatal(err)
	}
	stay, err := st.PromoteRawBookingBlockToNamedStay(ctx, pid, firstID, NamedStayCreateInput{
		DisplayName: "Gap Guest", StayType: StayTypeBookingCom,
		CheckInDate: "2026-09-10", CheckOutDate: "2026-09-12",
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := st.DB.ExecContext(ctx, `INSERT INTO stay_source_links (property_id, named_stay_id, raw_booking_block_id, source_type, source_event_uid, linked_check_in_date, linked_check_out_date, link_status, created_at, updated_at) VALUES (?, ?, ?, 'booking_ics', 'gap-b', '2026-09-10', '2026-09-15', 'active', ?, ?)`, pid, stay.ID, secondID, now, now); err != nil {
		t.Fatal(err)
	}
	_, err = st.UpdateNamedStayRecord(ctx, pid, stay.ID, NamedStayUpdateInput{CheckOutDate: ptrString("2026-09-15")})
	if !errors.Is(err, ErrNamedStayOutsideBlock) {
		t.Fatalf("gap update error=%v", err)
	}
}

func TestNamedStayLifecycleMetadata(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()
	stay, err := st.CreateNamedStayRecord(ctx, NamedStayCreateInput{
		PropertyID: pid, DisplayName: "Lifecycle Guest", StayType: StayTypeExternal,
		CheckInDate: "2026-10-10", CheckOutDate: "2026-10-12", CreatedByUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stay.FirstKnownAt.IsZero() {
		t.Fatal("first_known_at was not set on create")
	}

	outcome := StayOutcomeNoShow
	stay, err = st.UpdateNamedStayOutcome(ctx, pid, stay.ID, 1, &outcome, "  guest did not arrive  ")
	if err != nil {
		t.Fatal(err)
	}
	if stay.StayOutcome.String != StayOutcomeNoShow || stay.StayOutcomeReason.String != "guest did not arrive" ||
		stay.StayOutcomeActorUserID.Int64 != 1 || !stay.StayOutcomeMarkedAt.Valid {
		t.Fatalf("outcome metadata=%+v", stay)
	}

	stay, err = st.UpdateNamedStayReview(ctx, pid, stay.ID, 1, "rejected", "  duplicate evidence  ")
	if err != nil {
		t.Fatal(err)
	}
	if stay.ReviewStatus.String != "rejected" || stay.ReviewReason.String != "duplicate evidence" ||
		stay.ReviewActorUserID.Int64 != 1 || !stay.ReviewedAt.Valid {
		t.Fatalf("review metadata=%+v", stay)
	}
	var storedStatus string
	var resolution sql.NullString
	if err := st.DB.QueryRowContext(ctx, `SELECT review_status, review_resolution FROM named_stays WHERE id = ?`, stay.ID).Scan(&storedStatus, &resolution); err != nil {
		t.Fatal(err)
	}
	if storedStatus != "needs_review" || resolution.String != "rejected" {
		t.Fatalf("stored review state=%q/%v want needs_review/rejected", storedStatus, resolution)
	}

	stay, err = st.UpdateNamedStayStatus(ctx, pid, stay.ID, NamedStayStatusCancelled, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !stay.CancellationEffectiveAt.Valid {
		t.Fatal("cancellation_effective_at was not set")
	}
	cancelledAt := stay.CancellationEffectiveAt.Time
	stay, err = st.UpdateNamedStayStatus(ctx, pid, stay.ID, NamedStayStatusCancelled, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !stay.CancellationEffectiveAt.Time.Equal(cancelledAt) {
		t.Fatalf("repeated cancellation overwrote timestamp: got %s want %s", stay.CancellationEffectiveAt.Time, cancelledAt)
	}
	stay, err = st.UpdateNamedStayStatus(ctx, pid, stay.ID, NamedStayStatusActive, 1)
	if err != nil {
		t.Fatal(err)
	}
	if stay.CancellationEffectiveAt.Valid {
		t.Fatal("cancellation_effective_at was not cleared on reactivation")
	}

	stay, err = st.UpdateNamedStayOutcome(ctx, pid, stay.ID, 1, nil, "ignored")
	if err != nil {
		t.Fatal(err)
	}
	if stay.StayOutcome.Valid || stay.StayOutcomeReason.Valid || stay.StayOutcomeActorUserID.Valid || stay.StayOutcomeMarkedAt.Valid {
		t.Fatalf("outcome clear left metadata: %+v", stay)
	}
}

func TestNamedStayCRUDNeverWritesLegacyOccupanciesOrMigrationMap(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()
	rawID := stage4RawBlock(t, st, pid, "canonical-only", "2026-11-01", "2026-11-04")
	stay, err := st.PromoteRawBookingBlockToNamedStay(ctx, pid, rawID, NamedStayCreateInput{
		DisplayName: "Canonical Guest", StayType: StayTypeBookingCom,
		CheckInDate: "2026-11-01", CheckOutDate: "2026-11-04", CreatedByUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	name := "Canonical Guest Updated"
	if _, err := st.UpdateNamedStayRecord(ctx, pid, stay.ID, NamedStayUpdateInput{DisplayName: &name, UpdatedByUserID: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateNamedStayStatus(ctx, pid, stay.ID, NamedStayStatusCancelled, 1); err != nil {
		t.Fatal(err)
	}
	var legacyTables int
	if err := st.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sqlite_schema
		WHERE type = 'table' AND name IN ('occupancies', 'occupancy_stay_migration_map')`).Scan(&legacyTables); err != nil {
		t.Fatal(err)
	}
	if legacyTables != 0 {
		t.Fatalf("legacy occupancy tables=%d want 0", legacyTables)
	}
}
