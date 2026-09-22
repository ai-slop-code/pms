package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"pms/backend/internal/finance/statements"
	"pms/backend/internal/permissions"
	"pms/backend/internal/store"
)

// ---- preview cache ----------------------------------------------------------

// financeImportPreview is the parsed-and-merged plan kept in memory between
// the preview and commit calls. The cache is per-process and short-lived
// (TTL 15 minutes); a server restart invalidates outstanding tokens, which
// is fine because uploaders just retry.
type financeImportPreview struct {
	PropertyID           int64
	UserID               int64
	SourceType           statements.SourceType
	HotelID              string
	FileSHA256           string
	PeriodStart          string
	PeriodEnd            string
	Plan                 []financePreviewPlanEntry
	Pending              []financePreviewPendingEntry
	Evidence             []statements.Row
	SkippedCancellations []financePreviewSkippedCancellation
	Rejected             []statements.Rejection
	Skipped              []financePreviewSkippedEntry
	CreatedAt            time.Time
}

type financePreviewPlanEntry struct {
	Action          statements.MergeAction
	Reference       string
	ExistingID      int64
	Result          statements.CanonicalBooking
	StatusChanged   bool
	ChangedFields   []string
	NamedStayMatch  *store.NamedStay // populated for payout inserts
	BookingIncomeID int64            // resolved booking_income category for payouts
	NetCents        int              // payout net (used to upsert finance_transactions)
	PayoutDate      time.Time
	PayoutID        string
	Line            int
	MatchBasis      string
	StayBeforeName  string
	StayCheckIn     string
	StayCheckOut    string
	StayNameChanged bool
	Input           statements.Row
}

type financePreviewPendingEntry struct {
	Line       int
	Reference  string
	GuestName  string
	CheckIn    string
	CheckOut   string
	NetCents   int
	Reason     string
	Candidates []store.NamedStayFinanceCandidate
	Result     statements.CanonicalBooking
	ExistingID int64
	Entry      financePreviewPlanEntry
}

type financePreviewSkippedEntry struct {
	Reference string `json:"reference"`
	Reason    string `json:"reason"`
	HotelID   string `json:"hotel_id,omitempty"`
}

type financePreviewSkippedCancellation struct {
	Line         int    `json:"line"`
	Reference    string `json:"reference"`
	GuestName    string `json:"guest_name"`
	CheckInDate  string `json:"check_in_date"`
	CheckOutDate string `json:"check_out_date"`
	BookedOn     string `json:"booked_on"`
	Reason       string `json:"reason"`
}

var (
	financePreviewCache    = sync.Map{}
	financePreviewCommitMu sync.Mutex
)

const financePreviewTTL = 15 * time.Minute

func storeFinancePreview(p *financeImportPreview) string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	token := hex.EncodeToString(buf)
	p.CreatedAt = time.Now()
	financePreviewCache.Store(token, p)
	return token
}

func loadFinancePreview(token string) *financeImportPreview {
	v, ok := financePreviewCache.Load(token)
	if !ok {
		return nil
	}
	p, _ := v.(*financeImportPreview)
	if p == nil {
		return nil
	}
	if time.Since(p.CreatedAt) > financePreviewTTL {
		financePreviewCache.Delete(token)
		return nil
	}
	return p
}

func evictFinancePreview(token string) {
	financePreviewCache.Delete(token)
}

// ---- response DTOs ---------------------------------------------------------

type financeImportPreviewInsert struct {
	Reference     string `json:"reference"`
	GuestName     string `json:"guest_name,omitempty"`
	CheckInDate   string `json:"check_in_date,omitempty"`
	CheckOutDate  string `json:"check_out_date,omitempty"`
	AmountCents   int    `json:"amount_cents,omitempty"`
	Status        string `json:"status,omitempty"`
	StatusChanged bool   `json:"status_changed,omitempty"`
	Line          int    `json:"line,omitempty"`
	NamedStayID   int64  `json:"named_stay_id,omitempty"`
	MatchBasis    string `json:"match_basis,omitempty"`
	StayName      string `json:"stay_name,omitempty"`
	StayCheckIn   string `json:"stay_check_in_date,omitempty"`
	StayCheckOut  string `json:"stay_check_out_date,omitempty"`
}

type financeImportPreviewUpdate struct {
	Reference     string                  `json:"reference"`
	GuestName     string                  `json:"guest_name,omitempty"`
	StatusChanged bool                    `json:"status_changed,omitempty"`
	Changes       []financePreviewFieldKV `json:"changes,omitempty"`
	Line          int                     `json:"line,omitempty"`
	NamedStayID   int64                   `json:"named_stay_id,omitempty"`
	MatchBasis    string                  `json:"match_basis,omitempty"`
}

type financeImportStayNameChange struct {
	Line                int64  `json:"line"`
	Reference           string `json:"reference"`
	NamedStayID         int64  `json:"named_stay_id"`
	PreviousDisplayName string `json:"previous_display_name"`
	PayoutGuestName     string `json:"payout_guest_name"`
}

