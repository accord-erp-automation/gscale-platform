package mobileapi

import (
	bridgestate "bridge/state"
	"bytes"
	"context"
	corepkg "core"
	"core/batchcontrol"
	"core/erpread"
	"core/workflow"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const mobileBatchOwnerID int64 = 1
const erpCatalogResponseLimit = 8 * 1024 * 1024

func validateERPWriteSetup(ctx context.Context, setup ERPSetup) error {
	client := &erpClient{
		baseURL:   strings.TrimRight(strings.TrimSpace(setup.ERPURL), "/"),
		apiKey:    strings.TrimSpace(setup.ERPAPIKey),
		apiSecret: strings.TrimSpace(setup.ERPAPISecret),
		http:      &http.Client{Timeout: 12 * time.Second},
	}
	_, err := client.CheckConnection(ctx)
	return err
}

func (s *Server) applyERPSetup(setup ERPSetup) {
	if s == nil {
		return
	}
	setup.ERPURL = strings.TrimSpace(setup.ERPURL)
	setup.ERPReadURL = strings.TrimSpace(setup.ERPReadURL)
	setup.ERPAPIKey = strings.TrimSpace(setup.ERPAPIKey)
	setup.ERPAPISecret = strings.TrimSpace(setup.ERPAPISecret)

	oldControl := s.control
	oldCancel := s.controlCancel

	s.cfg.ERPURL = setup.ERPURL
	s.cfg.ERPReadURL = setup.ERPReadURL
	s.cfg.ERPAPIKey = setup.ERPAPIKey
	s.cfg.ERPAPISecret = setup.ERPAPISecret
	s.controlCtx, s.controlCancel = context.WithCancel(context.Background())
	s.control = s.newControlService()

	if oldControl != nil {
		oldControl.StopAll()
	}
	if oldCancel != nil {
		oldCancel()
	}
}

func (s *Server) applyWarehouseSetup(setup ERPSetup) {
	if s == nil {
		return
	}
	s.cfg.WarehouseMode = normalizeWarehouseMode(setup.WarehouseMode)
	s.cfg.DefaultWarehouse = strings.TrimSpace(setup.DefaultWarehouse)
}

func (s *Server) newControlService() *batchcontrol.Service {
	bridgeStore := bridgestate.New(s.cfg.BridgeStateFile)
	erp := newERPClient(s.cfg)
	return batchcontrol.New(batchcontrol.Dependencies{
		Catalog:    erp,
		BatchState: bridgeBatchStateWriter{store: bridgeStore},
		Runner: workflow.NewMaterialReceiptRunner(workflow.MaterialReceiptDependencies{
			QtyReader:               workflowBridgeClient{store: bridgeStore},
			ERP:                     erp,
			PrintRequests:           bridgePrintRequestWriter{store: bridgeStore},
			EPCGenerator:            corepkg.NewEPCGenerator(),
			History:                 discardHistory{},
			Logger:                  log.Default(),
			IsDuplicateBarcodeError: IsDuplicateBarcodeError,
		}),
		Logger: log.Default(),
	})
}

type discardHistory struct{}

func (discardHistory) Add(string) {}

type bridgeBatchStateWriter struct {
	store *bridgestate.Store
}

func (w bridgeBatchStateWriter) Set(active bool, ownerID int64, selection workflow.Selection) error {
	if w.store == nil {
		return nil
	}
	selection = selection.Normalize()
	at := time.Now().UTC().Format(time.RFC3339Nano)
	return w.store.Update(func(snapshot *bridgestate.Snapshot) {
		snapshot.Batch.Active = active
		snapshot.Batch.ChatID = ownerID
		if active {
			snapshot.Batch.ItemCode = selection.ItemCode
			snapshot.Batch.ItemName = selection.ItemName
			snapshot.Batch.Warehouse = selection.Warehouse
			snapshot.Batch.TotalQty = 0
		} else {
			snapshot.PrintRequest = bridgestate.PrintRequestSnapshot{}
		}
		snapshot.Batch.UpdatedAt = at
	})
}

type bridgePrintRequestWriter struct {
	store *bridgestate.Store
}

func (w bridgePrintRequestWriter) SetPrintRequest(epc string, qty float64, unit string, selection workflow.Selection) {
	if w.store == nil {
		return
	}
	epc = strings.ToUpper(strings.TrimSpace(epc))
	unit = strings.TrimSpace(unit)
	if unit == "" {
		unit = "kg"
	}
	selection = selection.Normalize()
	at := time.Now().UTC().Format(time.RFC3339Nano)
	q := qty
	_ = w.store.Update(func(snapshot *bridgestate.Snapshot) {
		snapshot.PrintRequest.EPC = epc
		snapshot.PrintRequest.Qty = &q
		snapshot.PrintRequest.Unit = unit
		snapshot.PrintRequest.ItemCode = selection.ItemCode
		snapshot.PrintRequest.ItemName = selection.ItemName
		snapshot.PrintRequest.Status = "pending"
		snapshot.PrintRequest.Error = ""
		snapshot.PrintRequest.RequestedAt = at
		snapshot.PrintRequest.UpdatedAt = at
	})
}

func (w bridgePrintRequestWriter) ClearPrintRequest() {
	if w.store == nil {
		return
	}
	_ = w.store.Update(func(snapshot *bridgestate.Snapshot) {
		snapshot.PrintRequest = bridgestate.PrintRequestSnapshot{}
	})
}

type workflowBridgeClient struct {
	store *bridgestate.Store
}

func (c workflowBridgeClient) WaitStablePositiveReading(ctx context.Context, timeout, pollInterval time.Duration) (workflow.StableReading, error) {
	if c.store == nil || strings.TrimSpace(c.store.Path()) == "" {
		return workflow.StableReading{}, fmt.Errorf("bridge state path bo'sh")
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if pollInterval <= 0 {
		pollInterval = 220 * time.Millisecond
	}

	deadline := time.Now().Add(timeout)
	var lastWeight float64
	var haveLast bool
	stableCount := 0

	for {
		if time.Now().After(deadline) {
			return workflow.StableReading{}, fmt.Errorf("scale qty timeout (%s)", timeout)
		}
		select {
		case <-ctx.Done():
			return workflow.StableReading{}, ctx.Err()
		default:
		}

		snap, err := c.store.Read()
		if err != nil {
			time.Sleep(pollInterval)
			continue
		}
		scale := snap.Scale
		if strings.TrimSpace(scale.Error) != "" {
			haveLast = false
			stableCount = 0
			time.Sleep(pollInterval)
			continue
		}
		if scale.Weight == nil || *scale.Weight <= 0 {
			haveLast = false
			stableCount = 0
			time.Sleep(pollInterval)
			continue
		}
		updatedAt, ok := parseBridgeSnapshotTime(scale.UpdatedAt)
		if !ok || !isFreshBridgeTime(updatedAt, 4*time.Second) {
			time.Sleep(pollInterval)
			continue
		}

		weight := *scale.Weight
		if scale.Stable != nil && *scale.Stable {
			return workflow.StableReading{
				Qty:       weight,
				Unit:      normalizeBridgeUnit(scale.Unit),
				UpdatedAt: updatedAt,
			}, nil
		}

		if haveLast && almostEqualBridge(lastWeight, weight, 0.001) {
			stableCount++
		} else {
			stableCount = 1
		}
		haveLast = true
		lastWeight = weight

		if stableCount >= 4 {
			return workflow.StableReading{
				Qty:       weight,
				Unit:      normalizeBridgeUnit(scale.Unit),
				UpdatedAt: updatedAt,
			}, nil
		}
		time.Sleep(pollInterval)
	}
}

func (c workflowBridgeClient) WaitPrintRequestResult(ctx context.Context, timeout, pollInterval time.Duration, epc string) (workflow.PrintRequestResult, error) {
	if c.store == nil || strings.TrimSpace(c.store.Path()) == "" {
		return workflow.PrintRequestResult{}, fmt.Errorf("bridge state path bo'sh")
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if pollInterval <= 0 {
		pollInterval = 120 * time.Millisecond
	}
	epc = strings.ToUpper(strings.TrimSpace(epc))
	if epc == "" {
		return workflow.PrintRequestResult{}, fmt.Errorf("print request epc bo'sh")
	}

	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return workflow.PrintRequestResult{}, fmt.Errorf("print request timeout (%s)", timeout)
		}
		select {
		case <-ctx.Done():
			return workflow.PrintRequestResult{}, ctx.Err()
		default:
		}

		snap, err := c.store.Read()
		if err != nil {
			time.Sleep(pollInterval)
			continue
		}

		req := snap.PrintRequest
		gotEPC := strings.ToUpper(strings.TrimSpace(req.EPC))
		if gotEPC != epc {
			time.Sleep(pollInterval)
			continue
		}

		status := strings.ToLower(strings.TrimSpace(req.Status))
		if status != "done" && status != "error" {
			time.Sleep(pollInterval)
			continue
		}

		at, _ := parseBridgeSnapshotTime(req.UpdatedAt)
		return workflow.PrintRequestResult{
			EPC:       gotEPC,
			Status:    status,
			Error:     strings.TrimSpace(req.Error),
			UpdatedAt: at,
		}, nil
	}
}

func (c workflowBridgeClient) WaitForNextCycle(ctx context.Context, timeout, pollInterval time.Duration, lastQty float64) error {
	if c.store == nil || strings.TrimSpace(c.store.Path()) == "" {
		return fmt.Errorf("bridge state path bo'sh")
	}
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	if pollInterval <= 0 {
		pollInterval = 220 * time.Millisecond
	}

	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("scale next-cycle timeout (%s)", timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		snap, err := c.store.Read()
		if err != nil {
			time.Sleep(pollInterval)
			continue
		}
		scale := snap.Scale
		if !isFreshBridgeSnapshot(scale.UpdatedAt, 4*time.Second) {
			time.Sleep(pollInterval)
			continue
		}
		if scale.Weight == nil || *scale.Weight <= 0 {
			return nil
		}
		if lastQty > 0 && math.Abs(*scale.Weight-lastQty) > 0.005 {
			return nil
		}

		time.Sleep(pollInterval)
	}
}

func parseBridgeSnapshotTime(updated string) (time.Time, bool) {
	updated = strings.TrimSpace(updated)
	if updated == "" {
		return time.Time{}, false
	}
	ts, err := time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
}

func isFreshBridgeSnapshot(updated string, maxAge time.Duration) bool {
	ts, ok := parseBridgeSnapshotTime(updated)
	if !ok {
		return false
	}
	return isFreshBridgeTime(ts, maxAge)
}

func isFreshBridgeTime(ts time.Time, maxAge time.Duration) bool {
	age := time.Since(ts)
	if age < 0 {
		age = 0
	}
	return age <= maxAge
}

func normalizeBridgeUnit(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "kg"
	}
	return v
}

