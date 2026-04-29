package invoicepdf

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"time"

	_ "embed"

	"github.com/go-pdf/fpdf"
)

//go:embed fonts/Roboto-Regular.ttf
var robotoRegularTTF []byte

//go:embed fonts/Roboto-Bold.ttf
var robotoBoldTTF []byte

const fontRoboto = "Roboto"

const (
	pageMargin   = 11.0
	contentW     = 210.0 - 2*pageMargin
	cornerRadius = 3.0
)

// Color palette matching mockup
const (
	cNavyR, cNavyG, cNavyB       = 26, 43, 75
	cPageR, cPageG, cPageB       = 248, 250, 252
	cCardBlueR, cCardBlueG       = 232, 240
	cCardBlueB                   = 254
	cCardCustR, cCardCustG       = 241, 245
	cCardCustB                   = 249
	cBorderR, cBorderG, cBorderB = 226, 232, 240
	cMutedR, cMutedG, cMutedB    = 100, 116, 139
	cBodyR, cBodyG, cBodyB       = 71, 85, 105
	cAccentR, cAccentG, cAccentB = 37, 99, 235
	cTotalR, cTotalG, cTotalB    = 29, 78, 216
	cGreenBgR, cGreenBgG         = 209, 250
	cGreenBgB                    = 229
	cGreenR, cGreenG, cGreenB    = 6, 95, 70
	cOrangeR, cOrangeG, cOrangeB = 234, 88, 12
	cIconBgR, cIconBgG, cIconBgB = 232, 240, 254
	cFootBgR, cFootBgG, cFootBgB = 232, 242, 254
)

type Party struct {
	Name         string
	CompanyName  string
	AddressLine1 string
	City         string
	PostalCode   string
	Country      string
	ICO          string
	DIC          string
	VATID        string
}

type Document struct {
	Language          string
	InvoiceNumber     string
	IssueDate         time.Time
	TaxableSupplyDate time.Time
	DueDate           time.Time
	StayStartDate     time.Time
	StayEndDate       time.Time
	AmountTotalCents  int
	Currency          string
	PaymentStatus     string
	PaymentNote       string
	PropertyName      string
	Supplier          Party
	Customer          Party
}

func Render(doc Document) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(pageMargin, pageMargin, pageMargin)
	pdf.SetAutoPageBreak(true, pageMargin+24)
	pdf.AddUTF8FontFromBytes(fontRoboto, "", robotoRegularTTF)
	pdf.AddUTF8FontFromBytes(fontRoboto, "B", robotoBoldTTF)

	pdf.SetTitle(docTitle(doc.Language), true)
	pdf.SetAuthor("PMS", true)
	pdf.SetCreator("PMS", true)
	pdf.AddPage()

	// Page background
	pdf.SetFillColor(cPageR, cPageG, cPageB)
	pdf.Rect(0, 0, 210, 297, "F")

	y := pdf.GetY()
	y = drawHeader(pdf, doc, y)
	pdf.SetY(y + 5)

	y = drawPartyCards(pdf, doc, pdf.GetY())
	pdf.SetY(y + 6)

	y = drawDetailsCard(pdf, doc, pdf.GetY())
	pdf.SetY(y + 6)

	y = drawInfoCards(pdf, doc, pdf.GetY())
	pdf.SetY(y + 8)

	drawFooter(pdf, doc, pdf.GetY())

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ---------------------------------------------------------------------------
// Header: icon tile + title + subtitle + paid badge
// ---------------------------------------------------------------------------

