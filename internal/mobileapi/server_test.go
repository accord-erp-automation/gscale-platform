package mobileapi

import (
	bridgestate "bridge/state"
	"bufio"
	"bytes"
	"context"
	"core/batchcontrol"
	"core/workflow"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
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

	server := newServer(Config{
		ServerName:      "gscale-dev",
		BridgeStateFile: t.TempDir() + "/bridge_state.json",
	}, batchcontrol.New(batchcontrol.Dependencies{
		Catalog: stubCatalog{
			items: []batchcontrol.Item{
				{ItemCode: "ITEM-001", ItemName: "Green Tea"},
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

type stubCatalog struct {
	items      []batchcontrol.Item
	warehouses []batchcontrol.WarehouseStock
}

func (s stubCatalog) CheckConnection(context.Context) (string, error) {
	return "ERP DB Reader", nil
}

func (s stubCatalog) SearchItems(context.Context, string, int) ([]batchcontrol.Item, error) {
	return slices.Clone(s.items), nil
}

func (s stubCatalog) SearchItemWarehouses(context.Context, string, string, int) ([]batchcontrol.WarehouseStock, error) {
	return slices.Clone(s.warehouses), nil
}

type blockingRunner struct {
	started chan struct{}
	stopped chan struct{}
}

func (r *blockingRunner) Run(ctx context.Context, selection workflow.Selection, hooks workflow.Hooks) error {
	if r.started != nil {
		r.started <- struct{}{}
	}
	<-ctx.Done()
	if r.stopped != nil {
		r.stopped <- struct{}{}
	}
	return nil
}
