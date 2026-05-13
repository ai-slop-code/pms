package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

// TestOccupancyClosureLifecycle exercises the close → reopen → external_sale →
// reopen lifecycle and confirms the labelled rows are excluded from
// ListActiveOccupanciesInDateRange (closed) but kept (external_sale) per
// PMS_14 §4.
func TestOccupancyClosureLifecycle(t *testing.T) {
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

	start := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)
	occ := &Occupancy{
		PropertyID:     p.ID,
		SourceType:     "booking_ics",
		SourceEventUID: "uid-A",
		StartAt:        start,
		EndAt:          end,
		Status:         "active",
		RawSummary:     sql.NullString{String: "Booking Block", Valid: true},
		ContentHash:    "h",
	}
	if err := st.UpsertOccupancy(ctx, occ, runID); err != nil {
		t.Fatal(err)
	}
	row, err := st.GetOccupancyBySourceEventUID(ctx, p.ID, "uid-A")
	if err != nil || row == nil {
		t.Fatalf("get after upsert: %v", err)
	}
	occID := row.ID

	// 1. Close the row.
	if err := st.CloseOccupancy(ctx, p.ID, occID, u.ID, "owner needs unit", "owner_stay"); err != nil {
		t.Fatalf("close: %v", err)
	}
	row, err = st.GetOccupancyByID(ctx, p.ID, occID)
	if err != nil {
		t.Fatal(err)
	}
	if got := row.ClosureState.String; got != "closed" {
		t.Fatalf("closure_state = %q, want closed", got)
	}
	if !row.ClosedByUserID.Valid || row.ClosedByUserID.Int64 != u.ID {
		t.Fatalf("closed_by_user_id = %v", row.ClosedByUserID)
	}
	if !row.ClosedAt.Valid {
		t.Fatalf("closed_at not set")
	}

	// 2. Closing again must fail with ErrOccupancyAlreadyLabelled.
	err = st.CloseOccupancy(ctx, p.ID, occID, u.ID, "x", "")
	if !errors.Is(err, ErrOccupancyAlreadyLabelled) {
		t.Fatalf("re-close err = %v, want ErrOccupancyAlreadyLabelled", err)
	}

	// 3. Closed rows are excluded from active analytics range.
	from := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	stays, err := st.ListActiveOccupanciesInDateRange(ctx, p.ID, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(stays) != 0 {
		t.Fatalf("active stays after close = %d, want 0", len(stays))
	}

	// 4. Reopen.
	if err := st.ReopenOccupancy(ctx, p.ID, occID); err != nil {
		t.Fatalf("reopen: %v", err)
	}
	row, _ = st.GetOccupancyByID(ctx, p.ID, occID)
	if row.ClosureState.Valid {
		t.Fatalf("closure_state still set after reopen: %q", row.ClosureState.String)
	}

	// 5. Mark as externally sold (12 000 cents = 120.00 EUR).
	if err := st.MarkOccupancyExternalSale(ctx, p.ID, occID, u.ID, 12000, "EUR", "airbnb", "Airbnb walk-in"); err != nil {
		t.Fatalf("external sale: %v", err)
	}
	row, _ = st.GetOccupancyByID(ctx, p.ID, occID)
	if got := row.ClosureState.String; got != "external_sale" {
		t.Fatalf("closure_state = %q, want external_sale", got)
	}
	if got := row.ExternalNetAmountCents.Int64; got != 12000 {
		t.Fatalf("external_net_amount_cents = %d, want 12000", got)
	}

	// 6. Externally-sold rows stay in the active analytics set.
	stays, err = st.ListActiveOccupanciesInDateRange(ctx, p.ID, from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(stays) != 1 {
		t.Fatalf("active stays after external_sale = %d, want 1", len(stays))
	}
	if stays[0].ClosureState != "external_sale" {
		t.Fatalf("loaded ClosureState = %q", stays[0].ClosureState)
	}
	if stays[0].ExternalNetAmountCents != 12000 {
		t.Fatalf("loaded ExternalNetAmountCents = %d", stays[0].ExternalNetAmountCents)
	}

	// 7. ExternalSaleRevenueCentsInRange prorates correctly when the range
	//    overlaps the whole stay.
	got := ExternalSaleRevenueCentsInRange(stays, from, to)
	if got != 12000 {
		t.Fatalf("ExternalSaleRevenueCentsInRange full overlap = %d, want 12000", got)
	}

	// 8. Half-overlap (only 1 of 2 nights) → half the amount.
	half := ExternalSaleRevenueCentsInRange(stays,
		time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC))
	if half != 6000 {
		t.Fatalf("ExternalSaleRevenueCentsInRange half overlap = %d, want 6000", half)
	}

	// 9. Reopen clears external-sale fields.
	if err := st.ReopenOccupancy(ctx, p.ID, occID); err != nil {
		t.Fatal(err)
	}
	row, _ = st.GetOccupancyByID(ctx, p.ID, occID)
	if row.ClosureState.Valid || row.ExternalNetAmountCents.Valid {
		t.Fatalf("reopen did not clear external-sale fields: state=%v amount=%v",
			row.ClosureState, row.ExternalNetAmountCents)
	}

	// 10. Reopen on an unlabelled row returns sql.ErrNoRows.
	if err := st.ReopenOccupancy(ctx, p.ID, occID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("reopen unlabelled err = %v, want sql.ErrNoRows", err)
	}
}

// TestOccupancyClosure_UpsertPreservesLabel verifies that a follow-up
// ICS resync (UpsertOccupancy on a row that was manually closed) does
// NOT clear the closure label. This is the key persistence guarantee
// from PMS_14 §3.5.
func TestOccupancyClosure_UpsertPreservesLabel(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u, _ := st.CreateUser(ctx, "x@test.local", "h", "owner")
	p, _ := st.CreateProperty(ctx, u.ID, "P", "UTC", "en")
	runID, _ := st.StartOccupancySyncRun(ctx, p.ID, "manual")

	occ := &Occupancy{
		PropertyID:     p.ID,
		SourceType:     "booking_ics",
		SourceEventUID: "uid-Z",
		StartAt:        time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		EndAt:          time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC),
		Status:         "active",
		ContentHash:    "h1",
	}
	if err := st.UpsertOccupancy(ctx, occ, runID); err != nil {
		t.Fatal(err)
	}
	row, _ := st.GetOccupancyBySourceEventUID(ctx, p.ID, "uid-Z")

	if err := st.CloseOccupancy(ctx, p.ID, row.ID, u.ID, "soft block", "soft_block"); err != nil {
		t.Fatal(err)
	}

	// Resync: ICS feed sends the same row again with a refreshed hash.
	occ.ContentHash = "h2"
	if err := st.UpsertOccupancy(ctx, occ, runID); err != nil {
		t.Fatal(err)
	}
	row, _ = st.GetOccupancyByID(ctx, p.ID, row.ID)
	if !row.ClosureState.Valid || row.ClosureState.String != "closed" {
		t.Fatalf("closure_state lost after resync: %v", row.ClosureState)
	}
	if !row.ClosureCategory.Valid || row.ClosureCategory.String != "soft_block" {
		t.Fatalf("closure_category lost after resync: %v", row.ClosureCategory)
	}
}
