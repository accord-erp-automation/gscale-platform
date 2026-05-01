package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"batch"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	text := flag.String("text", "TEST", "label text to print, or QR payload when -qr is set")
	qr := flag.Bool("qr", false, "print a QR code instead of plain text")
	packLabel := flag.Bool("pack-label", false, "print product/company/EPC pack label with QR")
	statusOnly := flag.Bool("status-only", false, "only read printer status and exit")
	skipRecover := flag.Bool("skip-recover", false, "do not run recovery before printing")
	calibrate := flag.Bool("calibrate", false, "run recovery + sensor calibration and exit without printing")
	labelLengthMM := flag.Int("label-length-mm", 50, "EZPL label length in mm used for ^Q")
	labelGapMM := flag.Int("label-gap-mm", 3, "EZPL gap length in mm used for ^Q")
	labelWidthMM := flag.Int("label-width-mm", 50, "EZPL label width in mm used for ^W")
	dpi := flag.Int("dpi", 203, "printer resolution in dpi")
	center := flag.Bool("center", false, "place simple text/QR content near the center")
	qrBoxMM := flag.Float64("qr-box-mm", 18.0, "approximate QR bounding box size in mm")
	qrMul := flag.Int("qr-mul", 8, "EZPL QR module size multiplier for simple QR labels")
	contentXMM := flag.Float64("content-x-mm", -1, "optional absolute X position in mm for simple labels")
	contentYMM := flag.Float64("content-y-mm", -1, "optional absolute Y position in mm for simple labels")
	companyName := flag.String("company-name", "", "company name for pack label")
	productName := flag.String("product-name", "", "product name for pack label")
	kg := flag.String("kg", "", "kg value for pack label")
	epc := flag.String("epc", "", "EPC code for pack label")
	brutto := flag.String("brutto", "5", "brutto value for pack label")
	qrMode := flag.String("qr-mode", "url", "QR payload mode, kept for Python CLI compatibility")
	safeMarginMM := flag.Float64("safe-margin-mm", 4.0, "inner margin to keep content inside the printable area")
	regularFont := flag.String("regular-font", batch.DefaultNotoSansRegular, "regular TTF font path")
	boldFont := flag.String("bold-font", batch.DefaultNotoSansBold, "bold TTF font path")
	flag.Parse()

	if flag.NArg() > 0 && *text == "TEST" {
		*text = strings.Join(flag.Args(), " ")
	}

	printer, err := batch.OpenG500()
	if err != nil {
		return err
	}
	defer printer.Close()

	status, err := printer.Status()
	if err != nil {
		return err
	}
	fmt.Printf("status: %s\n", emptyStatus(status))

	if *statusOnly {
		return nil
	}
	if *calibrate {
		fmt.Println("calibrate: running sensor calibration")
		finalStatus, err := printer.Calibrate()
		if err != nil {
			return err
		}
		fmt.Printf("final_status: %s\n", emptyStatus(finalStatus))
		return nil
	}
	if status != "" && status != "00,00000" && !*skipRecover {
		fmt.Println("recover: running recovery sequence")
		recovered, err := printer.Recover()
		if err != nil {
			return err
		}
		fmt.Printf("recovered: %s\n", emptyStatus(recovered))
	}

	options := batch.LabelOptions{
		LabelLengthMM: *labelLengthMM,
		LabelGapMM:    *labelGapMM,
		LabelWidthMM:  *labelWidthMM,
		DPI:           *dpi,
		SafeMarginMM:  *safeMarginMM,
		QRBoxMM:       *qrBoxMM,
		QRMode:        *qrMode,
		RegularFont:   *regularFont,
		BoldFont:      *boldFont,
	}

	var finalStatus string
	switch {
	case *qr:
		payload := *text
		if payload == "TEST" {
			generated, err := batch.RandomQRPayload()
			if err != nil {
				return err
			}
			payload = generated
		}
		fmt.Printf("qr_payload: %s\n", payload)
		finalStatus, err = printer.PrintQR(payload, options, *center, int(*qrBoxMM), *qrMul, *contentXMM, *contentYMM)
	case *packLabel:
		finalStatus, err = printer.PrintPack(batch.PackLabel{
			CompanyName: *companyName,
			ProductName: *productName,
			KGText:      *kg,
			BruttoText:  *brutto,
			EPC:         *epc,
		}, options)
	default:
		finalStatus, err = printer.PrintText(*text, options, *center, *contentXMM, *contentYMM)
	}
	if err != nil {
		return err
	}
	fmt.Printf("final_status: %s\n", emptyStatus(finalStatus))
	return nil
}

func emptyStatus(status string) string {
	if strings.TrimSpace(status) == "" {
		return "(empty)"
	}
	return status
}
