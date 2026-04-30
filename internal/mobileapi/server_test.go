package mobileapi

import (
	bridgestate "bridge/state"
	"bufio"
	"bytes"
	"context"
	"core/batchcontrol"
	"core/workflow"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestLoginAndProfile(t *testing.T) {
	t.Parallel()

	server := New(Config{
		ServerName:      "gscale-dev",
		BridgeStateFile: t.TempDir() + "/bridge_state.json",
		LoginPhone:      "998900000000",
		LoginCode:       "1234",
		Profile: SessionProfile{
			Role:        "admin",
			DisplayName: "Polygon Operator",
			LegalName:   "Polygon Operator",
			Ref:         "dev-operator",
			Phone:       "998900000000",
		},
	})

	body := bytes.NewBufferString(`{"phone":"998900000000","code":"1234"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/mobile/auth/login", body)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d", rec.Code)
	}

	var loginResp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}

	token, _ := loginResp["token"].(string)
	if token == "" {
		t.Fatal("token is empty")
	}

	profileReq := httptest.NewRequest(http.MethodGet, "/v1/mobile/profile", nil)
	profileReq.Header.Set("Authorization", "Bearer "+token)
	profileRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(profileRec, profileReq)

	if profileRec.Code != http.StatusOK {
		t.Fatalf("profile status = %d", profileRec.Code)
	}
	if !bytes.Contains(profileRec.Body.Bytes(), []byte(`"role":"admin"`)) {
		t.Fatalf("profile body = %s", profileRec.Body.String())
	}
}

func TestHandshakeReturnsServerIdentity(t *testing.T) {
	t.Parallel()

	server := New(Config{
		ServerName:      "gscale-dev",
		BridgeStateFile: t.TempDir() + "/bridge_state.json",
		Profile: SessionProfile{
			Role:        "admin",
			DisplayName: "Polygon Operator",
			LegalName:   "Polygon Operator",
			Ref:         "dev-operator",
			Phone:       "998900000000",
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/mobile/handshake", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("handshake status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"server_name":"gscale-dev"`)) {
		t.Fatalf("handshake body = %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"app":"gscale-zebra"`)) {
		t.Fatalf("handshake body = %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"candidate_ports"`)) {
		t.Fatalf("handshake body = %s", rec.Body.String())
	}
}

func TestMonitorStateReturnsBridgeSnapshot(t *testing.T) {
	t.Parallel()

	stateFile := t.TempDir() + "/bridge_state.json"
	store := bridgestate.New(stateFile)
	weight := 1.25
	stable := true
	if err := store.Update(func(snapshot *bridgestate.Snapshot) {
		snapshot.Scale = bridgestate.ScaleSnapshot{
			Source:    "polygon",
			Port:      "polygon://scale",
			Weight:    &weight,
			Unit:      "kg",
			Stable:    &stable,
			UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		}
	}); err != nil {
		t.Fatalf("seed bridge: %v", err)
	}

	server := New(Config{
		ServerName:      "gscale-dev",
		BridgeStateFile: stateFile,
		LoginPhone:      "998900000000",
		LoginCode:       "1234",
		Profile: SessionProfile{
			Role:        "admin",
			DisplayName: "Polygon Operator",
			LegalName:   "Polygon Operator",
			Ref:         "dev-operator",
			Phone:       "998900000000",
		},
	})

	loginReq := httptest.NewRequest(http.MethodPost, "/v1/mobile/auth/login", bytes.NewBufferString(`{"phone":"998900000000","code":"1234"}`))
	loginRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(loginRec, loginReq)

	var loginResp map[string]any
	if err := json.Unmarshal(loginRec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	token, _ := loginResp["token"].(string)

	req := httptest.NewRequest(http.MethodGet, "/v1/mobile/monitor/state", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("monitor status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"source":"polygon"`)) {
		t.Fatalf("monitor body = %s", rec.Body.String())
	}
}

func TestMonitorStreamReturnsInitialSnapshotEvent(t *testing.T) {
	t.Parallel()

	stateFile := t.TempDir() + "/bridge_state.json"
	store := bridgestate.New(stateFile)
	weight := 2.5
	stable := true
	if err := store.Update(func(snapshot *bridgestate.Snapshot) {
		snapshot.Scale = bridgestate.ScaleSnapshot{
			Source: "polygon",
			Weight: &weight,
			Unit:   "kg",
			Stable: &stable,
		}
	}); err != nil {
		t.Fatalf("seed bridge: %v", err)
	}

	server := New(Config{
		ServerName:      "gscale-dev",
		BridgeStateFile: stateFile,
		Profile: SessionProfile{
			Role:        "admin",
			DisplayName: "Polygon Operator",
			LegalName:   "Polygon Operator",
			Ref:         "dev-operator",
			Phone:       "998900000000",
		},
	})

	ts := httptest.NewServer(server.Handler())
	defer ts.Close()

	resp, err := ts.Client().Get(ts.URL + "/v1/mobile/monitor/stream")
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("content-type = %q", got)
	}

	reader := bufio.NewReader(resp.Body)
	foundEvent := false
	foundScale := false
	for i := 0; i < 8; i++ {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read stream: %v", err)
		}
		if bytes.Contains([]byte(line), []byte("event: snapshot")) {
			foundEvent = true
		}
		if bytes.Contains([]byte(line), []byte(`"weight":2.5`)) {
			foundScale = true
		}
		if foundEvent && foundScale {
			return
		}
	}

	t.Fatalf("stream did not include initial snapshot event")
}

func TestItemsEndpointReturnsCatalogResults(t *testing.T) {
	t.Parallel()

	const expectedWarehouse = "Stores - A"
	server := newServer(Config{
		ServerName:       "gscale-dev",
		BridgeStateFile:  t.TempDir() + "/bridge_state.json",
		WarehouseMode:    "default",
		DefaultWarehouse: expectedWarehouse,
	}, batchcontrol.New(batchcontrol.Dependencies{
		Catalog: stubCatalog{
			searchItemsFn: func(ctx context.Context, query string, limit int, warehouse string) ([]batchcontrol.Item, error) {
				if query != "tea" {
					t.Fatalf("query = %q", query)
				}
				if warehouse != expectedWarehouse {
					t.Fatalf("warehouse = %q", warehouse)
				}
				if limit != 0 {
					t.Fatalf("limit = %d", limit)
				}
				return []batchcontrol.Item{
					{ItemCode: "ITEM-001", ItemName: "Green Tea"},
				}, nil
			},
		},
	}))
	defer server.Close()

	req := httptest.NewRequest(http.MethodGet, "/v1/mobile/items?query=tea", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("items status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"item_code":"ITEM-001"`)) {
		t.Fatalf("items body = %s", rec.Body.String())
	}
}

func TestWarehousesEndpointReturnsCatalogResults(t *testing.T) {
	t.Parallel()

	server := newServer(Config{
		ServerName:      "gscale-dev",
		BridgeStateFile: t.TempDir() + "/bridge_state.json",
	}, batchcontrol.New(batchcontrol.Dependencies{
		Catalog: stubCatalog{
			warehouses: []batchcontrol.WarehouseStock{
				{Warehouse: "Stores - A", ActualQty: 12.5},
			},
		},
	}))
	defer server.Close()

	req := httptest.NewRequest(http.MethodGet, "/v1/mobile/items/ITEM-001/warehouses?query=stores", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("warehouses status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"warehouse":"Stores - A"`)) {
		t.Fatalf("warehouses body = %s", rec.Body.String())
	}
}

func TestWarehouseListEndpointReturnsWarehouseChoices(t *testing.T) {
	t.Parallel()

	erpService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/resource/Warehouse" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("fields"); got == "" {
			t.Fatalf("fields query missing")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"name":"Stores - A","company":"Main Co"}]}`))
	}))
	defer erpService.Close()

	server := New(Config{
		ServerName:      "gscale-dev",
		BridgeStateFile: t.TempDir() + "/bridge_state.json",
		ERPURL:          erpService.URL,
		ERPAPIKey:       "key-123",
		ERPAPISecret:    "secret-123",
	})
	defer server.Close()

	req := httptest.NewRequest(http.MethodGet, "/v1/mobile/warehouses?query=stores", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("warehouse list status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"warehouse":"Stores - A"`)) {
		t.Fatalf("warehouse list body = %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"company":"Main Co"`)) {
		t.Fatalf("warehouse list body = %s", rec.Body.String())
	}
}

