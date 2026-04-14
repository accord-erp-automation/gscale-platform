package mobileapi

import (
	bridgestate "bridge/state"
	"context"
	"core/batchcontrol"
	"core/erpread"
	"core/workflow"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SessionProfile struct {
	Role        string `json:"role"`
	DisplayName string `json:"display_name"`
	LegalName   string `json:"legal_name"`
	Ref         string `json:"ref"`
	Phone       string `json:"phone"`
	AvatarURL   string `json:"avatar_url"`
}

type authLoginRequest struct {
	Phone string `json:"phone"`
	Code  string `json:"code"`
}

type setupERPRequest struct {
	ERPURL       string `json:"erp_url"`
	ERPReadURL   string `json:"erp_read_url"`
	ERPAPIKey    string `json:"erp_api_key"`
	ERPAPISecret string `json:"erp_api_secret"`
}

type batchStartRequest struct {
	ItemCode  string `json:"item_code"`
	ItemName  string `json:"item_name"`
	Warehouse string `json:"warehouse"`
}

type Server struct {
	cfg              Config
	store            *bridgestate.Store
	http             *http.Client
	control          *batchcontrol.Service
	controlCtx       context.Context
	controlCancel    context.CancelFunc
	validateERPSetup func(context.Context, ERPSetup) error

	mu     sync.Mutex
	tokens map[string]SessionProfile
}

func New(cfg Config) *Server {
	return newServer(cfg, nil)
}

func newServer(cfg Config, control *batchcontrol.Service) *Server {
	controlCtx, controlCancel := context.WithCancel(context.Background())
	server := &Server{
		cfg:           cfg,
		store:         bridgestate.New(cfg.BridgeStateFile),
		http:          &http.Client{Timeout: 1500 * time.Millisecond},
		controlCtx:    controlCtx,
		controlCancel: controlCancel,
		tokens:        make(map[string]SessionProfile),
	}
	if control != nil {
		server.control = control
	} else {
		server.control = server.newControlService()
	}
	return server
}

func (s *Server) Close() {
	if s == nil {
		return
	}
	if s.control != nil {
		s.control.StopAll()
	}
	if s.controlCancel != nil {
		s.controlCancel()
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/v1/mobile/handshake", s.handleHandshake)
	mux.HandleFunc("/v1/mobile/auth/login", s.handleLogin)
	mux.HandleFunc("/v1/mobile/auth/logout", s.handleLogout)
	mux.HandleFunc("/v1/mobile/profile", s.handleProfile)
	mux.HandleFunc("/v1/mobile/setup/status", s.handleSetupStatus)
	mux.HandleFunc("/v1/mobile/setup/erp", s.handleSetupERP)
	mux.HandleFunc("/v1/mobile/monitor/state", s.handleMonitorState)
	mux.HandleFunc("/v1/mobile/monitor/stream", s.handleMonitorStream)
	mux.HandleFunc("/v1/mobile/items", s.handleItems)
	mux.HandleFunc("/v1/mobile/items/", s.handleItemRoutes)
	mux.HandleFunc("/v1/mobile/batch/state", s.handleBatchState)
	mux.HandleFunc("/v1/mobile/batch/start", s.handleBatchStart)
	mux.HandleFunc("/v1/mobile/batch/stop", s.handleBatchStop)
	return mux
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"service": "mobileapi",
	})
}

func (s *Server) handleHandshake(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	profile := s.currentProfile()

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":               true,
		"service":          "mobileapi",
		"app":              "gscale-zebra",
		"server_name":      s.cfg.ServerName,
		"server_ref":       profile.Ref,
		"display_name":     profile.DisplayName,
		"role":             profile.Role,
		"phone":            profile.Phone,
		"http_port":        httpPortFromListenAddr(s.cfg.ListenAddr),
		"discovery_port":   httpPortFromListenAddr(s.cfg.DiscoveryAddr),
		"monitor_path":     "/v1/mobile/monitor/state",
		"profile_path":     "/v1/mobile/profile",
		"items_path":       "/v1/mobile/items",
		"batch_state_path": "/v1/mobile/batch/state",
		"requires_auth":    false,
	})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	var req authLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "invalid_json",
		})
		return
	}

	if strings.TrimSpace(req.Phone) != s.cfg.LoginPhone || strings.TrimSpace(req.Code) != s.cfg.LoginCode {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"error": "invalid_credentials",
		})
		return
	}

	token, err := generateToken()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "token_generation_failed",
		})
		return
	}

	s.mu.Lock()
	s.tokens[token] = s.cfg.Profile
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]any{
		"token":   token,
		"profile": s.currentProfile(),
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	token := bearerToken(r.Header.Get("Authorization"))
	if token != "" {
		s.mu.Lock()
		delete(s.tokens, token)
		s.mu.Unlock()
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		writeMethodNotAllowed(w)
		return
	}

	profile := s.currentProfile()
	authProfile, ok := s.authorize(r)
	if ok {
		profile = authProfile
	}

	if r.Method == http.MethodPut {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err == nil {
			if nickname := strings.TrimSpace(asString(payload["nickname"])); nickname != "" {
				profile.DisplayName = nickname
				profile.LegalName = nickname
				if err := s.saveCurrentProfile(profile); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]any{
						"error": err.Error(),
					})
					return
				}
				s.updateAuthorizedProfile(r, profile)
			}
		}
	}

	writeJSON(w, http.StatusOK, profile)
}

