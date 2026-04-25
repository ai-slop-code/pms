package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"pms/backend/internal/invoicepdf"
	"pms/backend/internal/permissions"
	"pms/backend/internal/store"
)

type invoicePartySnapshot struct {
	Name         string `json:"name"`
	CompanyName  string `json:"company_name,omitempty"`
	AddressLine1 string `json:"address_line_1,omitempty"`
	City         string `json:"city,omitempty"`
	PostalCode   string `json:"postal_code,omitempty"`
	Country      string `json:"country,omitempty"`
	ICO          string `json:"ico,omitempty"`
	DIC          string `json:"dic,omitempty"`
	VATID        string `json:"vat_id,omitempty"`
}

type invoiceFileRow struct {
	ID            int64  `json:"id"`
	Version       int    `json:"version"`
	FilePath      string `json:"file_path"`
	FileSizeBytes int64  `json:"file_size_bytes"`
	CreatedAt     string `json:"created_at"`
}

type invoiceRow struct {
	ID                  int64                `json:"id"`
	OccupancyID         *int64               `json:"occupancy_id"`
	BookingPayoutID     *int64               `json:"booking_payout_id"`
	InvoiceNumber       string               `json:"invoice_number"`
	SequenceYear        int                  `json:"sequence_year"`
	SequenceValue       int                  `json:"sequence_value"`
	Language            string               `json:"language"`
	IssueDate           string               `json:"issue_date"`
	TaxableSupplyDate   string               `json:"taxable_supply_date"`
	DueDate             string               `json:"due_date"`
	StayStartDate       string               `json:"stay_start_date"`
	StayEndDate         string               `json:"stay_end_date"`
	Supplier            invoicePartySnapshot `json:"supplier"`
	Customer            invoicePartySnapshot `json:"customer"`
	AmountTotalCents    int                  `json:"amount_total_cents"`
	Currency            string               `json:"currency"`
	PaymentStatus       string               `json:"payment_status"`
	PaymentNote         string               `json:"payment_note"`
	Version             int                  `json:"version"`
	LatestFilePath      *string              `json:"latest_file_path,omitempty"`
	LatestFileSizeBytes *int64               `json:"latest_file_size_bytes,omitempty"`
	LatestFileCreatedAt *string              `json:"latest_file_created_at,omitempty"`
	DownloadURL         string               `json:"download_url"`
	Files               *[]invoiceFileRow    `json:"files,omitempty"`
	CreatedAt           string               `json:"created_at"`
	UpdatedAt           string               `json:"updated_at"`
}

type invoicesResponse struct {
	Invoices []invoiceRow `json:"invoices"`
}

type invoiceResponse struct {
	Invoice invoiceRow `json:"invoice"`
}

type invoiceSequencePreviewResponse struct {
	Year          int    `json:"year"`
	SequenceValue int    `json:"sequence_value"`
	InvoiceNumber string `json:"invoice_number"`
}

type invoiceRequestBody struct {
	OccupancyID            *int64                `json:"occupancy_id"`
	BookingPayoutID        *int64                `json:"booking_payout_id"`
	BookingPayoutReference *string               `json:"booking_payout_reference"`
	Language               string                `json:"language"`
	IssueDate         string                `json:"issue_date"`
	TaxableSupplyDate string                `json:"taxable_supply_date"`
	DueDate           string                `json:"due_date"`
	StayStartDate     string                `json:"stay_start_date"`
	StayEndDate       string                `json:"stay_end_date"`
	AmountTotalCents  *int                  `json:"amount_total_cents"`
	PaymentNote       *string               `json:"payment_note"`
	Customer          *invoicePartySnapshot `json:"customer"`
}

type invoiceOccupancyCandidate struct {
	ID              int64   `json:"id"`
	StartAt         string  `json:"start_at"`
	EndAt           string  `json:"end_at"`
	Status          string  `json:"status"`
	Summary         string  `json:"summary"`
	GuestDisplayName *string `json:"guest_display_name,omitempty"`
	HasPayoutData   bool    `json:"has_payout_data"`
}

type invoiceOccupancyCandidatesResponse struct {
	Occupancies []invoiceOccupancyCandidate `json:"occupancies"`
}

