// Package statements parses Booking.com Payout Info and Statement CSV
// exports into a canonical Row struct used by the finance ingestion
// pipeline. The package is HTTP-unaware — it only deals with bytes in
// and structured rows out, so the same parser is reusable from CLI
// tools or future channels (Airbnb/direct).
//
// File detection is header-signature based:
//   - "Payout date" + "Payout ID"     → Payout Info (cash basis)
//   - "Booked on"  + "Persons" + "Status" → Statement (accrual basis)
//
// Sign normalisation: commission and payment service fee are always
// stored as positive cents on output, regardless of how the source
// expresses them. Currency must be EUR; rows in other currencies are
// returned via the Rejected list, never silently dropped.
package statements

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// SourceType identifies which Booking.com export a row came from.
type SourceType string

const (
	SourcePayout    SourceType = "payout"
	SourceStatement SourceType = "statement"
)

// Row is the canonical representation of one parsed CSV record. Fields
// not provided by the source are left zero — the merger uses
// the source flag to decide which values are authoritative.
type Row struct {
	Source SourceType

	// Merge key.
	ReferenceNumber string

	// Statement-only.
	BookedOn            time.Time // UTC; zero when missing.
	InvoiceNumber       string
	HotelID             string
	PropertyLabel       string
	Country             string
	BookerName          string
	GuestRequest        string
	Persons             int
	Rooms               int
	RoomNights          int
	OriginalAmountCents int
	CommissionPct       float64

	// Provided by both sources (precedence applied at merge time).
	CheckInDate     string // "2006-01-02" in property TZ.
	CheckOutDate    string
	GuestName       string
	Currency        string
	AmountCents     int // gross amount (positive)
	CommissionCents int // positive
	PaymentFeeCents int // positive
	Status          string

	// Payout-only.
	NetCents          int
	PayoutDate        time.Time // UTC; zero when missing.
	PayoutID          string
	RowType           string
	PaymentStatus     string
	ReservationStatus string

	// Raw row used for audit + idempotence (stable-key serialised by
	// the caller).
	Raw map[string]string
}

// Rejection is one CSV row that failed validation. Rejected rows are
// surfaced in the preview UI without aborting the whole upload.
type Rejection struct {
	Line   int    `json:"line"`
	Reason string `json:"reason"`
}

// ParseResult is the structured outcome of a parse.
type ParseResult struct {
	Source   SourceType  `json:"source"`
	Rows     []Row       `json:"rows"`
	Warnings []string    `json:"warnings,omitempty"`
	Rejected []Rejection `json:"rejected,omitempty"`
}

// ErrUnknownFormat is returned when neither header signature matches.
var ErrUnknownFormat = errors.New("unknown CSV format: expected Booking.com Payout Info or Statement headers")

// DetectAndParse reads the entire CSV from in, detects its format from
// the header signature, and returns parsed rows plus any rejections.
// loc is the property timezone used to anchor "Booked on" datetimes
// and "Payout date" / arrival / departure dates before they are stored
// as UTC.
func DetectAndParse(in io.Reader, loc *time.Location) (*ParseResult, error) {
	if loc == nil {
		loc = time.UTC
	}
	raw, err := io.ReadAll(in)
	if err != nil {
		return nil, fmt.Errorf("read csv: %w", err)
	}
	// Strip optional UTF-8 BOM, then try cp1252 fallback when the
	// payload doesn't decode as valid UTF-8 (Booking payout exports
	// occasionally arrive Windows-encoded).
	raw = bytes.TrimPrefix(raw, []byte{0xEF, 0xBB, 0xBF})
	if !utf8.Valid(raw) {
		dec := charmap.Windows1252.NewDecoder()
		decoded, derr := dec.Bytes(raw)
		if derr == nil && utf8.Valid(decoded) {
			raw = decoded
		}
	}
	r := csv.NewReader(bytes.NewReader(raw))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("invalid csv header: %w", err)
	}
	idx := map[string]int{}
	for i, h := range header {
		key := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")))
		idx[key] = i
	}
	switch detectFormat(idx) {
	case SourcePayout:
		return parsePayout(r, idx, loc)
	case SourceStatement:
		return parseStatement(r, idx, loc)
	default:
		return nil, ErrUnknownFormat
	}
}

func detectFormat(idx map[string]int) SourceType {
	hasPayout := has(idx, "payout date") && has(idx, "payout id")
	hasStatement := has(idx, "booked on") && has(idx, "persons") && has(idx, "status")
	switch {
	case hasStatement:
		return SourceStatement
	case hasPayout:
		return SourcePayout
	default:
		return ""
	}
}

func has(idx map[string]int, key string) bool { _, ok := idx[key]; return ok }

// aliasPayoutReferenceColumn maps Booking.com payout header variants onto the
// canonical "reference number" key (legacy "Reference number", typo
// "Refference number", 2026+ "Booking number").
func aliasPayoutReferenceColumn(idx map[string]int) {
	if has(idx, "reference number") {
		return
	}
	for _, alias := range []string{"refference number", "booking number"} {
		if i, ok := idx[alias]; ok {
			idx["reference number"] = i
			return
		}
	}
}

