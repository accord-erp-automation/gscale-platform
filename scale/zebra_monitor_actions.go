package main

import (
	"fmt"
	"strings"
	"time"
)

func runZebraRead(preferredDevice string, timeout time.Duration) ZebraStatus {
	lg := workerLog("worker.zebra_action")
	lg.Printf("read start: preferred_device=%s timeout=%s", preferredDevice, timeout)
	zebraIOMutex.Lock()
	defer zebraIOMutex.Unlock()

	st := ZebraStatus{
		Action:    "read",
		Verify:    "-",
		UpdatedAt: time.Now(),
		Attempts:  1,
	}

	p, err := SelectZebraPrinter(preferredDevice)
	if err != nil {
		st.Error = err.Error()
		lg.Printf("read printer select error: %v", err)
		return st
	}
	st.Connected = true
	st.DevicePath = p.DevicePath
	st.Name = p.DisplayName()

	line1, line2, verify := readbackRFIDResult(p.DevicePath, "", timeout, 5)
	st.ReadLine1 = safeText("-", line1)
	st.ReadLine2 = safeText("-", line2)
	st.Verify = verify
	st.DeviceState = safeText("-", queryVarRetry(p.DevicePath, "device.status", timeout, 3, 90*time.Millisecond))
	st.MediaState = safeText("-", queryVarRetry(p.DevicePath, "media.status", timeout, 3, 90*time.Millisecond))
	lg.Printf("read done: device=%s verify=%s line1=%s line2=%s error=%s", st.DevicePath, st.Verify, st.ReadLine1, st.ReadLine2, st.Error)
	return st
}

func runZebraEncodeAndRead(preferredDevice, epc, qtyText, bruttoText, itemName string, timeout time.Duration) ZebraStatus {
	lg := workerLog("worker.zebra_action")
	lg.Printf("encode start: preferred_device=%s epc=%s qty=%s brutto=%s item=%s timeout=%s", preferredDevice, strings.TrimSpace(epc), strings.TrimSpace(qtyText), strings.TrimSpace(bruttoText), strings.TrimSpace(itemName), timeout)
	zebraIOMutex.Lock()
	defer zebraIOMutex.Unlock()

	st := ZebraStatus{
		Action:    "encode",
		Verify:    "-",
		UpdatedAt: time.Now(),
		Attempts:  1,
	}

	norm, err := normalizeEPC(epc)
	if err != nil {
		st.Error = err.Error()
		lg.Printf("encode epc normalize error: %v", err)
		return st
	}
	attemptedEPC := norm

	p, err := SelectZebraPrinter(preferredDevice)
	if err != nil {
		st.Error = err.Error()
		lg.Printf("encode printer select error: %v", err)
		return st
	}
	st.Connected = true
	st.DevicePath = p.DevicePath
	st.Name = p.DisplayName()

	line1, line2, verify, attempts, autoTuned, err := encodeAndVerify(p.DevicePath, norm, qtyText, bruttoText, itemName, timeout)
	if err != nil {
		st.Error = err.Error()
		lg.Printf("encode attempt error: device=%s err=%v", p.DevicePath, err)
		applyZebraSnapshot(&st, p, timeout)
		return st
	}
	st.ReadLine1 = safeText("-", line1)
	st.ReadLine2 = safeText("-", line2)
	st.Verify = verify
	st.Attempts = attempts
	st.AutoTuned = autoTuned
	// Operator talabi bo'yicha encode urinish EPC'si har doim bridge'ga beriladi.
	// Verify alohida signal sifatida qoladi (MATCH/WRITTEN/MISMATCH/NO TAG).
	st.LastEPC = attemptedEPC

	st.DeviceState = safeText("-", queryVarRetry(p.DevicePath, "device.status", timeout, 3, 90*time.Millisecond))
	st.MediaState = safeText("-", queryVarRetry(p.DevicePath, "media.status", timeout, 3, 90*time.Millisecond))
	if !isVerifySuccess(st.Verify) && strings.TrimSpace(st.Error) == "" {
		st.Note = strings.TrimSpace(strings.Join([]string{st.Note, "verify=" + st.Verify, "epc_attempt=" + attemptedEPC}, " "))
	}
	lg.Printf("encode done: device=%s verify=%s last_epc=%s line1=%s line2=%s attempts=%d autotuned=%v note=%s error=%s", st.DevicePath, st.Verify, st.LastEPC, st.ReadLine1, st.ReadLine2, st.Attempts, st.AutoTuned, st.Note, st.Error)
	return st
}