type financeImportStayCandidate struct {
	NamedStayID  int64  `json:"named_stay_id"`
	DisplayName  string `json:"display_name"`
	StayType     string `json:"stay_type"`
	CheckInDate  string `json:"check_in_date"`
	CheckOutDate string `json:"check_out_date"`
}

type financeImportNeedsStaySelection struct {
	Line         int                          `json:"line"`
	Reference    string                       `json:"reference"`
	GuestName    string                       `json:"guest_name"`
	CheckInDate  string                       `json:"check_in_date"`
	CheckOutDate string                       `json:"check_out_date"`
	NetCents     int                          `json:"net_cents"`
	Reason       string                       `json:"reason"`
	Candidates   []financeImportStayCandidate `json:"candidates"`
}

type financePreviewFieldKV struct {
	Field string `json:"field"`
}

type financeImportPreviewResponse struct {
	OK                   bool                                `json:"ok"`
	PreviewToken         string                              `json:"preview_token"`
	SourceType           string                              `json:"source_type"`
	HotelID              string                              `json:"hotel_id,omitempty"`
	FileSHA256           string                              `json:"file_sha256"`
	PeriodStart          string                              `json:"period_start,omitempty"`
	PeriodEnd            string                              `json:"period_end,omitempty"`
	DuplicateOfImportID  *int64                              `json:"duplicate_of_import_id,omitempty"`
	Inserts              []financeImportPreviewInsert        `json:"inserts"`
	Updates              []financeImportPreviewUpdate        `json:"updates"`
	UnchangedCount       int                                 `json:"unchanged_count"`
	Skipped              []financePreviewSkippedEntry        `json:"skipped_other_hotel"`
	Rejected             []statements.Rejection              `json:"rejected"`
	StayNameChanges      []financeImportStayNameChange       `json:"stay_name_changes"`
	NeedsStaySelection   []financeImportNeedsStaySelection   `json:"needs_stay_selection"`
	SkippedCancellations []financePreviewSkippedCancellation `json:"skipped_cancellations"`
}

type financeImportCommitRequest struct {
	PreviewToken   string                       `json:"preview_token"`
	StaySelections []financeImportStaySelection `json:"stay_selections,omitempty"`
}

type financeImportStaySelection struct {
	Line        int   `json:"line"`
	NamedStayID int64 `json:"named_stay_id"`
}

type financeImportCommitResponse struct {
	OK                           bool   `json:"ok"`
	ImportID                     int64  `json:"import_id"`
	SourceType                   string `json:"source_type"`
	RowCountTotal                int    `json:"row_count_total"`
	RowCountInserted             int    `json:"row_count_inserted"`
	RowCountUpdated              int    `json:"row_count_updated"`
	RowCountUnchanged            int    `json:"row_count_unchanged"`
	RowCountSkipped              int    `json:"row_count_skipped_other_hotel"`
	RowCountRejected             int    `json:"row_count_rejected"`
	RowCountSkippedCancellations int    `json:"row_count_skipped_cancellations"`
}

type financeImportListItem struct {
	ID                           int64  `json:"id"`
	SourceType                   string `json:"source_type"`
	SourceChannel                string `json:"source_channel"`
	HotelID                      string `json:"hotel_id,omitempty"`
	UploadedAt                   string `json:"uploaded_at"`
	FileSHA256                   string `json:"file_sha256,omitempty"`
	RowCountTotal                int    `json:"row_count_total"`
	RowCountInserted             int    `json:"row_count_inserted"`
	RowCountUpdated              int    `json:"row_count_updated"`
	RowCountUnchanged            int    `json:"row_count_unchanged"`
	RowCountSkippedOtherHotel    int    `json:"row_count_skipped_other_hotel"`
	RowCountRejected             int    `json:"row_count_rejected"`
	RowCountSkippedCancellations int    `json:"row_count_skipped_cancellations"`
}

// ---- preview ---------------------------------------------------------------