func TestWarehouseListEndpointUsesReadService(t *testing.T) {
	t.Parallel()

	readService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/warehouses" {
			http.NotFound(w, r)
			return
		}
		if got := r.URL.Query().Get("query"); got != "stores" {
			t.Fatalf("query = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"name":"Stores - A","company":"Read Co"}]}`))
	}))
	defer readService.Close()

	server := New(Config{
		ServerName:      "gscale-dev",
		BridgeStateFile: t.TempDir() + "/bridge_state.json",
		ERPReadURL:      readService.URL,
	})
	defer server.Close()

	req := httptest.NewRequest(http.MethodGet, "/v1/mobile/warehouses?query=stores", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("warehouse list status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"warehouse":"Stores - A"`)) {
		t.Fatalf("warehouse list body = %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"company":"Read Co"`)) {
		t.Fatalf("warehouse list body = %s", rec.Body.String())
	}
}

func TestBatchStartStopEndpoints(t *testing.T) {
	t.Parallel()

	stateFile := t.TempDir() + "/bridge_state.json"
	store := bridgestate.New(stateFile)
	writer := bridgeBatchStateWriter{store: store}
	runner := &blockingRunner{
		started: make(chan struct{}, 1),
		stopped: make(chan struct{}, 1),
	}
	server := newServer(Config{
		ServerName:      "gscale-dev",
		BridgeStateFile: stateFile,
		ERPURL:          "http://localhost:8000",
		ERPAPIKey:       "key-123",
		ERPAPISecret:    "secret-123",
	}, batchcontrol.New(batchcontrol.Dependencies{
		Catalog:    stubCatalog{},
		BatchState: writer,
		Runner:     runner,
	}))
	defer server.Close()

	startReq := httptest.NewRequest(http.MethodPost, "/v1/mobile/batch/start", bytes.NewBufferString(`{"item_code":"ITEM-001","item_name":"Green Tea","warehouse":"Stores - A"}`))
	startRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(startRec, startReq)

	if startRec.Code != http.StatusOK {
		t.Fatalf("start status = %d body=%s", startRec.Code, startRec.Body.String())
	}
	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not start")
	}

	stateReq := httptest.NewRequest(http.MethodGet, "/v1/mobile/batch/state", nil)
	stateRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(stateRec, stateReq)
	if stateRec.Code != http.StatusOK {
		t.Fatalf("state status = %d body=%s", stateRec.Code, stateRec.Body.String())
	}
	if !bytes.Contains(stateRec.Body.Bytes(), []byte(`"active":true`)) {
		t.Fatalf("state body = %s", stateRec.Body.String())
	}

	stopReq := httptest.NewRequest(http.MethodPost, "/v1/mobile/batch/stop", nil)
	stopRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(stopRec, stopReq)
	if stopRec.Code != http.StatusOK {
		t.Fatalf("stop status = %d body=%s", stopRec.Code, stopRec.Body.String())
	}

	select {
	case <-runner.stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not stop")
	}

	snap, err := store.Read()
	if err != nil {
		t.Fatalf("read bridge after stop: %v", err)
	}
	if snap.Batch.Active {
		t.Fatalf("batch should be inactive after stop: %+v", snap.Batch)
	}
}

