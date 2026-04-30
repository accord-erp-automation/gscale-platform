package main

import "strings"

const (
	printBackendZebra = "zebra"
	printBackendGoDEX = "godex"
)

func normalizePrintBackend(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", printBackendZebra, "zpl", "rfid":
		return printBackendZebra
	case printBackendGoDEX, "go-dex", "g500":
		return printBackendGoDEX
	default:
		return printBackendZebra
	}
}

func normalizePrintRequestPrinter(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case printBackendZebra, "zpl", "rfid":
		return printBackendZebra
	case printBackendGoDEX, "go-dex", "g500":
		return printBackendGoDEX
	default:
		return ""
	}
}

func resolvePrintBackend(requestPrinter, defaultBackend string) string {
	if p := normalizePrintRequestPrinter(requestPrinter); p != "" {
		return p
	}
	return normalizePrintBackend(defaultBackend)
}
