package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"godex"
)

var godexIOMutex sync.Mutex

func runGoDEXPackLabel(companyName, bruttoText, epc, kgText, productName string, timeout time.Duration) ZebraStatus {
	lg := workerLog("worker.godex_action")
	lg.Printf("label start: company=%s epc=%s kg=%s product=%s timeout=%s", strings.TrimSpace(companyName), strings.TrimSpace(epc), strings.TrimSpace(kgText), strings.TrimSpace(productName), timeout)

	godexIOMutex.Lock()
	defer godexIOMutex.Unlock()
	zebraIOMutex.Lock()
	defer zebraIOMutex.Unlock()

	st := ZebraStatus{
		Action:     "label",
		Verify:     "SKIPPED",
		DevicePath: fmt.Sprintf("usb:%04x:%04x", godex.VendorID, godex.ProductID),
		Name:       "GoDEX G500",
		LastEPC:    strings.ToUpper(strings.TrimSpace(epc)),
		UpdatedAt:  time.Now(),
		Attempts:   1,
		Note:       "godex label printed; rfid encode skipped",
	}
	if strings.TrimSpace(companyName) == "" {
		companyName = "Accord"
	}
	if strings.TrimSpace(bruttoText) == "" {
		bruttoText = "5kg"
	}
	if strings.TrimSpace(productName) == "" {
		productName = st.LastEPC
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	startedAt := time.Now()
	out := printGoDEXPackLabel(st, companyName, bruttoText, kgText, productName)
	if elapsed := time.Since(startedAt); elapsed > timeout && strings.TrimSpace(out.Error) == "" {
		out.Note = strings.TrimSpace(out.Note + " slow_print=" + elapsed.Round(time.Millisecond).String())
	}
	lg.Printf("label done: device=%s line1=%s line2=%s note=%s error=%s", out.DevicePath, out.ReadLine1, out.ReadLine2, out.Note, out.Error)
	return out
}

func printGoDEXPackLabel(st ZebraStatus, companyName, bruttoText, kgText, productName string) ZebraStatus {
	var last ZebraStatus
	for attempt := 1; attempt <= 3; attempt++ {
		out := printGoDEXPackLabelOnce(st, companyName, bruttoText, kgText, productName)
		out.Attempts = attempt
		last = out
		if strings.TrimSpace(out.Error) == "" {
			return out
		}
		if !isRetryableGoDEXPrintError(out.Error) {
			return out
		}
		time.Sleep(time.Duration(attempt) * 700 * time.Millisecond)
	}
	return last
}

func printGoDEXPackLabelOnce(st ZebraStatus, companyName, bruttoText, kgText, productName string) ZebraStatus {
	printer, err := godex.OpenG500()
	if err != nil {
		st.Error = err.Error()
		return st
	}
	defer func() {
		if err := printer.Close(); err != nil && strings.TrimSpace(st.Error) == "" {
			st.Error = err.Error()
		}
	}()

	st.Connected = true
	status, err := printer.Status()
	if err != nil {
		st.Error = err.Error()
		return st
	}
	st.ReadLine1 = safeText("-", status)

	if strings.TrimSpace(status) != "" && !strings.HasPrefix(strings.TrimSpace(status), "00,") {
		recovered, err := printer.Recover()
		if err != nil {
			st.Error = err.Error()
			return st
		}
		st.ReadLine1 = safeText("-", recovered)
	}

	finalStatus, err := printer.PrintPack(godex.PackLabel{
		CompanyName: companyName,
		ProductName: productName,
		KGText:      kgText,
		BruttoText:  bruttoText,
		EPC:         st.LastEPC,
	}, godex.DefaultPackLabelOptions())
	if err != nil {
		st.Error = err.Error()
		return st
	}
	st.ReadLine2 = safeText("-", finalStatus)
	st.DeviceState = safeText("-", finalStatus)
	st.MediaState = "label"
	return st
}

func isRetryableGoDEXPrintError(errText string) bool {
	errText = strings.ToLower(strings.TrimSpace(errText))
	if errText == "" {
		return false
	}
	// Once an EZPL print command has been sent, retrying can create duplicate labels.
	if strings.Contains(errText, "send print command") {
		return false
	}
	return strings.Contains(errText, "no device") ||
		strings.Contains(errText, "not found") ||
		strings.Contains(errText, "busy") ||
		strings.Contains(errText, "timeout")
}