func TestBatchStopReturnsSummaryMessage(t *testing.T) {
	t.Parallel()

	tempDir, err := os.MkdirTemp("", "mobileapi-batch-stop-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})
	stateFile := tempDir + "/bridge_state.json"
	store := bridgestate.New(stateFile)
	runner := &blockingRunner{
		started: make(chan struct{}, 1),
		stopped: make(chan struct{}, 1),
		progresses: []workflow.Progress{
			{
				Selection: workflow.Selection{
					ItemCode:  "ITEM-001",
					ItemName:  "Green Tea",
					Warehouse: "Stores - A",
				},
				TotalQty: 7.25,
			},
		},
	}
	server := newServer(Config{
		ServerName:      "gscale-dev",
		BridgeStateFile: stateFile,
		ERPURL:          "http://localhost:8000",
		ERPAPIKey:       "key-123",
		ERPAPISecret:    "secret-123",
	}, batchcontrol.New(batchcontrol.Dependencies{
		Catalog:    stubCatalog{},
		BatchState: bridgeBatchStateWriter{store: store},
		Runner:     runner,
	}))
	defer server.Close()

	startReq := httptest.NewRequest(http.MethodPost, "/v1/mobile/batch/start", bytes.NewBufferString(`{"item_code":"ITEM-001","item_name":"Green Tea","warehouse":"Stores - A"}`))
	startRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("start status = %d body=%s", startRec.Code, startRec.Body.String())
	}
	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not start")
	}

	stopReq := httptest.NewRequest(http.MethodPost, "/v1/mobile/batch/stop", nil)
	stopRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(stopRec, stopReq)
	if stopRec.Code != http.StatusOK {
		t.Fatalf("stop status = %d body=%s", stopRec.Code, stopRec.Body.String())
	}
	if !bytes.Contains(stopRec.Body.Bytes(), []byte(`"Green Tea avvalgi deb 7.250 kg bo'ldi"`)) {
		t.Fatalf("stop body = %s", stopRec.Body.String())
	}
	select {
	case <-runner.stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not stop")
	}
}

