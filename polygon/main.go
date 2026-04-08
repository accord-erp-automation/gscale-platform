package main

import (
	bridgestate "bridge/state"
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultBridgeStateFile = "/tmp/gscale-zebra/bridge_state.json"
	defaultHTTPAddr        = "127.0.0.1:18000"
)

type config struct {
	bridgeStateFile string
	httpAddr        string
	tick            time.Duration
	printDelay      time.Duration
	auto            bool
	printMode       string
}

type scaleAPIResponse struct {
	OK     bool     `json:"ok"`
	Weight *float64 `json:"weight"`
	Unit   string   `json:"unit"`
	Stable *bool    `json:"stable"`
	Port   string   `json:"port"`
	Raw    string   `json:"raw"`
	Error  string   `json:"error"`
}

type controlPayload struct {
	Enabled *bool    `json:"enabled"`
	Weight  *float64 `json:"weight"`
	Stable  *bool    `json:"stable"`
	Unit    string   `json:"unit"`
	Mode    string   `json:"mode"`
}

type cycleFrame struct {
	weight   float64
	stable   bool
	duration time.Duration
}

type printerCommand struct {
	ID        int64  `json:"id"`
	EPC       string `json:"epc"`
	QtyText   string `json:"qty_text"`
	ItemName  string `json:"item_name"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	Preview   string `json:"preview"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type simulator struct {
	mu sync.Mutex

	store      *bridgestate.Store
	auto       bool
	printMode  string
	printDelay time.Duration

	scaleScale  bridgestate.ScaleSnapshot
	scaleRaw    string
	zebra       bridgestate.ZebraSnapshot
	cycle       []cycleFrame
	cycleIndex  int
	nextCycleAt time.Time

	activePrintEPC string
	printFinishAt  time.Time
	alternateFail  bool
	printerSeq     int64
	printerHistory []printerCommand
}

func main() {
	cfg := parseFlags()

	logger := log.New(os.Stdout, "polygon: ", log.LstdFlags)
	sim := newSimulator(cfg)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := sim.bootstrap(time.Now()); err != nil {
		logger.Fatalf("bootstrap error: %v", err)
	}

	server := &http.Server{
		Addr:    cfg.httpAddr,
		Handler: sim.routes(),
	}

	go func() {
		logger.Printf("http listening on %s", cfg.httpAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Printf("http error: %v", err)
			stop()
		}
	}()

	go sim.run(ctx, cfg.tick)

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
	logger.Printf("stopped")
}

func parseFlags() config {
	cfg := config{}
	flag.StringVar(&cfg.bridgeStateFile, "bridge-state-file", defaultBridgeStateFile, "shared bridge JSON file")
	flag.StringVar(&cfg.httpAddr, "http-addr", defaultHTTPAddr, "polygon HTTP listen address")
	flag.DurationVar(&cfg.tick, "tick", 250*time.Millisecond, "polygon tick interval")
	flag.DurationVar(&cfg.printDelay, "print-delay", 1100*time.Millisecond, "fake print completion delay")
	flag.BoolVar(&cfg.auto, "auto", true, "enable automatic fake scale cycles")
	flag.StringVar(&cfg.printMode, "print-mode", "success", "fake print mode: success|fail|alternate")
	flag.Parse()

	cfg.printMode = normalizePrintMode(cfg.printMode)
	if cfg.tick <= 0 {
		cfg.tick = 250 * time.Millisecond
	}
	if cfg.printDelay <= 0 {
		cfg.printDelay = 1100 * time.Millisecond
	}
	return cfg
}

func newSimulator(cfg config) *simulator {
	return &simulator{
		store:      bridgestate.New(strings.TrimSpace(cfg.bridgeStateFile)),
		auto:       cfg.auto,
		printMode:  normalizePrintMode(cfg.printMode),
		printDelay: cfg.printDelay,
		scaleScale: bridgestate.ScaleSnapshot{
			Source: "polygon",
			Port:   "polygon://scale",
			Unit:   "kg",
		},
		zebra: bridgestate.ZebraSnapshot{
			Connected:   true,
			DevicePath:  "polygon://zebra",
			Name:        "Polygon Zebra",
			DeviceState: "ready",
			MediaState:  "ok",
			Verify:      "-",
		},
		cycle: []cycleFrame{
			{weight: 0, stable: false, duration: 1200 * time.Millisecond},
			{weight: 0.318, stable: false, duration: 700 * time.Millisecond},
			{weight: 0.742, stable: false, duration: 900 * time.Millisecond},
			{weight: 0.742, stable: true, duration: 2800 * time.Millisecond},
			{weight: 0, stable: false, duration: 1400 * time.Millisecond},
			{weight: 1.125, stable: false, duration: 900 * time.Millisecond},
			{weight: 1.126, stable: true, duration: 3000 * time.Millisecond},
			{weight: 0, stable: false, duration: 1500 * time.Millisecond},
		},
	}
}

func (s *simulator) bootstrap(now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.applyCycleFrameLocked(now)
	return s.writeScaleAndZebraLocked(now)
}

func (s *simulator) run(ctx context.Context, tick time.Duration) {
	ticker := time.NewTicker(tick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := s.tick(now); err != nil {
				log.Printf("polygon: tick error: %v", err)
			}
		}
	}
}

