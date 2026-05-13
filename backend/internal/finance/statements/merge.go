package statements

import (
	"sort"
	"strings"
)

// CanonicalBooking is the in-memory shape of a finance_bookings row that
// the merger reads/writes. Pointer fields = "value not set"; the merger
// uses nil to mean "this column has never been populated and the new
// source can fill it" without confusion against zero-valued ints.
//
// The struct is intentionally decoupled from store.FinanceBookingPayout
// so the parser package has zero database awareness.
type CanonicalBooking struct {
	ReferenceNumber string
	SourceChannel   string

	HasPayoutData    bool
	HasStatementData bool

	// Optional canonical fields. Nil means "not populated".
	BookedOn            *string // RFC3339 UTC
	CheckInDate         *string
	CheckOutDate        *string
	GuestName           *string
	BookerName          *string
	GuestRequest        *string
	Status              *string // raw status (e.g. "OK", "CANCELLED")
	ReservationStatus   *string
	PaymentStatus       *string
	Currency            *string
	AmountCents         *int
	OriginalAmountCents *int
	CommissionCents     *int
	CommissionPct       *float64
	PaymentFeeCents     *int
	NetCents            *int
	Persons             *int
	Rooms               *int
	RoomNights          *int
	PayoutDate          *string // RFC3339 UTC
	PayoutID            *string
	RowType             *string
	InvoiceNumber       *string
	HotelID             *string
	PropertyLabel       *string
	Country             *string

	RawPayoutRowJSON    *string
	RawStatementRowJSON *string
}

// MergeAction is the outcome of merging one parsed Row into a booking.
type MergeAction string

const (
	ActionInsert    MergeAction = "insert"
	ActionUpdate    MergeAction = "update"
	ActionUnchanged MergeAction = "unchanged"
)

// MergeOutcome is the per-row diff produced by Merge.
type MergeOutcome struct {
	Action        MergeAction
	Changed       []string
	Result        CanonicalBooking
	StatusChanged bool // true when canonical Status flipped (used for cancellation reconciliation)
}

// Merge applies a parsed Row against the existing booking (or nil for
// new). It does not touch the database — it returns the merged
// CanonicalBooking which the caller persists.
//
// Precedence (per spec PMS_Statement_Ingestion_Spec):
//   - statement wins for: amount, commission, fee, dates, guest_name,
//     status, original_amount, persons, rooms, room_nights, booker_name,
//     guest_request, invoice_number, hotel_id, property_label, country,
//     commission_pct
//   - payout wins for: net_cents, payout_id, payout_date, row_type,
//     reservation_status, payment_status
//   - currency: first writer wins (never overwrite)
//   - any non-null value is never overwritten with NULL
func Merge(existing *CanonicalBooking, row Row) MergeOutcome {
	out := MergeOutcome{}
	var b CanonicalBooking
	if existing != nil {
		b = *existing
	} else {
		b.ReferenceNumber = row.ReferenceNumber
		b.SourceChannel = "booking_com"
	}
	if b.SourceChannel == "" {
		b.SourceChannel = "booking_com"
	}
	rawJSON := CanonicalRawJSON(row.Raw)

	switch row.Source {
	case SourcePayout:
		mergePayout(&b, row, rawJSON, &out)
	case SourceStatement:
		mergeStatement(&b, row, rawJSON, &out)
	}
	out.Result = b
	if existing == nil {
		out.Action = ActionInsert
		// Inserts always carry the canonical reference number as a
		// "set" change — but the audit log only cares about field
		// changes against an existing row. Leave Changed empty so the
		// caller can distinguish insert vs update by Action alone.
		out.Changed = nil
	} else if len(out.Changed) == 0 {
		out.Action = ActionUnchanged
	} else {
		out.Action = ActionUpdate
		sort.Strings(out.Changed)
	}
	return out
}

func mergePayout(b *CanonicalBooking, row Row, rawJSON string, out *MergeOutcome) {
	b.HasPayoutData = true
	// Payout-side authoritative.
	setStrIfWins(&b.PayoutID, valOrNil(row.PayoutID), "payout_id", true, out)
	if !row.PayoutDate.IsZero() {
		v := row.PayoutDate.Format("2006-01-02T15:04:05Z07:00")
		setStrIfWins(&b.PayoutDate, &v, "payout_date", true, out)
	}
	setStrIfWins(&b.RowType, valOrNil(row.RowType), "row_type", true, out)
	setStrIfWins(&b.ReservationStatus, valOrNil(row.ReservationStatus), "reservation_status", true, out)
	setStrIfWins(&b.PaymentStatus, valOrNil(row.PaymentStatus), "payment_status", true, out)
	setIntPtrIfWins(&b.NetCents, &row.NetCents, "net_cents", true, out)

	// Statement-priority fields: only fill if absent.
	setStrIfWins(&b.CheckInDate, valOrNil(row.CheckInDate), "check_in_date", false, out)
	setStrIfWins(&b.CheckOutDate, valOrNil(row.CheckOutDate), "check_out_date", false, out)
	setStrIfWins(&b.GuestName, valOrNil(row.GuestName), "guest_name", false, out)
	if row.AmountCents != 0 {
		setIntPtrIfWins(&b.AmountCents, &row.AmountCents, "amount_cents", false, out)
	}
	if row.CommissionCents != 0 {
		setIntPtrIfWins(&b.CommissionCents, &row.CommissionCents, "commission_cents", false, out)
	}
	if row.PaymentFeeCents != 0 {
		setIntPtrIfWins(&b.PaymentFeeCents, &row.PaymentFeeCents, "payment_service_fee_cents", false, out)
	}
	setStrIfWins(&b.Status, deriveStatusFromPayout(row), "status", false, out)
	setStrIfWins(&b.Currency, valOrNil(row.Currency), "currency", false, out)

	if rawJSON != "" {
		if b.RawPayoutRowJSON == nil || *b.RawPayoutRowJSON != rawJSON {
			b.RawPayoutRowJSON = &rawJSON
			// Raw blob change alone is not a canonical-field change;
			// don't add to Changed.
		}
	}
}