func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}

	writeJSON(w, http.StatusOK, s.setupStatusPayload())
}

func (s *Server) handleSetupERP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		writeMethodNotAllowed(w)
		return
	}
	if s.control != nil && s.control.ActiveBatch().Active {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "batch_active"})
		return
	}
	if r.Method == http.MethodDelete {
		if err := clearERPSetup(s.cfg.SetupFile); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		s.applyERPSetup(ERPSetup{})
		writeJSON(w, http.StatusOK, s.setupStatusPayload())
		return
	}

	var req setupERPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json"})
		return
	}

	setup := ERPSetup{
		ERPURL:       strings.TrimSpace(req.ERPURL),
		ERPReadURL:   strings.TrimSpace(req.ERPReadURL),
		ERPAPIKey:    strings.TrimSpace(req.ERPAPIKey),
		ERPAPISecret: strings.TrimSpace(req.ERPAPISecret),
	}
	if setup.ERPURL == "" || setup.ERPAPIKey == "" || setup.ERPAPISecret == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "erp_url_api_key_api_secret_required"})
		return
	}

	validate := s.validateERPSetup
	if validate == nil {
		validate = validateERPWriteSetup
	}
	if err := validate(r.Context(), setup); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "erp_validation_failed", "message": err.Error()})
		return
	}
	resolvedRead, err := erpread.Resolve(r.Context(), nil, setup.ERPURL, setup.ERPReadURL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "erp_read_discovery_failed", "message": err.Error()})
		return
	}
	setup.ERPReadURL = resolvedRead.BaseURL

	if err := saveERPSetup(s.cfg.SetupFile, setup); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	s.applyERPSetup(setup)

	writeJSON(w, http.StatusOK, s.setupStatusPayload())
}

func (s *Server) setupStatusPayload() map[string]any {
	return map[string]any{
		"ok":                   true,
		"erp_write_configured": s.cfg.HasERPWriteConfig(),
		"erp_read_configured":  strings.TrimSpace(s.cfg.ERPReadURL) != "",
		"batch_actions_ready":  s.cfg.HasERPWriteConfig(),
		"erp_url":              strings.TrimSpace(s.cfg.ERPURL),
		"erp_read_url":         strings.TrimSpace(s.cfg.ERPReadURL),
	}
}

func (s *Server) handleMonitorState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}

	payload, err := s.readMonitorPayload(r)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) handleMonitorStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "stream_unsupported",
		})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	writeSSEComment(w, "connected")
	flusher.Flush()

	ticker := time.NewTicker(350 * time.Millisecond)
	defer ticker.Stop()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	lastPayload := []byte(nil)
	for {
		payload, err := s.readMonitorPayload(r)
		if err != nil {
			writeSSEEvent(w, "error", map[string]any{"error": err.Error()})
			flusher.Flush()
			return
		}

		encoded, err := json.Marshal(payload)
		if err != nil {
			writeSSEEvent(w, "error", map[string]any{"error": err.Error()})
			flusher.Flush()
			return
		}
		if string(encoded) != string(lastPayload) {
			lastPayload = encoded
			if _, err := fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", encoded); err != nil {
				return
			}
			flusher.Flush()
		}

		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		case <-heartbeat.C:
			writeSSEComment(w, "ping")
			flusher.Flush()
		}
	}
}

func (s *Server) readMonitorPayload(r *http.Request) (map[string]any, error) {
	profile := s.currentProfile()
	authProfile, ok := s.authorize(r)
	if ok {
		profile = authProfile
	}

	snap, err := s.store.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			snap = bridgestate.Snapshot{}
		} else {
			return nil, err
		}
	}

	printer := map[string]any{
		"ok": false,
	}
	if value, err := s.fetchPrinterTrace(); err == nil {
		printer = value
	}

	return map[string]any{
		"ok":      true,
		"profile": profile,
		"state":   snap,
		"printer": printer,
	}, nil
}

