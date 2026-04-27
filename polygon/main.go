package main

import (
	bridgestate "bridge/state"
	"context"
	"encoding/json"
	"flag"
	"log"
	"math/rand"
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
	disableScale    bool
	disablePrinter  bool
	auto            bool
	printMode       string
	scenario        string
	seed            int64
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

type scenarioPayload struct {
	Scenario string `json:"scenario"`
	Seed     *int64 `json:"seed"`
	Auto     *bool  `json:"auto"`
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
	scaleSim   bool
	printerSim bool
	auto       bool
	printMode  string
	scenario   string
	seed       int64
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
	flag.BoolVar(&cfg.disableScale, "no-scale-sim", false, "disable fake scale snapshots and /api/v1/scale")
	flag.BoolVar(&cfg.disablePrinter, "no-printer-sim", false, "disable fake Zebra snapshots and print_request completion")
	flag.BoolVar(&cfg.auto, "auto", true, "enable automatic fake scale cycles")
	flag.StringVar(&cfg.printMode, "print-mode", "success", "fake print mode: success|fail|alternate")
	flag.StringVar(&cfg.scenario, "scenario", "batch-flow", "fake scale scenario: batch-flow|idle|stress|calibration")
	flag.Int64Var(&cfg.seed, "seed", 42, "seed for deterministic scenario generation")
	flag.Parse()

	cfg.printMode = normalizePrintMode(cfg.printMode)
	cfg.scenario = normalizeScenario(cfg.scenario)
	if cfg.tick <= 0 {
		cfg.tick = 250 * time.Millisecond
	}
	if cfg.printDelay <= 0 {
		cfg.printDelay = 1100 * time.Millisecond
	}
	return cfg
}

func newSimulator(cfg config) *simulator {
	cycle := buildScenarioCycle(cfg.scenario, cfg.seed)
	if len(cycle) == 0 {
		cycle = buildScenarioCycle("batch-flow", 42)
	}
	return &simulator{
		store:      bridgestate.New(strings.TrimSpace(cfg.bridgeStateFile)),
		scaleSim:   !cfg.disableScale,
		printerSim: !cfg.disablePrinter,
		auto:       cfg.auto,
		printMode:  normalizePrintMode(cfg.printMode),
		scenario:   normalizeScenario(cfg.scenario),
		seed:       cfg.seed,
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
		cycle: cycle,
	}
}

func normalizeScenario(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "idle", "quiet":
		return "idle"
	case "stress", "noisy", "chaos":
		return "stress"
	case "calibration", "calibrate":
		return "calibration"
	case "demo", "batch", "batch-flow", "":
		return "batch-flow"
	default:
		return "batch-flow"
	}
}

func buildScenarioCycle(name string, seed int64) []cycleFrame {
	rng := rand.New(rand.NewSource(seed))
	switch normalizeScenario(name) {
	case "idle":
		return buildIdleCycle(rng)
	case "stress":
		return buildStressCycle(rng)
	case "calibration":
		return buildCalibrationCycle(rng)
	default:
		return buildBatchFlowCycle(rng)
	}
}

func buildIdleCycle(rng *rand.Rand) []cycleFrame {
	frames := make([]cycleFrame, 0, 24)
	for i := 0; i < 24; i++ {
		frames = append(frames, cycleFrame{
			weight:   roundWeight(jitterWeight(rng, 0, 0.004)),
			stable:   true,
			duration: randDuration(rng, 650*time.Millisecond, 1100*time.Millisecond),
		})
	}
	return frames
}

