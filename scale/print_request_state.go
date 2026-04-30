package main

import (
	bridgestate "bridge/state"
	"strings"
	"time"
)

type printRequestReader struct {
	store      *bridgestate.Store
	cached     bool
	req        bridgestate.PrintRequestSnapshot
	nextReadAt time.Time
}

func newPrintRequestReader(path string) *printRequestReader {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	return &printRequestReader{store: bridgestate.New(path)}
}

func (r *printRequestReader) Pending(now time.Time) (bridgestate.PrintRequestSnapshot, bool) {
	r.refresh(now)
	if r == nil {
		return bridgestate.PrintRequestSnapshot{}, false
	}
	if strings.ToLower(strings.TrimSpace(r.req.Status)) != "pending" {
		return bridgestate.PrintRequestSnapshot{}, false
	}
	if strings.TrimSpace(r.req.EPC) == "" {
		return bridgestate.PrintRequestSnapshot{}, false
	}
	return r.req, true
}

func (r *printRequestReader) refresh(now time.Time) {
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
		r.req = bridgestate.PrintRequestSnapshot{}
		r.cached = true
		r.nextReadAt = now.Add(200 * time.Millisecond)
		return
	}

	req := snap.PrintRequest
	req.EPC = strings.ToUpper(strings.TrimSpace(req.EPC))
	req.Status = strings.ToLower(strings.TrimSpace(req.Status))
	req.ItemCode = strings.TrimSpace(req.ItemCode)
	req.ItemName = strings.TrimSpace(req.ItemName)
	req.Unit = strings.TrimSpace(req.Unit)
	req.Mode = normalizePrintRequestMode(req.Mode)
	req.Printer = normalizePrintRequestPrinter(req.Printer)
	if !req.Tare || req.TareKG <= 0 {
		req.Tare = false
		req.TareKG = 0
	}
	if req.ItemName == "" {
		req.ItemName = req.ItemCode
	}
	if req.Unit == "" && req.Qty != nil {
		req.Unit = "kg"
	}

	r.req = req
	r.cached = true
	r.nextReadAt = now.Add(200 * time.Millisecond)
}
