package occupancy

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
)

// ParsedEvent is a normalized view of a VEVENT for occupancy sync.
type ParsedEvent struct {
	UID         string
	StartUTC    time.Time
	EndUTC      time.Time
	Summary     string
	Sequence    int
	ICalStatus  string
	Cancelled   bool
	RawICS      string
	ContentHash string
}

type ParseResult struct {
	Events      []*ParsedEvent
	ParseErrors []string
	SeenEvents  int
}

var serialCfg = &ics.SerializationConfiguration{
	MaxLength:         75,
	PropertyMaxLength: 75,
	NewLine:           "\r\n",
}

// ParseICalendar extracts stay events from ICS bytes. Times are stored in UTC.
//
// All-day events (DTSTART;VALUE=DATE) use the calendar day numbers from the feed
// as UTC civil dates (midnight UTC). DATE has no timezone in iCalendar; using the
// server's time.Local (as golang-ical does by default) shifts stays by one day on
// many deployments — see https://www.rfc-editor.org/rfc/rfc5545#section-3.3.4
func ParseICalendar(data []byte) ([]*ParsedEvent, error) {
	res, err := ParseICalendarDetailed(data)
	if err != nil {
		return nil, err
	}
	return res.Events, nil
}

func ParseICalendarDetailed(data []byte) (*ParseResult, error) {
	cal, err := ics.ParseCalendar(strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	out := &ParseResult{}
	for _, ev := range cal.Events() {
		out.SeenEvents++
		p, err := parseVEvent(ev)
		if err != nil {
			uid := strings.TrimSpace(ev.Id())
			if uid == "" {
				uid = "no-uid"
			}
			out.ParseErrors = append(out.ParseErrors, fmt.Sprintf("uid=%s: %v", uid, err))
			continue
		}
		if p == nil {
			continue
		}
		out.Events = append(out.Events, p)
	}
	return out, nil
}

func parseVEvent(ev *ics.VEvent) (*ParsedEvent, error) {
	var start, end time.Time

	// VALUE=DATE: interpret YYYYMMDD as civil dates at UTC midnight (not server local).
	if s, e, ok, err := parseAllDayDateRangeUTC(ev); err != nil {
		return nil, err
	} else if ok {
		start, end = s, e
	} else {
		var err error
		start, err = ev.GetStartAt()
		if err != nil {
			return nil, err
		}
		end, err = ev.GetEndAt()
		if err != nil || end.IsZero() {
			end = start.Add(24 * time.Hour)
		}
		start = start.UTC()
		end = end.UTC()
	}
	if !end.After(start) {
		end = start.Add(24 * time.Hour)
	}

	summary := ""
	if sp := ev.GetProperty(ics.ComponentPropertySummary); sp != nil {
		summary = sp.Value
	}
	seq := 0
	if sq := ev.GetProperty(ics.ComponentPropertySequence); sq != nil {
		if n, err := strconv.Atoi(strings.TrimSpace(sq.Value)); err == nil {
			seq = n
		}
	}
	st := ""
	cancelled := false
	if stp := ev.GetProperty(ics.ComponentPropertyStatus); stp != nil {
		st = strings.ToUpper(strings.TrimSpace(stp.Value))
		if st == "CANCELLED" {
			cancelled = true
		}
	}
	uid := strings.TrimSpace(ev.Id())
	if uid == "" {
		uid = fallbackEventUID(start, end, summary, seq, st)
	}
	raw := ev.Serialize(serialCfg)
	h := hashOccupancy(uid, start, end, summary, seq, st)
	return &ParsedEvent{
		UID:         uid,
		StartUTC:    start,
		EndUTC:      end,
		Summary:     summary,
		Sequence:    seq,
		ICalStatus:  st,
		Cancelled:   cancelled,
		RawICS:      raw,
		ContentHash: h,
	}, nil
}

func fallbackEventUID(start, end time.Time, summary string, seq int, status string) string {
	seed := fmt.Sprintf("%d|%d|%s|%d|%s", start.Unix(), end.Unix(), summary, seq, status)
	sum := sha256.Sum256([]byte(seed))
	return "fp-" + hex.EncodeToString(sum[:16])
}

func parseYYYYMMDDField(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if i := strings.IndexByte(value, 'T'); i >= 0 {
		value = value[:i]
	}
	value = strings.ReplaceAll(value, "-", "")
	if len(value) != 8 {
		return time.Time{}, fmt.Errorf("invalid DATE value %q", value)
	}
	return time.Parse("20060102", value)
}

// parseAllDayDateRangeUTC maps DTSTART/DTEND with VALUE=DATE to UTC midnights for those
// calendar days. DTEND is exclusive per RFC 5545 all-day rules.
func parseAllDayDateRangeUTC(ev *ics.VEvent) (start, end time.Time, ok bool, err error) {
	st := ev.GetProperty(ics.ComponentPropertyDtStart)
	if st == nil || st.GetValueType() != ics.ValueDataTypeDate {
		return time.Time{}, time.Time{}, false, nil
	}
	t0, err := parseYYYYMMDDField(st.Value)
	if err != nil {
		return time.Time{}, time.Time{}, false, err
	}
	y1, m1, d1 := t0.Date()
	start = time.Date(y1, m1, d1, 0, 0, 0, 0, time.UTC)
	if en := ev.GetProperty(ics.ComponentPropertyDtEnd); en != nil && en.GetValueType() == ics.ValueDataTypeDate {
		t1, err := parseYYYYMMDDField(en.Value)
		if err != nil {
			return time.Time{}, time.Time{}, false, err
		}
		y2, m2, d2 := t1.Date()
		end = time.Date(y2, m2, d2, 0, 0, 0, 0, time.UTC)
	} else {
		end = start.AddDate(0, 0, 1)
	}
	return start, end, true, nil
}

func hashOccupancy(uid string, start, end time.Time, summary string, seq int, status string) string {
	s := fmt.Sprintf("%s|%d|%d|%s|%d|%s", uid, start.Unix(), end.Unix(), summary, seq, status)
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