func (s *Server) listInvoiceOccupancyCandidates(w http.ResponseWriter, r *http.Request) {
	_, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Invoices, permissions.LevelRead)
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
	month := strings.TrimSpace(r.URL.Query().Get("month"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	list, err := s.Store.ListOccupancies(r.Context(), pid, month, loc, nil, limit, offset)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	ids := make([]int64, 0, len(list))
	for _, o := range list {
		ids = append(ids, o.ID)
	}
	payoutMap, _ := s.Store.OccupancyIDsWithPayoutData(r.Context(), pid, ids)
	out := make([]invoiceOccupancyCandidate, 0, len(list))
	for _, o := range list {
		summary := ""
		if o.RawSummary.Valid {
			summary = o.RawSummary.String
		}
		var guest *string
		if o.GuestDisplayName.Valid && strings.TrimSpace(o.GuestDisplayName.String) != "" {
			g := strings.TrimSpace(o.GuestDisplayName.String)
			guest = &g
		}
		out = append(out, invoiceOccupancyCandidate{
			ID:               o.ID,
			StartAt:          o.StartAt.UTC().Format(time.RFC3339),
			EndAt:            o.EndAt.UTC().Format(time.RFC3339),
			Status:           o.Status,
			Summary:          summary,
			GuestDisplayName: guest,
			HasPayoutData:    payoutMap[o.ID],
		})
	}
	WriteJSON(w, http.StatusOK, invoiceOccupancyCandidatesResponse{Occupancies: out})
}

func (s *Server) listInvoicePayoutLinkCandidates(w http.ResponseWriter, r *http.Request) {
	_, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Invoices, permissions.LevelRead)
	if !ok {
		return
	}
	propName := ""
	if p, err := s.Store.GetProperty(r.Context(), pid); err == nil {
		propName = strings.TrimSpace(p.Name)
	}
	month := strings.TrimSpace(r.URL.Query().Get("month"))
	if month != "" {
		if _, _, err := parseFinanceMonth(month); err != nil {
			WriteError(w, http.StatusBadRequest, "month must be YYYY-MM")
			return
		}
	}
	mappedOnly := true
	if raw := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("mapped_only"))); raw == "0" || raw == "false" || raw == "no" {
		mappedOnly = false
	}
	mo := mappedOnly
	rows, err := s.Store.ListBookingPayouts(r.Context(), pid, month, &mo)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]financeBookingPayoutRow, 0, len(rows))
	for _, rr := range rows {
		out = append(out, financeBookingPayoutRow{
			ID:                     rr.ID,
			ReferenceNumber:        rr.ReferenceNumber,
			PayoutID:               nullStringPtr(rr.PayoutID),
			RowType:                nullStringPtr(rr.RowType),
			CheckInDate:            nullStringPtr(rr.CheckInDate),
			CheckOutDate:           nullStringPtr(rr.CheckOutDate),
			GuestName:              fixCSVMojibakePtr(nullStringPtr(rr.GuestName)),
			HostName:               bookingPayoutHostName(rr.RawRowJSON),
			PayoutSummary:          financeBookingPayoutSummary(rr.RawRowJSON, rr.GuestName, rr.OccupancySummary, propName),
			ReservationStatus:      nullStringPtr(rr.ReservationStatus),
			Currency:               nullStringPtr(rr.Currency),
			PaymentStatus:          nullStringPtr(rr.PaymentStatus),
			AmountCents:            nullInt64Ptr(rr.AmountCents),
			CommissionCents:        nullInt64Ptr(rr.CommissionCents),
			PaymentServiceFeeCents: nullInt64Ptr(rr.PaymentServiceFeeCents),
			NetCents:               rr.NetCents,
			PayoutDate:             rr.PayoutDate.UTC().Format(time.RFC3339),
			TransactionID:          nullInt64Ptr(rr.TransactionID),
			OccupancyID:            nullInt64Ptr(rr.OccupancyID),
			OccupancyStartAt:       nullTimePtr(rr.OccupancyStartAt),
			OccupancyEndAt:         nullTimePtr(rr.OccupancyEndAt),
			OccupancySummary:       fixCSVMojibakePtr(nullStringPtr(rr.OccupancySummary)),
			LinkedInvoiceID:        nullInt64Ptr(rr.LinkedInvoiceID),
		})
	}
	mappedRaw := "false"
	if mappedOnly {
		mappedRaw = "true"
	}
	WriteJSON(w, http.StatusOK, financeBookingPayoutsResponse{Month: month, MappedOnly: mappedRaw, Payouts: out})
}

func (s *Server) listInvoices(w http.ResponseWriter, r *http.Request) {
	_, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Invoices, permissions.LevelRead)
	if !ok {
		return
	}
	rows, err := s.Store.ListInvoices(r.Context(), pid)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]invoiceRow, 0, len(rows))
	for _, row := range rows {
		item, err := invoiceToRow(pid, row, nil)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "invoice snapshot error")
			return
		}
		out = append(out, item)
	}
	WriteJSON(w, http.StatusOK, invoicesResponse{Invoices: out})
}

