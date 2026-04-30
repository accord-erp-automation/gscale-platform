package godex

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

func RandomQRPayload() (string, error) {
	return randomHex(18)
}

func RandomBatchCode() (string, error) {
	return randomHex(12)
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(b)), nil
}

func BuildTextLabel(text string, options LabelOptions, center bool, contentXMM, contentYMM float64) []string {
	options = normalizeSimpleOptions(options)
	text = SanitizeLabelText(text)
	x, y := 20, 20
	if contentXMM >= 0 && contentYMM >= 0 {
		x = MMDots(contentXMM, options.DPI)
		y = MMDots(contentYMM, options.DPI)
	}
	if center {
		x, y = labelCenterDots(options.LabelLengthMM, options.LabelWidthMM, options.DPI)
	}
	return []string{
		"~S,ESG",
		"^AD",
		"^XSET,IMMEDIATE,1",
		"^XSET,ACTIVERESPONSE,1",
		"^XSET,CODEPAGE,16",
		fmt.Sprintf("^Q%d,%d", options.LabelLengthMM, options.LabelGapMM),
		fmt.Sprintf("^W%d", options.LabelWidthMM),
		"^H10",
		"^P1",
		"^L",
		fmt.Sprintf("AC,%d,%d,1,1,0,0,%s", x, y, text),
		"E",
	}
}

func BuildQRLabel(payload string, options LabelOptions, center bool, qrBoxMM int, qrMul int, contentXMM, contentYMM float64) []string {
	options = normalizeSimpleOptions(options)
	payload = SanitizeLabelText(payload)
	x, y := 10, 10
	if contentXMM >= 0 && contentYMM >= 0 {
		x = MMDots(contentXMM, options.DPI)
		y = MMDots(contentYMM, options.DPI)
	}
	if center {
		cx, cy := labelCenterDots(options.LabelLengthMM, options.LabelWidthMM, options.DPI)
		box := maxInt(1, MMDots(float64(qrBoxMM), options.DPI))
		x = maxInt(0, cx-box/2)
		y = maxInt(0, cy-box/2)
	}
	if qrMul < 1 {
		qrMul = 1
	}
	return []string{
		"~S,ESG",
		"^AD",
		"^XSET,IMMEDIATE,1",
		"^XSET,ACTIVERESPONSE,1",
		"^XSET,CODEPAGE,16",
		fmt.Sprintf("^Q%d,%d", options.LabelLengthMM, options.LabelGapMM),
		fmt.Sprintf("^W%d", options.LabelWidthMM),
		"^H10",
		"^P1",
		"^L",
		fmt.Sprintf("W%d,%d,2,1,L,8,%d,%d,0", x, y, qrMul, len(payload)),
		payload,
		"E",
	}
}

