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
	PropertyID  int64
	UserID      int64
	SourceType  statements.SourceType
	HotelID     string
	FileSHA256  string
	PeriodStart string
	PeriodEnd   string
	Plan        []financePreviewPlanEntry
	Rejected    []statements.Rejection
	Skipped     []financePreviewSkippedEntry
	CreatedAt   time.Time
}

type financePreviewPlanEntry struct {
	Action          statements.MergeAction
	Reference       string
	ExistingID      int64
	Result          statements.CanonicalBooking
	StatusChanged   bool
	ChangedFields   []string
	OccupancyMatch  *store.Occupancy // populated for payout inserts
	BookingIncomeID int64            // resolved booking_income category for payouts
	NetCents        int              // payout net (used to upsert finance_transactions)
	PayoutDate      time.Time
	PayoutID        string
}

type financePreviewSkippedEntry struct {
	Reference string `json:"reference"`
	Reason    string `json:"reason"`
	HotelID   string `json:"hotel_id,omitempty"`
}

var (
	financePreviewCache = sync.Map{}
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
}

type financeImportPreviewUpdate struct {
	Reference     string                  `json:"reference"`
	GuestName     string                  `json:"guest_name,omitempty"`
	StatusChanged bool                    `json:"status_changed,omitempty"`
	Changes       []financePreviewFieldKV `json:"changes,omitempty"`
}

type financePreviewFieldKV struct {
	Field string `json:"field"`
}

type financeImportPreviewResponse struct {
	OK                  bool                         `json:"ok"`
	PreviewToken        string                       `json:"preview_token"`
	SourceType          string                       `json:"source_type"`
	HotelID             string                       `json:"hotel_id,omitempty"`
	FileSHA256          string                       `json:"file_sha256"`
	PeriodStart         string                       `json:"period_start,omitempty"`
	PeriodEnd           string                       `json:"period_end,omitempty"`
	DuplicateOfImportID *int64                       `json:"duplicate_of_import_id,omitempty"`
	Inserts             []financeImportPreviewInsert `json:"inserts"`
	Updates             []financeImportPreviewUpdate `json:"updates"`
	UnchangedCount      int                          `json:"unchanged_count"`
	Skipped             []financePreviewSkippedEntry `json:"skipped_other_hotel"`
	Rejected            []statements.Rejection       `json:"rejected"`
}

type financeImportCommitRequest struct {
	PreviewToken string `json:"preview_token"`
}

type financeImportCommitResponse struct {
	OK                bool   `json:"ok"`
	ImportID          int64  `json:"import_id"`
	SourceType        string `json:"source_type"`
	RowCountTotal     int    `json:"row_count_total"`
	RowCountInserted  int    `json:"row_count_inserted"`
	RowCountUpdated   int    `json:"row_count_updated"`
	RowCountUnchanged int    `json:"row_count_unchanged"`
	RowCountSkipped   int    `json:"row_count_skipped_other_hotel"`
	RowCountRejected  int    `json:"row_count_rejected"`
}

