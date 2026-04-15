package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/tarm/serial"
)

func startSerialReader(ctx context.Context, device string, baud int, unit string, out chan<- Reading) error {
	lg := workerLog("worker.serial")
	lg.Printf("start: device=%s baud=%d unit=%s", strings.TrimSpace(device), baud, strings.TrimSpace(unit))
	go func() {
		waiting := false
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			port, err := serial.OpenPort(&serial.Config{Name: device, Baud: baud, ReadTimeout: 250 * time.Millisecond})
			if err != nil {
				if !waiting {
					lg.Printf("wait: serial device=%s baud=%d", device, baud)
					waiting = true
				}
				push(out, Reading{
					Source:    "serial",
					Port:      device,
					Baud:      baud,
					Unit:      unit,
					Error:     fmt.Sprintf("open error: %v", err),
					UpdatedAt: time.Now(),
				})
				if !sleepWithContext(ctx, 900*time.Millisecond) {
					return
				}
				continue
			}

			if waiting {
				lg.Printf("ready: serial device=%s baud=%d", device, baud)
				waiting = false
			}
			lg.Printf("port opened: device=%s baud=%d", device, baud)
			push(out, Reading{
				Source:    "serial",
				Port:      device,
				Baud:      baud,
				Unit:      unit,
				UpdatedAt: time.Now(),
			})

			err = streamSerial(ctx, port, device, baud, unit, out)
			_ = port.Close()
			lg.Printf("port closed: device=%s err=%v", device, err)

			if ctx.Err() != nil {
				return
			}

			if err != nil {
				lg.Printf("stream read error: %v", err)
				push(out, Reading{
					Source:    "serial",
					Port:      device,
					Baud:      baud,
					Unit:      unit,
					Error:     fmt.Sprintf("read error: %v", err),
					UpdatedAt: time.Now(),
				})
			}

			if !sleepWithContext(ctx, 400*time.Millisecond) {
				return
			}
		}
	}()

	return nil
}

func streamSerial(ctx context.Context, port *serial.Port, device string, baud int, unit string, out chan<- Reading) error {
	lg := workerLog("worker.serial")
	buf := make([]byte, 256)
	pending := ""
	lastUnit := strings.ToLower(strings.TrimSpace(unit))
	if lastUnit == "" {
		lastUnit = "kg"
	}
	seenParsedValue := false

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		n, err := port.Read(buf)
		if err != nil {
			return err
		}
		if n == 0 {
			continue
		}

		chunk := string(buf[:n])
		pending = appendRaw(pending, chunk, 1024)

		for {
			frame, rest, ok := popSerialFrame(pending)
			if !ok {
				break
			}
			pending = rest

			trimmed := strings.TrimSpace(frame)
			if trimmed == "" {
				if !seenParsedValue {
					continue
				}
				zero := 0.0
				lg.Printf("frame empty -> weight=0")
				push(out, Reading{
					Source:    "serial",
					Port:      device,
					Baud:      baud,
					Weight:    &zero,
					Unit:      lastUnit,
					Raw:       "<empty-frame>",
					UpdatedAt: time.Now(),
				})
				continue
			}

			weight, parsedUnit, stable, ok := parseWeight(trimmed, unit)
			if !ok {
				// Keep stream alive even when a frame cannot be parsed.
				lg.Printf("frame parse miss: raw=%q", trimmed)
				push(out, Reading{
					Source:    "serial",
					Port:      device,
					Baud:      baud,
					Unit:      lastUnit,
					Raw:       trimmed,
					UpdatedAt: time.Now(),
				})
				continue
			}

			w := weight
			if strings.TrimSpace(parsedUnit) != "" {
				lastUnit = parsedUnit
			}
			seenParsedValue = true
			stableText := "unknown"
			if stable != nil {
				if *stable {
					stableText = "true"
				} else {
					stableText = "false"
				}
			}
			lg.Printf("frame parsed: weight=%.3f unit=%s stable=%s raw=%q", w, lastUnit, stableText, trimmed)
			push(out, Reading{
				Source:    "serial",
				Port:      device,
				Baud:      baud,
				Weight:    &w,
				Unit:      lastUnit,
				Stable:    stable,
				Raw:       trimmed,
				UpdatedAt: time.Now(),
			})
		}
	}
}

func popSerialFrame(buf string) (frame, rest string, ok bool) {
	idx := strings.IndexAny(buf, "\r\n")
	if idx < 0 {
		return "", buf, false
	}

	frame = buf[:idx]
	j := idx
	for j < len(buf) {
		if buf[j] != '\r' && buf[j] != '\n' {
			break
		}
		j++
	}
	rest = buf[j:]
	return frame, rest, true
}

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
