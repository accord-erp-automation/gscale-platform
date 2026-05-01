package batch

import (
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"strings"
	"time"
)

func EncodeArchiveBatchPayload(sessionID, itemName, qtyText, batchTime string) string {
	parts := []string{
		"ARCHIVE",
		strings.TrimSpace(sessionID),
		strings.TrimSpace(itemName),
		strings.TrimSpace(qtyText),
		strings.TrimSpace(batchTime),
	}
	raw := strings.Join(parts, "\n")
	return DefaultArchiveQRBaseURL + base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func BuildArchiveBatchLabel(input ArchiveBatchLabel, options LabelOptions) (ArchiveBatchData, error) {
	options = normalizeArchiveOptions(options)

	fonts, err := LoadFontSet(options.RegularFont, options.BoldFont)
	if err != nil {
		return ArchiveBatchData{}, err
	}
	defer fonts.Close()

	sessionID := SanitizeLabelText(input.SessionID)
	itemName := strings.ToUpper(SanitizeLabelText(input.ItemName))
	qtyText := NormalizeKGValue(input.QtyText)
	batchTime := FormatArchiveBatchTime(input.BatchTime)
	if sessionID == "" {
		sessionID = "-"
	}
	if itemName == "" {
		itemName = "-"
	}
	if qtyText == "" {
		qtyText = "0"
	}
	if batchTime == "" {
		batchTime = "-"
	}

	qrPayload := EncodeArchiveBatchPayload(sessionID, itemName, qtyText, batchTime)
	labelWidthDots := MMDots(float64(options.LabelWidthMM), options.DPI)
	labelLengthDots := MMDots(float64(options.LabelLengthMM), options.DPI)
	safeMarginDots := MMDots(options.SafeMarginMM, options.DPI)
	leftX := maxInt(0, safeMarginDots-MMDots(2.0, options.DPI))
	lineStep := MMDots(5.0, options.DPI)

	qrBoxDots := MMDots(options.QRBoxMM, options.DPI)
	qrRightGapDots := MMDots(4.0, options.DPI)
	baseQRX := labelWidthDots - qrBoxDots - qrRightGapDots
	qrX := minInt(labelWidthDots-qrBoxDots, maxInt(leftX, baseQRX))
	productFirstLineWidthDots := maxInt(1, labelWidthDots-leftX)
	productRestLineWidthDots := maxInt(1, qrX-leftX-MMDots(5.0, options.DPI))

	itemLines := wrapPrefixedTextPixels(
		"MAHSULOT NOMI:",
		itemName,
		fonts.Bold21,
		productFirstLineWidthDots,
		productRestLineWidthDots,
	)
	if len(itemLines) == 0 {
		itemLines = []string{"-"}
	}

	// Keep the same text rhythm as the production pack label, but drop the
	// company/EPC/barcode content. Date is placed into the lower slot.
	companyY := safeMarginDots + lineStep*2
	itemY := companyY + lineStep
	qtyY := MMDots(33.0, options.DPI)
	bruttoY := maxInt(0, qtyY+lineStep)
	qrY := maxInt(safeMarginDots+lineStep*2, qtyY+lineStep)
	qrY = minInt(labelLengthDots-safeMarginDots-MMDots(18.0, options.DPI), qrY+MMDots(8.0, options.DPI))
	dateY := maxInt(bruttoY+lineStep, labelLengthDots-safeMarginDots-MMDots(10.0, options.DPI))

	nettoText := strings.ToUpper("NETTO: " + qtyText + " KG")
	bruttoText := strings.ToUpper("BRUTTO: " + qtyText + " KG")
	dateText := "DATE: " + batchTime

	qrGraphicBytes, err := RenderQRGraphic(qrPayload, qrBoxDots)
	if err != nil {
		return ArchiveBatchData{}, err
	}
	textGraphicBytes, err := renderArchiveTextGraphic(
		labelWidthDots,
		labelLengthDots,
		leftX,
		itemY,
		qtyY,
		bruttoY,
		dateY,
		itemLines,
		nettoText,
		bruttoText,
		dateText,
		fonts,
	)
	if err != nil {
		return ArchiveBatchData{}, err
	}

	commands := []string{
		"~S,ESG",
		"^AD",
		"^XSET,UNICODE,1",
		"^XSET,IMMEDIATE,1",
		"^XSET,ACTIVERESPONSE,1",
		"^XSET,CODEPAGE,16",
		fmt.Sprintf("^Q%d,%d", options.LabelLengthMM, options.LabelGapMM),
		fmt.Sprintf("^W%d", options.LabelWidthMM),
		"^H10",
		"^P1",
		"^L",
		fmt.Sprintf("Y0,0,%s", TextGraphicName),
		fmt.Sprintf("Y%d,%d,%s", qrX, qrY, QRGraphicName),
		"E",
	}

	return ArchiveBatchData{
		Commands:       commands,
		TextGraphicBMP: textGraphicBytes,
		QRGraphicBMP:   qrGraphicBytes,
		QRPayload:      qrPayload,
	}, nil
}

func normalizeArchiveOptions(options LabelOptions) LabelOptions {
	defaults := DefaultArchiveLabelOptions()
	if options.LabelLengthMM <= 0 {
		options.LabelLengthMM = defaults.LabelLengthMM
	}
	if options.LabelGapMM <= 0 {
		options.LabelGapMM = defaults.LabelGapMM
	}
	if options.LabelWidthMM <= 0 {
		options.LabelWidthMM = defaults.LabelWidthMM
	}
	if options.DPI <= 0 {
		options.DPI = defaults.DPI
	}
	if options.SafeMarginMM <= 0 {
		options.SafeMarginMM = defaults.SafeMarginMM
	}
	if options.QRBoxMM <= 0 {
		options.QRBoxMM = defaults.QRBoxMM
	}
	if options.QRMode == "" {
		options.QRMode = defaults.QRMode
	}
	if options.RegularFont == "" {
		options.RegularFont = defaults.RegularFont
	}
	if options.BoldFont == "" {
		options.BoldFont = defaults.BoldFont
	}
	return options
}

func renderArchiveTextGraphic(
	labelWidthDots int,
	labelLengthDots int,
	leftX int,
	itemY int,
	qtyY int,
	bruttoY int,
	dateY int,
	itemLines []string,
	nettoText string,
	bruttoText string,
	dateText string,
	fonts *FontSet,
) ([]byte, error) {
	canvas := image.NewRGBA(image.Rect(0, 0, labelWidthDots, labelLengthDots))
	for y := 0; y < canvas.Bounds().Dy(); y++ {
		for x := 0; x < canvas.Bounds().Dx(); x++ {
			canvas.SetRGBA(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}

	for idx, line := range itemLines {
		drawTextTop(canvas, leftX, itemY+idx*28, fonts.Bold21, line)
	}
	drawTextTop(canvas, leftX, qtyY, fonts.Regular26, nettoText)
	drawTextTop(canvas, leftX, bruttoY, fonts.Regular26, bruttoText)
	drawTextTop(canvas, leftX, dateY, fonts.Regular20, dateText)

	cropped := cropInk(canvas)
	return EncodeMonoBMP(cropped)
}

func FormatArchiveBatchTime(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "-"
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339, raw)
	}
	if err != nil {
		return raw
	}
	return parsed.Local().Format("02 Jan 2006 15:04")
}

func FormatArchiveBatchQty(qty float64) string {
	text := fmt.Sprintf("%.1f", roundToOneDecimal(qty))
	for strings.Contains(text, ".") && strings.HasSuffix(text, "0") {
		text = strings.TrimSuffix(text, "0")
	}
	return strings.TrimSuffix(text, ".")
}