func drawHeader(pdf *fpdf.Fpdf, doc Document, y float64) float64 {
	x0 := pageMargin
	iconS := 12.0

	// Icon tile
	pdf.SetFillColor(cIconBgR, cIconBgG, cIconBgB)
	pdf.RoundedRect(x0, y, iconS, iconS, 2.2, "1234", "F")
	drawClipboardIcon(pdf, x0+1.8, y+1.5, iconS-3.5, cNavyR, cNavyG, cNavyB)

	// Title + subtitle
	tx := x0 + iconS + 4
	pdf.SetXY(tx, y+0.5)
	pdf.SetFont(fontRoboto, "B", 18)
	pdf.SetTextColor(cNavyR, cNavyG, cNavyB)
	pdf.CellFormat(100, 8, docTitle(doc.Language), "", 2, "L", false, 0, "")
	pdf.SetFont(fontRoboto, "", 9)
	pdf.SetTextColor(cMutedR, cMutedG, cMutedB)
	pdf.CellFormat(100, 4, docTitleSub(doc.Language), "", 1, "L", false, 0, "")

	titleBlockH := pdf.GetY() - y
	if titleBlockH < iconS {
		titleBlockH = iconS
	}

	// Paid badge
	if isPaid(doc) {
		badgeText := paidLabel(doc.Language)
		pdf.SetFont(fontRoboto, "B", 8)
		tw := pdf.GetStringWidth(badgeText)
		circleD := 5.0
		bw := circleD + 3 + tw + 6
		bh := 7.0
		bx := pageMargin + contentW - bw
		by := y + (titleBlockH-bh)/2

		pdf.SetFillColor(cGreenBgR, cGreenBgG, cGreenBgB)
		pdf.SetDrawColor(cGreenR, cGreenG, cGreenB)
		pdf.SetLineWidth(0.15)
		pdf.RoundedRect(bx, by, bw, bh, bh/2, "1234", "FD")

		// Circle with checkmark
		ccx := bx + bw - circleD/2 - 2
		ccy := by + bh/2
		pdf.SetFillColor(cGreenR, cGreenG, cGreenB)
		pdf.Circle(ccx, ccy, circleD/2, "F")
		drawCheckmark(pdf, ccx, ccy, circleD*0.32)

		pdf.SetTextColor(cGreenR, cGreenG, cGreenB)
		pdf.SetXY(bx+4, by+1.5)
		pdf.CellFormat(tw, 4, badgeText, "", 0, "L", false, 0, "")

		pdf.SetDrawColor(cBorderR, cBorderG, cBorderB)
		pdf.SetLineWidth(0.2)
	}

	pdf.SetTextColor(0, 0, 0)
	return y + titleBlockH
}

func drawCheckmark(pdf *fpdf.Fpdf, cx, cy, r float64) {
	pdf.SetDrawColor(255, 255, 255)
	pdf.SetLineWidth(0.35)
	pdf.Line(cx-r*0.7, cy, cx-r*0.15, cy+r*0.55)
	pdf.Line(cx-r*0.15, cy+r*0.55, cx+r*0.7, cy-r*0.5)
}

// Clipboard icon: filled rounded rect with a tab at the top, and two horizontal lines.
func drawClipboardIcon(pdf *fpdf.Fpdf, x, y, s float64, r, g, b int) {
	bw := s * 0.85
	bh := s
	bx := x + (s-bw)/2
	by := y + s*0.15
	cr := s * 0.08

	pdf.SetFillColor(r, g, b)
	pdf.SetDrawColor(r, g, b)
	pdf.SetLineWidth(0.15)
	pdf.RoundedRect(bx, by, bw, bh, cr, "1234", "FD")

	// Tab on top
	tw := s * 0.35
	th := s * 0.13
	tx := x + (s-tw)/2
	ty := by - th*0.5
	pdf.SetFillColor(r, g, b)
	pdf.RoundedRect(tx, ty, tw, th, th*0.3, "1234", "F")

	// White lines representing text
	pdf.SetDrawColor(255, 255, 255)
	pdf.SetLineWidth(0.3)
	lx1 := bx + bw*0.2
	lx2 := bx + bw*0.8
	lx2mid := bx + bw*0.65
	lx2short := bx + bw*0.5
	pdf.Line(lx1, by+bh*0.38, lx2, by+bh*0.38)
	pdf.Line(lx1, by+bh*0.55, lx2mid, by+bh*0.55)
	pdf.Line(lx1, by+bh*0.72, lx2short, by+bh*0.72)
}

// ---------------------------------------------------------------------------
// Party cards (supplier / customer)
// ---------------------------------------------------------------------------

func drawPartyCards(pdf *fpdf.Fpdf, doc Document, y float64) float64 {
	gap := 5.0
	w := (contentW - gap) / 2
	xL := pageMargin
	xR := pageMargin + w + gap
	hL := measurePartyH(pdf, doc.Supplier, w)
	hR := measurePartyH(pdf, doc.Customer, w)
	h := maxF(hL, hR)
	drawPartyCard(pdf, xL, y, w, h, doc.Language, true, doc.Supplier)
	drawPartyCard(pdf, xR, y, w, h, doc.Language, false, doc.Customer)
	return y + h
}

