package store

import (
	"context"
	"fmt"
	"testing"
)

func TestLateCheckoutLedgerIsIdempotentAndMovesWithCheckout(t *testing.T) {
	st, propertyID := recTestProperty(t)
	ctx := context.Background()
	cleaning := true
	late := true
	stay, err := st.CreateNamedStayRecord(ctx, NamedStayCreateInput{
		PropertyID: propertyID, DisplayName: "Late guest", StayType: StayTypeExternal,
		CheckInDate: "2026-10-30", CheckOutDate: "2026-11-02", CleaningRequired: &cleaning,
		LateCheckout: &late, CreatedByUserID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	assertLateCheckoutRows(t, st, propertyID, stay.ID, 2, "2026-11")
	if _, err := st.UpdateNamedStayRecord(ctx, propertyID, stay.ID, NamedStayUpdateInput{CheckOutDate: stringPtrForTest("2026-10-31"), UpdatedByUserID: 1}); err != nil {
		t.Fatal(err)
	}
	assertLateCheckoutRows(t, st, propertyID, stay.ID, 2, "2026-10")
	if _, err := st.UpdateNamedStayRecord(ctx, propertyID, stay.ID, NamedStayUpdateInput{LateCheckout: boolPtrForTest(false), UpdatedByUserID: 1}); err != nil {
		t.Fatal(err)
	}
	assertLateCheckoutRows(t, st, propertyID, stay.ID, 0, "")
}

func TestLateCheckoutCancellationRollsBackFinancialOwnership(t *testing.T) {
	st, propertyID := recTestProperty(t)
	ctx := context.Background()
	late := true
	stay, err := st.CreateNamedStayRecord(ctx, NamedStayCreateInput{PropertyID: propertyID, DisplayName: "No show", StayType: StayTypeBookingCom, CheckInDate: "2026-11-01", CheckOutDate: "2026-11-02", LateCheckout: &late, CreatedByUserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	outcome := StayOutcomeNoShow
	if _, err := st.UpdateNamedStayOutcome(ctx, propertyID, stay.ID, 1, &outcome, "guest absent"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := st.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM finance_transactions WHERE property_id = ? AND source_reference_id = ?`, propertyID, stayIDRefForTest(stay.ID)).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("late checkout rows after no-show = %d, want 0", count)
	}
}

func assertLateCheckoutRows(t *testing.T, st *Store, propertyID, stayID int64, want int, month string) {
	t.Helper()
	var count int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM finance_transactions WHERE property_id = ? AND source_reference_id = ?`, propertyID, stayIDRefForTest(stayID)).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("late checkout row count = %d, want %d", count, want)
	}
	if month != "" {
		var months int
		if err := st.DB.QueryRow(`SELECT COUNT(DISTINCT substr(transaction_date, 1, 7)) FROM finance_transactions WHERE property_id = ? AND source_reference_id = ? AND substr(transaction_date, 1, 7) = ?`, propertyID, stayIDRefForTest(stayID), month).Scan(&months); err != nil {
			t.Fatal(err)
		}
		if months != 1 {
			t.Fatalf("late checkout month rows = %d, want 1", months)
		}
	}
}

func stringPtrForTest(v string) *string { return &v }
func boolPtrForTest(v bool) *bool       { return &v }
func stayIDRefForTest(id int64) string  { return fmt.Sprintf("%d", id) }