func runZebraLabelOnlyPrint(preferredDevice, epc, qtyText, bruttoText, itemName string, timeout time.Duration) ZebraStatus {
	lg := workerLog("worker.zebra_action")
	lg.Printf("label start: preferred_device=%s epc=%s qty=%s brutto=%s item=%s timeout=%s", preferredDevice, strings.TrimSpace(epc), strings.TrimSpace(qtyText), strings.TrimSpace(bruttoText), strings.TrimSpace(itemName), timeout)
	zebraIOMutex.Lock()
	defer zebraIOMutex.Unlock()

	st := ZebraStatus{
		Action:    "label",
		Verify:    "SKIPPED",
		UpdatedAt: time.Now(),
		Attempts:  1,
		Note:      "rfid encode skipped",
	}

	norm, err := normalizeEPC(epc)
	if err != nil {
		st.Error = err.Error()
		lg.Printf("label epc normalize error: %v", err)
		return st
	}

	p, err := SelectZebraPrinter(preferredDevice)
	if err != nil {
		st.Error = err.Error()
		lg.Printf("label printer select error: %v", err)
		return st
	}
	st.Connected = true
	st.DevicePath = p.DevicePath
	st.Name = p.DisplayName()

	stream, err := buildLabelOnlyPrintCommandWithWeights(norm, qtyText, bruttoText, itemName)
	if err != nil {
		st.Error = err.Error()
		lg.Printf("label stream build error: device=%s err=%v", p.DevicePath, err)
		applyZebraSnapshot(&st, p, timeout)
		return st
	}

	if err := sendRawRetry(p.DevicePath, []byte(stream), 8, 120*time.Millisecond); err != nil {
		if isBusyLikeError(err) {
			st.Error = fmt.Errorf("%w (printer busy: boshqa process /dev/usb/lp0 ni band qilgan)", err).Error()
		} else {
			st.Error = err.Error()
		}
		lg.Printf("label send error: device=%s err=%v", p.DevicePath, err)
		applyZebraSnapshot(&st, p, timeout)
		return st
	}

	waitReady(p.DevicePath, 1600*time.Millisecond)
	st.DeviceState = safeText("-", queryVarRetry(p.DevicePath, "device.status", timeout, 3, 90*time.Millisecond))
	st.MediaState = safeText("-", queryVarRetry(p.DevicePath, "media.status", timeout, 3, 90*time.Millisecond))
	st.Note = strings.TrimSpace(strings.Join([]string{st.Note, "epc=" + norm}, " "))
	lg.Printf("label done: device=%s note=%s error=%s", st.DevicePath, st.Note, st.Error)
	return st
}

func encodeAndVerify(device, epc, qtyText, bruttoText, itemName string, timeout time.Duration) (string, string, string, int, bool, error) {
	const attempts = 1
	const autoTuned = false

	// Har encode oldidan "ultra" profilni majburan qo'llaymiz:
	// - rfid.enable=on
	// - rfid.error_handling=none
	// - rfid.label_tries=3
	// - read/write power = 30 (max)
	applyRFIDUltraSettings(device)

	stream, err := buildRFIDEncodeCommandWithWeights(epc, qtyText, bruttoText, itemName)
	if err != nil {
		return "", "", "UNKNOWN", attempts, autoTuned, err
	}

	if err := sendRawRetry(device, []byte(stream), 8, 120*time.Millisecond); err != nil {
		if isBusyLikeError(err) {
			return "", "", "UNKNOWN", attempts, autoTuned, fmt.Errorf("%w (printer busy: boshqa process /dev/usb/lp0 ni band qilgan)", err)
		}
		return "", "", "UNKNOWN", attempts, autoTuned, err
	}

	// Fixed time.Sleep emas: printer RFID yozishni tugatib "ready" ga qaytguncha
	// faol kutamiz. Amalda ayrim formatlarda 1.5s kamlik qilgani uchun oynani kengaytiramiz.
	waitReady(device, 2400*time.Millisecond)

	respSamples := sampleRFIDErrorResponses(device, timeout)
	verify := inferVerifyFromRFIDSamples(respSamples)

	line1, line2 := "-", "-"
	if len(respSamples) > 0 {
		line1 = respSamples[len(respSamples)-1]
	}

	// response "WRITTEN" bo'lmasa readback bilan yana tasdiqlaymiz.
	// Bu NO TAG/UNKNOWN holatlarini kamaytiradi va haqiqiy EPC matchni ushlaydi.
	if verify != "WRITTEN" {
		r1, r2, rv := readbackRFIDResult(device, epc, timeout, 3)
		if strings.TrimSpace(r1) != "" {
			line1 = r1
		}
		if strings.TrimSpace(r2) != "" {
			line2 = r2
		}
		if strings.TrimSpace(rv) != "" && rv != "UNKNOWN" {
			verify = rv
		}
	}
	return line1, line2, verify, attempts, autoTuned, nil
}