func buildCalibrationCycle(rng *rand.Rand) []cycleFrame {
	weights := []float64{0.500, 1.000, 2.000}
	frames := make([]cycleFrame, 0, 48)
	for _, target := range weights {
		frames = append(frames,
			cycleFrame{weight: 0, stable: true, duration: randDuration(rng, 700*time.Millisecond, 1200*time.Millisecond)},
		)
		frames = append(frames, appendRamp(rng, 0, target, 4, 170*time.Millisecond, 260*time.Millisecond, 0.004)...)
		frames = append(frames,
			cycleFrame{weight: roundWeight(jitterWeight(rng, target, 0.002)), stable: false, duration: randDuration(rng, 180*time.Millisecond, 240*time.Millisecond)},
			cycleFrame{weight: roundWeight(jitterWeight(rng, target, 0.001)), stable: true, duration: randDuration(rng, 1500*time.Millisecond, 2500*time.Millisecond)},
			cycleFrame{weight: roundWeight(jitterWeight(rng, target, 0.001)), stable: true, duration: randDuration(rng, 800*time.Millisecond, 1300*time.Millisecond)},
		)
		frames = append(frames, appendRamp(rng, target, 0, 4, 160*time.Millisecond, 260*time.Millisecond, 0.003)...)
		frames = append(frames,
			cycleFrame{weight: 0, stable: true, duration: randDuration(rng, 900*time.Millisecond, 1400*time.Millisecond)},
		)
	}
	return frames
}

func buildStressCycle(rng *rand.Rand) []cycleFrame {
	frames := make([]cycleFrame, 0, 96)
	for i := 0; i < 8; i++ {
		target := roundWeight(0.180 + rng.Float64()*2.800)
		frames = append(frames,
			cycleFrame{weight: roundWeight(jitterWeight(rng, 0, 0.010)), stable: true, duration: randDuration(rng, 150*time.Millisecond, 420*time.Millisecond)},
		)
		frames = append(frames, appendRamp(rng, 0, target, 6, 90*time.Millisecond, 180*time.Millisecond, 0.020)...)
		frames = append(frames,
			cycleFrame{weight: roundWeight(jitterWeight(rng, target, 0.035)), stable: false, duration: randDuration(rng, 70*time.Millisecond, 150*time.Millisecond)},
			cycleFrame{weight: roundWeight(jitterWeight(rng, target, 0.020)), stable: false, duration: randDuration(rng, 90*time.Millisecond, 180*time.Millisecond)},
			cycleFrame{weight: roundWeight(jitterWeight(rng, target, 0.008)), stable: true, duration: randDuration(rng, 350*time.Millisecond, 900*time.Millisecond)},
		)
		frames = append(frames, appendRamp(rng, target, 0, 5, 80*time.Millisecond, 160*time.Millisecond, 0.018)...)
		frames = append(frames,
			cycleFrame{weight: 0, stable: true, duration: randDuration(rng, 180*time.Millisecond, 700*time.Millisecond)},
			cycleFrame{weight: roundWeight(jitterWeight(rng, 0, 0.006)), stable: true, duration: randDuration(rng, 180*time.Millisecond, 500*time.Millisecond)},
		)
	}
	return frames
}

func buildBatchFlowCycle(rng *rand.Rand) []cycleFrame {
	frames := make([]cycleFrame, 0, 128)
	rounds := 6 + rng.Intn(3)
	for i := 0; i < rounds; i++ {
		target := roundWeight(0.360 + rng.Float64()*1.650 + float64(i%2)*0.075)
		frames = append(frames,
			cycleFrame{weight: 0, stable: true, duration: randDuration(rng, 700*time.Millisecond, 1100*time.Millisecond)},
		)
		frames = append(frames, appendRamp(rng, 0, target, 5, 180*time.Millisecond, 320*time.Millisecond, 0.008)...)
		frames = append(frames,
			cycleFrame{weight: roundWeight(jitterWeight(rng, target, 0.005)), stable: false, duration: randDuration(rng, 220*time.Millisecond, 420*time.Millisecond)},
			cycleFrame{weight: roundWeight(jitterWeight(rng, target, 0.0015)), stable: true, duration: randDuration(rng, 4600*time.Millisecond, 5600*time.Millisecond)},
		)
		frames = append(frames, appendRamp(rng, target, 0, 4, 180*time.Millisecond, 320*time.Millisecond, 0.006)...)
		frames = append(frames,
			cycleFrame{weight: 0, stable: true, duration: randDuration(rng, 1200*time.Millisecond, 1800*time.Millisecond)},
		)
	}
	return frames
}