func almostEqualBridge(a, b, eps float64) bool {
	return math.Abs(a-b) <= eps
}

type erpClient struct {
	baseURL   string
	readURL   string
	apiKey    string
	apiSecret string
	http      *http.Client
}

type erpHealthResponse struct {
	OK bool `json:"ok"`
}

type erpGetUserResponse struct {
	Message string `json:"message"`
}

type erpListItemsResponse struct {
	Data []struct {
		Name     string `json:"name"`
		ItemCode string `json:"item_code"`
		ItemName string `json:"item_name"`
	} `json:"data"`
}

type erpListBinsResponse struct {
	Data []struct {
		Warehouse string  `json:"warehouse"`
		ActualQty float64 `json:"actual_qty"`
	} `json:"data"`
}

type erpItemDetailResponse struct {
	Data struct {
		Name     string `json:"name"`
		ItemCode string `json:"item_code"`
		ItemName string `json:"item_name"`
		StockUOM string `json:"stock_uom"`
	} `json:"data"`
}

type erpWarehouseDetailResponse struct {
	Data struct {
		Name    string `json:"name"`
		Company string `json:"company"`
	} `json:"data"`
}

type erpWarehouseListResponse struct {
	Data []struct {
		Name    string `json:"name"`
		Company string `json:"company"`
	} `json:"data"`
}

