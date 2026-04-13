package erpread

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveUsesHandshake(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/handshake" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"service":"gscale_erp_read"}`))
	}))
	defer ts.Close()

	result, err := Resolve(context.Background(), ts.Client(), "", ts.URL)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if result.BaseURL != ts.URL {
		t.Fatalf("BaseURL = %q", result.BaseURL)
	}
	if result.Service != ServiceName {
		t.Fatalf("Service = %q", result.Service)
	}
}

func TestResolveFallsBackToHealthz(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/handshake":
			http.NotFound(w, r)
		case "/healthz":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer ts.Close()

	result, err := Resolve(context.Background(), ts.Client(), "", ts.URL)
	if err != nil {
		t.Fatalf("Resolve error: %v", err)
	}
	if result.BaseURL != ts.URL {
		t.Fatalf("BaseURL = %q", result.BaseURL)
	}
}

func TestCandidatesForERPURLIncludesSameHostPort8090(t *testing.T) {
	t.Parallel()

	candidates, err := candidatesFor("http://erp.example.com:8000", "")
	if err != nil {
		t.Fatalf("candidatesFor error: %v", err)
	}
	want := "http://erp.example.com:8090"
	found := false
	for _, candidate := range candidates {
		if candidate == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("candidates = %#v, want %q", candidates, want)
	}
}