func TestArchiveEndpointReturnsBatchHistory(t *testing.T) {
	t.Parallel()

	stateDir, err := os.MkdirTemp("", "mobileapi-archive-state-*")
	if err != nil {
		t.Fatal(err)
	}
	archiveDir, err := os.MkdirTemp("", "mobileapi-archive-file-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(stateDir)
		_ = os.RemoveAll(archiveDir)
	})
	stateFile := stateDir + "/bridge_state.json"
	archiveFile := archiveDir + "/archive.fb"
	store := bridgestate.New(stateFile)
	runner := &blockingRunner{
		started: make(chan struct{}, 1),
		stopped: make(chan struct{}, 1),
		progresses: []workflow.Progress{
			{
				Selection: workflow.Selection{
					ItemCode:  "ITEM-001",
					ItemName:  "Green Tea",
					Warehouse: "Stores - A",
				},
				DraftCount: 1,
				LastSuccess: workflow.LastSuccess{
					DraftName: "MAT-STE-0001",
					Qty:       7.25,
					Unit:      "kg",
					EPC:       "EPC-1",
				},
				TotalQty: 7.25,
			},
		},
	}
	server := newServer(Config{
		ServerName:      "gscale-dev",
		BridgeStateFile: stateFile,
		ArchiveFile:     archiveFile,
		ERPURL:          "http://localhost:8000",
		ERPAPIKey:       "key-123",
		ERPAPISecret:    "secret-123",
	}, batchcontrol.New(batchcontrol.Dependencies{
		Catalog:    stubCatalog{},
		BatchState: bridgeBatchStateWriter{store: store},
		Runner:     runner,
	}))
	defer server.Close()

	startReq := httptest.NewRequest(http.MethodPost, "/v1/mobile/batch/start", bytes.NewBufferString(`{"item_code":"ITEM-001","item_name":"Green Tea","warehouse":"Stores - A"}`))
	startRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(startRec, startReq)
	if startRec.Code != http.StatusOK {
		t.Fatalf("start status = %d body=%s", startRec.Code, startRec.Body.String())
	}
	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not start")
	}

	stopReq := httptest.NewRequest(http.MethodPost, "/v1/mobile/batch/stop", nil)
	stopRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(stopRec, stopReq)
	if stopRec.Code != http.StatusOK {
		t.Fatalf("stop status = %d body=%s", stopRec.Code, stopRec.Body.String())
	}
	select {
	case <-runner.stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not stop")
	}

	archiveReq := httptest.NewRequest(http.MethodGet, "/v1/mobile/archive", nil)
	archiveRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(archiveRec, archiveReq)
	if archiveRec.Code != http.StatusOK {
		t.Fatalf("archive status = %d body=%s", archiveRec.Code, archiveRec.Body.String())
	}
	if !bytes.Contains(archiveRec.Body.Bytes(), []byte(`"item_name":"Green Tea"`)) {
		t.Fatalf("archive body = %s", archiveRec.Body.String())
	}
}

