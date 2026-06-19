package statements

import (
	"testing"
	"time"
)

func mustParse(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

func mkPayoutRow(ref, status string, net int) Row {
	return Row{
		Source:            SourcePayout,
		ReferenceNumber:   ref,
		NetCents:          net,
		Currency:          "EUR",
		AmountCents:       10000,
		CommissionCents:   1500,
		PaymentFeeCents:   100,
		ReservationStatus: status,
		PaymentStatus:     "by_booking",
		RowType:           "Reservation",
		Raw:               map[string]string{"reference": ref},
	}
}

func mkStatementRow(ref, status string) Row {
	return Row{
		Source:            SourceStatement,
		ReferenceNumber:   ref,
		AmountCents:       10000,
		CommissionCents:   1500,
		PaymentFeeCents:   100,
		Currency:          "EUR",
		Status:            status,
		Persons:           2,
		Rooms:             1,
		RoomNights:        1,
		HotelID:           "13452548",
		BookedOn:          mustParse("2025-12-16T23:41:28Z"),
		CheckInDate:       "2025-12-31",
		CheckOutDate:      "2026-01-01",
		Raw:               map[string]string{"reference": ref, "status": status},
	}
}

func TestMerge_PayoutInsertsNew(t *testing.T) {
	o := Merge(nil, mkPayoutRow("X1", "ok", 5000))
	if o.Action != ActionInsert {
		t.Fatalf("action = %v", o.Action)
	}
	if !o.Result.HasPayoutData {
		t.Fatal("expected has_payout_data true")
	}
	if o.Result.HasStatementData {
		t.Fatal("expected has_statement_data false")
	}
	if o.Result.NetCents == nil || *o.Result.NetCents != 5000 {
		t.Fatalf("net = %v", o.Result.NetCents)
	}
}

func TestMerge_StatementThenPayout(t *testing.T) {
	first := Merge(nil, mkStatementRow("X1", "OK"))
	if first.Action != ActionInsert || !first.Result.HasStatementData {
		t.Fatalf("first action=%v stmt=%v", first.Action, first.Result.HasStatementData)
	}
	cb := first.Result
	second := Merge(&cb, mkPayoutRow("X1", "ok", 7000))
	if second.Action != ActionUpdate {
		t.Fatalf("second action = %v", second.Action)
	}
	if !second.Result.HasPayoutData || !second.Result.HasStatementData {
		t.Fatalf("flags: payout=%v stmt=%v", second.Result.HasPayoutData, second.Result.HasStatementData)
	}
	if second.Result.NetCents == nil || *second.Result.NetCents != 7000 {
		t.Fatalf("net = %v", second.Result.NetCents)
	}
}

func TestMerge_PayoutThenStatement(t *testing.T) {
	first := Merge(nil, mkPayoutRow("X1", "ok", 7000))
	cb := first.Result
	second := Merge(&cb, mkStatementRow("X1", "OK"))
	if second.Action != ActionUpdate {
		t.Fatalf("action = %v", second.Action)
	}
	// Payout-owned columns must survive.
	if second.Result.NetCents == nil || *second.Result.NetCents != 7000 {
		t.Fatalf("net dropped: %v", second.Result.NetCents)
	}
	if second.Result.AmountCents == nil || *second.Result.AmountCents != 10000 {
		t.Fatalf("amount_cents overwritten by statement: %v", second.Result.AmountCents)
	}
	// Statement-owned column must be filled.
	if second.Result.HotelID == nil || *second.Result.HotelID != "13452548" {
		t.Fatalf("hotel id missing: %v", second.Result.HotelID)
	}
}

func TestMerge_StatementThenPayout_AmountFromPayout(t *testing.T) {
	stmt := mkStatementRow("X1", "OK")
	stmt.AmountCents = 6598 // statement Original/Final amount
	first := Merge(nil, stmt)
	cb := first.Result
	payout := mkPayoutRow("X1", "ok", 7000)
	payout.AmountCents = 6489 // payout CSV Amount (guest paid)
	second := Merge(&cb, payout)
	if second.Result.AmountCents == nil || *second.Result.AmountCents != 6489 {
		t.Fatalf("amount_cents = %v, want 6489 from payout", second.Result.AmountCents)
	}
}

func TestMerge_CancelAfterPayout(t *testing.T) {
	first := Merge(nil, mkPayoutRow("X1", "ok", 7000))
	cb := first.Result
	cancel := Merge(&cb, mkStatementRow("X1", "CANCELLED"))
	if !cancel.StatusChanged {
		t.Fatal("expected status change flag")
	}
	if cancel.Result.Status == nil || *cancel.Result.Status != "CANCELLED" {
		t.Fatalf("status = %v", cancel.Result.Status)
	}
}

func TestMerge_PayoutAfterCancelDoesNotOverwrite(t *testing.T) {
	first := Merge(nil, mkStatementRow("X1", "CANCELLED"))
	cb := first.Result
	second := Merge(&cb, mkPayoutRow("X1", "ok", 7000))
	if second.StatusChanged {
		t.Fatal("payout must not flip status away from CANCELLED")
	}
	if second.Result.Status == nil || *second.Result.Status != "CANCELLED" {
		t.Fatalf("status = %v", second.Result.Status)
	}
}

func TestMerge_IdempotentReupload(t *testing.T) {
	first := Merge(nil, mkPayoutRow("X1", "ok", 7000))
	cb := first.Result
	second := Merge(&cb, mkPayoutRow("X1", "ok", 7000))
	if second.Action != ActionUnchanged {
		t.Fatalf("action = %v want unchanged", second.Action)
	}
}