func (s *Server) getInvoice(w http.ResponseWriter, r *http.Request) {
	_, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Invoices, permissions.LevelRead)
	if !ok {
		return
	}
	invoiceID, ok := parseInvoiceIDParam(w, r)
	if !ok {
		return
	}
	row, err := s.Store.GetInvoiceByID(r.Context(), pid, invoiceID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "invoice not found")
		return
	}
	files, err := s.Store.ListInvoiceFiles(r.Context(), pid, invoiceID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	item, err := invoiceToRow(pid, *row, files)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "invoice snapshot error")
		return
	}
	WriteJSON(w, http.StatusOK, invoiceResponse{Invoice: item})
}

func (s *Server) previewNextInvoiceSequence(w http.ResponseWriter, r *http.Request) {
	_, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Invoices, permissions.LevelRead)
	if !ok {
		return
	}
	year := time.Now().UTC().Year()
	if raw := strings.TrimSpace(r.URL.Query().Get("year")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 2000 || parsed > 3000 {
			WriteError(w, http.StatusBadRequest, "year must be a valid number")
			return
		}
		year = parsed
	}
	number, seq, err := s.Store.PreviewNextInvoiceNumber(r.Context(), pid, year)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	WriteJSON(w, http.StatusOK, invoiceSequencePreviewResponse{
		Year:          year,
		SequenceValue: seq,
		InvoiceNumber: number,
	})
}

func (s *Server) postInvoice(w http.ResponseWriter, r *http.Request) {
	actor, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Invoices, permissions.LevelWrite)
	if !ok {
		return
	}
	var body invoiceRequestBody
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	row, err := s.parseInvoiceCreateRequest(r, pid, &body, actor)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := s.Store.CreateInvoice(r.Context(), row)
	if err != nil {
		WriteInvoiceStoreError(w, err)
		return
	}
	if _, err := s.renderAndAttachInvoiceVersion(r.Context(), pid, created, created.Version); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to generate invoice pdf")
		return
	}
	refreshed, err := s.Store.GetInvoiceByID(r.Context(), pid, created.ID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	item, err := invoiceToRow(pid, *refreshed, nil)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "invoice snapshot error")
		return
	}
	s.audit(r, actor, "invoice_create", "property", strconv.FormatInt(pid, 10), "success")
	WriteJSON(w, http.StatusCreated, invoiceResponse{Invoice: item})
}

func (s *Server) patchInvoice(w http.ResponseWriter, r *http.Request) {
	actor, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Invoices, permissions.LevelWrite)
	if !ok {
		return
	}
	invoiceID, ok := parseInvoiceIDParam(w, r)
	if !ok {
		return
	}
	current, err := s.Store.GetInvoiceByID(r.Context(), pid, invoiceID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "invoice not found")
		return
	}
	var body invoiceRequestBody
	if err := ReadJSON(r, &body); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid json")
		return
	}
	row, err := s.parseInvoicePatchRequest(r, current, &body)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := s.Store.UpdateInvoice(r.Context(), row)
	if err != nil {
		WriteInvoiceStoreError(w, err)
		return
	}
	if _, err := s.renderAndAttachInvoiceVersion(r.Context(), pid, updated, updated.Version+1); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to generate invoice pdf")
		return
	}
	refreshed, err := s.Store.GetInvoiceByID(r.Context(), pid, invoiceID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	item, err := invoiceToRow(pid, *refreshed, nil)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "invoice snapshot error")
		return
	}
	s.audit(r, actor, "invoice_update", "property", strconv.FormatInt(pid, 10), "success")
	WriteJSON(w, http.StatusOK, invoiceResponse{Invoice: item})
}

func (s *Server) regenerateInvoice(w http.ResponseWriter, r *http.Request) {
	actor, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Invoices, permissions.LevelWrite)
	if !ok {
		return
	}
	if s.InvoiceRegenLimiter != nil {
		if !s.InvoiceRegenLimiter.Allow(fmt.Sprintf("invoice_regen:%d", actor.ID)) {
			w.Header().Set("Retry-After", "5")
			WriteError(w, http.StatusTooManyRequests, "too many regeneration requests")
			return
		}
	}
	invoiceID, ok := parseInvoiceIDParam(w, r)
	if !ok {
		return
	}
	row, err := s.Store.GetInvoiceByID(r.Context(), pid, invoiceID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "invoice not found")
		return
	}
	if _, err := s.renderAndAttachInvoiceVersion(r.Context(), pid, row, row.Version+1); err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to generate invoice pdf")
		return
	}
	refreshed, err := s.Store.GetInvoiceByID(r.Context(), pid, invoiceID)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "database error")
		return
	}
	item, err := invoiceToRow(pid, *refreshed, nil)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "invoice snapshot error")
		return
	}
	s.audit(r, actor, "invoice_regenerate", "property", strconv.FormatInt(pid, 10), "success")
	WriteJSON(w, http.StatusOK, invoiceResponse{Invoice: item})
}