func mergeStatement(b *CanonicalBooking, row Row, rawJSON string, out *MergeOutcome) {
	b.HasStatementData = true
	if !row.BookedOn.IsZero() {
		v := row.BookedOn.Format("2006-01-02T15:04:05Z07:00")
		setStrIfWins(&b.BookedOn, &v, "booked_on", true, out)
	}
	setStrIfWins(&b.CheckInDate, valOrNil(row.CheckInDate), "check_in_date", true, out)
	setStrIfWins(&b.CheckOutDate, valOrNil(row.CheckOutDate), "check_out_date", true, out)
	setStrIfWins(&b.GuestName, valOrNil(row.GuestName), "guest_name", true, out)
	setStrIfWins(&b.BookerName, valOrNil(row.BookerName), "booker_name", true, out)
	setStrIfWins(&b.GuestRequest, valOrNil(row.GuestRequest), "guest_request", true, out)
	setStrIfWins(&b.InvoiceNumber, valOrNil(row.InvoiceNumber), "invoice_number", true, out)
	setStrIfWins(&b.HotelID, valOrNil(row.HotelID), "hotel_id", true, out)
	setStrIfWins(&b.PropertyLabel, valOrNil(row.PropertyLabel), "property_label", true, out)
	setStrIfWins(&b.Country, valOrNil(row.Country), "country", true, out)
	statusBefore := strDeref(b.Status)
	setStrIfWins(&b.Status, valOrNil(row.Status), "status", true, out)
	if !strings.EqualFold(statusBefore, strDeref(b.Status)) {
		out.StatusChanged = true
	}
	if row.AmountCents != 0 {
		setIntPtrIfWins(&b.AmountCents, &row.AmountCents, "amount_cents", true, out)
	}
	if row.OriginalAmountCents != 0 {
		setIntPtrIfWins(&b.OriginalAmountCents, &row.OriginalAmountCents, "original_amount_cents", true, out)
	}
	if row.CommissionCents != 0 {
		setIntPtrIfWins(&b.CommissionCents, &row.CommissionCents, "commission_cents", true, out)
	}
	if row.PaymentFeeCents != 0 {
		setIntPtrIfWins(&b.PaymentFeeCents, &row.PaymentFeeCents, "payment_service_fee_cents", true, out)
	}
	if row.CommissionPct != 0 {
		v := row.CommissionPct
		setFloatPtrIfWins(&b.CommissionPct, &v, "commission_pct", true, out)
	}
	if row.Persons != 0 {
		setIntPtrIfWins(&b.Persons, &row.Persons, "persons", true, out)
	}
	if row.Rooms != 0 {
		setIntPtrIfWins(&b.Rooms, &row.Rooms, "rooms", true, out)
	}
	if row.RoomNights != 0 {
		setIntPtrIfWins(&b.RoomNights, &row.RoomNights, "room_nights", true, out)
	}
	setStrIfWins(&b.Currency, valOrNil(row.Currency), "currency", false, out)

	if rawJSON != "" {
		if b.RawStatementRowJSON == nil || *b.RawStatementRowJSON != rawJSON {
			b.RawStatementRowJSON = &rawJSON
		}
	}
}

func deriveStatusFromPayout(row Row) *string {
	s := strings.TrimSpace(row.ReservationStatus)
	if s == "" {
		return nil
	}
	v := strings.ToUpper(s)
	return &v
}

// setStrIfWins applies precedence rules: when wins=true the new value
// overwrites a non-equal existing value; when wins=false it only fills
// an empty slot. Never overwrites with NULL.
func setStrIfWins(target **string, incoming *string, field string, wins bool, out *MergeOutcome) {
	if incoming == nil || strings.TrimSpace(*incoming) == "" {
		return
	}
	v := strings.TrimSpace(*incoming)
	if *target == nil {
		copy := v
		*target = &copy
		out.Changed = append(out.Changed, field)
		return
	}
	if !wins {
		return
	}
	if *(*target) == v {
		return
	}
	copy := v
	*target = &copy
	out.Changed = append(out.Changed, field)
}

func setIntPtrIfWins(target **int, incoming *int, field string, wins bool, out *MergeOutcome) {
	if incoming == nil {
		return
	}
	if *target == nil {
		v := *incoming
		*target = &v
		out.Changed = append(out.Changed, field)
		return
	}
	if !wins || *(*target) == *incoming {
		return
	}
	v := *incoming
	*target = &v
	out.Changed = append(out.Changed, field)
}

func setFloatPtrIfWins(target **float64, incoming *float64, field string, wins bool, out *MergeOutcome) {
	if incoming == nil {
		return
	}
	if *target == nil {
		v := *incoming
		*target = &v
		out.Changed = append(out.Changed, field)
		return
	}
	if !wins || *(*target) == *incoming {
		return
	}
	v := *incoming
	*target = &v
	out.Changed = append(out.Changed, field)
}

func valOrNil(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return &s
}

func strDeref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