type financeImportListItem struct {
	ID                        int64  `json:"id"`
	SourceType                string `json:"source_type"`
	SourceChannel             string `json:"source_channel"`
	HotelID                   string `json:"hotel_id,omitempty"`
	UploadedAt                string `json:"uploaded_at"`
	FileSHA256                string `json:"file_sha256,omitempty"`
	RowCountTotal             int    `json:"row_count_total"`
	RowCountInserted          int    `json:"row_count_inserted"`
	RowCountUpdated           int    `json:"row_count_updated"`
	RowCountUnchanged         int    `json:"row_count_unchanged"`
	RowCountSkippedOtherHotel int    `json:"row_count_skipped_other_hotel"`
	RowCountRejected          int    `json:"row_count_rejected"`
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
		existing, existingID, err := s.Store.FinanceBookingByReference(r.Context(), pid, "booking_com", row.ReferenceNumber)
		if err != nil && err != sql.ErrNoRows {
			WriteError(w, http.StatusInternalServerError, "lookup booking")
			return
		}
		outcome := statements.Merge(existing, row)
		entry := financePreviewPlanEntry{
			Action:        outcome.Action,
			Reference:     row.ReferenceNumber,
			ExistingID:    existingID,
			Result:        outcome.Result,
			StatusChanged: outcome.StatusChanged,
			ChangedFields: outcome.Changed,
		}
		if parsed.Source == statements.SourcePayout {
			entry.BookingIncomeID = bookingIncomeID
			entry.NetCents = row.NetCents
			entry.PayoutDate = row.PayoutDate
			entry.PayoutID = row.PayoutID
			if existingID == 0 {
				occ, _ := s.Store.FindOccupancyForStayDates(r.Context(), pid, row.CheckInDate, row.CheckOutDate, loc)
				entry.OccupancyMatch = occ
			}
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
		OK:           true,
		PreviewToken: token,
		SourceType:   string(p.SourceType),
		HotelID:      p.HotelID,
		FileSHA256:   p.FileSHA256,
		PeriodStart:  p.PeriodStart,
		PeriodEnd:    p.PeriodEnd,
		Skipped:      p.Skipped,
		Rejected:     p.Rejected,
	}
	for _, entry := range p.Plan {
		switch entry.Action {
		case statements.ActionInsert:
			ins := financeImportPreviewInsert{
				Reference:     entry.Reference,
				GuestName:     strDeref(entry.Result.GuestName),
				CheckInDate:   strDeref(entry.Result.CheckInDate),
				CheckOutDate:  strDeref(entry.Result.CheckOutDate),
				Status:        strDeref(entry.Result.Status),
				StatusChanged: entry.StatusChanged,
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
			}
			for _, f := range entry.ChangedFields {
				upd.Changes = append(upd.Changes, financePreviewFieldKV{Field: f})
			}
			resp.Updates = append(resp.Updates, upd)
		case statements.ActionUnchanged:
			resp.UnchangedCount++
		}
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
		RowCountRejected:          len(preview.Rejected),
	}
	if preview.UserID > 0 {
		imp.UploadedByUserID = sql.NullInt64{Int64: preview.UserID, Valid: true}
	}
	importID, err := s.Store.CreateFinanceImport(r.Context(), imp)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "audit row")
		return
	}

	for _, entry := range preview.Plan {
		bookingID, err := s.Store.UpsertFinanceBookingFromCanonical(r.Context(), pid, entry.ExistingID, entry.Result)
		if err != nil {
			imp.RowCountRejected++
			continue
		}
		switch entry.Action {
		case statements.ActionInsert:
			imp.RowCountInserted++
		case statements.ActionUpdate:
			imp.RowCountUpdated++
		case statements.ActionUnchanged:
			imp.RowCountUnchanged++
		}
		if entry.StatusChanged && strings.EqualFold(strDeref(entry.Result.Status), "CANCELLED") {
			_, _ = s.Store.CancelOccupancyForBooking(r.Context(), bookingID)
		}
		if entry.Action != statements.ActionUnchanged && len(entry.ChangedFields) > 0 {
			fieldsJSON, _ := json.Marshal(entry.ChangedFields)
			_ = s.Store.CreateFinanceBookingMerge(r.Context(), &store.FinanceBookingMerge{
				BookingID:         bookingID,
				ImportID:          importID,
				SourceType:        string(preview.SourceType),
				ChangedFieldsJSON: sql.NullString{String: string(fieldsJSON), Valid: true},
			})
		}
		if preview.SourceType == statements.SourcePayout {
			s.commitPayoutBookingSideEffects(r.Context(), pid, bookingID, entry, loc)
		} else if preview.SourceType == statements.SourceStatement {
			s.commitStatementBookingSideEffects(r.Context(), pid, bookingID, entry, loc)
		}
	}
	imp.RowCountTotal = imp.RowCountInserted + imp.RowCountUpdated + imp.RowCountUnchanged + imp.RowCountSkippedOtherHotel + imp.RowCountRejected

	// Update audit counts (re-write).
	_ = s.Store.UpdateFinanceImportCounts(r.Context(), importID, imp)

	evictFinancePreview(req.PreviewToken)
	s.audit(r, actor, "finance_import_commit", "property", strconv.FormatInt(pid, 10), "success")
	WriteJSON(w, http.StatusOK, financeImportCommitResponse{
		OK:                true,
		ImportID:          importID,
		SourceType:        string(preview.SourceType),
		RowCountTotal:     imp.RowCountTotal,
		RowCountInserted:  imp.RowCountInserted,
		RowCountUpdated:   imp.RowCountUpdated,
		RowCountUnchanged: imp.RowCountUnchanged,
		RowCountSkipped:   imp.RowCountSkippedOtherHotel,
		RowCountRejected:  imp.RowCountRejected,
	})
}