func (s *Server) downloadInvoice(w http.ResponseWriter, r *http.Request) {
	actor, pid, ok := s.requirePropertyModuleAccess(w, r, permissions.Invoices, permissions.LevelRead)
	if !ok {
		return
	}
	invoiceID, ok := parseInvoiceIDParam(w, r)
	if !ok {
		return
	}
	row, err := s.Store.GetInvoiceByID(r.Context(), pid, invoiceID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "invoice not found")
		return
	}
	file, err := s.Store.GetLatestInvoiceFile(r.Context(), pid, invoiceID)
	if err != nil {
		WriteError(w, http.StatusNotFound, "invoice pdf not found")
		return
	}
	fullPath, err := s.resolveDataFilePath(file.FilePath)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "invalid invoice file")
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", sanitizedInvoiceFilename(row.InvoiceNumber, file.Version)))
	// Audit BEFORE streaming so a truncated write still leaves a record. We
	// can't reliably detect ServeFile errors after headers are flushed, so we
	// optimistically record "success" once authorization and file resolution
	// succeed. See PMS_11/T2.11.
	s.audit(r, actor, "invoice_download", "property", strconv.FormatInt(pid, 10), "success")
	http.ServeFile(w, r, fullPath)
}

func (s *Server) parseInvoiceCreateRequest(r *http.Request, propertyID int64, body *invoiceRequestBody, actor *store.User) (*store.Invoice, error) {
	property, err := s.Store.GetProperty(r.Context(), propertyID)
	if err != nil {
		return nil, fmt.Errorf("property not found")
	}
	profile, err := s.Store.GetPropertyProfile(r.Context(), propertyID)
	if err != nil {
		return nil, fmt.Errorf("property profile not found")
	}
	loc, err := time.LoadLocation(property.Timezone)
	if err != nil {
		loc = time.UTC
	}
	linkedPayout, err := s.resolveInvoiceBookingPayout(r.Context(), propertyID, body)
	if err != nil {
		return nil, err
	}
	applyInvoicePayoutPrefill(body, linkedPayout)
	if body.AmountTotalCents == nil {
		return nil, fmt.Errorf("amount_total_cents is required")
	}
	customer, err := normalizeInvoiceCustomer(body.Customer)
	if err != nil {
		return nil, err
	}
	supplier := defaultInvoiceSupplier(property, profile)
	return s.buildInvoiceRowFromBody(r, propertyID, nil, body, loc, supplier, customer, actor, linkedPayout)
}

func (s *Server) parseInvoicePatchRequest(r *http.Request, current *store.Invoice, body *invoiceRequestBody) (*store.Invoice, error) {
	property, err := s.Store.GetProperty(r.Context(), current.PropertyID)
	if err != nil {
		return nil, fmt.Errorf("property not found")
	}
	loc, err := time.LoadLocation(property.Timezone)
	if err != nil {
		loc = time.UTC
	}
	supplier, customer, err := decodeInvoiceSnapshots(current)
	if err != nil {
		return nil, fmt.Errorf("invalid stored invoice snapshot")
	}
	var linkedPayout *store.FinanceBookingPayout
	if !(body.BookingPayoutID != nil && *body.BookingPayoutID == 0) {
		linkedPayout, err = s.resolveInvoiceBookingPayout(r.Context(), current.PropertyID, body)
		if err != nil {
			return nil, err
		}
	}
	return s.buildInvoiceRowFromBody(r, current.PropertyID, current, body, loc, supplier, customer, nil, linkedPayout)
}

