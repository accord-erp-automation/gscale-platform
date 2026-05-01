package main

import (
	bridgestate "bridge/state"
	"fmt"
	"strings"
	"time"

	"batch"
)

type archivePrintRequestReader struct {
	store      *bridgestate.Store
	cached     bool
	req        bridgestate.ArchivePrintRequestSnapshot
	nextReadAt time.Time
}

func newArchivePrintRequestReader(path string) *archivePrintRequestReader {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	return &archivePrintRequestReader{store: bridgestate.New(path)}
}

func (r *archivePrintRequestReader) Pending(now time.Time) (bridgestate.ArchivePrintRequestSnapshot, bool) {
	r.refresh(now)
	if r == nil {
		return bridgestate.ArchivePrintRequestSnapshot{}, false
	}
	if strings.ToLower(strings.TrimSpace(r.req.Status)) != "pending" {
		return bridgestate.ArchivePrintRequestSnapshot{}, false
	}
	if strings.TrimSpace(r.req.RequestID) == "" {
		return bridgestate.ArchivePrintRequestSnapshot{}, false
	}
	return r.req, true
}

func (r *archivePrintRequestReader) refresh(now time.Time) {
	if r == nil {
		return
	}
	if now.IsZero() {
		now = time.Now()
	}
	if r.cached && now.Before(r.nextReadAt) {
		return
	}

	snap, err := r.store.Read()
	if err != nil {
		r.req = bridgestate.ArchivePrintRequestSnapshot{}
		r.cached = true
		r.nextReadAt = now.Add(200 * time.Millisecond)
		return
	}

	req := snap.ArchivePrint
	req.RequestID = strings.TrimSpace(req.RequestID)
	req.SessionID = strings.TrimSpace(req.SessionID)
	req.ItemCode = strings.TrimSpace(req.ItemCode)
	req.ItemName = strings.TrimSpace(req.ItemName)
	req.Unit = normalizeArchivePrintUnit(req.Unit)
	req.BatchTime = strings.TrimSpace(req.BatchTime)
	req.Status = strings.ToLower(strings.TrimSpace(req.Status))
	req.Printer = normalizePrintRequestPrinter(req.Printer)
	if req.ItemName == "" {
		req.ItemName = req.ItemCode
	}
	if req.Unit == "" {
		req.Unit = "kg"
	}

	r.req = req
	r.cached = true
	r.nextReadAt = now.Add(200 * time.Millisecond)
}

func writeArchivePrintRequestStatus(store *bridgestate.Store, requestID, status, errText string) error {
	if store == nil {
		return nil
	}

	requestID = strings.TrimSpace(requestID)
	status = strings.ToLower(strings.TrimSpace(status))
	errText = strings.TrimSpace(errText)
	at := time.Now().UTC().Format(time.RFC3339Nano)

	return store.Update(func(snapshot *bridgestate.Snapshot) {
		if strings.TrimSpace(snapshot.ArchivePrint.RequestID) != requestID {
			return
		}
		snapshot.ArchivePrint.Status = status
		snapshot.ArchivePrint.Error = errText
		snapshot.ArchivePrint.UpdatedAt = at
	})
}

func (rs *runtimeState) processPendingArchivePrintRequest(now time.Time) {
	if rs == nil || rs.archivePrintRequest == nil {
		return
	}

	req, ok := rs.archivePrintRequest.Pending(now)
	if !ok {
		return
	}

	lg := workerLog("worker.archive_print")
	requestID := strings.TrimSpace(req.RequestID)
	itemLabel := strings.TrimSpace(req.ItemName)
	if itemLabel == "" {
		itemLabel = strings.TrimSpace(req.ItemCode)
	}
	qtyText := batch.FormatArchiveBatchQty(req.TotalQty)
	printer := resolvePrintBackend(req.Printer, rs.printBackend)
	if !rs.printBackendEnabled(printer) {
		errText := printer + " disabled"
		lg.Printf("request blocked: %s request_id=%s item=%s qty=%s", errText, requestID, itemLabel, qtyText)
		if err := writeArchivePrintRequestStatus(rs.bridgeStore, requestID, "error", errText); err != nil {
			rs.info = "archive print status xato: " + err.Error()
			return
		}
		rs.info = "archive print xato: " + errText
		return
	}
	if strings.EqualFold(strings.TrimSpace(rs.activeArchivePrintRequestID), requestID) {
		return
	}

	if err := writeArchivePrintRequestStatus(rs.bridgeStore, requestID, "processing", ""); err != nil {
		rs.info = "archive print status xato: " + err.Error()
		return
	}

	rs.activeArchivePrintRequestID = requestID
	defer func() {
		rs.activeArchivePrintRequestID = ""
	}()

	rs.info = "archive QR print queued: request_id=" + requestID
	lg.Printf("request queued: request_id=%s item=%s qty=%s printer=%s", requestID, itemLabel, qtyText, printer)

	startedAt := time.Now()
	var err error
	if printer == printBackendGoDEX {
		err = runBatchArchiveBatchLabel(req, 5*time.Second)
	} else {
		err = runZebraArchiveBatchLabel(rs.zebraPreferred, req, 1400*time.Millisecond)
	}
	if elapsed := time.Since(startedAt); elapsed > 5*time.Second && err == nil {
		lg.Printf("request slow: request_id=%s elapsed=%s", requestID, elapsed.Round(time.Millisecond))
	}
	status := "done"
	errText := ""
	if err != nil {
		status = "error"
		errText = err.Error()
	}
	if err := writeArchivePrintRequestStatus(rs.bridgeStore, requestID, status, errText); err != nil {
		rs.info = "archive print status xato: " + err.Error()
		return
	}
	if status == "done" {
		rs.info = "archive QR print done: request_id=" + requestID
		lg.Printf("request done: request_id=%s printer=%s", requestID, printer)
	} else {
		rs.info = "archive print xato: " + errText
		lg.Printf("request error: request_id=%s printer=%s err=%v", requestID, printer, err)
	}
}