func (s *Server) postFinanceImportPreview(w http.ResponseWriter, r *http.Request) {
	actor, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Finance, permissions.LevelWrite)
	if !ok {
		return
	}
	prop, err := s.Store.GetProperty(r.Context(), pid)
	if err != nil {
		WriteError(w, http.StatusNotFound, "property not found")
		return
	}
	loc, err := time.LoadLocation(prop.Timezone)
	if err != nil {
		loc = time.UTC
	}
	r.Body = http.MaxBytesReader(w, r.Body, (25<<20)+(1<<20))
	if err := r.ParseMultipartForm(25 << 20); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		WriteError(w, http.StatusBadRequest, "missing file")
		return
	}
	defer file.Close()
	raw, err := io.ReadAll(file)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "read file")
		return
	}
	sum := sha256.Sum256(raw)
	sha := hex.EncodeToString(sum[:])

	parsed, err := statements.DetectAndParse(strings.NewReader(string(raw)), loc)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Capture / verify hotel id for statement uploads.
	hotelID := ""
	if parsed.Source == statements.SourceStatement {
		if len(parsed.Rows) > 0 {
			hotelID = parsed.Rows[0].HotelID
		}
		if hotelID != "" {
			if _, err := s.Store.SetPropertyBookingHotelIDIfEmpty(r.Context(), pid, hotelID); err != nil {
				WriteError(w, http.StatusInternalServerError, "capture hotel id")
				return
			}
		}
	}

	configuredHotel, _ := s.Store.GetPropertyBookingHotelID(r.Context(), pid)

	periodStart, periodEnd := derivePeriod(parsed.Rows, parsed.Source)

	preview := &financeImportPreview{
		PropertyID:  pid,
		UserID:      0,
		SourceType:  parsed.Source,
		HotelID:     hotelID,
		FileSHA256:  sha,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Rejected:    parsed.Rejected,
	}
	if actor != nil {
		preview.UserID = actor.ID
	}

	bookingIncomeID := int64(0)
	if parsed.Source == statements.SourcePayout {
		bookingIncomeID, _ = s.Store.FinanceCategoryIDByCode(r.Context(), pid, "booking_income")
		if bookingIncomeID == 0 {
			WriteError(w, http.StatusBadRequest, "booking_income category missing")
			return
		}
	}

	for _, row := range parsed.Rows {
		// Multi-hotel filter (statements only).
		if parsed.Source == statements.SourceStatement && configuredHotel != "" && row.HotelID != "" && row.HotelID != configuredHotel {
			preview.Skipped = append(preview.Skipped, financePreviewSkippedEntry{
				Reference: row.ReferenceNumber,
				Reason:    "hotel_id mismatch",
				HotelID:   row.HotelID,
			})
			continue
		}
		if parsed.Source == statements.SourceStatement {
			preview.Evidence = append(preview.Evidence, row)
		}
		existing, existingID, err := s.Store.FinanceBookingByReference(r.Context(), pid, "booking_com", row.ReferenceNumber)
		if err != nil && err != sql.ErrNoRows {
			WriteError(w, http.StatusInternalServerError, "lookup booking")
			return
		}
		outcome := statements.Merge(existing, row)
		if row.IsCommissionAdjustment() || row.IsRefund() {
			missingBaseline := existing == nil || !existing.HasPayoutData || existing.PayoutDate == nil || strings.TrimSpace(*existing.PayoutDate) == ""
			if row.IsCommissionAdjustment() {
				missingBaseline = missingBaseline || existing.CommissionCents == nil || existing.NetCents == nil
			} else {
				missingBaseline = missingBaseline || existing.AmountCents == nil || existing.NetCents == nil
			}
			if missingBaseline {
				reason := "original payout must be imported before applying this correction"
				preview.Rejected = append(preview.Rejected, statements.Rejection{Line: row.Line, Reason: reason + " (reference " + row.ReferenceNumber + ")"})
				continue
			}
		}
		cancelled := parsed.Source == statements.SourceStatement && strings.EqualFold(strings.TrimSpace(row.Status), "CANCELLED")
		if cancelled && existing == nil {
			matches, err := s.Store.ListNamedStaysForFinanceReferenceDates(r.Context(), pid, row.ReferenceNumber, row.CheckInDate, row.CheckOutDate)
			if err != nil {
				WriteError(w, http.StatusInternalServerError, "match cancellation reference")
				return
			}
			if len(matches) != 1 {
				preview.SkippedCancellations = append(preview.SkippedCancellations, financePreviewSkippedCancellation{
					Line: row.Line, Reference: row.ReferenceNumber, GuestName: row.GuestName,
					CheckInDate: row.CheckInDate, CheckOutDate: row.CheckOutDate,
					BookedOn: row.BookedOn.UTC().Format(time.RFC3339),
					Reason:   "Skipped booking creation — cancellation retained for statement analytics",
				})
				continue
			}
		}
		entry := financePreviewPlanEntry{
			Action:        outcome.Action,
			Reference:     row.ReferenceNumber,
			ExistingID:    existingID,
			Result:        outcome.Result,
			StatusChanged: outcome.StatusChanged,
			ChangedFields: outcome.Changed,
			Line:          row.Line,
			Input:         row,
		}
		var stay *store.NamedStay
		if existingID > 0 {
			// Persisted associations are authoritative on re-upload.
			if linked, err := s.Store.FinanceBookingNamedStay(r.Context(), pid, existingID); err != nil {
				WriteError(w, http.StatusInternalServerError, "lookup booking stay")
				return
			} else {
				stay = linked
				if stay != nil {
					entry.MatchBasis = "existing finance association"
				}
			}
		} else {
			stay, err = s.Store.FindNamedStayForFinanceStayDates(r.Context(), pid, row.ReferenceNumber, row.CheckInDate, row.CheckOutDate, row.GuestName)
			if err != nil {
				WriteError(w, http.StatusInternalServerError, "match named stay")
				return
			}
			if stay != nil {
				entry.MatchBasis = "reference/date or name/date"
			} else if cancelled {
				matches, err := s.Store.ListNamedStaysForFinanceReferenceDates(r.Context(), pid, row.ReferenceNumber, row.CheckInDate, row.CheckOutDate)
				if err != nil {
					WriteError(w, http.StatusInternalServerError, "match cancellation reference")
					return
				}
				if len(matches) == 1 {
					stay = &matches[0]
					entry.MatchBasis = "unique reference/date"
				}
				if stay == nil {
					continue
				}
			} else if (parsed.Source == statements.SourcePayout || parsed.Source == statements.SourceStatement) && outcome.Action == statements.ActionInsert && !cancelled {
				candidates, err := s.Store.ListNamedStaysForFinanceExactDates(r.Context(), pid, row.CheckInDate, row.CheckOutDate)
				if err != nil {
					WriteError(w, http.StatusInternalServerError, "find named stay candidates")
					return
				}
				switch {
				case len(candidates) == 1:
					stayID := candidates[0].ID
					stay, err = s.Store.GetNamedStay(r.Context(), pid, stayID)
					if err != nil {
						WriteError(w, http.StatusInternalServerError, "load named stay")
						return
					}
					entry.MatchBasis = "unique exact dates"
				case len(candidates) > 1:
					reason := "No exact name/reference match. Select the existing stay for these dates."
					pending := financePreviewPendingEntry{Line: row.Line, Reference: row.ReferenceNumber, GuestName: row.GuestName, CheckIn: row.CheckInDate, CheckOut: row.CheckOutDate, NetCents: row.NetCents, Reason: reason, Candidates: candidates, Result: outcome.Result, ExistingID: existingID, Entry: entry}
					pending.Entry.BookingIncomeID = bookingIncomeID
					pending.Entry.NetCents = row.NetCents
					pending.Entry.PayoutDate = row.PayoutDate
					pending.Entry.PayoutID = row.PayoutID
					preview.Pending = append(preview.Pending, pending)
					continue
				default:
					reason := "no matching named stay for " + row.ReferenceNumber
					if parsed.Source == statements.SourceStatement {
						reason = "no named stay; statement evidence retained, canonical booking not created"
					}
					preview.Rejected = append(preview.Rejected, statements.Rejection{Line: row.Line, Reason: reason})
					continue
				}
			} else if outcome.Action == statements.ActionInsert {
				if parsed.Source == statements.SourceStatement {
					preview.Rejected = append(preview.Rejected, statements.Rejection{Line: row.Line, Reason: "no named stay; statement evidence retained, canonical booking not created"})
					continue
				}
				preview.Rejected = append(preview.Rejected, statements.Rejection{Line: row.Line, Reason: "no matching named stay for " + row.ReferenceNumber})
				continue
			}
		}
		if parsed.Source == statements.SourcePayout {
			entry.BookingIncomeID = bookingIncomeID
			if entry.Result.NetCents != nil {
				entry.NetCents = *entry.Result.NetCents
			}
			if entry.Result.PayoutDate != nil {
				entry.PayoutDate, _ = time.Parse(time.RFC3339, *entry.Result.PayoutDate)
			}
			if entry.Result.PayoutID != nil {
				entry.PayoutID = *entry.Result.PayoutID
			}
		}
		if stay != nil {
			entry.StayBeforeName = stay.DisplayName
			entry.StayCheckIn = stay.CheckInDate
			entry.StayCheckOut = stay.CheckOutDate
			entry.NamedStayMatch = stay
			name := strings.TrimSpace(row.GuestName)
			entry.StayNameChanged = parsed.Source == statements.SourcePayout && stay.StayType == store.StayTypeBookingCom && stay.CheckInDate == row.CheckInDate && stay.CheckOutDate == row.CheckOutDate && name != "" && name != stay.DisplayName
		}
		preview.Plan = append(preview.Plan, entry)
	}

	token := storeFinancePreview(preview)
	resp := buildFinancePreviewResponse(preview, token)

	if dup, _ := s.Store.LastFinanceImportBySHA(r.Context(), pid, sha); dup != nil {
		id := dup.ID
		resp.DuplicateOfImportID = &id
	}

	s.audit(r, actor, "finance_import_preview", "property", strconv.FormatInt(pid, 10), "success")
	WriteJSON(w, http.StatusOK, resp)
}