func (s *Server) buildInvoiceRowFromBody(r *http.Request, propertyID int64, current *store.Invoice, body *invoiceRequestBody, loc *time.Location, supplier, customer invoicePartySnapshot, actor *store.User, linkedPayout *store.FinanceBookingPayout) (*store.Invoice, error) {
	row := &store.Invoice{
		PropertyID:    propertyID,
		Language:      supplierLanguageDefault(current, body),
		Currency:      "EUR",
		PaymentStatus: "paid",
		PaymentNote:   "Already paid via Booking.com.",
	}
	if current != nil {
		*row = *current
	}
	if current == nil {
		row.PropertyID = propertyID
		row.Currency = "EUR"
		row.PaymentStatus = "paid"
		row.PaymentNote = "Already paid via Booking.com."
		if actor != nil {
			row.CreatedBy = sql.NullInt64{Int64: actor.ID, Valid: true}
		}
	}
	if body.OccupancyID != nil {
		if *body.OccupancyID <= 0 {
			row.OccupancyID = sql.NullInt64{}
		} else {
			if _, err := s.Store.GetOccupancyByID(r.Context(), propertyID, *body.OccupancyID); err != nil {
				return nil, fmt.Errorf("invalid occupancy_id")
			}
			row.OccupancyID = sql.NullInt64{Int64: *body.OccupancyID, Valid: true}
		}
	}
	if current != nil && body.BookingPayoutID != nil && *body.BookingPayoutID == 0 {
		row.FinanceBookingPayoutID = sql.NullInt64{}
	} else if linkedPayout != nil {
		row.FinanceBookingPayoutID = sql.NullInt64{Int64: linkedPayout.ID, Valid: true}
		if linkedPayout.OccupancyID.Valid {
			if row.OccupancyID.Valid && row.OccupancyID.Int64 != linkedPayout.OccupancyID.Int64 {
				return nil, fmt.Errorf("occupancy_id does not match booking payout stay")
			}
			if !row.OccupancyID.Valid {
				row.OccupancyID = linkedPayout.OccupancyID
			}
		}
	}
	if raw := strings.TrimSpace(body.Language); raw != "" {
		row.Language = strings.ToLower(raw)
	}
	if row.Language != "sk" && row.Language != "en" {
		return nil, fmt.Errorf("language must be sk or en")
	}
	if strings.TrimSpace(body.IssueDate) != "" {
		t, err := parseFinanceDate(body.IssueDate, loc)
		if err != nil {
			return nil, fmt.Errorf("issue_date must be RFC3339 or YYYY-MM-DD")
		}
		row.IssueDate = t
	}
	if row.IssueDate.IsZero() {
		return nil, fmt.Errorf("issue_date is required")
	}
	if strings.TrimSpace(body.TaxableSupplyDate) != "" {
		t, err := parseFinanceDate(body.TaxableSupplyDate, loc)
		if err != nil {
			return nil, fmt.Errorf("taxable_supply_date must be RFC3339 or YYYY-MM-DD")
		}
		row.TaxableSupplyDate = t
	}
	if row.TaxableSupplyDate.IsZero() {
		row.TaxableSupplyDate = row.IssueDate
	}
	if strings.TrimSpace(body.DueDate) != "" {
		t, err := parseFinanceDate(body.DueDate, loc)
		if err != nil {
			return nil, fmt.Errorf("due_date must be RFC3339 or YYYY-MM-DD")
		}
		row.DueDate = t
	}
	if row.DueDate.IsZero() {
		row.DueDate = row.IssueDate
	}
	if strings.TrimSpace(body.StayStartDate) != "" {
		t, err := parseFinanceDate(body.StayStartDate, loc)
		if err != nil {
			return nil, fmt.Errorf("stay_start_date must be RFC3339 or YYYY-MM-DD")
		}
		row.StayStartDate = t
	}
	if row.StayStartDate.IsZero() {
		return nil, fmt.Errorf("stay_start_date is required")
	}
	if strings.TrimSpace(body.StayEndDate) != "" {
		t, err := parseFinanceDate(body.StayEndDate, loc)
		if err != nil {
			return nil, fmt.Errorf("stay_end_date must be RFC3339 or YYYY-MM-DD")
		}
		row.StayEndDate = t
	}
	if row.StayEndDate.IsZero() {
		return nil, fmt.Errorf("stay_end_date is required")
	}
	if row.StayEndDate.Before(row.StayStartDate) {
		return nil, fmt.Errorf("stay_end_date must be on or after stay_start_date")
	}
	if body.AmountTotalCents != nil {
		row.AmountTotalCents = *body.AmountTotalCents
	}
	if row.AmountTotalCents < 0 {
		return nil, fmt.Errorf("amount_total_cents must be >= 0")
	}
	if body.PaymentNote != nil {
		row.PaymentNote = strings.TrimSpace(*body.PaymentNote)
	}
	if body.Customer != nil {
		nextCustomer, err := normalizeInvoiceCustomer(body.Customer)
		if err != nil {
			return nil, err
		}
		customer = nextCustomer
	}
	supplierJSON, err := json.Marshal(supplier)
	if err != nil {
		return nil, fmt.Errorf("failed to encode supplier snapshot")
	}
	customerJSON, err := json.Marshal(customer)
	if err != nil {
		return nil, fmt.Errorf("failed to encode customer snapshot")
	}
	row.SupplierSnapshotJSON = string(supplierJSON)
	row.CustomerSnapshotJSON = string(customerJSON)
	return row, nil
}

