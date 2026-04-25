package occupancy

import (
	"strings"
	"testing"
	"time"
)

func TestParseICalendar_BookingStyle(t *testing.T) {
	const ics = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//PMS//EN
BEGIN:VEVENT
UID:booking-123@test
DTSTAMP:20260401T120000Z
DTSTART;VALUE=DATE:20260410
DTEND;VALUE=DATE:20260413
SUMMARY:Reserved - Guest A
END:VEVENT
END:VCALENDAR
`
	evs, err := ParseICalendar([]byte(strings.ReplaceAll(ics, "\n", "\r\n")))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 {
		t.Fatalf("events: %d", len(evs))
	}
	e := evs[0]
	if e.UID != "booking-123@test" {
		t.Fatalf("uid %q", e.UID)
	}
	if !strings.Contains(e.Summary, "Reserved") {
		t.Fatalf("summary %q", e.Summary)
	}
	if e.Cancelled {
		t.Fatal("cancelled")
	}
	if e.ContentHash == "" {
		t.Fatal("empty hash")
	}
	// Civil DATE must not follow server local TZ (Booking: 10th → 10th UTC midnight, end 11th exclusive).
	if g, w := e.StartUTC.UTC().Format(time.RFC3339), "2026-04-10T00:00:00Z"; g != w {
		t.Fatalf("start %s want %s", g, w)
	}
	if g, w := e.EndUTC.UTC().Format(time.RFC3339), "2026-04-13T00:00:00Z"; g != w {
		t.Fatalf("end %s want %s (exclusive checkout)", g, w)
	}
}

func TestParseICalendar_SingleNightBookingComStyle(t *testing.T) {
	const ics = `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:single@test
DTSTAMP:20260405T211849Z
DTSTART;VALUE=DATE:20260410
DTEND;VALUE=DATE:20260411
SUMMARY:One night
END:VEVENT
END:VCALENDAR
`
	evs, err := ParseICalendar([]byte(strings.ReplaceAll(ics, "\n", "\r\n")))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 {
		t.Fatal(len(evs))
	}
	e := evs[0]
	if g, w := e.StartUTC.Format(time.RFC3339), "2026-04-10T00:00:00Z"; g != w {
		t.Fatalf("start %s want %s", g, w)
	}
	if g, w := e.EndUTC.Format(time.RFC3339), "2026-04-11T00:00:00Z"; g != w {
		t.Fatalf("end %s want %s", g, w)
	}
}

func TestParseICalendar_Cancelled(t *testing.T) {
	const ics = `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:x@y
DTSTAMP:20260401T120000Z
DTSTART;VALUE=DATE:20260401
DTEND;VALUE=DATE:20260402
STATUS:CANCELLED
SUMMARY:Cancelled stay
END:VEVENT
END:VCALENDAR
`
	evs, err := ParseICalendar([]byte(strings.ReplaceAll(ics, "\n", "\r\n")))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 || !evs[0].Cancelled {
		t.Fatalf("got %+v", evs)
	}
}

func TestParseICalendar_FallbackFingerprintWhenUIDMissing(t *testing.T) {
	const ics = `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
DTSTAMP:20260401T120000Z
DTSTART;VALUE=DATE:20260420
DTEND;VALUE=DATE:20260422
SUMMARY:No UID event
END:VEVENT
END:VCALENDAR
`
	evs, err := ParseICalendar([]byte(strings.ReplaceAll(ics, "\n", "\r\n")))
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 {
		t.Fatalf("events: %d", len(evs))
	}
	if evs[0].UID == "" || !strings.HasPrefix(evs[0].UID, "fp-") {
		t.Fatalf("unexpected fallback uid: %q", evs[0].UID)
	}
}

func TestParseICalendarDetailed_ReportsParseErrors(t *testing.T) {
	const ics = `BEGIN:VCALENDAR
VERSION:2.0
BEGIN:VEVENT
UID:valid@test
DTSTAMP:20260401T120000Z
DTSTART;VALUE=DATE:20260410
DTEND;VALUE=DATE:20260411
SUMMARY:Valid
END:VEVENT
BEGIN:VEVENT
UID:broken@test
DTSTAMP:20260401T120000Z
SUMMARY:Missing start date
END:VEVENT
END:VCALENDAR
`
	res, err := ParseICalendarDetailed([]byte(strings.ReplaceAll(ics, "\n", "\r\n")))
	if err != nil {
		t.Fatal(err)
	}
	if res.SeenEvents != 2 {
		t.Fatalf("seen=%d", res.SeenEvents)
	}
	if len(res.Events) != 1 {
		t.Fatalf("events=%d", len(res.Events))
	}
	if len(res.ParseErrors) != 1 {
		t.Fatalf("parseErrors=%d (%v)", len(res.ParseErrors), res.ParseErrors)
	}
}