func buildFinancePreviewResponse(p *financeImportPreview, token string) *financeImportPreviewResponse {
	resp := &financeImportPreviewResponse{
		OK:                   true,
		PreviewToken:         token,
		SourceType:           string(p.SourceType),
		HotelID:              p.HotelID,
		FileSHA256:           p.FileSHA256,
		PeriodStart:          p.PeriodStart,
		PeriodEnd:            p.PeriodEnd,
		Skipped:              p.Skipped,
		Rejected:             p.Rejected,
		SkippedCancellations: p.SkippedCancellations,
	}
	for _, entry := range p.Plan {
		namedStayID := int64(0)
		if entry.NamedStayMatch != nil {
			namedStayID = entry.NamedStayMatch.ID
		}
		switch entry.Action {
		case statements.ActionInsert:
			ins := financeImportPreviewInsert{
				Reference:     entry.Reference,
				GuestName:     strDeref(entry.Result.GuestName),
				CheckInDate:   strDeref(entry.Result.CheckInDate),
				CheckOutDate:  strDeref(entry.Result.CheckOutDate),
				Status:        strDeref(entry.Result.Status),
				StatusChanged: entry.StatusChanged,
				Line:          entry.Line, NamedStayID: namedStayID, MatchBasis: entry.MatchBasis,
				StayName: entry.StayBeforeName, StayCheckIn: entry.StayCheckIn, StayCheckOut: entry.StayCheckOut,
			}
			if entry.Result.AmountCents != nil {
				ins.AmountCents = *entry.Result.AmountCents
			}
			resp.Inserts = append(resp.Inserts, ins)
		case statements.ActionUpdate:
			upd := financeImportPreviewUpdate{
				Reference:     entry.Reference,
				GuestName:     strDeref(entry.Result.GuestName),
				StatusChanged: entry.StatusChanged,
				Line:          entry.Line, NamedStayID: namedStayID, MatchBasis: entry.MatchBasis,
			}
			for _, f := range entry.ChangedFields {
				upd.Changes = append(upd.Changes, financePreviewFieldKV{Field: f})
			}
			resp.Updates = append(resp.Updates, upd)
		case statements.ActionUnchanged:
			resp.UnchangedCount++
		}
	}
	for _, change := range p.Plan {
		if !change.StayNameChanged || change.NamedStayMatch == nil {
			continue
		}
		resp.StayNameChanges = append(resp.StayNameChanges, financeImportStayNameChange{
			Line: int64(change.Line), Reference: change.Reference, NamedStayID: change.NamedStayMatch.ID,
			PreviousDisplayName: change.NamedStayMatch.DisplayName, PayoutGuestName: strDeref(change.Result.GuestName),
		})
	}
	for _, pending := range p.Pending {
		item := financeImportNeedsStaySelection{Line: pending.Line, Reference: pending.Reference, GuestName: pending.GuestName, CheckInDate: pending.CheckIn, CheckOutDate: pending.CheckOut, NetCents: pending.NetCents, Reason: pending.Reason}
		for _, candidate := range pending.Candidates {
			item.Candidates = append(item.Candidates, financeImportStayCandidate{NamedStayID: candidate.ID, DisplayName: candidate.DisplayName, StayType: candidate.StayType, CheckInDate: candidate.CheckInDate, CheckOutDate: candidate.CheckOutDate})
		}
		resp.NeedsStaySelection = append(resp.NeedsStaySelection, item)
	}
	if resp.Inserts == nil {
		resp.Inserts = []financeImportPreviewInsert{}
	}
	if resp.Updates == nil {
		resp.Updates = []financeImportPreviewUpdate{}
	}
	if resp.Skipped == nil {
		resp.Skipped = []financePreviewSkippedEntry{}
	}
	if resp.Rejected == nil {
		resp.Rejected = []statements.Rejection{}
	}
	if resp.StayNameChanges == nil {
		resp.StayNameChanges = []financeImportStayNameChange{}
	}
	if resp.NeedsStaySelection == nil {
		resp.NeedsStaySelection = []financeImportNeedsStaySelection{}
	}
	if resp.SkippedCancellations == nil {
		resp.SkippedCancellations = []financePreviewSkippedCancellation{}
	}
	return resp
}

