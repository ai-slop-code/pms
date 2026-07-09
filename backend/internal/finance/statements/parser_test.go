package statements

import (
	"strings"
	"testing"
	"time"
)

const payoutCSV = `"Reference number","Type","Guest name","Check-in","Checkout","Amount","Commission","Payments service fee","Net","Currency","Payout date","Payout ID","Payment status","Reservation status"
"1234567890","reservation","Test Guest","1 Sept 2026","3 Sept 2026","250.00","-37.50","-3.50","209.00","EUR","5 Sept 2026","PO-123","paid","OK"`

const payoutBookingNumberCSV = `"Booking number","Type","Guest name","Check-in","Checkout","Amount","Commission","Payments service fee","Net","Currency","Payout date","Payout ID","Payment status","Reservation status"
"6756848168","reservation","Synthetic Guest","10 May 2026","12 May 2026","180.00","-27.00","-2.00","151.00","EUR","15 May 2026","PO-456","paid","OK"`

const statementCSV = `"Reservation number","Invoice number","Booked on","Arrival","Departure","Booker name","Guest name","Rooms","Persons","Room nights","Commission %","Original amount","Final amount","Commission amount","Payment fee","Status","Guest request","Currency","Hotel id","Property name","City","Country"
"1234567890","INV-1","2026-08-20T14:30:00","2026-09-01","2026-09-03","Booker","Test Guest","1","2","2","15.00","250.00","250.00","37.50","3.50","OK","Late arrival","EUR","HOTEL-1","Synthetic Property","Bratislava","SK"`

// loc returns a deterministic property timezone for tests.
func testLoc(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Bratislava")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	return loc
}

func TestParsePayout_HappyPath(t *testing.T) {
	body := payoutCSV
	res, err := DetectAndParse(strings.NewReader(body), testLoc(t))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if res.Source != SourcePayout {
		t.Fatalf("source = %v, want payout", res.Source)
	}
	if len(res.Rows) == 0 {
		t.Fatal("no rows parsed")
	}
	row := res.Rows[0]
	if row.ReferenceNumber == "" {
		t.Fatal("missing reference")
	}
	if row.NetCents == 0 {
		t.Fatal("missing net cents")
	}
	if row.CommissionCents < 0 || row.PaymentFeeCents < 0 {
		t.Fatalf("commission/fee not abs: %d %d", row.CommissionCents, row.PaymentFeeCents)
	}
	if row.PayoutDate.IsZero() {
		t.Fatal("missing payout date")
	}
	if row.Currency != "EUR" {
		t.Fatalf("currency = %q", row.Currency)
	}
}

func TestParsePayout_BookingNumberColumn(t *testing.T) {
	// Booking.com renamed "Reference number" → "Booking number" in 2026 exports.
	body := payoutBookingNumberCSV
	res, err := DetectAndParse(strings.NewReader(body), testLoc(t))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if res.Source != SourcePayout {
		t.Fatalf("source = %v, want payout", res.Source)
	}
	if len(res.Rows) == 0 {
		t.Fatal("no rows parsed")
	}
	if res.Rows[0].ReferenceNumber != "6756848168" {
		t.Fatalf("reference = %q, want 6756848168", res.Rows[0].ReferenceNumber)
	}
}

func TestParseStatement_HappyPath(t *testing.T) {
	body := statementCSV
	res, err := DetectAndParse(strings.NewReader(body), testLoc(t))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if res.Source != SourceStatement {
		t.Fatalf("source = %v, want statement", res.Source)
	}
	if len(res.Rows) == 0 {
		t.Fatal("no rows")
	}
	row := res.Rows[0]
	if row.HotelID == "" {
		t.Fatal("missing hotel id")
	}
	if row.CheckInDate == "" || row.CheckOutDate == "" {
		t.Fatal("missing dates")
	}
	if row.RoomNights <= 0 {
		t.Fatalf("room nights = %d", row.RoomNights)
	}
	if row.AmountCents <= 0 {
		t.Fatalf("amount cents = %d", row.AmountCents)
	}
}

func TestParser_RejectsNonEUR(t *testing.T) {
	body := `"Reservation number","Invoice number","Booked on","Arrival","Departure","Booker name","Guest name","Rooms","Persons","Room nights","Commission %","Original amount","Final amount","Commission amount","Payment fee","Status","Guest request","Currency","Hotel id","Property name","City","Country"
"X1","I1","2025-12-16T23:41:28","2025-12-31","2026-01-01","B","G","1","2","1","20.00","100.00","100.00","20.00","1.00","OK","","USD","H","P","C","SK"`
	res, err := DetectAndParse(strings.NewReader(body), testLoc(t))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(res.Rejected) == 0 {
		t.Fatal("expected USD row to be rejected")
	}
	if len(res.Rows) != 0 {
		t.Fatalf("expected 0 accepted rows, got %d", len(res.Rows))
	}
}

func TestCanonicalRawJSON_Stable(t *testing.T) {
	a := CanonicalRawJSON(map[string]string{"b": "1", "a": "2"})
	b := CanonicalRawJSON(map[string]string{"a": "2", "b": "1"})
	if a != b {
		t.Fatalf("not stable: %q vs %q", a, b)
	}
}
