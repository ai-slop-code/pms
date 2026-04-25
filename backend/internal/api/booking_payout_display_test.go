package api

import (
	"database/sql"
	"testing"
)

func TestBookingPayoutHostName(t *testing.T) {
	raw := `{"host name":"  My Listing  ","guest name":"Ann"}`
	got := bookingPayoutHostName(sql.NullString{String: raw, Valid: true})
	if got == nil || *got != "My Listing" {
		t.Fatalf("host=%v want My Listing", got)
	}
	alt := `{"listing name":"Sea View Apt"}`
	got2 := bookingPayoutHostName(sql.NullString{String: alt, Valid: true})
	if got2 == nil || *got2 != "Sea View Apt" {
		t.Fatalf("listing=%v want Sea View Apt", got2)
	}
	fuzzy := `{"reference number":"X","Property title on booking":"Cozy Loft"}`
	got3 := bookingPayoutHostName(sql.NullString{String: fuzzy, Valid: true})
	if got3 == nil || *got3 != "Cozy Loft" {
		t.Fatalf("fuzzy=%v want Cozy Loft", got3)
	}
	if bookingPayoutHostName(sql.NullString{}) != nil {
		t.Fatal("expected nil")
	}
}

func TestFinanceBookingPayoutSummary_guestFallback(t *testing.T) {
	s := financeBookingPayoutSummary(sql.NullString{}, sql.NullString{String: "  Jane D.  ", Valid: true}, sql.NullString{}, "")
	if s == nil || *s != "Jane D." {
		t.Fatalf("got %v", s)
	}
	s2 := financeBookingPayoutSummary(sql.NullString{}, sql.NullString{}, sql.NullString{}, "  Villa X  ")
	if s2 == nil || *s2 != "Villa X" {
		t.Fatalf("property fallback got %v", s2)
	}
}

func TestFixCSVMojibake(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		// Classic UTF-8-as-Windows-1252 double encoding (Turkish: "Fatıh Altuntaş")
		{"FatÄ±h AltuntaÅŸ", "Fatıh Altuntaş"},
		// Slovak diacritics ("Čierny") — Č is UTF-8 C4 8C → "ÄŒ"
		{"ÄŒierny", "Čierny"},
		// German umlauts ("Müller") — ü is UTF-8 C3 BC → "Ã¼"
		{"Müller", "Müller"},
		// Already clean UTF-8 should pass through
		{"Fatih Altuntas", "Fatih Altuntas"},
		{"Čierny vŕšok", "Čierny vŕšok"},
		// Empty/plain ASCII pass through untouched
		{"", ""},
		{"Ann", "Ann"},
	}
	for _, c := range cases {
		got := fixCSVMojibake(c.in)
		if got != c.want {
			t.Errorf("fixCSVMojibake(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBookingPayoutHostName_fixesMojibake(t *testing.T) {
	raw := `{"listing name":"ChalupÃ¡ VysokÃ© Tatry"}`
	got := bookingPayoutHostName(sql.NullString{String: raw, Valid: true})
	if got == nil || *got != "Chalupá Vysoké Tatry" {
		t.Fatalf("host=%v want Chalupá Vysoké Tatry", got)
	}
}

func TestFinanceBookingPayoutSummary_fixesGuestMojibake(t *testing.T) {
	s := financeBookingPayoutSummary(sql.NullString{}, sql.NullString{String: "FatÄ±h AltuntaÅŸ", Valid: true}, sql.NullString{}, "")
	if s == nil || *s != "Fatıh Altuntaş" {
		t.Fatalf("got %v want Fatıh Altuntaş", s)
	}
}
