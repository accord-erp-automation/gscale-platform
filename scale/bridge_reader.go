package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func startBridgeReader(ctx context.Context, url string, interval time.Duration, out chan<- Reading) {
	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}

	lg := workerLog("worker.bridge")
	lg.Printf("start: url=%s interval=%s", strings.TrimSpace(url), interval)
	client := &http.Client{Timeout: 2 * time.Second}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			resp, err := client.Get(url)
			if err != nil {
				lg.Printf("request error: %v", err)
				push(out, Reading{
					Source:    "bridge",
					Error:     err.Error(),
					UpdatedAt: time.Now(),
				})
				continue
			}

			body, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr != nil {
				lg.Printf("read body error: %v", readErr)
				push(out, Reading{
					Source:    "bridge",
					Error:     readErr.Error(),
					UpdatedAt: time.Now(),
				})
				continue
			}

			if resp.StatusCode < 200 || resp.StatusCode > 299 {
				lg.Printf("http error: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
				push(out, Reading{
					Source:    "bridge",
					Error:     fmt.Sprintf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body))),
					UpdatedAt: time.Now(),
				})
				continue
			}

			var payload scaleAPIResponse
			if err := json.Unmarshal(body, &payload); err != nil {
				lg.Printf("json decode error: %v body=%s", err, strings.TrimSpace(string(body)))
				push(out, Reading{
					Source:    "bridge",
					Error:     err.Error(),
					UpdatedAt: time.Now(),
				})
				continue
			}

			push(out, Reading{
				Source:    "bridge",
				Port:      payload.Port,
				Weight:    payload.Weight,
				Unit:      payload.Unit,
				Stable:    payload.Stable,
				Raw:       payload.Raw,
				Error:     payload.Error,
				UpdatedAt: time.Now(),
			})
			stableText := "unknown"
			if payload.Stable != nil {
				if *payload.Stable {
					stableText = "true"
				} else {
					stableText = "false"
				}
			}
			weightText := "-"
			if payload.Weight != nil {
				weightText = fmt.Sprintf("%.3f", *payload.Weight)
			}
			unitText := strings.TrimSpace(payload.Unit)
			if unitText == "" {
				unitText = "kg"
			}
			lg.Printf("bridge reading: weight=%s %s stable=%s err=%s raw=%q", weightText, unitText, stableText, strings.TrimSpace(payload.Error), strings.TrimSpace(payload.Raw))
		}
	}()
}
