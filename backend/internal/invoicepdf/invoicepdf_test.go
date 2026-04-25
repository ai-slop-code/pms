package invoicepdf

import (
	"testing"
	"time"

	"github.com/go-pdf/fpdf"
)

func TestWrapLinesMeasured_wrapsSlovakInvoiceLabel(t *testing.T) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddUTF8FontFromBytes(fontRoboto, "", robotoRegularTTF)
	pdf.AddUTF8FontFromBytes(fontRoboto, "B", robotoBoldTTF)
	pdf.AddPage()
	pdf.SetFont(fontRoboto, "B", 11)
	skLabel := lbl("sk", "invoice_number")
	maxW := detailLabelW - 3
	t.Logf("GetStringWidth(%q)=%.2f mm, max inner=%.2f", skLabel, pdf.GetStringWidth(skLabel), maxW)
	if pdf.GetStringWidth(skLabel) < 0.01 {
		t.Fatal("GetStringWidth is 0 — bold Roboto must be registered like in Render()")
	}
	narrow := 45.0
	lines := wrapLinesMeasured(pdf, skLabel, narrow)
	if len(lines) < 2 {
		t.Fatalf("expected wrap at narrow width, got %d lines: %q", len(lines), lines)
	}
}

func TestRender_smoke(t *testing.T) {
	doc := Document{
		Language:          "sk",
		InvoiceNumber:     "P001/2026/0001",
		IssueDate:         time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC),
		TaxableSupplyDate: time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC),
		DueDate:           time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC),
		StayStartDate:     time.Date(2026, 4, 7, 0, 0, 0, 0, time.UTC),
		StayEndDate:       time.Date(2026, 4, 8, 0, 0, 0, 0, time.UTC),
		AmountTotalCents:  7359,
		Currency:          "EUR",
		PaymentStatus:     "paid",
		PaymentNote:       "Already paid via Booking.com.",
		PropertyName:      "Airport",
		Supplier: Party{
			CompanyName:  "Airport",
			AddressLine1: "Ivanska Cesta 32/D",
			City:         "Bratislava",
			PostalCode:   "83106",
			Country:      "Slovakia",
		},
		Customer: Party{
			CompanyName:  "aaa",
			Name:         "Sijia Tang",
			AddressLine1: "223",
			City:         "dd",
			PostalCode:   "122",
			Country:      "dd",
			VATID:        "dd",
		},
	}
	b, err := Render(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 2500 {
		t.Fatalf("unexpectedly small PDF (%d bytes)", len(b))
	}
}

func TestWrapLinesMeasured_explicitNewline(t *testing.T) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.AddUTF8FontFromBytes(fontRoboto, "", robotoRegularTTF)
	pdf.AddUTF8FontFromBytes(fontRoboto, "B", robotoBoldTTF)
	pdf.AddPage()
	pdf.SetFont(fontRoboto, "B", 11)
	lines := wrapLinesMeasured(pdf, "First line\nSecond line", detailLabelW-3)
	if len(lines) < 2 {
		t.Fatalf("expected 2+ lines from explicit newline, got %d: %q", len(lines), lines)
	}
}