// ---------------- Payout parser ----------------

func parsePayout(r *csv.Reader, idx map[string]int, loc *time.Location) (*ParseResult, error) {
	aliasPayoutReferenceColumn(idx)
	for _, k := range []string{"reference number", "net", "payout date"} {
		if !has(idx, k) {
			return nil, fmt.Errorf("payout csv missing required column: %s", k)
		}
	}
	res := &ParseResult{Source: SourcePayout}
	line := 1
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		line++
		if err != nil {
			res.Rejected = append(res.Rejected, Rejection{Line: line, Reason: err.Error()})
			continue
		}
		ref := strings.TrimSpace(csvVal(rec, idx, "reference number"))
		if ref == "" {
			res.Rejected = append(res.Rejected, Rejection{Line: line, Reason: "missing reference number"})
			continue
		}
		currency := strings.ToUpper(strings.TrimSpace(csvVal(rec, idx, "currency")))
		if currency != "" && currency != "EUR" {
			res.Rejected = append(res.Rejected, Rejection{Line: line, Reason: "currency not EUR"})
			continue
		}
		net, err := parseMoneyToCents(csvVal(rec, idx, "net"))
		if err != nil {
			res.Rejected = append(res.Rejected, Rejection{Line: line, Reason: "invalid net"})
			continue
		}
		payoutDate, err := parsePayoutDate(csvVal(rec, idx, "payout date"), loc)
		if err != nil {
			res.Rejected = append(res.Rejected, Rejection{Line: line, Reason: "invalid payout date"})
			continue
		}
		amount, _ := parseMoneyToCents(csvVal(rec, idx, "amount"))
		commission, _ := parseMoneyToCents(csvVal(rec, idx, "commission"))
		payFee, _ := parseMoneyToCents(csvVal(rec, idx, "payments service fee"))
		// Sign normalisation: payout file expresses cost as negative;
		// canonical storage is positive.
		commission = abs(commission)
		payFee = abs(payFee)
		checkIn, _ := parseShortDate(csvVal(rec, idx, "check-in"), loc)
		checkOut, _ := parseShortDate(csvVal(rec, idx, "checkout"), loc)
		raw := buildRawMap(rec, idx)
		res.Rows = append(res.Rows, Row{
			Source:            SourcePayout,
			ReferenceNumber:   ref,
			RowType:           strings.TrimSpace(csvVal(rec, idx, "type")),
			CheckInDate:       checkIn,
			CheckOutDate:      checkOut,
			GuestName:         strings.TrimSpace(csvVal(rec, idx, "guest name")),
			ReservationStatus: strings.TrimSpace(csvVal(rec, idx, "reservation status")),
			Currency:          currency,
			PaymentStatus:     strings.TrimSpace(csvVal(rec, idx, "payment status")),
			AmountCents:       amount,
			CommissionCents:   commission,
			PaymentFeeCents:   payFee,
			NetCents:          net,
			PayoutDate:        payoutDate,
			PayoutID:          strings.TrimSpace(csvVal(rec, idx, "payout id")),
			Status:            strings.TrimSpace(csvVal(rec, idx, "reservation status")),
			Raw:               raw,
		})
	}
	return res, nil
}

// ---------------- Statement parser ----------------