func TestSetupStatusEndpoint(t *testing.T) {
	t.Parallel()

	server := New(Config{
		ServerName:       "gscale-dev",
		BridgeStateFile:  t.TempDir() + "/bridge_state.json",
		ERPReadURL:       "http://127.0.0.1:8090",
		ERPURL:           "http://localhost:8000",
		ERPAPIKey:        "key-123",
		ERPAPISecret:     "secret-123",
		WarehouseMode:    "default",
		DefaultWarehouse: "Stores - A",
	})
	defer server.Close()

	req := httptest.NewRequest(http.MethodGet, "/v1/mobile/setup/status", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"erp_write_configured":true`)) {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"erp_read_configured":true`)) {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"erp_url":"http://localhost:8000"`)) {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"erp_read_url":"http://127.0.0.1:8090"`)) {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"warehouse_mode":"default"`)) {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"default_warehouse":"Stores - A"`)) {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"warehouse_default_active":true`)) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestSetupWarehouseEndpointStoresDefaultWarehouse(t *testing.T) {
	t.Parallel()

	setupPath := t.TempDir() + "/mobile_setup.json"
	server := New(Config{
		ServerName:      "gscale-dev",
		BridgeStateFile: t.TempDir() + "/bridge_state.json",
		SetupFile:       setupPath,
		ERPURL:          "http://localhost:8000",
		ERPAPIKey:       "key-123",
		ERPAPISecret:    "secret-123",
	})
	defer server.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/mobile/setup/warehouse", bytes.NewBufferString(`{"warehouse_mode":"default","default_warehouse":"Stores - A"}`))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"warehouse_mode":"default"`)) {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"default_warehouse":"Stores - A"`)) {
		t.Fatalf("body = %s", rec.Body.String())
	}
	saved, err := loadERPSetup(setupPath)
	if err != nil {
		t.Fatalf("loadERPSetup: %v", err)
	}
	if saved.WarehouseMode != "default" {
		t.Fatalf("saved WarehouseMode = %q", saved.WarehouseMode)
	}
	if saved.DefaultWarehouse != "Stores - A" {
		t.Fatalf("saved DefaultWarehouse = %q", saved.DefaultWarehouse)
	}
	if server.cfg.WarehouseMode != "default" {
		t.Fatalf("server cfg WarehouseMode = %q", server.cfg.WarehouseMode)
	}
	if server.cfg.DefaultWarehouse != "Stores - A" {
		t.Fatalf("server cfg DefaultWarehouse = %q", server.cfg.DefaultWarehouse)
	}
}

func TestBatchStartUsesDefaultWarehouseWhenConfigured(t *testing.T) {
	t.Parallel()

	stateFile := t.TempDir() + "/bridge_state.json"
	store := bridgestate.New(stateFile)
	writer := bridgeBatchStateWriter{store: store}
	runner := &blockingRunner{
		started: make(chan struct{}, 1),
		stopped: make(chan struct{}, 1),
	}
	server := newServer(Config{
		ServerName:       "gscale-dev",
		BridgeStateFile:  stateFile,
		ERPURL:           "http://localhost:8000",
		ERPAPIKey:        "key-123",
		ERPAPISecret:     "secret-123",
		WarehouseMode:    "default",
		DefaultWarehouse: "Stores - A",
	}, batchcontrol.New(batchcontrol.Dependencies{
		Catalog:    stubCatalog{},
		BatchState: writer,
		Runner:     runner,
	}))
	defer func() {
		server.Close()
		select {
		case <-runner.stopped:
		case <-time.After(2 * time.Second):
		}
	}()

	startReq := httptest.NewRequest(http.MethodPost, "/v1/mobile/batch/start", bytes.NewBufferString(`{"item_code":"ITEM-001","item_name":"Green Tea"}`))
	startRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(startRec, startReq)

	if startRec.Code != http.StatusOK {
		t.Fatalf("start status = %d body=%s", startRec.Code, startRec.Body.String())
	}
	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not start")
	}

	snap, err := store.Read()
	if err != nil {
		t.Fatalf("read bridge after start: %v", err)
	}
	if snap.Batch.Warehouse != "Stores - A" {
		t.Fatalf("batch warehouse = %q", snap.Batch.Warehouse)
	}
	if snap.Batch.PrintMode != workflow.PrintModeRFID {
		t.Fatalf("batch print mode = %q", snap.Batch.PrintMode)
	}
}

func TestBatchStartFailsFastWhenERPNotConfigured(t *testing.T) {
	t.Parallel()

	server := newServer(Config{
		ServerName:      "gscale-dev",
		BridgeStateFile: t.TempDir() + "/bridge_state.json",
	}, batchcontrol.New(batchcontrol.Dependencies{
		Catalog:    stubCatalog{},
		BatchState: bridgeBatchStateWriter{store: bridgestate.New(t.TempDir() + "/bridge_state.json")},
		Runner:     &blockingRunner{},
	}))
	defer server.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/mobile/batch/start", bytes.NewBufferString(`{"item_code":"ITEM-001","item_name":"Green Tea","warehouse":"Stores - A"}`))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"erp_not_configured"`)) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestBatchStartPropagatesPrintMode(t *testing.T) {
	t.Parallel()

	stateFile := t.TempDir() + "/bridge_state.json"
	store := bridgestate.New(stateFile)
	runner := &blockingRunner{
		started: make(chan struct{}, 1),
		stopped: make(chan struct{}, 1),
	}
	server := newServer(Config{
		ServerName:      "gscale-dev",
		BridgeStateFile: stateFile,
		ERPURL:          "http://localhost:8000",
		ERPAPIKey:       "key-123",
		ERPAPISecret:    "secret-123",
	}, batchcontrol.New(batchcontrol.Dependencies{
		Catalog:    stubCatalog{},
		BatchState: bridgeBatchStateWriter{store: store},
		Runner:     runner,
	}))
	defer func() {
		server.Close()
		select {
		case <-runner.stopped:
		case <-time.After(2 * time.Second):
		}
	}()

	startReq := httptest.NewRequest(http.MethodPost, "/v1/mobile/batch/start", bytes.NewBufferString(`{"item_code":"ITEM-001","item_name":"Green Tea","warehouse":"Stores - A","print_mode":"label","printer":"g500","quantity_source":"manual","manual_qty_kg":5,"tare_enabled":true,"tare_kg":0.78}`))
	startRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(startRec, startReq)

	if startRec.Code != http.StatusOK {
		t.Fatalf("start status = %d body=%s", startRec.Code, startRec.Body.String())
	}
	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not start")
	}
	if runner.lastSelection.PrintMode != workflow.PrintModeLabelOnly {
		t.Fatalf("runner print mode = %q", runner.lastSelection.PrintMode)
	}
	if runner.lastSelection.Printer != "godex" {
		t.Fatalf("runner printer = %q", runner.lastSelection.Printer)
	}
	if !runner.lastSelection.TareEnabled || runner.lastSelection.TareKG != 0.78 {
		t.Fatalf("runner tare = enabled:%v kg:%v", runner.lastSelection.TareEnabled, runner.lastSelection.TareKG)
	}
	if runner.lastSelection.QuantitySource != workflow.QuantitySourceManual || runner.lastSelection.ManualQtyKG != 5 {
		t.Fatalf("runner quantity source = %q manual=%v", runner.lastSelection.QuantitySource, runner.lastSelection.ManualQtyKG)
	}

	snap, err := store.Read()
	if err != nil {
		t.Fatalf("read bridge after start: %v", err)
	}
	if snap.Batch.PrintMode != workflow.PrintModeLabelOnly {
		t.Fatalf("batch print mode = %q", snap.Batch.PrintMode)
	}
	if snap.Batch.Printer != "godex" {
		t.Fatalf("batch printer = %q", snap.Batch.Printer)
	}
	if !snap.Batch.Tare || snap.Batch.TareKG != 0.78 {
		t.Fatalf("batch tare = enabled:%v kg:%v", snap.Batch.Tare, snap.Batch.TareKG)
	}
	if snap.Batch.QuantitySource != workflow.QuantitySourceManual || snap.Batch.ManualQtyKG != 5 {
		t.Fatalf("batch quantity source = %q manual=%v", snap.Batch.QuantitySource, snap.Batch.ManualQtyKG)
	}

	stopReq := httptest.NewRequest(http.MethodPost, "/v1/mobile/batch/stop", nil)
	stopRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(stopRec, stopReq)
	if stopRec.Code != http.StatusOK {
		t.Fatalf("stop status = %d body=%s", stopRec.Code, stopRec.Body.String())
	}
}