func strDeref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ---- commit ----------------------------------------------------------------

func (s *Server) postFinanceImportCommit(w http.ResponseWriter, r *http.Request) {
	actor, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Finance, permissions.LevelWrite)
	if !ok {
		return
	}
	var req financeImportCommitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.PreviewToken) == "" {
		WriteError(w, http.StatusBadRequest, "missing preview_token")
		return
	}
	financePreviewCommitMu.Lock()
	defer financePreviewCommitMu.Unlock()
	preview := loadFinancePreview(req.PreviewToken)
	if preview == nil || preview.PropertyID != pid {
		WriteError(w, http.StatusGone, "preview expired")
		return
	}
	prop, err := s.Store.GetProperty(r.Context(), pid)
	if err != nil {
		WriteError(w, http.StatusNotFound, "property not found")
		return
	}
	selectionByLine := make(map[int]financeImportStaySelection, len(req.StaySelections))
	for _, selection := range req.StaySelections {
		if selection.Line <= 0 || selection.NamedStayID <= 0 {
			WriteError(w, http.StatusBadRequest, "invalid stay selection")
			return
		}
		if _, exists := selectionByLine[selection.Line]; exists {
			WriteError(w, http.StatusBadRequest, "duplicate stay selection line")
			return
		}
		selectionByLine[selection.Line] = selection
	}
	selected := make(map[int]bool)
	plan := append([]financePreviewPlanEntry(nil), preview.Plan...)
	for _, pending := range preview.Pending {
		selection, ok := selectionByLine[pending.Line]
		if !ok {
			continue
		}
		var offered *store.NamedStayFinanceCandidate
		for i := range pending.Candidates {
			if pending.Candidates[i].ID == selection.NamedStayID {
				offered = &pending.Candidates[i]
				break
			}
		}
		if offered == nil {
			WriteError(w, http.StatusBadRequest, "selected stay was not offered for this row")
			return
		}
		stay, err := s.Store.GetNamedStay(r.Context(), pid, selection.NamedStayID)
		if err != nil || stay == nil {
			WriteError(w, http.StatusConflict, "selected stay changed; upload a fresh preview")
			return
		}
		if stay.Status != store.NamedStayStatusActive || stay.StayType != store.StayTypeBookingCom || stay.CheckInDate != pending.CheckIn || stay.CheckOutDate != pending.CheckOut || stay.DisplayName != offered.DisplayName {
			WriteError(w, http.StatusConflict, "selected stay changed; upload a fresh preview")
			return
		}
		entry := pending.Entry
		entry.NamedStayMatch = stay
		entry.MatchBasis = "manual exact dates"
		entry.StayBeforeName, entry.StayCheckIn, entry.StayCheckOut = stay.DisplayName, stay.CheckInDate, stay.CheckOutDate
		name := strings.TrimSpace(pending.GuestName)
		entry.StayNameChanged = preview.SourceType == statements.SourcePayout && name != "" && name != stay.DisplayName
		plan = append(plan, entry)
		selected[pending.Line] = true
	}
	for line := range selectionByLine {
		if !selected[line] {
			WriteError(w, http.StatusBadRequest, "stay selection line is not pending")
			return
		}
	}
	for _, entry := range plan {
		currentBooking, currentID, err := s.Store.FinanceBookingByReference(r.Context(), pid, "booking_com", entry.Reference)
		if err != nil {
			WriteError(w, http.StatusConflict, "finance booking changed; upload a fresh preview")
			return
		}
		if currentID != entry.ExistingID {
			WriteError(w, http.StatusConflict, "finance booking changed; upload a fresh preview")
			return
		}
		currentOutcome := statements.Merge(currentBooking, entry.Input)
		if currentOutcome.Action != entry.Action || !reflect.DeepEqual(currentOutcome.Result, entry.Result) {
			WriteError(w, http.StatusConflict, "finance booking changed; upload a fresh preview")
			return
		}
		if entry.ExistingID > 0 {
			currentStay, err := s.Store.FinanceBookingNamedStay(r.Context(), pid, entry.ExistingID)
			if err != nil || (entry.NamedStayMatch == nil) != (currentStay == nil) || (entry.NamedStayMatch != nil && (currentStay == nil || currentStay.ID != entry.NamedStayMatch.ID)) {
				WriteError(w, http.StatusConflict, "finance booking association changed; upload a fresh preview")
				return
			}
		}
		if preview.SourceType != statements.SourcePayout || entry.NamedStayMatch == nil {
			if preview.SourceType == statements.SourceStatement && entry.ExistingID == 0 && entry.NamedStayMatch != nil {
				current, err := s.Store.GetNamedStay(r.Context(), pid, entry.NamedStayMatch.ID)
				if err != nil || current == nil || current.Status != store.NamedStayStatusActive || current.StayType != store.StayTypeBookingCom || current.CheckInDate != entry.StayCheckIn || current.CheckOutDate != entry.StayCheckOut {
					WriteError(w, http.StatusConflict, "matched stay changed; upload a fresh preview")
					return
				}
				if entry.MatchBasis == "unique exact dates" || entry.MatchBasis == "manual exact dates" {
					candidates, err := s.Store.ListNamedStaysForFinanceExactDates(r.Context(), pid, entry.StayCheckIn, entry.StayCheckOut)
					if err != nil || (entry.MatchBasis == "unique exact dates" && (len(candidates) != 1 || candidates[0].ID != entry.NamedStayMatch.ID)) {
						WriteError(w, http.StatusConflict, "matched stay candidates changed; upload a fresh preview")
						return
					}
				}
			}
			continue
		}
		current, err := s.Store.GetNamedStay(r.Context(), pid, entry.NamedStayMatch.ID)
		if err != nil || current == nil || current.DisplayName != entry.StayBeforeName || current.CheckInDate != entry.StayCheckIn || current.CheckOutDate != entry.StayCheckOut || current.Status != store.NamedStayStatusActive || current.StayType != entry.NamedStayMatch.StayType {
			WriteError(w, http.StatusConflict, "matched stay changed; upload a fresh preview")
			return
		}
		if entry.MatchBasis == "unique exact dates" {
			candidates, err := s.Store.ListNamedStaysForFinanceExactDates(r.Context(), pid, entry.StayCheckIn, entry.StayCheckOut)
			if err != nil || len(candidates) != 1 || candidates[0].ID != entry.NamedStayMatch.ID {
				WriteError(w, http.StatusConflict, "matched stay candidates changed; upload a fresh preview")
				return
			}
		}
	}
	loc, err := time.LoadLocation(prop.Timezone)
	if err != nil {
		loc = time.UTC
	}

	imp := &store.FinanceImport{
		PropertyID:                pid,
		SourceType:                string(preview.SourceType),
		SourceChannel:             "booking_com",
		HotelID:                   sql.NullString{String: preview.HotelID, Valid: preview.HotelID != ""},
		PeriodStart:               sql.NullString{String: preview.PeriodStart, Valid: preview.PeriodStart != ""},
		PeriodEnd:                 sql.NullString{String: preview.PeriodEnd, Valid: preview.PeriodEnd != ""},
		FileSHA256:                sql.NullString{String: preview.FileSHA256, Valid: preview.FileSHA256 != ""},
		RowCountSkippedOtherHotel: len(preview.Skipped),
		RowCountRejected:          len(preview.Rejected) + len(preview.Pending) - len(selected),
	}
	if preview.UserID > 0 {
		imp.UploadedByUserID = sql.NullInt64{Int64: preview.UserID, Valid: true}
	}
	importID, err := s.Store.CreateFinanceImport(r.Context(), imp)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "audit row")
		return
	}

	for _, entry := range plan {
		var namedStayID int64
		if entry.NamedStayMatch != nil {
			namedStayID = entry.NamedStayMatch.ID
		}
		rowCtx := r.Context()
		var rowTx *sql.Tx
		if preview.SourceType == statements.SourcePayout || preview.SourceType == statements.SourceStatement {
			rowTx, err = s.Store.DB.BeginTx(rowCtx, nil)
			if err != nil {
				imp.RowCountRejected++
				continue
			}
			rowCtx = store.ContextWithTransaction(rowCtx, rowTx)
		}
		bookingID, err := s.Store.UpsertFinanceBookingFromCanonical(rowCtx, pid, entry.ExistingID, namedStayID, entry.Result)
		if err != nil {
			if rowTx != nil {
				_ = rowTx.Rollback()
			}
			imp.RowCountRejected++
			continue
		}
		if (entry.StatusChanged || (preview.SourceType == statements.SourceStatement && entry.ExistingID == 0)) && strings.EqualFold(strDeref(entry.Result.Status), "CANCELLED") {
			if err := s.Store.MarkNamedStayFinanceReviewForBooking(rowCtx, pid, bookingID, "finance_status_cancelled"); err != nil {
				if rowTx != nil {
					_ = rowTx.Rollback()
				}
				imp.RowCountRejected++
				continue
			}
		}
		if entry.Action != statements.ActionUnchanged && len(entry.ChangedFields) > 0 {
			fieldsJSON, _ := json.Marshal(entry.ChangedFields)
			if err := s.Store.CreateFinanceBookingMerge(rowCtx, &store.FinanceBookingMerge{
				BookingID:         bookingID,
				ImportID:          importID,
				SourceType:        string(preview.SourceType),
				ChangedFieldsJSON: sql.NullString{String: string(fieldsJSON), Valid: true},
			}); err != nil {
				if rowTx != nil {
					_ = rowTx.Rollback()
				}
				imp.RowCountRejected++
				continue
			}
		}
		if preview.SourceType == statements.SourcePayout {
			if err := s.commitPayoutBookingSideEffects(rowCtx, pid, bookingID, entry, loc, preview.UserID); err != nil {
				if rowTx != nil {
					_ = rowTx.Rollback()
				}
				imp.RowCountRejected++
				continue
			}
			if err := rowTx.Commit(); err != nil {
				imp.RowCountRejected++
				continue
			}
		} else if preview.SourceType == statements.SourceStatement {
			if err := s.Store.UpsertFinanceStatementEvidence(rowCtx, pid, importID, entry.Input); err != nil {
				_ = rowTx.Rollback()
				imp.RowCountRejected++
				continue
			}
			if err := rowTx.Commit(); err != nil {
				imp.RowCountRejected++
				continue
			}
		}
		switch entry.Action {
		case statements.ActionInsert:
			imp.RowCountInserted++
		case statements.ActionUpdate:
			imp.RowCountUpdated++
		case statements.ActionUnchanged:
			imp.RowCountUnchanged++
		}
	}
	plannedRefs := make(map[string]bool, len(plan))
	for _, entry := range plan {
		plannedRefs[entry.Reference] = true
	}
	for _, row := range preview.Evidence {
		if plannedRefs[row.ReferenceNumber] {
			continue
		}
		rowTx, txErr := s.Store.DB.BeginTx(r.Context(), nil)
		if txErr == nil {
			txCtx := store.ContextWithTransaction(r.Context(), rowTx)
			txErr = s.Store.UpsertFinanceStatementEvidence(txCtx, pid, importID, row)
			if txErr == nil {
				txErr = rowTx.Commit()
			} else {
				_ = rowTx.Rollback()
			}
		}
		if txErr != nil {
			imp.RowCountRejected++
			continue
		}
		if strings.EqualFold(strings.TrimSpace(row.Status), "CANCELLED") {
			imp.RowCountSkippedCancellations++
		}
	}
	imp.RowCountTotal = imp.RowCountInserted + imp.RowCountUpdated + imp.RowCountUnchanged + imp.RowCountSkippedOtherHotel + imp.RowCountSkippedCancellations + imp.RowCountRejected

	// Update audit counts (re-write).
	if err := s.Store.UpdateFinanceImportCounts(r.Context(), importID, imp); err != nil {
		WriteError(w, http.StatusInternalServerError, "update import audit counts")
		return
	}

	evictFinancePreview(req.PreviewToken)
	s.audit(r, actor, "finance_import_commit", "property", strconv.FormatInt(pid, 10), "success")
	WriteJSON(w, http.StatusOK, financeImportCommitResponse{
		OK:                           true,
		ImportID:                     importID,
		SourceType:                   string(preview.SourceType),
		RowCountTotal:                imp.RowCountTotal,
		RowCountInserted:             imp.RowCountInserted,
		RowCountUpdated:              imp.RowCountUpdated,
		RowCountUnchanged:            imp.RowCountUnchanged,
		RowCountSkipped:              imp.RowCountSkippedOtherHotel,
		RowCountRejected:             imp.RowCountRejected,
		RowCountSkippedCancellations: imp.RowCountSkippedCancellations,
	})
}