func measurePartyH(pdf *fpdf.Fpdf, p Party, w float64) float64 {
	pad := 4.0
	hdrH := 5.5
	lines := partyLines(p)
	if len(lines) < 1 {
		lines = []string{""}
	}
	return pad + hdrH + partyBodyH(pdf, lines, w-2*pad) + pad
}

func partyBodyH(pdf *fpdf.Fpdf, lines []string, maxW float64) float64 {
	h := 0.0
	for i, line := range lines {
		if i == 0 {
			pdf.SetFont(fontRoboto, "B", 11)
		} else {
			pdf.SetFont(fontRoboto, "", 10)
		}
		h += float64(len(wrapLinesMeasured(pdf, line, maxW))) * 5.2
	}
	if h == 0 {
		return 5.0
	}
	return h
}

func drawPartyCard(pdf *fpdf.Fpdf, x, y, w, h float64, lang string, supplier bool, p Party) {
	pad := 4.0
	hdrH := 5.5
	if supplier {
		pdf.SetFillColor(cCardBlueR, cCardBlueG, cCardBlueB)
	} else {
		pdf.SetFillColor(cCardCustR, cCardCustG, cCardCustB)
	}
	pdf.SetDrawColor(cBorderR, cBorderG, cBorderB)
	pdf.SetLineWidth(0.2)
	pdf.RoundedRect(x, y, w, h, cornerRadius, "1234", "FD")

	ix := x + pad
	iy := y + pad
	hdr := strings.ToUpper(partyTitle(lang, supplier))
	if supplier {
		pdf.SetTextColor(cAccentR, cAccentG, cAccentB)
	} else {
		pdf.SetTextColor(cMutedR, cMutedG, cMutedB)
	}
	pdf.SetFont(fontRoboto, "B", 8.5)
	pdf.SetXY(ix, iy)
	pdf.CellFormat(w-2*pad, 5, hdr, "", 1, "L", false, 0, "")

	lines := partyLines(p)
	if len(lines) == 0 {
		lines = []string{""}
	}
	innerW := w - 2*pad
	bodyH := partyBodyH(pdf, lines, innerW)
	room := h - 2*pad - hdrH - bodyH
	cy := y + pad + hdrH + maxF(0, room/2)
	for i, line := range lines {
		if i == 0 {
			pdf.SetFont(fontRoboto, "B", 11)
			pdf.SetTextColor(cNavyR, cNavyG, cNavyB)
		} else {
			pdf.SetFont(fontRoboto, "", 10)
			pdf.SetTextColor(cBodyR, cBodyG, cBodyB)
		}
		for _, wl := range wrapLinesMeasured(pdf, line, innerW) {
			pdf.SetXY(ix, cy)
			pdf.CellFormat(innerW, 5.2, wl, "", 1, "L", false, 0, "")
			cy = pdf.GetY()
		}
	}
}

func partyLines(p Party) []string {
	var out []string
	if p.CompanyName != "" {
		out = append(out, p.CompanyName)
	}
	if p.Name != "" && p.Name != p.CompanyName {
		out = append(out, p.Name)
	}
	if p.AddressLine1 != "" {
		out = append(out, p.AddressLine1)
	}
	city := strings.TrimSpace(p.PostalCode + " " + p.City)
	if city != "" {
		out = append(out, city)
	}
	if p.Country != "" {
		out = append(out, p.Country)
	}
	if p.ICO != "" {
		out = append(out, "ICO: "+p.ICO)
	}
	if p.DIC != "" {
		out = append(out, "DIC: "+p.DIC)
	}
	if p.VATID != "" {
		out = append(out, "VAT ID: "+p.VATID)
	}
	return out
}

// ---------------------------------------------------------------------------
// Invoice details card
// ---------------------------------------------------------------------------

const (
	detailPad      = 4.0
	detailGutter   = 6.0
	detailLabelW   = 100.0
	detailValueW   = contentW - 2*detailPad - detailLabelW - detailGutter
	detailTextPad  = 1.5
	labelWrapSafe  = 0.84
	valueWrapSafe  = 0.90
)