func TestBridgePrintRequestWriterUsesSelectionPrintMode(t *testing.T) {
	t.Parallel()

	stateFile := t.TempDir() + "/bridge_state.json"
	store := bridgestate.New(stateFile)
	if err := store.Update(func(snapshot *bridgestate.Snapshot) {
		snapshot.Batch.Active = true
		snapshot.Batch.PrintMode = workflow.PrintModeLabelOnly
	}); err != nil {
		t.Fatalf("seed batch state: %v", err)
	}

	writer := bridgePrintRequestWriter{store: store}
	writer.SetPrintRequest(
		"3034257BF7194E406994036B",
		1.72,
		2.5,
		"kg",
		workflow.Selection{
			ItemCode:    "ITEM-001",
			ItemName:    "Green Tea",
			Warehouse:   "Stores - A",
			PrintMode:   workflow.PrintModeLabelOnly,
			Printer:     "g500",
			TareEnabled: true,
			TareKG:      0.78,
		},
	)

	snap, err := store.Read()
	if err != nil {
		t.Fatalf("read bridge state: %v", err)
	}
	if snap.PrintRequest.Mode != workflow.PrintModeLabelOnly {
		t.Fatalf("print request mode = %q", snap.PrintRequest.Mode)
	}
	if snap.PrintRequest.Printer != "godex" {
		t.Fatalf("print request printer = %q", snap.PrintRequest.Printer)
	}
	if snap.PrintRequest.Qty == nil || *snap.PrintRequest.Qty != 1.72 {
		t.Fatalf("print request net qty = %+v", snap.PrintRequest.Qty)
	}
	if snap.PrintRequest.GrossQty == nil || *snap.PrintRequest.GrossQty != 2.5 {
		t.Fatalf("print request gross qty = %+v", snap.PrintRequest.GrossQty)
	}
	if !snap.PrintRequest.Tare || snap.PrintRequest.TareKG != 0.78 {
		t.Fatalf("print request tare = enabled:%v kg:%v", snap.PrintRequest.Tare, snap.PrintRequest.TareKG)
	}
}

