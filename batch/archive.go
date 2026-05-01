package batch

import (
	"encoding/base64"
	"fmt"
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
	itemName := SanitizeLabelText(input.ItemName)
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

	companyY := safeMarginDots + lineStep*2
	itemY := companyY + lineStep
	qtyY := MMDots(33.0, options.DPI)
	qrY := maxInt(safeMarginDots+lineStep*2, qtyY+lineStep)
	qrY = minInt(labelLengthDots-safeMarginDots-MMDots(18.0, options.DPI), qrY+MMDots(8.0, options.DPI))
	epcY := maxInt(0, safeMarginDots-lineStep*5)
	textBlockUpDots := MMDots(3.0, options.DPI)
	headerBlockUpDots := MMDots(5.0, options.DPI)
	companyY = maxInt(0, companyY-headerBlockUpDots)
	itemY = maxInt(0, itemY-headerBlockUpDots)
	qtyY = maxInt(0, qtyY-textBlockUpDots)
	bruttoY := maxInt(0, qtyY+lineStep)
	dateY := maxInt(0, epcY+MMDots(3.0, options.DPI))

	textGraphicBytes, err := renderTextGraphic(
		labelWidthDots,
		labelLengthDots,
		leftX,
		companyY,
		itemY,
		qtyY,
		bruttoY,
		dateY,
		"",
		itemLines,
		"NETTO: "+qtyText+" KG",
		"BRUTTO: "+qtyText+" KG",
		"DATE: "+batchTime,
		fonts,
	)
	if err != nil {
		return ArchiveBatchData{}, err
	}

	qrGraphicBytes, err := RenderQRGraphic(qrPayload, qrBoxDots)
	if err != nil {
		return ArchiveBatchData{}, err
	}

	commands := []string{
		"~S,ESG",
		"^AD",
		"^XSET,IMMEDIATE,1",
		"^XSET,ACTIVERESPONSE,1",
		"^XSET,CODEPAGE,16",
		"^XSET,UNICODE,1",
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
	if options.RegularFont == "" {
		options.RegularFont = defaults.RegularFont
	}
	if options.BoldFont == "" {
		options.BoldFont = defaults.BoldFont
	}
	return options
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
