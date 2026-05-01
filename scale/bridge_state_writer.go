package main

import (
	bridgestate "bridge/state"
	"strings"
	"time"
)

func writeBridgeStateSnapshot(store *bridgestate.Store, rd Reading, zebra ZebraStatus, printer bridgestate.PrinterSnapshot) error {
	if store == nil {
		return nil
	}

	scaleTS := rd.UpdatedAt
	if scaleTS.IsZero() {
		scaleTS = time.Now()
	}
	zebraTS := zebra.UpdatedAt
	if zebraTS.IsZero() {
		zebraTS = scaleTS
	}

	scaleSnap := bridgestate.ScaleSnapshot{
		Source:    strings.TrimSpace(rd.Source),
		Port:      strings.TrimSpace(rd.Port),
		Weight:    rd.Weight,
		Unit:      strings.TrimSpace(rd.Unit),
		Stable:    rd.Stable,
		Error:     strings.TrimSpace(rd.Error),
		UpdatedAt: scaleTS.UTC().Format(time.RFC3339Nano),
	}
	if scaleSnap.Unit == "" {
		scaleSnap.Unit = "kg"
	}

	zebraSnap := bridgestate.ZebraSnapshot{
		Connected:   zebra.Connected,
		DevicePath:  strings.TrimSpace(zebra.DevicePath),
		Name:        strings.TrimSpace(zebra.Name),
		DeviceState: strings.TrimSpace(zebra.DeviceState),
		MediaState:  strings.TrimSpace(zebra.MediaState),
		ReadLine1:   strings.TrimSpace(zebra.ReadLine1),
		ReadLine2:   strings.TrimSpace(zebra.ReadLine2),
		LastEPC:     strings.ToUpper(strings.TrimSpace(zebra.LastEPC)),
		Verify:      strings.ToUpper(strings.TrimSpace(zebra.Verify)),
		Action:      strings.TrimSpace(zebra.Action),
		Error:       strings.TrimSpace(zebra.Error),
		UpdatedAt:   zebraTS.UTC().Format(time.RFC3339Nano),
	}

	return store.Update(func(s *bridgestate.Snapshot) {
		s.Scale = scaleSnap
		s.Zebra = zebraSnap
		s.Printer = printer
	})
}