func drawDetailsCard(pdf *fpdf.Fpdf, doc Document, y float64) float64 {
	x := pageMargin
	w := contentW
	headerH := 10.0
	rowPad := 1.5
	rowH := 7.5
	totalH := 12.0

	saved := pdf.GetCellMargin()
	pdf.SetCellMargin(0)
	defer pdf.SetCellMargin(saved)

	type row struct{ l, v string }
	rows := []row{
		{lbl(doc.Language, "invoice_number"), doc.InvoiceNumber},
		{lbl(doc.Language, "issue_date"), fmtDate(doc.IssueDate)},
		{lbl(doc.Language, "taxable_supply_date"), fmtDate(doc.TaxableSupplyDate)},
		{lbl(doc.Language, "due_date"), fmtDate(doc.DueDate)},
		{lbl(doc.Language, "stay_period"), fmtDate(doc.StayStartDate) + " \u2013 " + fmtDate(doc.StayEndDate)},
	}
	totalRow := row{lbl(doc.Language, "amount_total"), fmtMoney(doc.Language, doc.AmountTotalCents, doc.Currency)}

	bodyH := 0.0
	for i := range rows {
		bodyH += measureRowH(pdf, rows[i].l, rows[i].v, rowH) + rowPad
	}
	bodyH += rowPad
	cardH := headerH + bodyH + totalH

	// Card outline
	pdf.SetFillColor(255, 255, 255)
	pdf.SetDrawColor(cBorderR, cBorderG, cBorderB)
	pdf.SetLineWidth(0.2)
	pdf.RoundedRect(x, y, w, cardH, cornerRadius, "1234", "FD")

	// Navy header bar
	pdf.SetFillColor(cNavyR, cNavyG, cNavyB)
	pdf.RoundedRect(x, y, w, headerH, cornerRadius, "12", "F")
	pdf.Rect(x, y+headerH-cornerRadius, w, cornerRadius, "F")

	drawClipboardIcon(pdf, x+detailPad, y+2.2, 5.5, 255, 255, 255)
	pdf.SetFont(fontRoboto, "B", 8.5)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetXY(x+detailPad+8, y+3)
	pdf.CellFormat(w-detailPad-10, 5, detailTitle(doc.Language), "", 0, "L", false, 0, "")

	// Rows
	cy := y + headerH + rowPad
	for i, r := range rows {
		h := measureRowH(pdf, r.l, r.v, rowH)
		drawRow(pdf, x+detailPad, cy, r.l, r.v, h, rowH, false)
		cy += h + rowPad
		if i < len(rows)-1 {
			pdf.SetDrawColor(cBorderR, cBorderG, cBorderB)
			pdf.SetLineWidth(0.1)
			pdf.Line(x+detailPad, cy-rowPad*0.5, x+w-detailPad, cy-rowPad*0.5)
		}
	}

	// Separator before total
	pdf.SetDrawColor(cBorderR, cBorderG, cBorderB)
	pdf.SetLineWidth(0.1)
	pdf.Line(x+detailPad, cy-rowPad*0.5, x+w-detailPad, cy-rowPad*0.5)

	// Total row
	pdf.SetFillColor(cCardBlueR, cCardBlueG, cCardBlueB)
	pdf.RoundedRect(x, cy, w, totalH, cornerRadius, "34", "F")
	pdf.Rect(x, cy, w, cornerRadius, "F")
	drawRow(pdf, x+detailPad, cy, totalRow.l, totalRow.v, totalH, totalH, true)

	pdf.SetY(y + cardH)
	return y + cardH
}

func measureRowH(pdf *fpdf.Fpdf, l, v string, lineH float64) float64 {
	lw := detailLabelW - 2*detailTextPad
	vw := detailValueW - 2*detailTextPad
	pdf.SetFont(fontRoboto, "", 10)
	nL := len(wrapLinesMeasuredSafe(pdf, l, lw, labelWrapSafe))
	pdf.SetFont(fontRoboto, "B", 10)
	nV := len(wrapLinesMeasuredSafe(pdf, v, vw, valueWrapSafe))
	n := maxI(nL, nV)
	if n < 1 {
		n = 1
	}
	return float64(n) * lineH
}