func (s *Server) renderAndAttachInvoiceVersion(ctx context.Context, propertyID int64, invoice *store.Invoice, version int) (*store.InvoiceFile, error) {
	supplier, customer, err := decodeInvoiceSnapshots(invoice)
	if err != nil {
		return nil, err
	}
	property, err := s.Store.GetProperty(ctx, propertyID)
	if err != nil {
		return nil, err
	}
	content, err := invoicepdf.Render(invoicepdf.Document{
		Language:          invoice.Language,
		InvoiceNumber:     invoice.InvoiceNumber,
		IssueDate:         invoice.IssueDate,
		TaxableSupplyDate: invoice.TaxableSupplyDate,
		DueDate:           invoice.DueDate,
		StayStartDate:     invoice.StayStartDate,
		StayEndDate:       invoice.StayEndDate,
		AmountTotalCents:  invoice.AmountTotalCents,
		Currency:          invoice.Currency,
		PaymentStatus:     invoice.PaymentStatus,
		PaymentNote:       invoice.PaymentNote,
		PropertyName:      property.Name,
		Supplier:          invoicePartyToPDF(supplier),
		Customer:          invoicePartyToPDF(customer),
	})
	if err != nil {
		return nil, err
	}
	filePath, err := s.saveInvoicePDF(invoice, version, content)
	if err != nil {
		return nil, err
	}
	return s.Store.AttachInvoiceFile(ctx, propertyID, invoice.ID, version, filePath, int64(len(content)))
}

func (s *Server) saveInvoicePDF(invoice *store.Invoice, version int, content []byte) (string, error) {
	baseDir := strings.TrimSpace(s.DataDir)
	if baseDir == "" {
		baseDir = "./data"
	}
	year := invoice.SequenceYear
	relativeDir := filepath.ToSlash(filepath.Join("invoices", fmt.Sprintf("%d", invoice.PropertyID), fmt.Sprintf("%04d", year)))
	if err := os.MkdirAll(filepath.Join(baseDir, relativeDir), 0o755); err != nil {
		return "", fmt.Errorf("unable to create invoice directory")
	}
	filename := sanitizedInvoiceFilename(invoice.InvoiceNumber, version)
	fullPath := filepath.Join(baseDir, filepath.FromSlash(relativeDir), filename)
	if err := os.WriteFile(fullPath, content, 0o644); err != nil {
		return "", fmt.Errorf("unable to save invoice pdf")
	}
	return filepath.ToSlash(filepath.Join(relativeDir, filename)), nil
}

func (s *Server) resolveDataFilePath(relativePath string) (string, error) {
	baseDir := strings.TrimSpace(s.DataDir)
	if baseDir == "" {
		baseDir = "./data"
	}
	cleanRelative := filepath.Clean(filepath.FromSlash(relativePath))
	fullPath := filepath.Join(baseDir, cleanRelative)
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}
	fullAbs, err := filepath.Abs(fullPath)
	if err != nil {
		return "", err
	}
	if fullAbs != baseAbs && !strings.HasPrefix(fullAbs, baseAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid data path")
	}
	return fullAbs, nil
}

func parseInvoiceIDParam(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "invoiceId"), 10, 64)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "invalid invoice id")
		return 0, false
	}
	return id, true
}

