package invoicepdf

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/go-pdf/fpdf"
	"pms/backend/internal/invoiceitems"
)

func pdfTextBytes(text string) []byte {
	var out []byte
	for _, r := range utf16.Encode([]rune(text)) {
		out = append(out, byte(r>>8), byte(r))
	}
	return out
}

func TestItemTableContentAndPagination(t *testing.T) {
	for _, lang := range []string{"en", "sk"} {
		t.Run(lang, func(t *testing.T) {
			pdf := fpdf.New("P", "mm", "A4", "")
			pdf.SetMargins(pageMargin, pageMargin, pageMargin)
			pdf.SetAutoPageBreak(true, pageMargin+24)
			pdf.SetCompression(false)
			pdf.AddUTF8FontFromBytes(fontRoboto, "", robotoRegularTTF)
			pdf.AddUTF8FontFromBytes(fontRoboto, "B", robotoBoldTTF)
			pdf.AddPage()
			doc := Document{Language: lang, Currency: "EUR"}
			for i := 0; i < 100; i++ {
				doc.Items = append(doc.Items, invoiceitems.Item{Description: fmt.Sprintf("Cleaning %03d", i), Quantity: "1.5", UnitPriceCents: 101, LineTotalCents: 152})
			}
			doc.AmountTotalCents = 15200
			y := drawItems(pdf, doc, 240)
			if pdf.PageCount() < 4 || y > 262 {
				t.Fatalf("pages=%d y=%f", pdf.PageCount(), y)
			}
			var output bytes.Buffer
			if err := pdf.Output(&output); err != nil {
				t.Fatal(err)
			}
			for _, item := range doc.Items {
				for _, value := range itemCells(doc, item) {
					if !bytes.Contains(output.Bytes(), pdfTextBytes(value)) {
						t.Fatalf("missing rendered value %q", value)
					}
				}
			}
			if !bytes.Contains(output.Bytes(), pdfTextBytes(fmtMoney(lang, 15200, "EUR"))) {
				t.Fatal("missing summed total")
			}
			heading := "Description"
			if lang == "sk" {
				heading = "Popis"
			}
			if bytes.Count(output.Bytes(), pdfTextBytes(heading)) < 4 {
				t.Fatal("missing repeated table headings")
			}
		})
	}
}

func TestItemDescriptionCanContinueAcrossPages(t *testing.T) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetAutoPageBreak(true, pageMargin+24)
	pdf.AddUTF8FontFromBytes(fontRoboto, "", robotoRegularTTF)
	pdf.AddUTF8FontFromBytes(fontRoboto, "B", robotoBoldTTF)
	pdf.SetCompression(false)
	pdf.AddPage()
	doc := Document{Language: "en", Currency: "EUR", AmountTotalCents: 100,
		Items: []invoiceitems.Item{{Description: strings.Repeat("X\n", 200) + "Last line", Quantity: "1", UnitPriceCents: 100, LineTotalCents: 100}},
	}
	y := drawItems(pdf, doc, 20)
	if y > 262 || pdf.PageCount() < 4 {
		t.Fatalf("pages=%d y=%f", pdf.PageCount(), y)
	}
	var output bytes.Buffer
	if err := pdf.Output(&output); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output.Bytes(), pdfTextBytes("Last line")) {
		t.Fatal("long item was truncated")
	}
}

func TestTwoItemDocumentsKeepFooterOnSamePage(t *testing.T) {
	date := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	for _, lang := range []string{"en", "sk"} {
		for _, kind := range []string{"invoice", "proforma"} {
			t.Run(lang+"/"+kind, func(t *testing.T) {
				doc := Document{
					Language: lang, DocumentType: kind, InvoiceNumber: "APT/2026/0001",
					BookingReference: "TEST-123", GuestName: "Test Guest", IssueDate: date,
					TaxableSupplyDate: date, DueDate: date, StayStartDate: date, StayEndDate: date.AddDate(0, 0, 30),
					AmountTotalCents: 104500, Currency: "EUR", PaymentStatus: "paid",
					PropertyName: "Test Apartment", PriceDetails: "Accommodation and cleaning.",
					Supplier: Party{Name: "Test Owner", AddressLine1: "Main Street 1", City: "Bratislava", PostalCode: "81101", Country: "Slovakia", DIC: "1234567890"},
					Customer: Party{CompanyName: "Example Company", AddressLine1: "Example Street 2", City: "Sofia", PostalCode: "1000", Country: "Bulgaria"},
					Items: []invoiceitems.Item{
						{Description: "Accommodation", Quantity: "1", UnitPriceCents: 100000, LineTotalCents: 100000},
						{Description: "Cleaning", Quantity: "1.5", UnitPriceCents: 3000, LineTotalCents: 4500},
					},
				}
				content, err := Render(doc)
				if err != nil {
					t.Fatal(err)
				}
				if count := len(regexp.MustCompile(`/Type /Page\b`).FindAll(content, -1)); count != 1 {
					t.Fatalf("two-item document page count=%d want 1 (no orphan footer)", count)
				}
			})
		}
	}
}