func drawRow(pdf *fpdf.Fpdf, x, y float64, label, value string, rowH, lineH float64, total bool) {
	lw := detailLabelW - 2*detailTextPad
	vw := detailValueW - 2*detailTextPad

	labelFont := ""; labelSz := 10.0
	valueFont := "B"; valueSz := 10.0
	if total {
		labelFont = "B"; labelSz = 11
		valueFont = "B"; valueSz = 13
	}

	pdf.SetFont(fontRoboto, labelFont, labelSz)
	tLines := wrapLinesMeasuredSafe(pdf, label, lw, labelWrapSafe)
	pdf.SetFont(fontRoboto, valueFont, valueSz)
	vLines := wrapLinesMeasuredSafe(pdf, value, vw, valueWrapSafe)

	n := maxI(len(tLines), len(vLines))
	if n < 1 { n = 1 }
	for len(tLines) < n { tLines = append(tLines, "") }
	for len(vLines) < n { vLines = append(vLines, "") }

	lx := x + detailTextPad
	rx := x + detailLabelW + detailGutter + detailTextPad
	blockH := float64(n) * lineH
	yOff := (rowH - blockH) / 2
	if yOff < 0 { yOff = 0 }

	for i := 0; i < n; i++ {
		yl := y + yOff + float64(i)*lineH
		if total {
			pdf.SetTextColor(cTotalR, cTotalG, cTotalB)
		} else {
			pdf.SetTextColor(cMutedR, cMutedG, cMutedB)
		}
		pdf.SetFont(fontRoboto, labelFont, labelSz)
		pdf.ClipRect(lx, yl, lw, lineH, false)
		pdf.SetXY(lx, yl)
		pdf.CellFormat(lw, lineH, tLines[i], "", 0, "LM", false, 0, "")
		pdf.ClipEnd()

		if total {
			pdf.SetTextColor(cTotalR, cTotalG, cTotalB)
		} else {
			pdf.SetTextColor(cNavyR, cNavyG, cNavyB)
		}
		pdf.SetFont(fontRoboto, valueFont, valueSz)
		pdf.ClipRect(rx, yl, vw, lineH, false)
		pdf.SetXY(rx, yl)
		pdf.CellFormat(vw, lineH, vLines[i], "", 0, "RM", false, 0, "")
		pdf.ClipEnd()
	}
}

// ---------------------------------------------------------------------------
// Bottom info cards (service + payment note)
// ---------------------------------------------------------------------------

func drawInfoCards(pdf *fpdf.Fpdf, doc Document, y float64) float64 {
	gap := 5.0
	w := (contentW - gap) / 2
	xL := pageMargin
	xR := pageMargin + w + gap

	lH := measureInfoH(pdf, doc, w, true)
	rH := measureInfoH(pdf, doc, w, false)
	h := maxF(lH, rH)

	drawInfoCard(pdf, xL, y, w, h, doc, true)
	drawInfoCard(pdf, xR, y, w, h, doc, false)
	return y + h
}

func measureInfoH(pdf *fpdf.Fpdf, doc Document, cardW float64, svc bool) float64 {
	pad := 5.0
	hdrH := 7.0
	bodyW := cardW - 2*pad
	var txt string
	if svc {
		// Measure with bold since dates render bold (wider glyphs → more lines).
		pdf.SetFont(fontRoboto, "B", 10)
		txt = svcPlain(doc.Language, doc.StayStartDate, doc.StayEndDate)
	} else {
		pdf.SetFont(fontRoboto, "", 10)
		txt = payNote(doc)
	}
	lines := wrapLinesMeasured(pdf, txt, bodyW)
	return pad + hdrH + float64(len(lines))*5.5 + pad
}

func drawInfoCard(pdf *fpdf.Fpdf, x, y, w, h float64, doc Document, svc bool) {
	pad := 5.0
	pdf.SetFillColor(255, 255, 255)
	pdf.SetDrawColor(cBorderR, cBorderG, cBorderB)
	pdf.SetLineWidth(0.2)
	pdf.RoundedRect(x, y, w, h, cornerRadius, "1234", "FD")

	ix := x + pad
	iy := y + pad
	iconSz := 5.0

	if svc {
		drawClipboardIcon(pdf, ix, iy, iconSz, cAccentR, cAccentG, cAccentB)
		pdf.SetTextColor(cAccentR, cAccentG, cAccentB)
		title := strings.ToUpper(lbl(doc.Language, "service_summary"))
		pdf.SetFont(fontRoboto, "B", 8.5)
		pdf.SetXY(ix+iconSz+3, iy)
		pdf.CellFormat(w-2*pad-iconSz-3, 5, title, "", 1, "L", false, 0, "")
		cy := pdf.GetY() + 2
		writeSvcRich(pdf, ix, cy, w-2*pad, doc)
	} else {
		drawClipboardIcon(pdf, ix, iy, iconSz, cOrangeR, cOrangeG, cOrangeB)
		pdf.SetTextColor(cOrangeR, cOrangeG, cOrangeB)
		title := strings.ToUpper(lbl(doc.Language, "payment_note_title"))
		pdf.SetFont(fontRoboto, "B", 8.5)
		pdf.SetXY(ix+iconSz+3, iy)
		pdf.CellFormat(w-2*pad-iconSz-3, 5, title, "", 1, "L", false, 0, "")
		cy := pdf.GetY() + 2
		pdf.SetFont(fontRoboto, "", 10)
		pdf.SetTextColor(cBodyR, cBodyG, cBodyB)
		pdf.SetXY(ix, cy)
		pdf.MultiCell(w-2*pad, 5.2, payNote(doc), "", "L", false)
	}
}