func TestSetupERPStoresValidatedConfig(t *testing.T) {
	t.Parallel()

	setupPath := t.TempDir() + "/mobile_setup.json"
	readService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/handshake":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"service":"gscale_erp_read"}`))
		case "/healthz":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer readService.Close()
	server := New(Config{
		ServerName:      "gscale-dev",
		BridgeStateFile: t.TempDir() + "/bridge_state.json",
		SetupFile:       setupPath,
	})
	server.validateERPSetup = func(context.Context, ERPSetup) error { return nil }
	defer server.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/mobile/setup/erp", bytes.NewBufferString(`{"erp_url":"http://localhost:8000","erp_read_url":"`+readService.URL+`","erp_api_key":"key-123","erp_api_secret":"secret-123"}`))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"erp_write_configured":true`)) {
		t.Fatalf("body = %s", rec.Body.String())
	}

	saved, err := loadERPSetup(setupPath)
	if err != nil {
		t.Fatalf("loadERPSetup: %v", err)
	}
	if saved.ERPURL != "http://localhost:8000" {
		t.Fatalf("saved ERPURL = %q", saved.ERPURL)
	}
	if saved.ERPReadURL != readService.URL {
		t.Fatalf("saved ERPReadURL = %q", saved.ERPReadURL)
	}
	if server.cfg.ERPAPIKey != "key-123" {
		t.Fatalf("server cfg ERPAPIKey = %q", server.cfg.ERPAPIKey)
	}
	if server.cfg.ERPReadURL != readService.URL {
		t.Fatalf("server cfg ERPReadURL = %q", server.cfg.ERPReadURL)
	}
}

func TestSetupERPCanSucceedWithoutReadService(t *testing.T) {
	t.Parallel()

	setupPath := t.TempDir() + "/mobile_setup.json"
	erpService := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/method/frappe.auth.get_logged_user":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"message":"Administrator"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer erpService.Close()

	server := New(Config{
		ServerName:      "gscale-dev",
		BridgeStateFile: t.TempDir() + "/bridge_state.json",
		SetupFile:       setupPath,
	})
	defer server.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/mobile/setup/erp", bytes.NewBufferString(`{"erp_url":"`+erpService.URL+`","erp_api_key":"key-123","erp_api_secret":"secret-123"}`))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"erp_write_configured":true`)) {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"erp_read_configured":false`)) {
		t.Fatalf("body = %s", rec.Body.String())
	}

	saved, err := loadERPSetup(setupPath)
	if err != nil {
		t.Fatalf("loadERPSetup: %v", err)
	}
	if saved.ERPURL != erpService.URL {
		t.Fatalf("saved ERPURL = %q", saved.ERPURL)
	}
	if saved.ERPReadURL != "" {
		t.Fatalf("saved ERPReadURL = %q", saved.ERPReadURL)
	}
}

func TestSetupERPRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	setupPath := t.TempDir() + "/mobile_setup.json"
	server := New(Config{
		ServerName:      "gscale-dev",
		BridgeStateFile: t.TempDir() + "/bridge_state.json",
		SetupFile:       setupPath,
	})
	server.validateERPSetup = func(context.Context, ERPSetup) error { return errors.New("bad credentials") }
	defer server.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/mobile/setup/erp", bytes.NewBufferString(`{"erp_url":"http://localhost:8000","erp_api_key":"key-123","erp_api_secret":"secret-123"}`))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"erp_validation_failed"`)) {
		t.Fatalf("body = %s", rec.Body.String())
	}
	got, err := loadERPSetup(setupPath)
	if err != nil {
		t.Fatalf("loadERPSetup: %v", err)
	}
	if got.ERPURL != "" || got.ERPAPIKey != "" || got.ERPAPISecret != "" {
		t.Fatalf("setup should stay empty after validation failure: %#v", got)
	}
}

func TestSetupERPCanBeCleared(t *testing.T) {
	t.Parallel()

	setupPath := t.TempDir() + "/mobile_setup.json"
	if err := saveERPSetup(setupPath, ERPSetup{
		ERPURL:       "http://localhost:8000",
		ERPReadURL:   "http://127.0.0.1:8090",
		ERPAPIKey:    "key-123",
		ERPAPISecret: "secret-123",
	}); err != nil {
		t.Fatalf("saveERPSetup: %v", err)
	}

	server := New(Config{
		ServerName:      "gscale-dev",
		BridgeStateFile: t.TempDir() + "/bridge_state.json",
		SetupFile:       setupPath,
		ERPURL:          "http://localhost:8000",
		ERPReadURL:      "http://127.0.0.1:8090",
		ERPAPIKey:       "key-123",
		ERPAPISecret:    "secret-123",
	})
	defer server.Close()

	req := httptest.NewRequest(http.MethodDelete, "/v1/mobile/setup/erp", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"erp_write_configured":false`)) {
		t.Fatalf("body = %s", rec.Body.String())
	}
	got, err := loadERPSetup(setupPath)
	if err != nil {
		t.Fatalf("loadERPSetup: %v", err)
	}
	if got.ERPURL != "" || got.ERPAPIKey != "" || got.ERPAPISecret != "" {
		t.Fatalf("setup should be cleared: %#v", got)
	}
	if got.ERPReadURL != "" {
		t.Fatalf("setup read URL should be cleared: %#v", got)
	}
	if server.cfg.HasERPWriteConfig() {
		t.Fatal("server cfg should be cleared")
	}
	if strings.TrimSpace(server.cfg.ERPReadURL) != "" {
		t.Fatalf("server cfg ERPReadURL should be cleared: %q", server.cfg.ERPReadURL)
	}
}

type stubCatalog struct {
	items         []batchcontrol.Item
	warehouses    []batchcontrol.WarehouseStock
	searchItemsFn func(context.Context, string, int, string) ([]batchcontrol.Item, error)
}

func (s stubCatalog) CheckConnection(context.Context) (string, error) {
	return "ERP DB Reader", nil
}

func (s stubCatalog) SearchItems(ctx context.Context, query string, limit int, warehouse string) ([]batchcontrol.Item, error) {
	if s.searchItemsFn != nil {
		return s.searchItemsFn(ctx, query, limit, warehouse)
	}
	return slices.Clone(s.items), nil
}

func (s stubCatalog) SearchItemWarehouses(context.Context, string, string, int) ([]batchcontrol.WarehouseStock, error) {
	return slices.Clone(s.warehouses), nil
}

type blockingRunner struct {
	started       chan struct{}
	stopped       chan struct{}
	progresses    []workflow.Progress
	lastSelection workflow.Selection
}

func (r *blockingRunner) Run(ctx context.Context, selection workflow.Selection, hooks workflow.Hooks) error {
	selection = selection.Normalize()
	r.lastSelection = selection
	if r.started != nil {
		r.started <- struct{}{}
	}
	for _, progress := range r.progresses {
		progress.Selection = selection
		if hooks.Progress != nil {
			hooks.Progress(progress)
		}
	}
	<-ctx.Done()
	if r.stopped != nil {
		r.stopped <- struct{}{}
	}
	return nil
}