func (s *simulator) tick(now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.auto && (s.nextCycleAt.IsZero() || !now.Before(s.nextCycleAt)) {
		s.advanceCycleLocked(now)
	}
	if err := s.processPrintRequestLocked(now); err != nil {
		return err
	}
	return s.writeScaleAndZebraLocked(now)
}

func (s *simulator) advanceCycleLocked(now time.Time) {
	if len(s.cycle) == 0 {
		return
	}
	s.cycleIndex++
	if s.cycleIndex >= len(s.cycle) {
		s.cycleIndex = 0
	}
	s.applyCycleFrameLocked(now)
}

func (s *simulator) applyCycleFrameLocked(now time.Time) {
	if len(s.cycle) == 0 {
		return
	}
	frame := s.cycle[s.cycleIndex]
	weight := frame.weight
	stable := frame.stable
	s.scaleScale.Weight = &weight
	s.scaleScale.Stable = &stable
	s.scaleScale.Unit = "kg"
	s.scaleScale.Source = "polygon"
	s.scaleScale.Port = "polygon://scale"
	if frame.weight == 0 {
		s.scaleRaw = "0.000 kg"
	} else {
		s.scaleRaw = formatScaleRaw(frame.weight, frame.stable)
	}
	s.scaleScale.Error = ""
	s.nextCycleAt = now.Add(frame.duration)
}

func (s *simulator) processPrintRequestLocked(now time.Time) error {
	snap, err := s.store.Read()
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	req := snap.PrintRequest
	reqEPC := strings.ToUpper(strings.TrimSpace(req.EPC))
	reqStatus := strings.ToLower(strings.TrimSpace(req.Status))

	if s.activePrintEPC != "" && reqEPC != s.activePrintEPC {
		s.activePrintEPC = ""
		s.printFinishAt = time.Time{}
	}

	if s.activePrintEPC == "" && reqEPC != "" && (reqStatus == "" || reqStatus == "pending") {
		s.activePrintEPC = reqEPC
		s.printFinishAt = now.Add(s.printDelay)
		s.zebra.Action = "encode"
		s.zebra.Error = ""
		s.zebra.Verify = "PROCESSING"
		s.startPrinterCommandLocked(req, now)
		return s.store.Update(func(snapshot *bridgestate.Snapshot) {
			if strings.ToUpper(strings.TrimSpace(snapshot.PrintRequest.EPC)) != reqEPC {
				return
			}
			snapshot.PrintRequest.Status = "processing"
			snapshot.PrintRequest.Error = ""
			snapshot.PrintRequest.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
		})
	}

	if s.activePrintEPC == "" || now.Before(s.printFinishAt) {
		return nil
	}

	epc := s.activePrintEPC
	success := s.resolvePrintSuccessLocked()
	s.activePrintEPC = ""
	s.printFinishAt = time.Time{}
	s.zebra.Action = "encode"
	s.zebra.LastEPC = epc
	if success {
		s.zebra.Verify = "WRITTEN"
		s.zebra.Error = ""
		s.finishPrinterCommandLocked(epc, "done", "", now)
	} else {
		s.zebra.Verify = "ERROR"
		s.zebra.Error = "polygon forced print failure"
		s.finishPrinterCommandLocked(epc, "error", "polygon forced print failure", now)
	}

	return s.store.Update(func(snapshot *bridgestate.Snapshot) {
		if strings.ToUpper(strings.TrimSpace(snapshot.PrintRequest.EPC)) != epc {
			return
		}
		if success {
			snapshot.PrintRequest.Status = "done"
			snapshot.PrintRequest.Error = ""
		} else {
			snapshot.PrintRequest.Status = "error"
			snapshot.PrintRequest.Error = "polygon forced print failure"
		}
		snapshot.PrintRequest.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
	})
}