func (s *Server) fetchPrinterTrace() (map[string]any, error) {
	base := strings.TrimRight(strings.TrimSpace(s.cfg.PolygonURL), "/")
	if base == "" {
		return nil, fmt.Errorf("polygon url empty")
	}

	resp, err := s.http.Get(base + "/api/v1/dev/printer")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("polygon printer status=%d", resp.StatusCode)
	}

	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Server) handleItems(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	items, err := s.control.SearchItems(r.Context(), strings.TrimSpace(r.URL.Query().Get("query")), parseLimit(r, 50))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"items": items,
	})
}

func (s *Server) handleItemRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	itemCode, ok := extractItemCodeFromWarehousesPath(r.URL.Path)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not_found"})
		return
	}
	stocks, err := s.control.SearchItemWarehouses(r.Context(), itemCode, strings.TrimSpace(r.URL.Query().Get("query")), parseLimit(r, 50))
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"item_code":  itemCode,
		"warehouses": stocks,
	})
}

func (s *Server) handleBatchState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	batch, err := s.readBatchSnapshot()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"batch": batch,
	})
}

func (s *Server) handleBatchStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req batchStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_json"})
		return
	}
	selection := workflow.Selection{
		ItemCode:  strings.TrimSpace(req.ItemCode),
		ItemName:  strings.TrimSpace(req.ItemName),
		Warehouse: strings.TrimSpace(req.Warehouse),
	}.Normalize()
	if selection.ItemCode == "" || selection.Warehouse == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "item_code_and_warehouse_required"})
		return
	}
	if !s.cfg.HasERPWriteConfig() {
		writeJSON(w, http.StatusPreconditionFailed, map[string]any{"error": "erp_not_configured"})
		return
	}
	if s.control.HasActiveBatch(mobileBatchOwnerID) {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "batch_already_active"})
		return
	}
	if _, ok := s.control.OtherActiveBatchOwner(mobileBatchOwnerID); ok {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "batch_active_elsewhere"})
		return
	}
	if !s.control.Start(s.controlCtx, mobileBatchOwnerID, selection, workflow.Hooks{}) {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "batch_start_failed"})
		return
	}
	batch, err := s.readBatchSnapshot()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"batch": batch,
	})
}

func (s *Server) handleBatchStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	if !s.control.Stop(mobileBatchOwnerID) {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "batch_not_active"})
		return
	}
	batch, err := s.readBatchSnapshot()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"batch": batch,
	})
}

func (s *Server) readBatchSnapshot() (bridgestate.BatchSnapshot, error) {
	snap, err := s.store.Read()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return bridgestate.BatchSnapshot{}, nil
		}
		return bridgestate.BatchSnapshot{}, err
	}
	return snap.Batch, nil
}

func extractItemCodeFromWarehousesPath(path string) (string, bool) {
	const prefix = "/v1/mobile/items/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(path, prefix)
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[1]) != "warehouses" {
		return "", false
	}
	itemCode, err := url.PathUnescape(strings.TrimSpace(parts[0]))
	if err != nil {
		return "", false
	}
	itemCode = strings.TrimSpace(itemCode)
	if itemCode == "" {
		return "", false
	}
	return itemCode, true
}

func parseLimit(r *http.Request, fallback int) int {
	limitText := strings.TrimSpace(r.URL.Query().Get("limit"))
	if limitText == "" {
		return fallback
	}
	limit, err := strconv.Atoi(limitText)
	if err != nil || limit <= 0 {
		return fallback
	}
	if limit > 50 {
		return 50
	}
	return limit
}

func (s *Server) authorize(r *http.Request) (SessionProfile, bool) {
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		return SessionProfile{}, false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	profile, ok := s.tokens[token]
	return profile, ok
}

func (s *Server) updateAuthorizedProfile(r *http.Request, profile SessionProfile) {
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		return
	}
	s.mu.Lock()
	s.tokens[token] = profile
	s.mu.Unlock()
}

func generateToken() (string, error) {
	var raw [24]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "dev-" + hex.EncodeToString(raw[:]), nil
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

func asString(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func writeMethodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
		"error": "method_not_allowed",
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeSSEEvent(w http.ResponseWriter, event string, payload any) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", strings.TrimSpace(event), encoded)
}

func writeSSEComment(w http.ResponseWriter, comment string) {
	_, _ = fmt.Fprintf(w, ": %s\n\n", strings.TrimSpace(comment))
}
