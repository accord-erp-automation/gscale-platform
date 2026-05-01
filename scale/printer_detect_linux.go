//go:build linux

package main

import (
	bridgestate "bridge/state"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"godex"
)

type detectedPrinter struct {
	kind       string
	devicePath string
}

func detectPrinterSnapshot() (bridgestate.PrinterSnapshot, error) {
	printers, err := scanConnectedPrinters()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err != nil {
		return bridgestate.PrinterSnapshot{
			Connected: false,
			Label:     "ulanmagan",
			Error:     err.Error(),
			UpdatedAt: now,
		}, err
	}

	if len(printers) == 0 {
		return bridgestate.PrinterSnapshot{
			Connected: false,
			Label:     "ulanmagan",
			UpdatedAt: now,
		}, nil
	}

	kinds := make([]string, 0, 2)
	devicePaths := make([]string, 0, len(printers))
	seenKind := map[string]struct{}{}
	for _, p := range printers {
		if p.kind == "" {
			continue
		}
		if _, ok := seenKind[p.kind]; !ok {
			seenKind[p.kind] = struct{}{}
			kinds = append(kinds, p.kind)
		}
		if strings.TrimSpace(p.devicePath) != "" {
			devicePaths = append(devicePaths, p.devicePath)
		}
	}
	sortPrinterKinds(kinds)
	sort.Strings(devicePaths)

	if len(kinds) == 0 {
		return bridgestate.PrinterSnapshot{
			Connected: false,
			Label:     "ulanmagan",
			UpdatedAt: now,
		}, nil
	}

	label := kinds[0]
	if len(kinds) > 1 {
		label = strings.Join(kinds, " + ")
	}
	label += ": ulangan"

	kind := kinds[0]
	if len(kinds) > 1 {
		kind = "mixed"
	}

	return bridgestate.PrinterSnapshot{
		Connected:   true,
		Kind:        kind,
		Label:       label,
		DevicePaths: devicePaths,
		UpdatedAt:   now,
	}, nil
}

func scanConnectedPrinters() ([]detectedPrinter, error) {
	vendorFiles, err := filepath.Glob("/sys/bus/usb/devices/*/idVendor")
	if err != nil {
		return nil, err
	}

	printers := make([]detectedPrinter, 0, len(vendorFiles))
	seen := map[string]struct{}{}
	for _, vendorFile := range vendorFiles {
		parent := filepath.Dir(vendorFile)
		vendor := strings.ToLower(readTrimFile(vendorFile))
		product := strings.ToLower(readTrimFile(filepath.Join(parent, "idProduct")))
		if vendor == "" || product == "" {
			continue
		}

		manufacturer := readTrimFile(filepath.Join(parent, "manufacturer"))
		productName := readTrimFile(filepath.Join(parent, "product"))
		serial := readTrimFile(filepath.Join(parent, "serial"))
		bus := readTrimFile(filepath.Join(parent, "busnum"))
		dev := readTrimFile(filepath.Join(parent, "devnum"))
		kind := classifyPrinterKind(vendor, product, manufacturer, productName)
		if kind == "" {
			continue
		}

		devicePath := fmt.Sprintf("usb:%s:%s", vendor, product)
		if bus != "" && dev != "" {
			devicePath = fmt.Sprintf("usb:%s:%s", bus, dev)
		}
		key := strings.Join([]string{kind, vendor, product, serial, bus, dev}, "|")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		printers = append(printers, detectedPrinter{kind: kind, devicePath: devicePath})
	}

	sort.Slice(printers, func(i, j int) bool {
		if printerKindOrder(printers[i].kind) == printerKindOrder(printers[j].kind) {
			return printers[i].devicePath < printers[j].devicePath
		}
		return printerKindOrder(printers[i].kind) < printerKindOrder(printers[j].kind)
	})
	return printers, nil
}

func classifyPrinterKind(vendor, product, manufacturer, productName string) string {
	text := strings.ToLower(strings.TrimSpace(manufacturer + " " + productName))
	switch {
	case vendor == fmt.Sprintf("%04x", godex.VendorID) && product == fmt.Sprintf("%04x", godex.ProductID):
		return "godex"
	case strings.Contains(text, "godex") || strings.Contains(text, "g500"):
		return "godex"
	case vendor == "0a5f" || strings.Contains(text, "zebra") || strings.Contains(text, "ztc"):
		return "zebra"
	default:
		return ""
	}
}

func sortPrinterKinds(kinds []string) {
	sort.SliceStable(kinds, func(i, j int) bool {
		oi := printerKindOrder(kinds[i])
		oj := printerKindOrder(kinds[j])
		if oi == oj {
			return kinds[i] < kinds[j]
		}
		return oi < oj
	})
}

func printerKindOrder(kind string) int {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "zebra":
		return 0
	case "godex":
		return 1
	default:
		return 2
	}
}
