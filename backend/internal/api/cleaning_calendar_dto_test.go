package api

import (
	"database/sql"
	"testing"

	"pms/backend/internal/store"
)

func TestCleaningCalendarEventDTOUsesPMS21Identity(t *testing.T) {
	named := cleaningCalendarEventDTOFromStore(store.CleaningCalendarEvent{
		ID: 1, NamedStayID: sql.NullInt64{Int64: 20, Valid: true},
		CleaningIdentity: sql.NullString{String: "stay:1:20:2026-08-03", Valid: true},
	})
	if named.NamedStayID == nil || *named.NamedStayID != 20 || named.CleaningIdentity == nil {
		t.Fatalf("named DTO: %+v", named)
	}
	raw := cleaningCalendarEventDTOFromStore(store.CleaningCalendarEvent{
		ID: 2, RawBookingBlockID: sql.NullInt64{Int64: 30, Valid: true},
		CleaningIdentity: sql.NullString{String: "raw-provisional:1:2026-08-03", Valid: true},
	})
	if raw.RawBookingBlockID == nil || *raw.RawBookingBlockID != 30 {
		t.Fatalf("raw DTO: %+v", raw)
	}
}