func invoiceToRow(propertyID int64, row store.Invoice, files []store.InvoiceFile) (invoiceRow, error) {
	supplier, customer, err := decodeInvoiceSnapshots(&row)
	if err != nil {
		return invoiceRow{}, err
	}
	out := invoiceRow{
		ID:                row.ID,
		OccupancyID:       nullInt64Ptr(row.OccupancyID),
		BookingPayoutID:   nullInt64Ptr(row.FinanceBookingPayoutID),
		InvoiceNumber:     row.InvoiceNumber,
		SequenceYear:      row.SequenceYear,
		SequenceValue:     row.SequenceValue,
		Language:          row.Language,
		IssueDate:         row.IssueDate.UTC().Format(time.RFC3339),
		TaxableSupplyDate: row.TaxableSupplyDate.UTC().Format(time.RFC3339),
		DueDate:           row.DueDate.UTC().Format(time.RFC3339),
		StayStartDate:     row.StayStartDate.UTC().Format(time.RFC3339),
		StayEndDate:       row.StayEndDate.UTC().Format(time.RFC3339),
		Supplier:          supplier,
		Customer:          customer,
		AmountTotalCents:  row.AmountTotalCents,
		Currency:          row.Currency,
		PaymentStatus:     row.PaymentStatus,
		PaymentNote:       row.PaymentNote,
		Version:           row.Version,
		LatestFilePath:    nullStringPtr(row.LatestFilePath),
		DownloadURL:       fmt.Sprintf("/api/properties/%d/invoices/%d/download", propertyID, row.ID),
		CreatedAt:         row.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:         row.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if row.LatestFileSizeBytes.Valid {
		size := row.LatestFileSizeBytes.Int64
		out.LatestFileSizeBytes = &size
	}
	if row.LatestFileCreatedAt.Valid {
		ts := row.LatestFileCreatedAt.Time.UTC().Format(time.RFC3339)
		out.LatestFileCreatedAt = &ts
	}
	if files != nil {
		items := make([]invoiceFileRow, 0, len(files))
		for _, file := range files {
			items = append(items, invoiceFileRow{
				ID:            file.ID,
				Version:       file.Version,
				FilePath:      file.FilePath,
				FileSizeBytes: file.FileSizeBytes,
				CreatedAt:     file.CreatedAt.UTC().Format(time.RFC3339),
			})
		}
		out.Files = &items
	}
	return out, nil
}

func decodeInvoiceSnapshots(row *store.Invoice) (invoicePartySnapshot, invoicePartySnapshot, error) {
	var supplier invoicePartySnapshot
	var customer invoicePartySnapshot
	if err := json.Unmarshal([]byte(row.SupplierSnapshotJSON), &supplier); err != nil {
		return invoicePartySnapshot{}, invoicePartySnapshot{}, err
	}
	if err := json.Unmarshal([]byte(row.CustomerSnapshotJSON), &customer); err != nil {
		return invoicePartySnapshot{}, invoicePartySnapshot{}, err
	}
	return supplier, customer, nil
}

func defaultInvoiceSupplier(property *store.Property, profile *store.PropertyProfile) invoicePartySnapshot {
	name := strings.TrimSpace(profile.BillingName.String)
	if name == "" {
		name = strings.TrimSpace(profile.LegalOwnerName.String)
	}
	if name == "" {
		name = property.Name
	}
	address := strings.TrimSpace(profile.BillingAddress.String)
	if address == "" {
		address = strings.TrimSpace(property.AddressLine1.String)
	}
	city := strings.TrimSpace(profile.City.String)
	if city == "" {
		city = strings.TrimSpace(property.City.String)
	}
	postalCode := strings.TrimSpace(profile.PostalCode.String)
	if postalCode == "" {
		postalCode = strings.TrimSpace(property.PostalCode.String)
	}
	country := strings.TrimSpace(profile.Country.String)
	if country == "" {
		country = strings.TrimSpace(property.Country.String)
	}
	return invoicePartySnapshot{
		Name:         name,
		CompanyName:  property.Name,
		AddressLine1: address,
		City:         city,
		PostalCode:   postalCode,
		Country:      country,
		ICO:          strings.TrimSpace(profile.ICO.String),
		DIC:          strings.TrimSpace(profile.DIC.String),
		VATID:        strings.TrimSpace(profile.VATID.String),
	}
}

func normalizeInvoiceCustomer(customer *invoicePartySnapshot) (invoicePartySnapshot, error) {
	if customer == nil {
		return invoicePartySnapshot{}, fmt.Errorf("customer is required")
	}
	out := invoicePartySnapshot{
		Name:         strings.TrimSpace(customer.Name),
		CompanyName:  strings.TrimSpace(customer.CompanyName),
		AddressLine1: strings.TrimSpace(customer.AddressLine1),
		City:         strings.TrimSpace(customer.City),
		PostalCode:   strings.TrimSpace(customer.PostalCode),
		Country:      strings.TrimSpace(customer.Country),
		VATID:        strings.TrimSpace(customer.VATID),
	}
	if out.Name == "" && out.CompanyName == "" {
		return invoicePartySnapshot{}, fmt.Errorf("customer name or company_name is required")
	}
	if out.AddressLine1 == "" {
		return invoicePartySnapshot{}, fmt.Errorf("customer address_line_1 is required")
	}
	return out, nil
}

func invoicePartyToPDF(p invoicePartySnapshot) invoicepdf.Party {
	return invoicepdf.Party{
		Name:         p.Name,
		CompanyName:  p.CompanyName,
		AddressLine1: p.AddressLine1,
		City:         p.City,
		PostalCode:   p.PostalCode,
		Country:      p.Country,
		ICO:          p.ICO,
		DIC:          p.DIC,
		VATID:        p.VATID,
	}
}

func sanitizedInvoiceFilename(invoiceNumber string, version int) string {
	safe := strings.NewReplacer("/", "_", "\\", "_", " ", "_").Replace(strings.TrimSpace(invoiceNumber))
	if safe == "" {
		safe = "invoice"
	}
	return fmt.Sprintf("%s_v%d.pdf", safe, version)
}

func supplierLanguageDefault(current *store.Invoice, body *invoiceRequestBody) string {
	if current != nil {
		return current.Language
	}
	if body != nil && strings.TrimSpace(body.Language) != "" {
		return strings.TrimSpace(body.Language)
	}
	return "sk"
}

func WriteInvoiceStoreError(w http.ResponseWriter, err error) {
	msg := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, sql.ErrNoRows):
		WriteError(w, http.StatusNotFound, "invoice not found")
	case strings.Contains(msg, "ux_invoices_property_booking_payout") || strings.Contains(msg, "finance_booking_payout_id"):
		WriteError(w, http.StatusConflict, "invoice already exists for this booking payout")
	case strings.Contains(msg, "unique"):
		WriteError(w, http.StatusConflict, "invoice already exists for this stay or sequence")
	default:
		WriteError(w, http.StatusInternalServerError, "database error")
	}
}

