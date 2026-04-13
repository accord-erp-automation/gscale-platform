package erpread

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const ServiceName = "gscale_erp_read"

type Result struct {
	BaseURL string
	Service string
}

type handshakeResponse struct {
	OK      bool   `json:"ok"`
	Service string `json:"service"`
}

type healthResponse struct {
	OK bool `json:"ok"`
}

func Resolve(ctx context.Context, httpClient *http.Client, erpURL, explicitReadURL string) (Result, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 3 * time.Second}
	}
	candidates, err := candidatesFor(erpURL, explicitReadURL)
	if err != nil {
		return Result{}, err
	}
	var failures []string
	for _, candidate := range candidates {
		result, err := probe(ctx, httpClient, candidate)
		if err == nil {
			return result, nil
		}
		failures = append(failures, fmt.Sprintf("%s: %v", candidate, err))
	}
	if len(failures) == 0 {
		return Result{}, fmt.Errorf("erp read service topilmadi")
	}
	return Result{}, fmt.Errorf("erp read service topilmadi (%s)", strings.Join(failures, "; "))
}

func candidatesFor(erpURL, explicitReadURL string) ([]string, error) {
	out := make([]string, 0, 4)
	seen := make(map[string]struct{})
	add := func(raw string) {
		raw = strings.TrimRight(strings.TrimSpace(raw), "/")
		if raw == "" {
			return
		}
		if _, ok := seen[raw]; ok {
			return
		}
		seen[raw] = struct{}{}
		out = append(out, raw)
	}
	add(explicitReadURL)
	erpURL = strings.TrimSpace(erpURL)
	if erpURL == "" {
		if len(out) == 0 {
			return nil, fmt.Errorf("erp url bo'sh")
		}
		return out, nil
	}
	parsed, err := url.Parse(erpURL)
	if err != nil {
		return nil, fmt.Errorf("erp url parse xato: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("erp url to'liq emas: %s", erpURL)
	}
	origin := parsed.Scheme + "://" + parsed.Host
	add(origin + "/gscale-read")
	add(origin + "/gscale_erp_read")
	host := parsed.Hostname()
	port := parsed.Port()
	if host != "" && (port == "" || port == "8000" || port == "80" || port == "443") {
		add(parsed.Scheme + "://" + net.JoinHostPort(host, "8090"))
		if parsed.Scheme == "https" && isLocalHost(host) {
			add("http://" + net.JoinHostPort(host, "8090"))
		}
	}
	return out, nil
}

func probe(ctx context.Context, httpClient *http.Client, baseURL string) (Result, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return Result{}, fmt.Errorf("base url bo'sh")
	}
	if result, err := probeHandshake(ctx, httpClient, baseURL); err == nil {
		return result, nil
	}
	if err := probeHealth(ctx, httpClient, baseURL); err != nil {
		return Result{}, err
	}
	return Result{BaseURL: baseURL, Service: ServiceName}, nil
}

func probeHandshake(ctx context.Context, httpClient *http.Client, baseURL string) (Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/handshake", nil)
	if err != nil {
		return Result{}, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Result{}, fmt.Errorf("handshake http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload handshakeResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return Result{}, fmt.Errorf("handshake json parse xato: %w", err)
	}
	service := strings.TrimSpace(payload.Service)
	if !payload.OK || service == "" {
		return Result{}, fmt.Errorf("handshake noto'g'ri javob")
	}
	if service != ServiceName {
		return Result{}, fmt.Errorf("boshqa service: %s", service)
	}
	return Result{BaseURL: baseURL, Service: service}, nil
}

func probeHealth(ctx context.Context, httpClient *http.Client, baseURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("health http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload healthResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("health json parse xato: %w", err)
	}
	if !payload.OK {
		return fmt.Errorf("health false")
	}
	return nil
}

func isLocalHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	switch host {
	case "localhost", "127.0.0.1", "10.0.2.2":
		return true
	}
	return strings.HasPrefix(host, "192.168.") || strings.HasPrefix(host, "10.")
}