func applyRFIDUltraSettings(device string) {
	_ = sendRawRetry(device, []byte("~PS\n"), 2, 70*time.Millisecond)
	_ = sendSGDRetry(device, `! U1 setvar "rfid.enable" "on"`, 2, 60*time.Millisecond)
	_ = sendSGDRetry(device, `! U1 setvar "rfid.error_handling" "none"`, 2, 60*time.Millisecond)
	_ = sendSGDRetry(device, `! U1 setvar "rfid.label_tries" "3"`, 2, 60*time.Millisecond)
	_ = sendSGDRetry(device, `! U1 setvar "rfid.tag.read.content" "epc"`, 2, 60*time.Millisecond)
	_ = sendSGDRetry(device, `! U1 setvar "rfid.tag.type" "gen2"`, 2, 60*time.Millisecond)

	// Firmware versiyasiga qarab key nomlari farq qilishi mumkin,
	// shu uchun barcha keng tarqalgan aliaslarga yozib chiqamiz.
	for _, cmd := range []string{
		`! U1 setvar "rfid.reader_1.power.read" "30"`,
		`! U1 setvar "rfid.reader_1.power.write" "30"`,
		`! U1 setvar "rfid.reader_power.read" "30"`,
		`! U1 setvar "rfid.reader_power.write" "30"`,
		`! U1 setvar "rfid.read_power" "30"`,
		`! U1 setvar "rfid.write_power" "30"`,
	} {
		_ = sendSGDRetry(device, cmd, 2, 60*time.Millisecond)
	}
}

func readbackRFIDResult(device, expected string, timeout time.Duration, retries int) (string, string, string) {
	if retries < 1 {
		retries = 1
	}

	var line1 string
	var line2 string
	verify := "UNKNOWN"

	for i := 0; i < retries; i++ {
		_ = sendSGDRetry(device, `! U1 setvar "rfid.tag.read.content" "epc"`, 3, 90*time.Millisecond)
		time.Sleep(70 * time.Millisecond)
		_ = sendSGDRetry(device, `! U1 do "rfid.tag.read.execute"`, 3, 90*time.Millisecond)
		time.Sleep(240 * time.Millisecond)

		line1 = queryVarRetry(device, "rfid.tag.read.result_line1", timeout, 3, 100*time.Millisecond)
		line2 = queryVarRetry(device, "rfid.tag.read.result_line2", timeout, 3, 100*time.Millisecond)
		verify = inferVerify(line1, line2, expected)
		if verify == "MATCH" || verify == "MISMATCH" || verify == "OK" {
			break
		}
	}

	return line1, line2, verify
}

func shouldAutoTune(verify string) bool {
	v := strings.ToUpper(strings.TrimSpace(verify))
	return v == "NO TAG" || v == "UNKNOWN" || v == "ERROR"
}

func isVerifySuccess(verify string) bool {
	switch strings.ToUpper(strings.TrimSpace(verify)) {
	case "MATCH", "OK", "WRITTEN":
		return true
	default:
		return false
	}
}

func inferVerifyFromRFIDResponse(resp string) string {
	u := strings.ToUpper(strings.TrimSpace(resp))
	switch {
	case strings.Contains(u, "RFID OK"):
		return "WRITTEN"
	case strings.Contains(u, "NO TAG"):
		return "NO TAG"
	case strings.Contains(u, "ERROR"):
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

func inferVerifyFromRFIDSamples(samples []string) string {
	if len(samples) == 0 {
		return "UNKNOWN"
	}
	hasNoTag := false
	hasError := false
	for _, s := range samples {
		v := inferVerifyFromRFIDResponse(s)
		switch v {
		case "WRITTEN":
			return "WRITTEN"
		case "NO TAG":
			hasNoTag = true
		case "ERROR":
			hasError = true
		}
	}
	if hasError {
		return "ERROR"
	}
	if hasNoTag {
		return "NO TAG"
	}
	return "UNKNOWN"
}

func sampleRFIDErrorResponses(device string, timeout time.Duration) []string {
	out := make([]string, 0, 8)
	for i := 0; i < 8; i++ {
		v := strings.TrimSpace(queryVarRetry(device, "rfid.error.response", timeout, 1, 0))
		if v != "" {
			out = append(out, v)
		}
		time.Sleep(90 * time.Millisecond)
	}
	return out
}

func runAutoTuneSequence(device string) string {
	notes := make([]string, 0, 4)

	if err := sendSGDRetry(device, `! U1 do "rfid.calibrate"`, 3, 120*time.Millisecond); err == nil {
		notes = append(notes, "rfid.calibrate")
		waitReady(device, 3*time.Second)
	}
	if err := sendRawRetry(device, []byte("^XA^HR^XZ\n"), 3, 140*time.Millisecond); err == nil {
		notes = append(notes, "^HR")
		waitReady(device, 3*time.Second)
	}
	if err := sendRawRetry(device, []byte("~JC\n"), 3, 140*time.Millisecond); err == nil {
		notes = append(notes, "~JC")
		waitReady(device, 4*time.Second)
	}
	if err := sendRawRetry(device, []byte("^XA^JUS^XZ\n"), 2, 120*time.Millisecond); err == nil {
		notes = append(notes, "save")
	}

	if len(notes) == 0 {
		return "auto-tune command yuborilmadi"
	}
	return "auto-tune: " + strings.Join(notes, ",")
}

func waitReady(device string, wait time.Duration) {
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		v := queryVarRetry(device, "device.status", 650*time.Millisecond, 1, 0)
		if strings.EqualFold(strings.TrimSpace(v), "ready") {
			return
		}
		time.Sleep(120 * time.Millisecond)
	}
}
