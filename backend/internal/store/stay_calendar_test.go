package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestOccupancyCalendarViewStage5CombinesRawNamedAndAvailability(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()
	rawID := stage4RawBlock(t, st, pid, "stage5-raw@booking.com", "2026-07-09", "2026-07-12")
	now := time.Now().UTC().Format(time.RFC3339)
	dupRes, err := st.DB.ExecContext(ctx, `
		INSERT INTO raw_booking_blocks (
			property_id, source_type, source_event_uid, check_in_date, check_out_date, status,
			raw_summary, content_hash, imported_at, last_synced_at, created_at, updated_at
		)
		VALUES (?, 'booking_ics', 'stage5-duplicate@booking.com', '2026-07-10', '2026-07-11', 'active', 'Duplicate raw', 'dup-hash', ?, ?, ?, ?)`, pid, now, now, now, now)
	if err != nil {
		t.Fatal(err)
	}
	dupRawID, err := dupRes.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx, `
		INSERT INTO raw_booking_block_nights (property_id, raw_booking_block_id, local_night_date, active, created_at, updated_at)
		VALUES (?, ?, '2026-07-10', 1, ?, ?)`, pid, dupRawID, now, now); err != nil {
		t.Fatal(err)
	}

	stay, err := st.PromoteRawBookingBlockToNamedStay(ctx, pid, rawID, NamedStayCreateInput{
		DisplayName:     "Stage Five Guest",
		StayType:        StayTypeBookingCom,
		CheckInDate:     "2026-07-10",
		CheckOutDate:    "2026-07-12",
		CreatedByUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx, `
		INSERT INTO finance_bookings (
			property_id, reference_number, check_in_date, check_out_date, guest_name,
			net_cents, payout_date, named_stay_id, created_at, updated_at,
			source_channel, has_payout_data, has_statement_data
		) VALUES (?, 'CAL-FINANCE', '2026-07-10', '2026-07-12', 'Stage Five Guest',
			10000, '2026-07-13', ?, ?, ?, 'booking_com', 1, 1)`, pid, stay.ID, now, now); err != nil {
		t.Fatal(err)
	}
	if err := st.MarkNamedStayNukiGeneration(ctx, pid, stay.ID, NukiGenerationError, "nuki_credentials_not_configured"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx, `
		INSERT INTO cleaning_calendar_events (
			property_id, occupancy_id, named_stay_id, checkout_date, cleaning_kind, google_calendar_id,
			cleaning_date, starts_at, ends_at, title, status, error_message, created_at, updated_at
		)
		VALUES (?, ?, ?, '2026-07-12', 'named_stay', 'calendar-id', '2026-07-12', ?, ?, 'Upratovanie: Stage Five Guest', 'error', 'google failed', ?, ?)`, pid, stay.LegacyOccupancyID.Int64, stay.ID, now, now, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.ExecContext(ctx, `
		INSERT INTO property_availability_blocks (property_id, block_type, start_date, end_date, reason, status, created_at, updated_at)
		VALUES (?, 'closed', '2026-07-20', '2026-07-22', 'owner repair', 'active', ?, ?)`, pid, now, now); err != nil {
		t.Fatal(err)
	}

	view, err := st.OccupancyCalendarView(ctx, pid, "2026-07")
	if err != nil {
		t.Fatal(err)
	}
	if view.PropertyID != pid || view.Month != "2026-07" {
		t.Fatalf("bad view identity: %#v", view)
	}
	if len(view.RawBlocks) != 2 {
		t.Fatalf("raw blocks=%d want 2", len(view.RawBlocks))
	}
	if len(view.NamedStays) != 1 {
		t.Fatalf("named stays=%d want 1", len(view.NamedStays))
	}
	if len(view.AvailabilityBlocks) != 1 {
		t.Fatalf("availability blocks=%d want 1", len(view.AvailabilityBlocks))
	}

	foundRaw := false
	for _, b := range view.RawBlocks {
		if b.ID == rawID {
			foundRaw = true
			if len(b.CoveredNights) != 3 {
				t.Fatalf("raw covered nights=%v want 3 nights", b.CoveredNights)
			}
		}
	}
	if !foundRaw {
		t.Fatal("promoted raw block missing from calendar")
	}

	calStay := view.NamedStays[0]
	if calStay.DisplayName != "Stage Five Guest" || calStay.StayType != StayTypeBookingCom {
		t.Fatalf("bad named stay: %#v", calStay)
	}
	if !calStay.CountsAsSold {
		t.Fatal("confirmed Booking.com stay must count as sold")
	}
	if !calStay.HasFinanceEvidence {
		t.Fatal("payout/statement-backed stay did not expose finance evidence")
	}
	if len(calStay.CoveredNights) != 2 {
		t.Fatalf("named covered nights=%v want 2 nights", calStay.CoveredNights)
	}
	if calStay.NukiGenerationStatus != NukiGenerationError || calStay.NukiGenerationError == nil || *calStay.NukiGenerationError != "nuki_credentials_not_configured" {
		t.Fatalf("bad nuki badge state: %#v", calStay)
	}
	if len(calStay.CleaningEvents) != 1 || calStay.CleaningEvents[0].Status != CleaningCalendarStatusError {
		t.Fatalf("bad cleaning status: %#v", calStay.CleaningEvents)
	}
	if len(calStay.SourceLinks) != 1 || calStay.SourceLinks[0].LinkStatus != "active" {
		t.Fatalf("bad source links: %#v", calStay.SourceLinks)
	}

	block := view.AvailabilityBlocks[0]
	if block.BlockType != "closed" || block.Reason == nil || *block.Reason != "owner repair" {
		t.Fatalf("bad availability block: %#v", block)
	}
	if len(block.CoveredNights) != 2 || block.CoveredNights[0] != "2026-07-20" || block.CoveredNights[1] != "2026-07-21" {
		t.Fatalf("bad availability covered nights: %#v", block.CoveredNights)
	}
}

func TestCalendarNamedStayCountsAsSoldMatchesAnalyticsRules(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()
	unfunded, err := st.CreateNamedStayRecord(ctx, NamedStayCreateInput{
		PropertyID: pid, DisplayName: "Unfunded External", StayType: StayTypeExternal,
		CheckInDate: "2026-11-01", CheckOutDate: "2026-11-02",
	})
	if err != nil {
		t.Fatal(err)
	}
	funded, err := st.CreateNamedStayRecord(ctx, NamedStayCreateInput{
		PropertyID: pid, DisplayName: "Funded External", StayType: StayTypeExternal,
		CheckInDate: "2026-11-03", CheckOutDate: "2026-11-04",
	})
	if err != nil {
		t.Fatal(err)
	}
	revenue := int64(12000)
	currency := "EUR"
	if _, err := st.UpdateNamedStayRecord(ctx, pid, funded.ID, NamedStayUpdateInput{ManualRevenueCents: &revenue, ManualRevenueCurrency: &currency}); err != nil {
		t.Fatal(err)
	}
	review, err := st.CreateNamedStayRecord(ctx, NamedStayCreateInput{
		PropertyID: pid, DisplayName: "Review Booking", StayType: StayTypeBookingCom,
		CheckInDate: "2026-11-05", CheckOutDate: "2026-11-06", ReviewStatus: "needs_review",
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := st.ListCalendarNamedStays(ctx, pid, "2026-11-01", "2026-12-01")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[int64]CalendarNamedStay{}
	for _, row := range rows {
		byID[row.ID] = row
	}
	if byID[unfunded.ID].CountsAsSold {
		t.Fatal("unfunded external stay counted as sold")
	}
	if !byID[funded.ID].CountsAsSold {
		t.Fatal("funded external stay did not count as sold")
	}
	if byID[review.ID].CountsAsSold {
		t.Fatal("review-required stay counted as sold")
	}
	if byID[review.ID].HasFinanceEvidence {
		t.Fatal("review-required stay unexpectedly reported finance evidence")
	}
}

func TestAvailabilityBlockStage5CreateRejectsNamedStayOverlap(t *testing.T) {
	st, pid := recTestProperty(t)
	ctx := context.Background()
	if _, err := st.CreateNamedStayRecord(ctx, NamedStayCreateInput{
		PropertyID:      pid,
		DisplayName:     "Existing Guest",
		StayType:        StayTypeExternal,
		CheckInDate:     "2026-10-03",
		CheckOutDate:    "2026-10-05",
		CreatedByUserID: 1,
	}); err != nil {
		t.Fatal(err)
	}
	_, err := st.CreateAvailabilityBlock(ctx, pid, AvailabilityBlockInput{
		BlockType:    "closed",
		StartDate:    "2026-10-04",
		EndDate:      "2026-10-06",
		Reason:       "Repair",
		ActingUserID: 1,
	})
	if !errors.Is(err, ErrNamedStayOverlap) {
		t.Fatalf("err=%v want %v", err, ErrNamedStayOverlap)
	}
	block, err := st.CreateAvailabilityBlock(ctx, pid, AvailabilityBlockInput{
		BlockType:    "off_market",
		StartDate:    "2026-10-05",
		EndDate:      "2026-10-06",
		Reason:       "Owner repair",
		ActingUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if block.BlockType != "off_market" || block.Reason == nil || *block.Reason != "Owner repair" {
		t.Fatalf("bad block: %#v", block)
	}
}

func TestOccupancyCalendarViewStage5RejectsInvalidMonth(t *testing.T) {
	st, pid := recTestProperty(t)
	_, err := st.OccupancyCalendarView(context.Background(), pid, "2026-7")
	if err == nil {
		t.Fatal("expected invalid month error")
	}
	if !errors.Is(err, ErrNamedStayInvalidRange) {
		t.Fatalf("err=%v want invalid range", err)
	}
}
