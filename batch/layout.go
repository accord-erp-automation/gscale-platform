package batch

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"golang.org/x/image/font"
)

func MMDots(mm float64, dpi int) int {
	return int(mm*float64(dpi)/25.4 + 0.5)
}

func BuildPackLabel(input PackLabel, options LabelOptions) (PackLabelData, error) {
	options = normalizePackOptions(options)
	fonts, err := LoadFontSet(options.RegularFont, options.BoldFont)
	if err != nil {
		return PackLabelData{}, err
	}
	defer fonts.Close()

	companyName := strings.ToUpper(SanitizeLabelText(input.CompanyName))
	productName := strings.ToUpper(SanitizeLabelText(input.ProductName))
	kgText := NormalizeKGValue(input.KGText)
	bruttoText := NormalizeKGValue(input.BruttoText)
	epc := strings.ToUpper(SanitizeLabelText(input.EPC))
	if companyName == "" || productName == "" || kgText == "" || epc == "" {
		return PackLabelData{}, fmt.Errorf("company, product, kg, and epc are required")
	}
	if bruttoText == "" {
		bruttoText = "5"
	}

	qrPayload := EncodeScanPayload(companyName, productName, kgText, bruttoText, epc)
	labelWidthDots := MMDots(float64(options.LabelWidthMM), options.DPI)
	labelLengthDots := MMDots(float64(options.LabelLengthMM), options.DPI)
	safeMarginDots := MMDots(options.SafeMarginMM, options.DPI)
	leftX := maxInt(0, safeMarginDots-MMDots(2.0, options.DPI))
	lineStep := MMDots(5.0, options.DPI)

	qrBoxDots := MMDots(options.QRBoxMM, options.DPI)
	// QR'ni biroz o'ngroqqa surish uchun o'ngdagi xavfsiz bo'shliqni
	// 6mm dan 4mm ga tushiramiz, lekin label chetidan ham chiqarmaymiz.
	qrRightGapDots := MMDots(4.0, options.DPI)
	baseQRX := labelWidthDots - qrBoxDots - qrRightGapDots
	qrX := minInt(labelWidthDots-qrBoxDots, maxInt(leftX, baseQRX))

	productFirstLineWidthDots := maxInt(1, labelWidthDots-leftX)
	productRestLineWidthDots := maxInt(1, qrX-leftX-MMDots(5.0, options.DPI))
	companyText, productLines, nettoText, bruttoLine, epcText := measurePackText(
		fonts,
		companyName,
		productName,
		kgText,
		bruttoText,
		epc,
		productFirstLineWidthDots,
		productRestLineWidthDots,
	)

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
	barcodeY := maxInt(0, epcY+MMDots(3.0, options.DPI))
	barcodeX := maxInt(0, leftX-MMDots(2.0, options.DPI))

	qrGraphicBytes, err := RenderQRGraphic(qrPayload, qrBoxDots)
	if err != nil {
		return PackLabelData{}, err
	}
	textGraphicBytes, err := renderTextGraphic(
		labelWidthDots,
		labelLengthDots,
		leftX,
		companyY,
		itemY,
		qtyY,
		bruttoY,
		epcY,
		companyText,
		productLines,
		nettoText,
		bruttoLine,
		epcText,
		fonts,
	)
	if err != nil {
		return PackLabelData{}, err
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
		fmt.Sprintf("BA,%d,%d,1,2,42,0,0,%s", barcodeX, barcodeY, epc),
		fmt.Sprintf("Y%d,%d,%s", qrX, qrY, QRGraphicName),
		"E",
	}

	return PackLabelData{
		Commands:       commands,
		TextGraphicBMP: textGraphicBytes,
		QRGraphicBMP:   qrGraphicBytes,
		QRPayload:      qrPayload,
	}, nil
}