func appendRamp(rng *rand.Rand, from, to float64, steps int, minDur, maxDur time.Duration, jitter float64) []cycleFrame {
	if steps <= 0 {
		return nil
	}
	frames := make([]cycleFrame, 0, steps)
	for i := 1; i <= steps; i++ {
		p := float64(i) / float64(steps)
		weight := lerp(from, to, easeOutCubic(p))
		if jitter > 0 {
			weight = jitterWeight(rng, weight, jitter)
		}
		frames = append(frames, cycleFrame{
			weight:   roundWeight(weight),
			stable:   false,
			duration: randDuration(rng, minDur, maxDur),
		})
	}
	return frames
}

func randDuration(rng *rand.Rand, min, max time.Duration) time.Duration {
	if max < min {
		min, max = max, min
	}
	if min <= 0 {
		min = 1
	}
	if max <= min {
		return min
	}
	return min + time.Duration(rng.Int63n(int64(max-min)+1))
}

func jitterWeight(rng *rand.Rand, base, amplitude float64) float64 {
	if amplitude <= 0 {
		return base
	}
	return base + ((rng.Float64()*2)-1)*amplitude
}

func easeOutCubic(p float64) float64 {
	if p <= 0 {
		return 0
	}
	if p >= 1 {
		return 1
	}
	oneMinus := 1 - p
	return 1 - oneMinus*oneMinus*oneMinus
}

func lerp(from, to, p float64) float64 {
	return from + (to-from)*p
}

func roundWeight(v float64) float64 {
	if v < 0 {
		v = 0
	}
	rounded, err := strconv.ParseFloat(strconv.FormatFloat(v, 'f', 3, 64), 64)
	if err != nil {
		return v
	}
	return rounded
}

func (s *simulator) bootstrap(now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.scaleSim {
		s.applyCycleFrameLocked(now)
	}
	return s.writeSimulatedStateLocked(now)
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

	if s.scaleSim && s.auto && (s.nextCycleAt.IsZero() || !now.Before(s.nextCycleAt)) {
		s.advanceCycleLocked(now)
	}
	if s.printerSim {
		if err := s.processPrintRequestLocked(now); err != nil {
			return err
		}
	}
	return s.writeSimulatedStateLocked(now)
}

func (s *simulator) writeSimulatedStateLocked(now time.Time) error {
	if !s.scaleSim && !s.printerSim {
		return nil
	}

	var scale bridgestate.ScaleSnapshot
	if s.scaleSim {
		scale = s.scaleScale
		scale.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
	}
	var zebra bridgestate.ZebraSnapshot
	if s.printerSim {
		zebra = s.zebra
		zebra.UpdatedAt = now.UTC().Format(time.RFC3339Nano)
	}

	return s.store.Update(func(snapshot *bridgestate.Snapshot) {
		if s.scaleSim {
			snapshot.Scale = scale
		}
		if s.printerSim {
			snapshot.Zebra = zebra
		}
	})
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
	if frame.stable && weight < 0.05 {
		weight = 0
	}
	stable := frame.stable
	s.scaleScale.Weight = &weight
	s.scaleScale.Stable = &stable
	s.scaleScale.Unit = "kg"
	s.scaleScale.Source = "polygon"
	s.scaleScale.Port = "polygon://scale"
	s.scaleRaw = formatScaleRaw(frame.weight, frame.stable)
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

	if s.activePrintEPC == "" && reqEPC != "" && (reqStatus == "" || reqStatus == "pending" || reqStatus == "processing") {
		s.activePrintEPC = reqEPC
		if reqStatus == "processing" {
			s.printFinishAt = now.Add(s.printDelay / 2)
		} else {
			s.printFinishAt = now.Add(s.printDelay)
		}
		s.zebra.Action = "print"
		s.zebra.DeviceState = "busy"
		s.zebra.MediaState = "printing"
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
	s.zebra.Action = "print"
	s.zebra.LastEPC = epc
	if success {
		s.zebra.DeviceState = "ready"
		s.zebra.MediaState = "ok"
		s.zebra.Verify = "WRITTEN"
		s.zebra.Error = ""
		s.finishPrinterCommandLocked(epc, "done", "", now)
	} else {
		s.zebra.DeviceState = "attention"
		s.zebra.MediaState = "paused"
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

func (s *simulator) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/v1/scale", s.handleScale)
	mux.HandleFunc("/api/v1/state", s.handleState)
	mux.HandleFunc("/api/v1/dev/printer", s.handlePrinter)
	mux.HandleFunc("/api/v1/dev/auto", s.handleAuto)
	mux.HandleFunc("/api/v1/dev/scenario", s.handleScenario)
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
		"ok":               true,
		"name":             "polygon",
		"simulate_scale":   s.scaleSim,
		"simulate_printer": s.printerSim,
	})
}