func BuildDirectPackLabel(companyName, productName, qtyText, barcode, batchCode, qrPayload string, options LabelOptions) []string {
	options = normalizeSimpleOptions(options)
	companyName = SanitizeLabelText(companyName)
	productName = SanitizeLabelText(productName)
	qtyText = NormalizeKGValue(qtyText)
	barcode = SanitizeLabelText(barcode)
	batchCode = SanitizeLabelText(batchCode)
	qrPayload = SanitizeLabelText(qrPayload)
	if qrPayload == "" {
		qrPayload = batchCode
	}

	labelWidthDots := MMDots(float64(options.LabelWidthMM), options.DPI)
	labelLengthDots := MMDots(float64(options.LabelLengthMM), options.DPI)
	safeMarginDots := MMDots(options.SafeMarginMM, options.DPI)
	leftX := safeMarginDots
	gapDots := MMDots(3.0, options.DPI)
	lineStep := MMDots(5.0, options.DPI)
	companyY := safeMarginDots
	itemY := companyY + lineStep
	qrBoxMM := maxFloat(16.0, minFloat(20.0, float64(options.LabelWidthMM)*0.30))
	qrBoxDots := MMDots(qrBoxMM, options.DPI)
	qrX := maxInt(leftX, labelWidthDots-safeMarginDots-qrBoxDots)
	qrX = minInt(labelWidthDots-qrBoxDots, qrX+MMDots(1.0, options.DPI))
	barcodeY := maxInt(itemY+lineStep*3, labelLengthDots-safeMarginDots-MMDots(12.0, options.DPI))
	qrMul := 5
	textWidthDots := maxInt(1, qrX-leftX-gapDots)
	productLines := wrapTextForEZPL(productName, textWidthDots, options.DPI, 1, 14, 8)
	qtyY := itemY + len(productLines)*lineStep
	qrY := maxInt(safeMarginDots+lineStep*2, qtyY+lineStep)
	barcodeTextY := barcodeY + MMDots(8.0, options.DPI)
	barcodeTextXMul := 2
	barcodeTextWidthDots := maxInt(1, len(barcode)*14*barcodeTextXMul)
	barcodeTextX := maxInt(leftX, leftX+((labelWidthDots-leftX-safeMarginDots)-barcodeTextWidthDots)/2)

	commands := []string{
		"~S,ESG",
		"^AD",
		"^XSET,IMMEDIATE,1",
		"^XSET,ACTIVERESPONSE,1",
		"^XSET,CODEPAGE,16",
		fmt.Sprintf("^Q%d,%d", options.LabelLengthMM, options.LabelGapMM),
		fmt.Sprintf("^W%d", options.LabelWidthMM),
		"^H10",
		"^P1",
		"^L",
		fmt.Sprintf("AC,%d,%d,1,1,0,0,company name: %s", leftX, companyY, companyName),
		fmt.Sprintf("AC,%d,%d,1,1,0,0,company name: %s", leftX+1, companyY+1, companyName),
		fmt.Sprintf("AC,%d,%d,1,1,0,0,item name: %s", leftX, itemY, productLines[0]),
		fmt.Sprintf("AC,%d,%d,1,1,0,0,kg: %s", leftX, qtyY, qtyText),
	}
	for idx, line := range productLines[1:] {
		commands = append(commands, fmt.Sprintf("AC,%d,%d,1,1,0,0,%s", leftX, itemY+(idx+1)*lineStep, line))
	}
	commands = append(commands,
		fmt.Sprintf("BA,%d,%d,1,2,42,0,0,%s", leftX, barcodeY, barcode),
		fmt.Sprintf("AC,%d,%d,%d,1,0,0,%s", barcodeTextX, barcodeTextY, barcodeTextXMul, barcode),
		fmt.Sprintf("W%d,%d,2,1,L,8,%d,%d,0", qrX, qrY, qrMul, len(qrPayload)),
		qrPayload,
	)
	if batchCode != "" {
		batchY := minInt(labelLengthDots-safeMarginDots, barcodeTextY+lineStep)
		commands = append(commands, fmt.Sprintf("AC,%d,%d,1,1,0,0,%s", leftX, batchY, batchCode))
	}
	commands = append(commands, "E")
	return commands
}

func normalizeSimpleOptions(options LabelOptions) LabelOptions {
	defaults := DefaultSimpleLabelOptions()
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
		options.SafeMarginMM = 4.0
	}
	return options
}

func labelCenterDots(labelLengthMM, labelWidthMM, dpi int) (int, int) {
	return MMDots(float64(labelWidthMM)/2.0, dpi), MMDots(float64(labelLengthMM)/2.0, dpi)
}

func wrapTextForEZPL(text string, widthDots, dpi, xMul, pitchDots, minChars int) []string {
	text = SanitizeLabelText(text)
	if text == "" {
		return []string{""}
	}
	charWidth := maxInt(1, pitchDots*maxInt(1, xMul))
	widthChars := maxInt(minChars, widthDots/charWidth)
	lines := wrapWordsByRuneCount(text, widthChars, false)
	for _, line := range lines {
		if len([]rune(line)) > widthChars {
			return wrapWordsByRuneCount(text, widthChars, true)
		}
	}
	return lines
}

func wrapWordsByRuneCount(text string, width int, breakLong bool) []string {
	words := strings.Fields(text)
	lines := []string{}
	current := ""
	for _, word := range words {
		candidate := word
		if current != "" {
			candidate = current + " " + word
		}
		if len([]rune(candidate)) <= width {
			current = candidate
			continue
		}
		if current != "" {
			lines = append(lines, current)
			current = ""
		}
		if !breakLong || len([]rune(word)) <= width {
			current = word
			continue
		}
		runes := []rune(word)
		for len(runes) > width {
			lines = append(lines, string(runes[:width]))
			runes = runes[width:]
		}
		current = string(runes)
	}
	if current != "" {
		lines = append(lines, current)
	}
	if len(lines) == 0 {
		return []string{text}
	}
	return lines
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