func writeSvcRich(pdf *fpdf.Fpdf, x, y, maxW float64, doc Document) {
	const lh = 5.2

	// pdf.Write() wraps at the page right margin, not at any cell width.
	// Temporarily move the right margin to the card's right edge so text wraps correctly.
	_, _, savedR, _ := pdf.GetMargins()
	pdf.SetRightMargin(210.0 - (x + maxW))
	defer pdf.SetRightMargin(savedR)

	pdf.SetXY(x, y)
	pdf.SetFont(fontRoboto, "", 10)
	pdf.SetTextColor(cBodyR, cBodyG, cBodyB)
	if doc.Language == "sk" {
		pdf.Write(lh, "Ubytovanie za pobyt v term\u00edne ")
	} else {
		pdf.Write(lh, "Accommodation service for stay from ")
	}
	pdf.SetFont(fontRoboto, "B", 10)
	pdf.SetTextColor(cNavyR, cNavyG, cNavyB)
	pdf.Write(lh, fmtDate(doc.StayStartDate))
	pdf.SetFont(fontRoboto, "", 10)
	pdf.SetTextColor(cBodyR, cBodyG, cBodyB)
	if doc.Language == "sk" {
		pdf.Write(lh, " \u2013 ")
	} else {
		pdf.Write(lh, " to ")
	}
	pdf.SetFont(fontRoboto, "B", 10)
	pdf.SetTextColor(cNavyR, cNavyG, cNavyB)
	pdf.Write(lh, fmtDate(doc.StayEndDate))
	pdf.SetFont(fontRoboto, "", 10)
	pdf.SetTextColor(cBodyR, cBodyG, cBodyB)
	pdf.Write(lh, ".")
}

// ---------------------------------------------------------------------------
// Footer banner
// ---------------------------------------------------------------------------

func drawFooter(pdf *fpdf.Fpdf, doc Document, y float64) {
	h := 22.0
	x := pageMargin
	w := contentW

	pdf.SetFillColor(cFootBgR, cFootBgG, cFootBgB)
	pdf.SetDrawColor(cBorderR, cBorderG, cBorderB)
	pdf.SetLineWidth(0.15)
	pdf.RoundedRect(x, y, w, h, cornerRadius, "1234", "FD")

	// Heart in circle
	ccx := x + 12.0
	ccy := y + h/2
	pdf.SetFillColor(cNavyR, cNavyG, cNavyB)
	pdf.Circle(ccx, ccy, 5.5, "F")
	drawHeart(pdf, ccx, ccy-0.3, 3.2)

	// Centered text
	mainTxt := footerMain(doc.Language)
	subTxt := footerSub(doc.Language)
	pdf.SetFont(fontRoboto, "B", 10)
	mw := pdf.GetStringWidth(mainTxt)
	pdf.SetFont(fontRoboto, "", 8)
	sw := pdf.GetStringWidth(subTxt)
	tw := maxF(mw, sw)
	mx := x + w/2

	pdf.SetFont(fontRoboto, "B", 10)
	pdf.SetTextColor(cNavyR, cNavyG, cNavyB)
	pdf.SetXY(mx-tw/2, y+h/2-5.5)
	pdf.CellFormat(tw, 5.5, mainTxt, "", 1, "C", false, 0, "")
	pdf.SetFont(fontRoboto, "", 8)
	pdf.SetTextColor(cMutedR, cMutedG, cMutedB)
	pdf.SetXY(mx-tw/2, pdf.GetY())
	pdf.CellFormat(tw, 4, subTxt, "", 0, "C", false, 0, "")

	// Decorative invoice + euro coin
	drawInvoiceDoodle(pdf, x+w-24, y+2.5, h-5)
}