func runBatchArchiveBatchLabel(req bridgestate.ArchivePrintRequestSnapshot, timeout time.Duration) error {
	lg := workerLog("worker.batch_action")
	lg.Printf("archive label start: session=%s item=%s qty=%s time=%s timeout=%s", req.SessionID, strings.TrimSpace(req.ItemName), batch.FormatArchiveBatchQty(req.TotalQty), batch.FormatArchiveBatchTime(req.BatchTime), timeout)

	godexIOMutex.Lock()
	defer godexIOMutex.Unlock()

	printer, err := batch.OpenG500()
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := printer.Close(); closeErr != nil {
			lg.Printf("archive label close warning: %v", closeErr)
		}
	}()

	if status, err := printer.Status(); err != nil {
		return err
	} else if strings.TrimSpace(status) != "" && !strings.HasPrefix(strings.TrimSpace(status), "00,") {
		if _, recoverErr := printer.Recover(); recoverErr != nil {
			return recoverErr
		}
	}

	_, err = printer.PrintArchiveBatch(batch.ArchiveBatchLabel{
		SessionID: req.SessionID,
		ItemName:  req.ItemName,
		QtyText:   batch.FormatArchiveBatchQty(req.TotalQty),
		BatchTime: batch.FormatArchiveBatchTime(req.BatchTime),
	}, batch.DefaultArchiveLabelOptions())
	if err != nil {
		return err
	}

	lg.Printf("archive label done: session=%s item=%s qty=%s", req.SessionID, strings.TrimSpace(req.ItemName), batch.FormatArchiveBatchQty(req.TotalQty))
	return nil
}

func runZebraArchiveBatchLabel(preferredDevice string, req bridgestate.ArchivePrintRequestSnapshot, timeout time.Duration) error {
	lg := workerLog("worker.zebra_action")
	lg.Printf("archive label start: preferred_device=%s session=%s item=%s qty=%s time=%s timeout=%s", preferredDevice, req.SessionID, strings.TrimSpace(req.ItemName), batch.FormatArchiveBatchQty(req.TotalQty), batch.FormatArchiveBatchTime(req.BatchTime), timeout)

	zebraIOMutex.Lock()
	defer zebraIOMutex.Unlock()

	p, err := SelectZebraPrinter(preferredDevice)
	if err != nil {
		return err
	}

	stream, err := buildArchiveBatchZPL(req)
	if err != nil {
		return err
	}
	if err := sendRawRetry(p.DevicePath, []byte(stream), 8, 120*time.Millisecond); err != nil {
		return err
	}
	waitReady(p.DevicePath, 1600*time.Millisecond)
	lg.Printf("archive label done: device=%s session=%s item=%s qty=%s", p.DevicePath, req.SessionID, strings.TrimSpace(req.ItemName), batch.FormatArchiveBatchQty(req.TotalQty))
	return nil
}

func buildArchiveBatchZPL(req bridgestate.ArchivePrintRequestSnapshot) (string, error) {
	item := sanitizeZPLText(strings.TrimSpace(req.ItemName))
	if item == "" {
		item = sanitizeZPLText(strings.TrimSpace(req.ItemCode))
	}
	if item == "" {
		item = "-"
	}
	qtyText := sanitizeZPLText(batch.FormatArchiveBatchQty(req.TotalQty))
	if qtyText == "" {
		qtyText = "0"
	}
	batchTime := sanitizeZPLText(batch.FormatArchiveBatchTime(req.BatchTime))
	if batchTime == "" {
		batchTime = "-"
	}
	qrPayload := sanitizeZPLText(batch.EncodeArchiveBatchPayload(req.SessionID, item, qtyText, batchTime))
	if qrPayload == "" {
		return "", fmt.Errorf("archive qr payload bo'sh")
	}

	return "^XA\n" +
		"^LH0,0\n" +
		"^CI28\n" +
		fmt.Sprintf("^FO20,20^A0N,30,26^FB310,2,0,L,0\n^FD%s^FS\n", item) +
		fmt.Sprintf("^FO20,112^A0N,28,24^FB310,1,0,L,0\n^FDBRUTTO: %s KG^FS\n", qtyText) +
		fmt.Sprintf("^FO20,152^A0N,28,24^FB310,1,0,L,0\n^FDNETTO: %s KG^FS\n", qtyText) +
		fmt.Sprintf("^FO20,196^A0N,24,20^FB310,2,0,L,0\n^FDDATE: %s^FS\n", batchTime) +
		fmt.Sprintf("^FO364,20^BQN,2,4\n^FDLA,%s^FS\n", qrPayload) +
		"^PQ1\n" +
		"^XZ\n", nil
}

func normalizeArchivePrintUnit(unit string) string {
	unit = strings.TrimSpace(unit)
	if unit == "" {
		return "kg"
	}
	return strings.ToLower(unit)
}
