package mobileapi

import "strings"

func normalizePrinter(printer string) string {
	switch strings.ToLower(strings.TrimSpace(printer)) {
	case "zebra", "zpl", "rfid":
		return "zebra"
	case "godex", "go-dex", "g500":
		return "godex"
	default:
		return ""
	}
}