func (s *simulator) handleScale(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}

	s.mu.Lock()
	if !s.scaleSim {
		s.mu.Unlock()
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"ok":    false,
			"error": "scale simulation is disabled",
		})
		return
	}
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
	if !s.printerSim {
		mode := s.printMode
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":               true,
			"simulate_printer": false,
			"print_mode":       mode,
			"active_epc":       "",
			"history":          []printerCommand{},
			"message":          "printer simulation is disabled",
		})
		return
	}
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
	if !s.scaleSim {
		s.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": "scale simulation is disabled"})
		return
	}
	s.auto = *payload.Enabled
	if s.auto {
		s.applyCycleFrameLocked(time.Now())
	}
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "auto": *payload.Enabled})
}

func (s *simulator) handleScenario(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.mu.Lock()
		payload := map[string]any{
			"ok":             true,
			"simulate_scale": s.scaleSim,
			"scenario":       s.scenario,
			"seed":           s.seed,
			"auto":           s.auto,
			"frames":         len(s.cycle),
		}
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, payload)
	case http.MethodPost:
		var payload scenarioPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid json"})
			return
		}

		scenario := normalizeScenario(payload.Scenario)
		seed := int64(42)
		if payload.Seed != nil {
			seed = *payload.Seed
		}
		applyAuto := s.auto
		if payload.Auto != nil {
			applyAuto = *payload.Auto
		}

		now := time.Now()
		s.mu.Lock()
		if !s.scaleSim {
			s.mu.Unlock()
			writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": "scale simulation is disabled"})
			return
		}
		s.scenario = scenario
		s.seed = seed
		s.auto = applyAuto
		s.cycle = buildScenarioCycle(scenario, seed)
		s.cycleIndex = 0
		s.nextCycleAt = time.Time{}
		s.applyCycleFrameLocked(now)
		frames := len(s.cycle)
		err := s.writeSimulatedStateLocked(now)
		s.mu.Unlock()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"ok":       true,
			"scenario": scenario,
			"seed":     seed,
			"auto":     applyAuto,
			"frames":   frames,
		})
	default:
		writeMethodNotAllowed(w)
	}
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
	if !s.scaleSim {
		s.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": "scale simulation is disabled"})
		return
	}
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
	s.scaleRaw = formatScaleRaw(weight, stable)
	s.scaleScale.Error = ""
	err := s.writeSimulatedStateLocked(now)
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
	if !s.scaleSim {
		s.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": "scale simulation is disabled"})
		return
	}
	s.auto = false
	zero := 0.0
	stable := true
	s.scaleScale.Weight = &zero
	s.scaleScale.Stable = &stable
	s.scaleScale.Unit = "kg"
	s.scaleRaw = formatScaleRaw(zero, stable)
	s.scaleScale.Error = ""
	err := s.writeSimulatedStateLocked(now)
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
	if !s.printerSim {
		s.mu.Unlock()
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": "printer simulation is disabled"})
		return
	}
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
	log.Printf("polygon: fake zebra accepted encode request epc=%s qty=%s item=%s", cmd.EPC, cmd.QtyText, cmd.ItemName)
	log.Printf("polygon: fake zebra will write RFID EPC and print label for item=%s qty=%s", cmd.ItemName, cmd.QtyText)
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
		if strings.EqualFold(strings.TrimSpace(status), "done") {
			log.Printf("polygon: fake zebra finished encode+print epc=%s status=%s", epc, status)
		} else {
			log.Printf("polygon: fake zebra finished encode+print epc=%s status=%s err=%s", epc, status, strings.TrimSpace(errText))
		}
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
