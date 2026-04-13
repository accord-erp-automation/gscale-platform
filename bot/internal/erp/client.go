package erp

import (
	"context"
	"core/erpread"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	baseURL   string
	readURL   string
	apiKey    string
	apiSecret string
	http      *http.Client
}

type getUserResponse struct {
	Message string `json:"message"`
}

type healthResponse struct {
	OK bool `json:"ok"`
}

type Item struct {
	Name     string
	ItemCode string
	ItemName string
}

type WarehouseStock struct {
	Warehouse string
	ActualQty float64
}

type listItemsResponse struct {
	Data []struct {
		Name     string `json:"name"`
		ItemCode string `json:"item_code"`
		ItemName string `json:"item_name"`
	} `json:"data"`
}

type listBinsResponse struct {
	Data []struct {
		Warehouse string  `json:"warehouse"`
		ActualQty float64 `json:"actual_qty"`
	} `json:"data"`
}

type itemDetailResponse struct {
	Data struct {
		Name     string `json:"name"`
		ItemCode string `json:"item_code"`
		ItemName string `json:"item_name"`
		StockUOM string `json:"stock_uom"`
	} `json:"data"`
}

type warehouseDetailResponse struct {
	Data struct {
		Name    string `json:"name"`
		Company string `json:"company"`
	} `json:"data"`
}

func New(baseURL, apiKey, apiSecret string) *Client {
	return NewWithReadURL(baseURL, apiKey, apiSecret, "")
}

func NewWithReadURL(baseURL, apiKey, apiSecret, readURL string) *Client {
	return &Client{
		baseURL:   strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		readURL:   strings.TrimRight(strings.TrimSpace(readURL), "/"),
		apiKey:    strings.TrimSpace(apiKey),
		apiSecret: strings.TrimSpace(apiSecret),
		http:      &http.Client{Timeout: 12 * time.Second},
	}
}

func (c *Client) resolveReadURL(ctx context.Context) {
	if c == nil || strings.TrimSpace(c.readURL) != "" || strings.TrimSpace(c.baseURL) == "" {
		return
	}
	result, err := erpread.Resolve(ctx, c.http, c.baseURL, "")
	if err != nil {
		return
	}
	c.readURL = strings.TrimRight(strings.TrimSpace(result.BaseURL), "/")
}

func (c *Client) CheckConnection(ctx context.Context) (string, error) {
	c.resolveReadURL(ctx)
	if c.readURL != "" {
		endpoint := c.readURL + "/healthz"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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

		var payload healthResponse
		if err := json.Unmarshal(body, &payload); err != nil {
			return "", fmt.Errorf("erp read json parse xato: %w", err)
		}
		if !payload.OK {
			return "", fmt.Errorf("erp read unhealthy")
		}
		return "ERP DB Reader", nil
	}

	endpoint := c.baseURL + "/api/method/frappe.auth.get_logged_user"
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

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return "", fmt.Errorf("erp http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload getUserResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("erp json parse xato: %w", err)
	}
	if strings.TrimSpace(payload.Message) == "" {
		return "", fmt.Errorf("erp javob bo'sh")
	}
	return payload.Message, nil
}

func (c *Client) SearchItems(ctx context.Context, query string, limit int) ([]Item, error) {
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
		query = strings.TrimSpace(query)
		if query != "" {
			q.Set("query", query)
		}

		endpoint := c.readURL + "/v1/items?" + q.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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
			return nil, fmt.Errorf("erp read items http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}

		var payload listItemsResponse
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("erp read items json parse xato: %w", err)
		}
		return normalizeItems(payload), nil
	}

	q := url.Values{}
	q.Set("fields", `[`+"\"name\",\"item_code\",\"item_name\""+`]`)
	q.Set("limit_page_length", strconv.Itoa(limit))
	q.Set("order_by", "modified desc")

	query = strings.TrimSpace(query)
	if query != "" {
		pattern := "%" + query + "%"
		orFilters := [][]interface{}{
			{"Item", "item_code", "like", pattern},
			{"Item", "item_name", "like", pattern},
			{"Item", "name", "like", pattern},
		}
		b, _ := json.Marshal(orFilters)
		q.Set("or_filters", string(b))
	}

	endpoint := c.baseURL + "/api/resource/Item?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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

	var payload listItemsResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("erp item json parse xato: %w", err)
	}

	return normalizeItems(payload), nil
}

func (c *Client) SearchItemWarehouses(ctx context.Context, itemCode, query string, limit int) ([]WarehouseStock, error) {
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
		query = strings.TrimSpace(query)
		if query != "" {
			q.Set("query", query)
		}

		endpoint := c.readURL + "/v1/items/" + url.PathEscape(itemCode) + "/warehouses?" + q.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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

		var payload listBinsResponse
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, fmt.Errorf("erp read warehouses json parse xato: %w", err)
		}
		return normalizeWarehouseStocks(payload), nil
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

	query = strings.TrimSpace(query)
	if query != "" {
		pattern := "%" + query + "%"
		orFilters := [][]interface{}{
			{"Bin", "warehouse", "like", pattern},
		}
		ob, _ := json.Marshal(orFilters)
		q.Set("or_filters", string(ob))
	}

	endpoint := c.baseURL + "/api/resource/Bin?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
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

	var payload listBinsResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("erp bin json parse xato: %w", err)
	}

	return normalizeWarehouseStocks(payload), nil
}

func (c *Client) setAuthHeader(req *http.Request) {
	req.Header.Set("Authorization", fmt.Sprintf("token %s:%s", c.apiKey, c.apiSecret))
}

func normalizeItems(payload listItemsResponse) []Item {
	items := make([]Item, 0, len(payload.Data))
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
		items = append(items, Item{
			Name:     strings.TrimSpace(r.Name),
			ItemCode: code,
			ItemName: name,
		})
	}
	return items
}

func normalizeWarehouseStocks(payload listBinsResponse) []WarehouseStock {
	stocks := make([]WarehouseStock, 0, len(payload.Data))
	for _, r := range payload.Data {
		wh := strings.TrimSpace(r.Warehouse)
		if wh == "" {
			continue
		}
		stocks = append(stocks, WarehouseStock{
			Warehouse: wh,
			ActualQty: r.ActualQty,
		})
	}
	return stocks
}