func (s *simulator) resolvePrintSuccessLocked() bool {
	switch s.printMode {
	case "fail":
		return false
	case "alternate":
		s.alternateFail = !s.alternateFail
		return !s.alternateFail
	default:
		return true
	}
}

func (s *simulator) writeScaleAndZebraLocked(now time.Time) error {
	scale := s.scaleScale
	scale.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
	zebra := s.zebra
	zebra.UpdatedAt = now.UTC().Format(time.RFC3339Nano)

	return s.store.Update(func(snapshot *bridgestate.Snapshot) {
		snapshot.Scale = scale
		snapshot.Zebra = zebra
	})
}

func (s *simulator) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/v1/scale", s.handleScale)
	mux.HandleFunc("/api/v1/state", s.handleState)
	mux.HandleFunc("/api/v1/dev/printer", s.handlePrinter)
	mux.HandleFunc("/api/v1/dev/auto", s.handleAuto)
	mux.HandleFunc("/api/v1/dev/weight", s.handleWeight)
	mux.HandleFunc("/api/v1/dev/reset", s.handleReset)
	mux.HandleFunc("/api/v1/dev/print-mode", s.handlePrintMode)
	return mux
}

func (s *simulator) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"name": "polygon",
	})
}

func (s *simulator) handleScale(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}

	s.mu.Lock()
	scale := s.scaleScale
	raw := s.scaleRaw
	s.mu.Unlock()

	resp := scaleAPIResponse{
		OK:     strings.TrimSpace(scale.Error) == "",
		Weight: scale.Weight,
		Unit:   fallback(scale.Unit, "kg"),
		Stable: scale.Stable,
		Port:   scale.Port,
		Raw:    raw,
		Error:  scale.Error,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *simulator) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}

	snap, err := s.store.Read()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"state": snap,
	})
}

func (s *simulator) handlePrinter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}

	s.mu.Lock()
	history := append([]printerCommand(nil), s.printerHistory...)
	mode := s.printMode
	active := s.activePrintEPC
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"print_mode": mode,
		"active_epc": active,
		"history":    history,
	})
}

func (s *simulator) handleAuto(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	var payload controlPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid json"})
		return
	}
	if payload.Enabled == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "enabled is required"})
		return
	}

	s.mu.Lock()
	s.auto = *payload.Enabled
	if s.auto {
		s.applyCycleFrameLocked(time.Now())
	}
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "auto": *payload.Enabled})
}

func (s *simulator) handleWeight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	var payload controlPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid json"})
		return
	}
	if payload.Weight == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "weight is required"})
		return
	}

	now := time.Now()

	s.mu.Lock()
	s.auto = false
	weight := *payload.Weight
	unit := fallback(payload.Unit, "kg")
	stable := false
	if payload.Stable != nil {
		stable = *payload.Stable
	}
	s.scaleScale.Weight = &weight
	s.scaleScale.Unit = unit
	s.scaleScale.Stable = &stable
	s.scaleScale.Source = "polygon"
	s.scaleScale.Port = "polygon://scale"
	if weight == 0 {
		s.scaleRaw = "0.000 " + unit
	} else {
		s.scaleRaw = formatScaleRaw(weight, stable)
	}
	s.scaleScale.Error = ""
	err := s.writeScaleAndZebraLocked(now)
	s.mu.Unlock()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"auto":   false,
		"weight": weight,
		"stable": stable,
		"unit":   unit,
	})
}