func normalizePackOptions(options LabelOptions) LabelOptions {
	defaults := DefaultPackLabelOptions()
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

func measurePackText(
	fonts *FontSet,
	companyName string,
	productName string,
	kgText string,
	bruttoText string,
	epc string,
	productFirstLineWidthDots int,
	productRestLineWidthDots int,
) (string, []string, string, string, string) {
	companyText := "COMPANY: " + companyName
	productLines := wrapPrefixedTextPixels(
		"MAHSULOT NOMI:",
		productName,
		fonts.Bold21,
		productFirstLineWidthDots,
		productRestLineWidthDots,
	)
	nettoText := strings.ToUpper("NETTO: " + kgText + " KG")
	bruttoLine := strings.ToUpper("BRUTTO: " + bruttoText + " KG")
	epcText := "EPC: " + epc
	return companyText, productLines, nettoText, bruttoLine, epcText
}

func wrapTextPixels(text string, face font.Face, maxWidth int) []string {
	text = SanitizeLabelText(text)
	if text == "" {
		return []string{""}
	}
	words := strings.Fields(text)
	lines := make([]string, 0, len(words))
	current := ""
	for _, word := range words {
		candidate := word
		if current != "" {
			candidate = current + " " + word
		}
		if textWidth(face, candidate) <= maxWidth {
			current = candidate
			continue
		}
		if current != "" {
			lines = append(lines, current)
		}
		if textWidth(face, word) <= maxWidth {
			current = word
			continue
		}
		chunk := ""
		for _, ch := range word {
			candidate = chunk + string(ch)
			if chunk == "" || textWidth(face, candidate) <= maxWidth {
				chunk = candidate
				continue
			}
			if chunk != "" {
				lines = append(lines, chunk)
			}
			chunk = string(ch)
		}
		current = chunk
	}
	if current != "" {
		lines = append(lines, current)
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func wrapPrefixedTextPixels(prefix, text string, face font.Face, firstLineWidth, restLineWidth int) []string {
	prefix = SanitizeLabelText(prefix)
	text = SanitizeLabelText(text)
	if text == "" {
		if prefix != "" {
			return []string{prefix}
		}
		return []string{""}
	}
	prefixRender := ""
	if prefix != "" {
		prefixRender = prefix + " "
	}
	prefixWidth := textWidth(face, prefixRender)
	if prefix == "" {
		return wrapTextPixels(text, face, firstLineWidth)
	}
	if prefixWidth >= firstLineWidth {
		return append([]string{prefix}, wrapTextPixels(text, face, restLineWidth)...)
	}

	bodyWords := strings.Fields(text)
	if len(bodyWords) == 0 {
		return []string{strings.TrimSpace(prefixRender)}
	}

	lines := []string{}
	currentWords := []string{}
	currentWidth := firstLineWidth - prefixWidth
	lineIndex := 0
	lineLimitFor := func(index int) int {
		if index < 6 {
			return firstLineWidth
		}
		return restLineWidth
	}

	for _, word := range bodyWords {
		candidateWords := append(append([]string{}, currentWords...), word)
		candidate := strings.Join(candidateWords, " ")
		if textWidth(face, candidate) <= currentWidth {
			currentWords = append(currentWords, word)
			continue
		}
		if len(lines) == 0 {
			lines = append(lines, strings.TrimSpace(prefixRender+strings.Join(currentWords, " ")))
		} else {
			lines = append(lines, strings.Join(currentWords, " "))
		}
		lineIndex++
		currentWords = []string{word}
		currentWidth = lineLimitFor(lineIndex)
	}
	if len(currentWords) > 0 {
		if len(lines) == 0 {
			lines = append(lines, strings.TrimSpace(prefixRender+strings.Join(currentWords, " ")))
		} else {
			lines = append(lines, strings.Join(currentWords, " "))
		}
	}
	return lines
}

func renderTextGraphic(
	labelWidthDots int,
	labelLengthDots int,
	leftX int,
	companyY int,
	itemY int,
	qtyY int,
	bruttoY int,
	epcY int,
	companyText string,
	productLines []string,
	nettoText string,
	bruttoText string,
	epcText string,
	fonts *FontSet,
) ([]byte, error) {
	canvas := image.NewRGBA(image.Rect(0, 0, labelWidthDots, labelLengthDots))
	for y := 0; y < canvas.Bounds().Dy(); y++ {
		for x := 0; x < canvas.Bounds().Dx(); x++ {
			canvas.SetRGBA(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}

	drawTextTop(canvas, leftX, epcY, fonts.Regular20, epcText)
	drawTextTop(canvas, leftX, companyY, fonts.Bold24, companyText)
	for idx, line := range productLines {
		drawTextTop(canvas, leftX, itemY+idx*28, fonts.Bold21, line)
	}
	drawTextTop(canvas, leftX, qtyY, fonts.Regular26, nettoText)
	drawTextTop(canvas, leftX, bruttoY, fonts.Regular26, bruttoText)

	cropped := cropInk(canvas)
	return EncodeMonoBMP(cropped)
}

func cropInk(img *image.RGBA) image.Image {
	b := img.Bounds()
	minX, minY := b.Max.X, b.Max.Y
	maxX, maxY := b.Min.X, b.Min.Y
	found := false
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if !isLight(img.At(x, y)) {
				if x < minX {
					minX = x
				}
				if y < minY {
					minY = y
				}
				if x+1 > maxX {
					maxX = x + 1
				}
				if y+1 > maxY {
					maxY = y + 1
				}
				found = true
			}
		}
	}
	if !found {
		return img
	}
	minX = maxInt(b.Min.X, minX-1)
	maxX = minInt(b.Max.X, maxX+1)
	return img.SubImage(image.Rect(minX, minY, maxX, maxY))
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
