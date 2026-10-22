package invoicepdf

import (
	"strings"

	"github.com/go-pdf/fpdf"
	"pms/backend/internal/invoiceitems"
)

func invoicePageRoom(pdf *fpdf.Fpdf, y, height float64) float64 {
	if y+height > 262 {
		pdf.AddPage()
		pdf.SetFillColor(cPageR, cPageG, cPageB)
		pdf.Rect(0, 0, 210, 297, "F")
		return pageMargin
	}
	return y
}

func itemCells(doc Document, item invoiceitems.Item) []string {
	quantity := item.Quantity
	if doc.Language == "sk" {
		quantity = strings.ReplaceAll(quantity, ".", ",")
	}
	return []string{item.Description, quantity,
		fmtMoney(doc.Language, item.UnitPriceCents, doc.Currency),
		fmtMoney(doc.Language, item.LineTotalCents, doc.Currency)}
}

func drawItems(pdf *fpdf.Fpdf, doc Document, y float64) float64 {
	widths := []float64{92, 22, 37, 37}
	headings := []string{"Description", "Quantity", "Unit price", "Total"}
	if doc.Language == "sk" {
		headings = []string{"Popis", "Množstvo", "Jednotková cena", "Spolu"}
	}
	header := func() {
		pdf.SetFillColor(cNavyR, cNavyG, cNavyB)
		pdf.SetTextColor(255, 255, 255)
		pdf.SetFont(fontRoboto, "B", 8.5)
		x := pageMargin
		for i, text := range headings {
			pdf.SetXY(x, y)
			pdf.CellFormat(widths[i], 8, text, "", 0, "L", true, 0, "")
			x += widths[i]
		}
		y += 8
	}
	y = invoicePageRoom(pdf, y, 20)
	header()
	for _, item := range doc.Items {
		pdf.SetFont(fontRoboto, "", 9)
		cells := itemCells(doc, item)
		lines := make([][]string, len(cells))
		count := 1
		for i, cell := range cells {
			lines[i] = wrapLinesMeasured(pdf, cell, widths[i]-4)
			count = maxI(count, len(lines[i]))
		}
		h := float64(count)*5 + 4
		if y+h > 262 && y > pageMargin+8 {
			y = invoicePageRoom(pdf, y, h)
			header()
		}
		for offset := 0; offset < count; {
			fit := int((262 - y - 4) / 5)
			if fit < 1 {
				y = invoicePageRoom(pdf, y, 9)
				header()
				continue
			}
			end := min(offset+fit, count)
			h = float64(end-offset)*5 + 4
			pdf.SetFillColor(255, 255, 255)
			pdf.SetDrawColor(cBorderR, cBorderG, cBorderB)
			pdf.Rect(pageMargin, y, contentW, h, "FD")
			pdf.SetFont(fontRoboto, "", 9)
			pdf.SetTextColor(cBodyR, cBodyG, cBodyB)
			x := pageMargin
			for i, cellLines := range lines {
				align := "R"
				if i == 0 {
					align = "L"
				}
				for n := offset; n < min(end, len(cellLines)); n++ {
					pdf.SetXY(x+2, y+2+float64(n-offset)*5)
					pdf.CellFormat(widths[i]-4, 5, cellLines[n], "", 0, align, false, 0, "")
				}
				x += widths[i]
			}
			y += h
			offset = end
		}
	}
	y = invoicePageRoom(pdf, y, 12)
	pdf.SetFillColor(cCardBlueR, cCardBlueG, cCardBlueB)
	pdf.Rect(pageMargin, y, contentW, 12, "F")
	label := "Total (EUR)"
	if doc.Language == "sk" {
		label = "Spolu (EUR)"
	}
	drawRow(pdf, pageMargin+detailPad, y, label, fmtMoney(doc.Language, doc.AmountTotalCents, doc.Currency), 12, 12, true)
	return y + 12
}
