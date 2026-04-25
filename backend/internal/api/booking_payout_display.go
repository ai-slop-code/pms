package api

import (
	"database/sql"
	"encoding/json"
	"strings"
	"unicode/utf8"
)

// windows1252Reverse maps characters that Windows-1252 places in the 0x80-0x9F
// range (which Latin-1 leaves undefined) back to their single-byte encoding,
// so we can undo mojibake produced via Windows-1252 decoding (e.g. "ş" whose
// UTF-8 byte 0x9F becomes U+0178 "Ÿ").
var windows1252Reverse = map[rune]byte{
	'€': 0x80, '‚': 0x82, 'ƒ': 0x83, '„': 0x84, '…': 0x85, '†': 0x86, '‡': 0x87,
	'ˆ': 0x88, '‰': 0x89, 'Š': 0x8A, '‹': 0x8B, 'Œ': 0x8C, 'Ž': 0x8E,
	'‘': 0x91, '’': 0x92, '“': 0x93, '”': 0x94, '•': 0x95, '–': 0x96, '—': 0x97,
	'˜': 0x98, '™': 0x99, 'š': 0x9A, '›': 0x9B, 'œ': 0x9C, 'ž': 0x9E, 'Ÿ': 0x9F,
}

// fixCSVMojibake repairs "double-encoded UTF-8" that Booking.com CSV exports
// sometimes contain: original UTF-8 bytes get decoded as Windows-1252/Latin-1
// and re-encoded as UTF-8, producing patterns like "FatÄ±h AltuntaÅŸ" instead
// of "Fatıh Altuntaş". When the string doesn't look mojibake'd, it is returned
// unchanged.
func fixCSVMojibake(s string) string {
	if s == "" {
		return s
	}
	if !strings.ContainsAny(s, "ÃÂÄÅ") {
		return s
	}
	b := make([]byte, 0, len(s))
	for _, r := range s {
		switch {
		case r <= 0xFF:
			b = append(b, byte(r))
		default:
			if bb, ok := windows1252Reverse[r]; ok {
				b = append(b, bb)
				continue
			}
			return s
		}
	}
	if !utf8.Valid(b) {
		return s
	}
	fixed := string(b)
	if fixed == s {
		return s
	}
	return fixed
}

func fixCSVMojibakePtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := fixCSVMojibake(*p)
	return &v
}

func fixCSVMojibakeNullString(v sql.NullString) sql.NullString {
	if !v.Valid {
		return v
	}
	return sql.NullString{String: fixCSVMojibake(v.String), Valid: true}
}

// bookingPayoutHostName picks a human-readable property/host/listing line from the payout import snapshot
// (CSV columns are stored as lowercase keys in raw_row_json).
func bookingPayoutHostName(raw sql.NullString) *string {
	if !raw.Valid {
		return nil
	}
	s := strings.TrimSpace(raw.String)
	if s == "" {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(s), &m); err != nil || len(m) == 0 {
		return nil
	}
	for _, k := range []string{
		"host name",
		"listing name",
		"property name",
		"accommodation name",
		"listing nickname",
		"listing title",
		"smart name",
		"listing",
		"apartment name",
		"room name",
		"unit name",
	} {
		if v := strings.TrimSpace(m[k]); v != "" {
			v = fixCSVMojibake(v)
			return &v
		}
	}
	// Booking.com and partner exports vary by locale and report version; fuzzy-match column headers.
	type cand struct {
		val string
		n   int
	}
	var best *cand
	for k, v := range m {
		kl := strings.ToLower(strings.TrimSpace(k))
		val := strings.TrimSpace(v)
		if val == "" || bookingPayoutRawKeySkip(kl) {
			continue
		}
		for _, hint := range []string{
			"host", "listing", "property", "accommodation",
			"apartment", "room name", "unit", "nickname", "title",
		} {
			if strings.Contains(kl, hint) {
				if best == nil || len(val) > best.n {
					best = &cand{val: val, n: len(val)}
				}
				break
			}
		}
	}
	if best != nil {
		v := fixCSVMojibake(best.val)
		return &v
	}
	return nil
}

func bookingPayoutRawKeySkip(kl string) bool {
	switch kl {
	case "guest name", "reference number", "refference number", "net", "payout date",
		"type", "currency", "reservation status", "payment status", "payout id":
		return true
	}
	if strings.Contains(kl, "guest") || strings.Contains(kl, "booker") ||
		strings.Contains(kl, "traveller") || strings.Contains(kl, "traveler") {
		return true
	}
	if strings.Contains(kl, "commission") || strings.Contains(kl, "service fee") ||
		strings.Contains(kl, "payments service fee") || kl == "amount" {
		return true
	}
	return false
}

// financeBookingPayoutSummary is a single line for invoice/finance pickers: listing/host from CSV
// snapshot, else guest name, else mapped occupancy summary, else property display name.
func financeBookingPayoutSummary(raw, guest, occSummary sql.NullString, propertyName string) *string {
	if h := bookingPayoutHostName(raw); h != nil {
		return h
	}
	if guest.Valid {
		if s := strings.TrimSpace(guest.String); s != "" {
			s = fixCSVMojibake(s)
			return &s
		}
	}
	if occSummary.Valid {
		if s := strings.TrimSpace(occSummary.String); s != "" {
			s = fixCSVMojibake(s)
			return &s
		}
	}
	if s := strings.TrimSpace(propertyName); s != "" {
		return &s
	}
	return nil
}