// commitPayoutBookingSideEffects handles the cash-basis bookkeeping that
// the legacy importFinanceBookingPayouts handler used to do inline:
// a finance_transactions row plus a named-stay mapping. The booking
// row itself was already written by UpsertFinanceBookingFromCanonical.
func (s *Server) commitPayoutBookingSideEffects(ctx context.Context, propertyID, bookingID int64, entry financePreviewPlanEntry, loc *time.Location, userID int64) error {
	// Existing rows may still need the match applied. Inserts already carry
	// this identity in their canonical write.
	stay := entry.NamedStayMatch
	if stay != nil {
		if err := s.Store.LinkBookingToNamedStay(ctx, propertyID, bookingID, stay.ID); err != nil {
			return err
		}
		if entry.StayNameChanged {
			if err := s.Store.UpdateFinanceMatchedStayName(ctx, propertyID, stay.ID, strDeref(entry.Result.GuestName), userID); err != nil {
				return err
			}
		}
	}
	// Upsert the linked finance_transactions row.
	if entry.NetCents != 0 && entry.BookingIncomeID > 0 {
		txDate := entry.PayoutDate
		if txDate.IsZero() {
			txDate = time.Now().UTC()
		}
		if err := s.Store.UpsertBookingFinanceTransaction(ctx, propertyID, bookingID, entry.Reference, entry.NetCents, txDate, entry.BookingIncomeID, entry.PayoutID); err != nil {
			return err
		}
	}
	return nil
}