func parseStatement(r *csv.Reader, idx map[string]int, loc *time.Location) (*ParseResult, error) {
	for _, k := range []string{"reservation number", "booked on", "arrival", "departure", "status", "currency"} {
		if !has(idx, k) {
			return nil, fmt.Errorf("statement csv missing required column: %s", k)
		}
	}
	res := &ParseResult{Source: SourceStatement}
	line := 1
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		line++
		if err != nil {
			res.Rejected = append(res.Rejected, Rejection{Line: line, Reason: err.Error()})
			continue
		}
		ref := strings.TrimSpace(csvVal(rec, idx, "reservation number"))
		if ref == "" {
			res.Rejected = append(res.Rejected, Rejection{Line: line, Reason: "missing reservation number"})
			continue
		}
		currency := strings.ToUpper(strings.TrimSpace(csvVal(rec, idx, "currency")))
		if currency != "" && currency != "EUR" {
			res.Rejected = append(res.Rejected, Rejection{Line: line, Reason: "currency not EUR"})
			continue
		}
		bookedOn, err := parseStatementDateTime(csvVal(rec, idx, "booked on"), loc)
		if err != nil {
			res.Rejected = append(res.Rejected, Rejection{Line: line, Reason: "invalid booked on"})
			continue
		}
		arrival, err := parseStatementDate(csvVal(rec, idx, "arrival"), loc)
		if err != nil {
			res.Rejected = append(res.Rejected, Rejection{Line: line, Reason: "invalid arrival"})
			continue
		}
		departure, err := parseStatementDate(csvVal(rec, idx, "departure"), loc)
		if err != nil {
			res.Rejected = append(res.Rejected, Rejection{Line: line, Reason: "invalid departure"})
			continue
		}
		original, _ := parseMoneyToCents(csvVal(rec, idx, "original amount"))
		final, _ := parseMoneyToCents(csvVal(rec, idx, "final amount"))
		commission, _ := parseMoneyToCents(csvVal(rec, idx, "commission amount"))
		payFee, _ := parseMoneyToCents(csvVal(rec, idx, "payment fee"))
		commission = abs(commission)
		payFee = abs(payFee)
		commissionPct, _ := strconv.ParseFloat(strings.TrimSpace(csvVal(rec, idx, "commission %")), 64)
		persons, _ := strconv.Atoi(strings.TrimSpace(csvVal(rec, idx, "persons")))
		rooms, _ := strconv.Atoi(strings.TrimSpace(csvVal(rec, idx, "rooms")))
		roomNights, _ := strconv.Atoi(strings.TrimSpace(csvVal(rec, idx, "room nights")))
		raw := buildRawMap(rec, idx)
		res.Rows = append(res.Rows, Row{
			Source:              SourceStatement,
			ReferenceNumber:     ref,
			BookedOn:            bookedOn,
			InvoiceNumber:       strings.TrimSpace(csvVal(rec, idx, "invoice number")),
			HotelID:             strings.TrimSpace(csvVal(rec, idx, "hotel id")),
			PropertyLabel:       strings.TrimSpace(csvVal(rec, idx, "property name")),
			Country:             strings.TrimSpace(csvVal(rec, idx, "country")),
			BookerName:          strings.TrimSpace(csvVal(rec, idx, "booker name")),
			GuestRequest:        strings.TrimSpace(csvVal(rec, idx, "guest request")),
			Persons:             persons,
			Rooms:               rooms,
			RoomNights:          roomNights,
			OriginalAmountCents: original,
			CommissionPct:       commissionPct,
			CheckInDate:         arrival,
			CheckOutDate:        departure,
			GuestName:           strings.TrimSpace(csvVal(rec, idx, "guest name")),
			Currency:            currency,
			AmountCents:         final,
			CommissionCents:     commission,
			PaymentFeeCents:     payFee,
			Status:              strings.TrimSpace(csvVal(rec, idx, "status")),
			Raw:                 raw,
		})
	}
	return res, nil
}

// ---------------- Helpers ----------------

func csvVal(rec []string, idx map[string]int, key string) string {
	i, ok := idx[key]
	if !ok || i < 0 || i >= len(rec) {
		return ""
	}
	return rec[i]
}

func buildRawMap(rec []string, idx map[string]int) map[string]string {
	out := make(map[string]string, len(idx))
	for k, i := range idx {
		if i < 0 || i >= len(rec) {
			continue
		}
		out[k] = rec[i]
	}
	return out
}

// CanonicalRawJSON returns a deterministic JSON string of the raw row map
// (sorted keys). The caller persists this in raw_*_row_json so a byte
// comparison with the next upload's canonical JSON gives us row-hash
// idempotence.
func CanonicalRawJSON(raw map[string]string) string {
	if len(raw) == 0 {
		return ""
	}
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Quote(k))
		b.WriteByte(':')
		b.WriteString(strconv.Quote(raw[k]))
	}
	b.WriteByte('}')
	return b.String()
}

func parseMoneyToCents(v string) (int, error) {
	s := strings.TrimSpace(v)
	if s == "" {
		return 0, nil
	}
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, ",", ".")
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return int(math.Round(f * 100)), nil
}

func parsePayoutDate(v string, loc *time.Location) (time.Time, error) {
	t, err := time.ParseInLocation("2 Jan 2006", normalizePayoutDateMonth(v), loc)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 12, 0, 0, 0, loc).UTC(), nil
}

func parseShortDate(v string, loc *time.Location) (string, error) {
	s := strings.TrimSpace(v)
	if s == "" {
		return "", nil
	}
	t, err := time.ParseInLocation("2 Jan 2006", normalizePayoutDateMonth(s), loc)
	if err != nil {
		return "", err
	}
	return t.Format("2006-01-02"), nil
}

// normalizePayoutDateMonth folds Booking.com's quirky four-letter month
// abbreviation "Sept" back to Go's canonical "Sep" so time.Parse can
// understand it. Other month abbreviations match the standard form.
func normalizePayoutDateMonth(v string) string {
	s := strings.TrimSpace(v)
	return strings.Replace(s, " Sept ", " Sep ", 1)
}

func parseStatementDate(v string, loc *time.Location) (string, error) {
	s := strings.TrimSpace(v)
	if s == "" {
		return "", nil
	}
	t, err := time.ParseInLocation("2006-01-02", s, loc)
	if err != nil {
		return "", err
	}
	return t.Format("2006-01-02"), nil
}

func parseStatementDateTime(v string, loc *time.Location) (time.Time, error) {
	s := strings.TrimSpace(v)
	if s == "" {
		return time.Time{}, fmt.Errorf("empty datetime")
	}
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised datetime: %q", s)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
