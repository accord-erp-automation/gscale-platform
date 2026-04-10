package erp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type MaterialReceiptDraftInput struct {
	ItemCode  string
	Warehouse string
	Qty       float64
	Barcode   string
}

type StockEntryDraft struct {
	Name      string
	ItemCode  string
	Warehouse string
	Qty       float64
	UOM       string
	Barcode   string
}

type warehouseLookupResponse struct {
	Data []struct {
		Name    string `json:"name"`
		Company string `json:"company"`
	} `json:"data"`
}

type itemUOMLookupResponse struct {
	Data []struct {
		Name     string `json:"name"`
		StockUOM string `json:"stock_uom"`
	} `json:"data"`
}

type createStockEntryResponse struct {
	Data struct {
		Name string `json:"name"`
	} `json:"data"`
}

type stockEntryResourceResponse struct {
	Data map[string]any `json:"data"`
}

type stockEntryMethodResponse struct {
	Message map[string]any `json:"message"`
}

func (c *Client) CreateMaterialReceiptDraft(ctx context.Context, in MaterialReceiptDraftInput) (StockEntryDraft, error) {
	in.ItemCode = strings.TrimSpace(in.ItemCode)
	in.Warehouse = strings.TrimSpace(in.Warehouse)
	in.Barcode = strings.ToUpper(strings.TrimSpace(in.Barcode))
	if in.ItemCode == "" {
		return StockEntryDraft{}, fmt.Errorf("item code bo'sh")
	}
	if in.Warehouse == "" {
		return StockEntryDraft{}, fmt.Errorf("warehouse bo'sh")
	}
	if in.Qty <= 0 {
		return StockEntryDraft{}, fmt.Errorf("qty > 0 bo'lishi kerak")
	}

	company, err := c.lookupWarehouseCompany(ctx, in.Warehouse)
	if err != nil {
		return StockEntryDraft{}, err
	}

	uom, err := c.lookupItemStockUOM(ctx, in.ItemCode)
	if err != nil {
		return StockEntryDraft{}, err
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
		return StockEntryDraft{}, err
	}
	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return StockEntryDraft{}, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return StockEntryDraft{}, fmt.Errorf("erp stock entry http %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var out createStockEntryResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return StockEntryDraft{}, fmt.Errorf("erp stock entry json parse xato: %w", err)
	}
	name := strings.TrimSpace(out.Data.Name)
	if name == "" {
		return StockEntryDraft{}, fmt.Errorf("erp stock entry name bo'sh")
	}

	return StockEntryDraft{
		Name:      name,
		ItemCode:  in.ItemCode,
		Warehouse: in.Warehouse,
		Qty:       in.Qty,
		UOM:       uom,
		Barcode:   in.Barcode,
	}, nil
}

func (c *Client) SubmitStockEntryDraft(ctx context.Context, name string) error {
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

	var doc stockEntryResourceResponse
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

	var out stockEntryMethodResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("erp stock entry submit json parse xato: %w", err)
	}
	if got := strings.TrimSpace(fmt.Sprint(out.Message["name"])); got != "" && got != name {
		return fmt.Errorf("erp stock entry submit name mismatch: %s", got)
	}
	return nil
}

func (c *Client) DeleteStockEntryDraft(ctx context.Context, name string) error {
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

func (c *Client) lookupWarehouseCompany(ctx context.Context, warehouse string) (string, error) {
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

		var payload warehouseDetailResponse
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

	endpoint := c.baseURL + "/api/resource/Warehouse?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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

	var payload warehouseLookupResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("erp warehouse json parse xato: %w", err)
	}
	if len(payload.Data) == 0 || strings.TrimSpace(payload.Data[0].Company) == "" {
		return "", fmt.Errorf("warehouse company topilmadi: %s", warehouse)
	}
	return strings.TrimSpace(payload.Data[0].Company), nil
}

func (c *Client) lookupItemStockUOM(ctx context.Context, itemCode string) (string, error) {
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

		var payload itemDetailResponse
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

	endpoint := c.baseURL + "/api/resource/Item?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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

	var payload itemUOMLookupResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("erp item uom json parse xato: %w", err)
	}
	if len(payload.Data) == 0 {
		return "", fmt.Errorf("item topilmadi: %s", itemCode)
	}
	return strings.TrimSpace(payload.Data[0].StockUOM), nil
}