// commitPayoutBookingSideEffects handles the cash-basis bookkeeping that
// the legacy importFinanceBookingPayouts handler used to do inline:
// a finance_transactions row plus an occupancy mapping. The booking
// row itself was already written by UpsertFinanceBookingFromCanonical.
func (s *Server) commitPayoutBookingSideEffects(ctx context.Context, propertyID, bookingID int64, entry financePreviewPlanEntry, loc *time.Location) {
	// Resolve / create occupancy mapping for new payouts.
	if entry.Action == statements.ActionInsert {
		occ := entry.OccupancyMatch
		if occ != nil {
			_ = s.Store.LinkBookingToOccupancy(ctx, propertyID, entry.Reference, occ.ID, bookingID)
		}
	}
	// Upsert the linked finance_transactions row.
	if entry.NetCents != 0 && entry.BookingIncomeID > 0 {
		txDate := entry.PayoutDate
		if txDate.IsZero() {
			txDate = time.Now().UTC()
		}
		_ = s.Store.UpsertBookingFinanceTransaction(ctx, propertyID, bookingID, entry.Reference, entry.NetCents, txDate, entry.BookingIncomeID, entry.PayoutID)
	}
}

func (s *Server) commitStatementBookingSideEffects(ctx context.Context, propertyID, bookingID int64, entry financePreviewPlanEntry, loc *time.Location) {
	if bookingID <= 0 || strings.EqualFold(strDeref(entry.Result.Status), "CANCELLED") {
		return
	}
	checkIn := strDeref(entry.Result.CheckInDate)
	checkOut := strDeref(entry.Result.CheckOutDate)
	guest := strDeref(entry.Result.GuestName)
	occ, err := s.Store.FindOrCreateOccupancyForStatementStayDates(ctx, propertyID, entry.Reference, checkIn, checkOut, guest, loc)
	if err != nil || occ == nil {
		return
	}
	if strings.TrimSpace(guest) != "" {
		_ = s.Store.UpdateOccupancyGuestDisplayName(ctx, propertyID, occ.ID, &guest)
	}
	_ = s.Store.LinkBookingToOccupancy(ctx, propertyID, entry.Reference, occ.ID, bookingID)
	_ = s.Store.SupersedeGenericICSBlocksForFinanceStayDates(ctx, propertyID, checkIn, checkOut, loc, occ.ID)
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
			ID:                        imp.ID,
			SourceType:                imp.SourceType,
			SourceChannel:             imp.SourceChannel,
			HotelID:                   hotelID,
			UploadedAt:                imp.UploadedAt.UTC().Format(time.RFC3339),
			FileSHA256:                ifString(imp.FileSHA256),
			RowCountTotal:             imp.RowCountTotal,
			RowCountInserted:          imp.RowCountInserted,
			RowCountUpdated:           imp.RowCountUpdated,
			RowCountUnchanged:         imp.RowCountUnchanged,
			RowCountSkippedOtherHotel: imp.RowCountSkippedOtherHotel,
			RowCountRejected:          imp.RowCountRejected,
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