func (s *simulator) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	now := time.Now()
	s.mu.Lock()
	s.auto = false
	zero := 0.0
	stable := false
	s.scaleScale.Weight = &zero
	s.scaleScale.Stable = &stable
	s.scaleScale.Unit = "kg"
	s.scaleRaw = "0.000 kg"
	s.scaleScale.Error = ""
	err := s.writeScaleAndZebraLocked(now)
	s.mu.Unlock()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "auto": false, "weight": 0})
}

func (s *simulator) handlePrintMode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	var payload controlPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid json"})
		return
	}
	mode := normalizePrintMode(payload.Mode)
	s.mu.Lock()
	s.printMode = mode
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "print_mode": mode})
}

func normalizePrintMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "fail":
		return "fail"
	case "alternate":
		return "alternate"
	default:
		return "success"
	}
}

func (s *simulator) startPrinterCommandLocked(req bridgestate.PrintRequestSnapshot, now time.Time) {
	s.printerSeq++
	cmd := printerCommand{
		ID:        s.printerSeq,
		EPC:       strings.ToUpper(strings.TrimSpace(req.EPC)),
		QtyText:   formatQtyText(req.Qty, req.Unit),
		ItemName:  fallback(req.ItemName, req.ItemCode),
		Status:    "processing",
		Preview:   buildPrinterPreview(req),
		CreatedAt: now.UTC().Format(time.RFC3339Nano),
		UpdatedAt: now.UTC().Format(time.RFC3339Nano),
	}
	s.printerHistory = append([]printerCommand{cmd}, s.printerHistory...)
	if len(s.printerHistory) > 20 {
		s.printerHistory = s.printerHistory[:20]
	}
	log.Printf("polygon: printer command accepted epc=%s qty=%s item=%s", cmd.EPC, cmd.QtyText, cmd.ItemName)
	log.Printf("polygon: printer preview:\n%s", cmd.Preview)
}

func (s *simulator) finishPrinterCommandLocked(epc, status, errText string, now time.Time) {
	epc = strings.ToUpper(strings.TrimSpace(epc))
	for i := range s.printerHistory {
		if strings.ToUpper(strings.TrimSpace(s.printerHistory[i].EPC)) != epc {
			continue
		}
		s.printerHistory[i].Status = strings.ToLower(strings.TrimSpace(status))
		s.printerHistory[i].Error = strings.TrimSpace(errText)
		s.printerHistory[i].UpdatedAt = now.UTC().Format(time.RFC3339Nano)
		log.Printf("polygon: printer command finished epc=%s status=%s err=%s", epc, status, strings.TrimSpace(errText))
		return
	}
}

func buildPrinterPreview(req bridgestate.PrintRequestSnapshot) string {
	epc := strings.ToUpper(strings.TrimSpace(req.EPC))
	itemName := sanitizeZPLText(fallback(req.ItemName, req.ItemCode))
	qtyText := sanitizeZPLText(formatQtyText(req.Qty, req.Unit))

	lines := []string{
		"~PS",
		"^XA",
		"^RS8,,,1,N",
		"^RFW,H,,,A^FD" + epc + "^FS",
		"^FO20,24^A0N,22,22^FDMAHSULOT: " + itemName + "^FS",
		"^FO20,52^A0N,22,22^FDVAZNI: " + qtyText + "^FS",
		"^FO20,80^A0N,22,22^FDEPC: " + epc + "^FS",
		"^XZ",
	}
	return strings.Join(lines, "\n")
}

func formatQtyText(qty *float64, unit string) string {
	u := fallback(unit, "kg")
	if qty == nil {
		return "- " + u
	}
	return strconv.FormatFloat(*qty, 'f', 3, 64) + " " + u
}

func sanitizeZPLText(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "-"
	}
	replacer := strings.NewReplacer("^", " ", "~", " ", "\n", " ", "\r", " ")
	return replacer.Replace(v)
}

func formatScaleRaw(weight float64, stable bool) string {
	if stable {
		return strings.TrimSpace(formatFloat(weight) + " kg ST")
	}
	return strings.TrimSpace(formatFloat(weight) + " kg US")
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', 3, 64)
}

func fallback(v, def string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return def
	}
	return v
}

func writeMethodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
		"ok":    false,
		"error": "method not allowed",
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