type erpCreateStockEntryResponse struct {
	Data struct {
		Name string `json:"name"`
	} `json:"data"`
}

type erpStockEntryResourceResponse struct {
	Data map[string]any `json:"data"`
}

type erpStockEntryMethodResponse struct {
	Message map[string]any `json:"message"`
}

func newERPClient(cfg Config) *erpClient {
	return &erpClient{
		baseURL:   strings.TrimRight(strings.TrimSpace(cfg.ERPURL), "/"),
		readURL:   strings.TrimRight(strings.TrimSpace(cfg.ERPReadURL), "/"),
		apiKey:    strings.TrimSpace(cfg.ERPAPIKey),
		apiSecret: strings.TrimSpace(cfg.ERPAPISecret),
		http:      &http.Client{Timeout: 12 * time.Second},
	}
}

func (c *erpClient) resolveReadURL(ctx context.Context) {
	if c == nil || strings.TrimSpace(c.readURL) != "" || strings.TrimSpace(c.baseURL) == "" {
		return
	}
	result, err := erpread.Resolve(ctx, c.http, c.baseURL, "")
	if err != nil {
		return
	}
	c.readURL = strings.TrimRight(strings.TrimSpace(result.BaseURL), "/")
}

func (c *erpClient) CheckConnection(ctx context.Context) (string, error) {
	c.resolveReadURL(ctx)
	if c.readURL != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.readURL+"/healthz", nil)
		if err != nil {
			return "", err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return "", fmt.Errorf("erp read http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		var payload erpHealthResponse
		if err := json.Unmarshal(body, &payload); err != nil {
			return "", fmt.Errorf("erp read json parse xato: %w", err)
		}
		if !payload.OK {
			return "", fmt.Errorf("erp read unhealthy")
		}
		return "ERP DB Reader", nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/method/frappe.auth.get_logged_user", nil)
	if err != nil {
		return "", err
	}
	c.setAuthHeader(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("erp http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload erpGetUserResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("erp json parse xato: %w", err)
	}
	if strings.TrimSpace(payload.Message) == "" {
		return "", fmt.Errorf("erp javob bo'sh")
	}
	return payload.Message, nil
}

func (c *erpClient) SearchItems(ctx context.Context, query string, limit int, warehouse string) ([]batchcontrol.Item, error) {
	warehouse = strings.TrimSpace(warehouse)

	c.resolveReadURL(ctx)
	if c.readURL != "" {
		q := url.Values{}
		if limit > 0 {
			q.Set("limit", strconv.Itoa(limit))
		}
		if warehouse != "" {
			q.Set("warehouse", warehouse)
		}
		if query = strings.TrimSpace(query); query != "" {
			q.Set("query", query)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.readURL+"/v1/items?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, erpCatalogResponseLimit))
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return nil, fmt.Errorf("erp read items http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		var payload erpListItemsResponse
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("erp read items json parse xato: %w", err)
		}
		return normalizeERPItems(payload), nil
	}

	q := url.Values{}
	q.Set("fields", `[`+"\"name\",\"item_code\",\"item_name\""+`]`)
	if limit <= 0 {
		limit = 10000
	}
	q.Set("limit_page_length", strconv.Itoa(limit))
	q.Set("order_by", "modified desc")
	if query = strings.TrimSpace(query); query != "" {
		pattern := "%" + query + "%"
		orFilters := [][]interface{}{
			{"Item", "item_code", "like", pattern},
			{"Item", "item_name", "like", pattern},
			{"Item", "name", "like", pattern},
		}
		b, _ := json.Marshal(orFilters)
		q.Set("or_filters", string(b))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/resource/Item?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	c.setAuthHeader(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("erp item http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload erpListItemsResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("erp item json parse xato: %w", err)
	}
	return normalizeERPItems(payload), nil
}

func (c *erpClient) SearchItemWarehouses(ctx context.Context, itemCode, query string, limit int) ([]batchcontrol.WarehouseStock, error) {
	itemCode = strings.TrimSpace(itemCode)
	if itemCode == "" {
		return nil, fmt.Errorf("item code bo'sh")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	c.resolveReadURL(ctx)
	if c.readURL != "" {
		q := url.Values{}
		q.Set("limit", strconv.Itoa(limit))
		if query = strings.TrimSpace(query); query != "" {
			q.Set("query", query)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.readURL+"/v1/items/"+url.PathEscape(itemCode)+"/warehouses?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return nil, fmt.Errorf("erp read warehouses http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		var payload erpListBinsResponse
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("erp read warehouses json parse xato: %w", err)
		}
		return normalizeERPWarehouseStocks(payload), nil
	}

	q := url.Values{}
	q.Set("fields", `[`+"\"warehouse\",\"actual_qty\""+`]`)
	q.Set("limit_page_length", strconv.Itoa(limit))
	q.Set("order_by", "actual_qty desc")
	filters := [][]interface{}{
		{"Bin", "item_code", "=", itemCode},
		{"Bin", "actual_qty", ">", 0},
	}
	fb, _ := json.Marshal(filters)
	q.Set("filters", string(fb))
	if query = strings.TrimSpace(query); query != "" {
		pattern := "%" + query + "%"
		orFilters := [][]interface{}{
			{"Bin", "warehouse", "like", pattern},
		}
		ob, _ := json.Marshal(orFilters)
		q.Set("or_filters", string(ob))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/resource/Bin?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	c.setAuthHeader(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("erp bin http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload erpListBinsResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("erp bin json parse xato: %w", err)
	}
	return normalizeERPWarehouseStocks(payload), nil
}

func (c *erpClient) SearchWarehouses(ctx context.Context, query string, limit int) ([]warehouseChoice, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	query = strings.TrimSpace(query)

	c.resolveReadURL(ctx)
	var readErr error
	if c.readURL != "" {
		q := url.Values{}
		q.Set("limit", strconv.Itoa(limit))
		if query != "" {
			q.Set("query", query)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.readURL+"/v1/warehouses?"+q.Encode(), nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			readErr = err
		} else {
			defer resp.Body.Close()
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
			if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
				var payload erpWarehouseListResponse
				if err := json.Unmarshal(body, &payload); err != nil {
					return nil, fmt.Errorf("erp read warehouse list json parse xato: %w", err)
				}
				return normalizeERPWarehouseChoices(payload), nil
			}
			readErr = fmt.Errorf("erp read warehouse list http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
	}
	if strings.TrimSpace(c.baseURL) == "" && readErr != nil {
		return nil, readErr
	}

	q := url.Values{}
	q.Set("fields", `[`+"\"name\",\"company\""+`]`)
	q.Set("limit_page_length", strconv.Itoa(limit))
	q.Set("order_by", "name asc")
	if query != "" {
		pattern := "%" + query + "%"
		orFilters := [][]interface{}{
			{"Warehouse", "name", "like", pattern},
			{"Warehouse", "company", "like", pattern},
		}
		ob, _ := json.Marshal(orFilters)
		q.Set("or_filters", string(ob))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/resource/Warehouse?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	c.setAuthHeader(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("erp warehouse list http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload erpWarehouseListResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("erp warehouse list json parse xato: %w", err)
	}
	return normalizeERPWarehouseChoices(payload), nil
}

func (c *erpClient) CreateMaterialReceiptDraft(ctx context.Context, in workflow.CreateMaterialReceiptDraftInput) (workflow.Draft, error) {
	in.ItemCode = strings.TrimSpace(in.ItemCode)
	in.Warehouse = strings.TrimSpace(in.Warehouse)
	in.Barcode = strings.ToUpper(strings.TrimSpace(in.Barcode))
	if in.ItemCode == "" {
		return workflow.Draft{}, fmt.Errorf("item code bo'sh")
	}
	if in.Warehouse == "" {
		return workflow.Draft{}, fmt.Errorf("warehouse bo'sh")
	}
	if in.Qty <= 0 {
		return workflow.Draft{}, fmt.Errorf("qty > 0 bo'lishi kerak")
	}

	company, err := c.lookupWarehouseCompany(ctx, in.Warehouse)
	if err != nil {
		return workflow.Draft{}, err
	}
	uom, err := c.lookupItemStockUOM(ctx, in.ItemCode)
	if err != nil {
		return workflow.Draft{}, err
	}
	if strings.TrimSpace(uom) == "" {
		uom = "Kg"
	}

	item := map[string]any{
		"item_code":         in.ItemCode,
		"t_warehouse":       in.Warehouse,
		"qty":               in.Qty,
		"uom":               uom,
		"stock_uom":         uom,
		"conversion_factor": 1,
	}
	if in.Barcode != "" {
		item["barcode"] = in.Barcode
	}

	payload := map[string]any{
		"stock_entry_type": "Material Receipt",
		"company":          company,
		"to_warehouse":     in.Warehouse,
		"items":            []map[string]any{item},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/resource/Stock%20Entry", bytes.NewReader(body))
	if err != nil {
		return workflow.Draft{}, err
	}
	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return workflow.Draft{}, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return workflow.Draft{}, fmt.Errorf("erp stock entry http %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	var out erpCreateStockEntryResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return workflow.Draft{}, fmt.Errorf("erp stock entry json parse xato: %w", err)
	}
	name := strings.TrimSpace(out.Data.Name)
	if name == "" {
		return workflow.Draft{}, fmt.Errorf("erp stock entry name bo'sh")
	}
	return workflow.Draft{
		Name:      name,
		ItemCode:  in.ItemCode,
		Warehouse: in.Warehouse,
		Qty:       in.Qty,
		UOM:       uom,
		Barcode:   in.Barcode,
	}, nil
}

func (c *erpClient) SubmitStockEntryDraft(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("stock entry name bo'sh")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/resource/Stock%20Entry/"+url.PathEscape(name), nil)
	if err != nil {
		return err
	}
	c.setAuthHeader(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("erp stock entry get http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var doc erpStockEntryResourceResponse
	if err := json.Unmarshal(body, &doc); err != nil {
		return fmt.Errorf("erp stock entry get json parse xato: %w", err)
	}
	if len(doc.Data) == 0 {
		return fmt.Errorf("erp stock entry doc bo'sh: %s", name)
	}
	payload := map[string]any{"doc": doc.Data}
	body, _ = json.Marshal(payload)
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/method/frappe.client.submit", bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err = c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ = io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("erp stock entry submit http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out erpStockEntryMethodResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("erp stock entry submit json parse xato: %w", err)
	}
	if got := strings.TrimSpace(fmt.Sprint(out.Message["name"])); got != "" && got != name {
		return fmt.Errorf("erp stock entry submit name mismatch: %s", got)
	}
	return nil
}

func (c *erpClient) DeleteStockEntryDraft(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("stock entry name bo'sh")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/api/resource/Stock%20Entry/"+url.PathEscape(name), nil)
	if err != nil {
		return err
	}
	c.setAuthHeader(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("erp stock entry delete http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (c *erpClient) lookupWarehouseCompany(ctx context.Context, warehouse string) (string, error) {
	c.resolveReadURL(ctx)
	if c.readURL != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.readURL+"/v1/warehouses/"+url.PathEscape(strings.TrimSpace(warehouse)), nil)
		if err != nil {
			return "", err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return "", fmt.Errorf("erp read warehouse http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		var payload erpWarehouseDetailResponse
		if err := json.Unmarshal(body, &payload); err != nil {
			return "", fmt.Errorf("erp read warehouse json parse xato: %w", err)
		}
		if strings.TrimSpace(payload.Data.Company) == "" {
			return "", fmt.Errorf("warehouse company topilmadi: %s", warehouse)
		}
		return strings.TrimSpace(payload.Data.Company), nil
	}

	q := url.Values{}
	q.Set("fields", `[`+"\"name\",\"company\""+`]`)
	filters := [][]interface{}{{"Warehouse", "name", "=", warehouse}}
	fb, _ := json.Marshal(filters)
	q.Set("filters", string(fb))
	q.Set("limit_page_length", "1")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/resource/Warehouse?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	c.setAuthHeader(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("erp warehouse http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload erpWarehouseDetailResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("erp warehouse json parse xato: %w", err)
	}
	if strings.TrimSpace(payload.Data.Name) == "" && strings.TrimSpace(payload.Data.Company) == "" {
		return "", fmt.Errorf("warehouse company topilmadi: %s", warehouse)
	}
	return strings.TrimSpace(payload.Data.Company), nil
}

func (c *erpClient) lookupItemStockUOM(ctx context.Context, itemCode string) (string, error) {
	c.resolveReadURL(ctx)
	if c.readURL != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.readURL+"/v1/items/"+url.PathEscape(strings.TrimSpace(itemCode)), nil)
		if err != nil {
			return "", err
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
		if resp.StatusCode < 200 || resp.StatusCode > 299 {
			return "", fmt.Errorf("erp read item http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}
		var payload erpItemDetailResponse
		if err := json.Unmarshal(body, &payload); err != nil {
			return "", fmt.Errorf("erp read item json parse xato: %w", err)
		}
		if strings.TrimSpace(payload.Data.ItemCode) == "" && strings.TrimSpace(payload.Data.Name) == "" {
			return "", fmt.Errorf("item topilmadi: %s", itemCode)
		}
		return strings.TrimSpace(payload.Data.StockUOM), nil
	}

	q := url.Values{}
	q.Set("fields", `[`+"\"name\",\"stock_uom\""+`]`)
	filters := [][]interface{}{{"Item", "item_code", "=", itemCode}}
	fb, _ := json.Marshal(filters)
	q.Set("filters", string(fb))
	q.Set("limit_page_length", "1")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/resource/Item?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	c.setAuthHeader(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("erp item uom http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload erpItemDetailResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("erp item uom json parse xato: %w", err)
	}
	if strings.TrimSpace(payload.Data.ItemCode) == "" && strings.TrimSpace(payload.Data.Name) == "" {
		return "", fmt.Errorf("item topilmadi: %s", itemCode)
	}
	return strings.TrimSpace(payload.Data.StockUOM), nil
}

func (c *erpClient) setAuthHeader(req *http.Request) {
	req.Header.Set("Authorization", fmt.Sprintf("token %s:%s", c.apiKey, c.apiSecret))
}

func normalizeERPItems(payload erpListItemsResponse) []batchcontrol.Item {
	items := make([]batchcontrol.Item, 0, len(payload.Data))
	for _, r := range payload.Data {
		code := strings.TrimSpace(r.ItemCode)
		if code == "" {
			code = strings.TrimSpace(r.Name)
		}
		name := strings.TrimSpace(r.ItemName)
		if name == "" {
			name = code
		}
		if code == "" {
			continue
		}
		items = append(items, batchcontrol.Item{
			Name:     strings.TrimSpace(r.Name),
			ItemCode: code,
			ItemName: name,
		})
	}
	return items
}

func normalizeERPWarehouseStocks(payload erpListBinsResponse) []batchcontrol.WarehouseStock {
	stocks := make([]batchcontrol.WarehouseStock, 0, len(payload.Data))
	for _, r := range payload.Data {
		warehouse := strings.TrimSpace(r.Warehouse)
		if warehouse == "" {
			continue
		}
		stocks = append(stocks, batchcontrol.WarehouseStock{
			Warehouse: warehouse,
			ActualQty: r.ActualQty,
		})
	}
	return stocks
}

func normalizeERPWarehouseChoices(payload erpWarehouseListResponse) []warehouseChoice {
	choices := make([]warehouseChoice, 0, len(payload.Data))
	for _, r := range payload.Data {
		name := strings.TrimSpace(r.Name)
		if name == "" {
			continue
		}
		choices = append(choices, warehouseChoice{
			Warehouse: name,
			Company:   strings.TrimSpace(r.Company),
		})
	}
	return choices
}

type warehouseChoice struct {
	Warehouse string `json:"warehouse"`
	Company   string `json:"company"`
}

func IsDuplicateBarcodeError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if msg == "" {
		return false
	}
	if strings.Contains(msg, "barcode") && strings.Contains(msg, "duplicate") {
		return true
	}
	if strings.Contains(msg, "barcode") && strings.Contains(msg, "already exists") {
		return true
	}
	if strings.Contains(msg, "barcode") && strings.Contains(msg, "unique") {
		return true
	}
	if strings.Contains(msg, "duplicate entry") && strings.Contains(msg, "barcode") {
		return true
	}
	return false
}