// payoutInvoiceBillableCents is the gross price to bill on the invoice: CSV "amount" (guest-facing
// total) when imported; otherwise net payout for legacy or incomplete rows.
func payoutInvoiceBillableCents(p *store.FinanceBookingPayout) int {
	if p == nil {
		return 0
	}
	if p.AmountCents.Valid && p.AmountCents.Int64 > 0 {
		return int(p.AmountCents.Int64)
	}
	return p.NetCents
}

func applyInvoicePayoutPrefill(body *invoiceRequestBody, payout *store.FinanceBookingPayout) {
	if payout == nil {
		return
	}
	if body.AmountTotalCents == nil {
		n := payoutInvoiceBillableCents(payout)
		body.AmountTotalCents = &n
	}
	if strings.TrimSpace(body.StayStartDate) == "" && payout.CheckInDate.Valid {
		body.StayStartDate = strings.TrimSpace(payout.CheckInDate.String)
	}
	if strings.TrimSpace(body.StayEndDate) == "" && payout.CheckOutDate.Valid {
		body.StayEndDate = strings.TrimSpace(payout.CheckOutDate.String)
	}
}

func (s *Server) resolveInvoiceBookingPayout(ctx context.Context, propertyID int64, body *invoiceRequestBody) (*store.FinanceBookingPayout, error) {
	var idPtr *int64
	if body.BookingPayoutID != nil && *body.BookingPayoutID > 0 {
		v := *body.BookingPayoutID
		idPtr = &v
	}
	var refPtr *string
	if body.BookingPayoutReference != nil {
		r := strings.TrimSpace(*body.BookingPayoutReference)
		if r != "" {
			refPtr = &r
		}
	}
	if idPtr == nil && refPtr == nil {
		return nil, nil
	}
	if idPtr != nil && refPtr != nil {
		byID, err := s.Store.GetBookingPayoutByID(ctx, propertyID, *idPtr)
		if err != nil {
			return nil, fmt.Errorf("booking payout not found")
		}
		byRef, err := s.Store.GetBookingPayoutByReference(ctx, propertyID, *refPtr)
		if err != nil {
			return nil, fmt.Errorf("booking payout not found")
		}
		if byID.ID != byRef.ID {
			return nil, fmt.Errorf("booking_payout_id does not match booking_payout_reference")
		}
		return byID, nil
	}
	if idPtr != nil {
		p, err := s.Store.GetBookingPayoutByID(ctx, propertyID, *idPtr)
		if err != nil {
			return nil, fmt.Errorf("booking payout not found")
		}
		return p, nil
	}
	p, err := s.Store.GetBookingPayoutByReference(ctx, propertyID, *refPtr)
	if err != nil {
		return nil, fmt.Errorf("booking payout not found")
	}
	return p, nil
}