func drawHeart(pdf *fpdf.Fpdf, cx, cy, r float64) {
	pdf.SetFillColor(255, 255, 255)
	// Two circles for top bumps
	pdf.Circle(cx-r*0.28, cy-r*0.1, r*0.34, "F")
	pdf.Circle(cx+r*0.28, cy-r*0.1, r*0.34, "F")
	// Triangle for bottom
	pdf.SetFillColor(255, 255, 255)
	pdf.MoveTo(cx-r*0.6, cy)
	pdf.LineTo(cx, cy+r*0.65)
	pdf.LineTo(cx+r*0.6, cy)
	pdf.ClosePath()
	pdf.DrawPath("F")
}

func drawInvoiceDoodle(pdf *fpdf.Fpdf, x, y, h float64) {
	w := h * 0.65
	cr := 1.2

	// Paper with folded corner
	pdf.SetFillColor(255, 255, 255)
	pdf.SetDrawColor(cNavyR, cNavyG, cNavyB)
	pdf.SetLineWidth(0.25)

	fold := w * 0.25
	pdf.MoveTo(x+cr, y)
	pdf.LineTo(x+w-fold, y)
	pdf.LineTo(x+w, y+fold)
	pdf.LineTo(x+w, y+h-cr)
	pdf.CurveBezierCubicTo(x+w, y+h, x+w, y+h, x+w-cr, y+h)
	pdf.LineTo(x+cr, y+h)
	pdf.CurveBezierCubicTo(x, y+h, x, y+h, x, y+h-cr)
	pdf.LineTo(x, y+cr)
	pdf.CurveBezierCubicTo(x, y, x, y, x+cr, y)
	pdf.ClosePath()
	pdf.DrawPath("FD")

	// Fold triangle
	pdf.SetFillColor(cFootBgR, cFootBgG, cFootBgB)
	pdf.MoveTo(x+w-fold, y)
	pdf.LineTo(x+w-fold, y+fold)
	pdf.LineTo(x+w, y+fold)
	pdf.ClosePath()
	pdf.DrawPath("FD")

	// Text lines on paper
	pdf.SetDrawColor(cBorderR, cBorderG, cBorderB)
	pdf.SetLineWidth(0.2)
	lx1 := x + w*0.15
	lx2 := x + w*0.75
	for i := 0; i < 4; i++ {
		ly := y + h*0.3 + float64(i)*h*0.12
		pdf.Line(lx1, ly, lx2, ly)
	}

	// Euro coin circle
	coinR := h * 0.18
	coinX := x + w - coinR*0.5
	coinY := y + h - coinR*0.5
	pdf.SetFillColor(cNavyR, cNavyG, cNavyB)
	pdf.Circle(coinX, coinY, coinR, "F")
	pdf.SetFont(fontRoboto, "B", coinR*2.8)
	pdf.SetTextColor(255, 255, 255)
	pdf.SetXY(coinX-coinR, coinY-coinR)
	pdf.CellFormat(coinR*2, coinR*2, "\u20ac", "", 0, "CM", false, 0, "")
}

// ---------------------------------------------------------------------------
// Labels and formatters
// ---------------------------------------------------------------------------

func docTitle(lang string) string {
	if lang == "sk" { return "Fakt\u00fara" }
	return "Invoice"
}

func docTitleSub(lang string) string {
	if lang == "sk" { return "Invoice" }
	return "Tax invoice"
}

func isPaid(doc Document) bool {
	return strings.EqualFold(strings.TrimSpace(doc.PaymentStatus), "paid")
}

func paidLabel(lang string) string {
	if lang == "sk" { return "ZAPLATEN\u00c1" }
	return "PAID"
}

func partyTitle(lang string, sup bool) string {
	if lang == "sk" {
		if sup { return "Dod\u00e1vate\u013e" }
		return "Odberate\u013e"
	}
	if sup { return "Supplier" }
	return "Customer"
}

func detailTitle(lang string) string {
	if lang == "sk" { return "DETAILY FAKT\u00daRY" }
	return "INVOICE DETAILS"
}

func lbl(lang, key string) string {
	if lang == "sk" {
		switch key {
		case "invoice_number":
			return "\u010c\u00edslo fakt\u00fary (variabiln\u00fd symbol)"
		case "issue_date":
			return "D\u00e1tum vystavenia"
		case "taxable_supply_date":
			return "D\u00e1tum dodania"
		case "due_date":
			return "D\u00e1tum splatnosti"
		case "stay_period":
			return "Term\u00edn pobytu"
		case "amount_total":
			return "Suma na \u00fahradu"
		case "service_summary":
			return "Popis slu\u017eby"
		case "payment_note_title":
			return "Pozn\u00e1mka k \u00fahrade"
		}
	}
	switch key {
	case "invoice_number":
		return "Invoice number (variable symbol)"
	case "issue_date":
		return "Issue Date"
	case "taxable_supply_date":
		return "Taxable Supply Date"
	case "due_date":
		return "Due Date"
	case "stay_period":
		return "Stay Period"
	case "amount_total":
		return "Amount due"
	case "service_summary":
		return "Service Summary"
	case "payment_note_title":
		return "Payment Note"
	default:
		return key
	}
}

