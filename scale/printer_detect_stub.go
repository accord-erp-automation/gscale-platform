//go:build !linux

package main

import (
	bridgestate "bridge/state"
	"time"
)

func detectPrinterSnapshot() (bridgestate.PrinterSnapshot, error) {
	return bridgestate.PrinterSnapshot{
		Connected: false,
		Label:     "ulanmagan",
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}