// ---- imports list ----------------------------------------------------------

func (s *Server) listFinanceImports(w http.ResponseWriter, r *http.Request) {
	_, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Finance, permissions.LevelRead)
	if !ok {
		return
	}
	limit := 50
	if v := strings.TrimSpace(r.URL.Query().Get("limit")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	rows, err := s.Store.ListFinanceImports(r.Context(), pid, limit)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "list imports")
		return
	}
	out := make([]financeImportListItem, 0, len(rows))
	for _, imp := range rows {
		hotelID := ""
		if imp.HotelID.Valid {
			hotelID = imp.HotelID.String
		}
		out = append(out, financeImportListItem{
			ID:                           imp.ID,
			SourceType:                   imp.SourceType,
			SourceChannel:                imp.SourceChannel,
			HotelID:                      hotelID,
			UploadedAt:                   imp.UploadedAt.UTC().Format(time.RFC3339),
			FileSHA256:                   ifString(imp.FileSHA256),
			RowCountTotal:                imp.RowCountTotal,
			RowCountInserted:             imp.RowCountInserted,
			RowCountUpdated:              imp.RowCountUpdated,
			RowCountUnchanged:            imp.RowCountUnchanged,
			RowCountSkippedOtherHotel:    imp.RowCountSkippedOtherHotel,
			RowCountSkippedCancellations: imp.RowCountSkippedCancellations,
			RowCountRejected:             imp.RowCountRejected,
		})
	}
	WriteJSON(w, http.StatusOK, map[string]any{"items": out})
}

func ifString(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return v.String
}

// derivePeriod returns the property-TZ "min"/"max" YYYY-MM-DD pair that
// covers the parsed rows. Uses arrival/departure for statement rows and
// payout date for payout rows.
func derivePeriod(rows []statements.Row, src statements.SourceType) (string, string) {
	if len(rows) == 0 {
		return "", ""
	}
	dates := make([]string, 0, len(rows))
	for _, r := range rows {
		switch src {
		case statements.SourcePayout:
			if !r.PayoutDate.IsZero() {
				dates = append(dates, r.PayoutDate.UTC().Format("2006-01-02"))
			}
		case statements.SourceStatement:
			if r.CheckInDate != "" {
				dates = append(dates, r.CheckInDate)
			}
		}
	}
	if len(dates) == 0 {
		return "", ""
	}
	sort.Strings(dates)
	return dates[0], dates[len(dates)-1]
}