func svcPlain(lang string, s, e time.Time) string {
	if lang == "sk" {
		return fmt.Sprintf("Ubytovanie za pobyt v term\u00edne %s \u2013 %s.", fmtDate(s), fmtDate(e))
	}
	return fmt.Sprintf("Accommodation service for stay from %s to %s.", fmtDate(s), fmtDate(e))
}

func payNote(doc Document) string {
	if strings.TrimSpace(doc.PaymentNote) != "" {
		return doc.PaymentNote
	}
	if doc.Language == "sk" {
		return "Fakt\u00fara je u\u017e uhraden\u00e1. Z\u00e1kazn\u00edk zaplatil cez Booking.com a \u010fal\u0161ia platba nie je potrebn\u00e1."
	}
	return "This invoice is already paid. The customer paid via Booking.com and no further payment is required."
}

func footerMain(lang string) string {
	if lang == "sk" { return "\u010eakujeme za Va\u0161u d\u00f4veru." }
	return "Thank you for your trust."
}

func footerSub(lang string) string {
	if lang == "sk" { return "Thank you for your trust." }
	return "Invoice document"
}

func fmtMoney(lang string, cents int, cur string) string {
	a := fmt.Sprintf("%.2f", float64(cents)/100)
	if lang == "sk" {
		a = strings.ReplaceAll(a, ".", ",")
	}
	return a + " " + strings.TrimSpace(cur)
}

func fmtDate(t time.Time) string { return t.UTC().Format("2006-01-02") }

// ---------------------------------------------------------------------------
// Text wrapping
// ---------------------------------------------------------------------------

func wrapLinesByRuneBudget(txt string, n int) []string {
	if n < 8 { n = 8 }
	var out []string
	for _, p := range strings.Split(txt, "\n") {
		p = strings.TrimSpace(p)
		if p == "" { out = append(out, ""); continue }
		r := []rune(p)
		for i := 0; i < len(r); i += n {
			j := i + n; if j > len(r) { j = len(r) }
			out = append(out, string(r[i:j]))
		}
	}
	if len(out) == 0 { return []string{""} }
	return out
}

func wrapLinesMeasured(pdf *fpdf.Fpdf, txt string, maxW float64) []string {
	return wrapLinesMeasuredSafe(pdf, txt, maxW, 0.92)
}

func wrapLinesMeasuredSafe(pdf *fpdf.Fpdf, txt string, maxW float64, safety float64) []string {
	if safety <= 0 || safety > 1 { safety = 0.92 }
	maxW *= safety
	if maxW < 1 { maxW = 1 }
	if pdf.GetStringWidth("x") < 0.0001 {
		return wrapLinesByRuneBudget(txt, 40)
	}
	txt = strings.TrimSpace(txt)
	if txt == "" { return []string{""} }
	var lines []string
	for _, para := range strings.Split(txt, "\n") {
		para = strings.TrimSpace(para)
		if para == "" { lines = append(lines, ""); continue }
		words := strings.Fields(para)
		var cur string
		flush := func() { if cur != "" { lines = append(lines, cur); cur = "" } }
		for _, w := range words {
			trial := w
			if cur != "" { trial = cur + " " + w }
			if pdf.GetStringWidth(trial) <= maxW { cur = trial; continue }
			flush()
			if pdf.GetStringWidth(w) <= maxW { cur = w; continue }
			var chunk strings.Builder
			for _, r := range w {
				next := chunk.String() + string(r)
				if chunk.Len() > 0 && pdf.GetStringWidth(next) > maxW {
					lines = append(lines, chunk.String())
					chunk.Reset()
				}
				chunk.WriteRune(r)
			}
			if chunk.Len() > 0 { cur = chunk.String() }
		}
		flush()
	}
	if len(lines) == 0 { return []string{""} }
	return lines
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func maxF(a, b float64) float64 { if a > b { return a }; return b }
func maxI(a, b int) int         { if a > b { return a }; return b }

// suppress unused import
var _ = math.Pi
