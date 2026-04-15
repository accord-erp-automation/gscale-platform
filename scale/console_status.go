package main

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

type consoleStatus struct {
	mu sync.Mutex
	w  io.Writer

	rendered bool
	serial   string
	zebra    string
}

func newConsoleStatus(w io.Writer, serial, zebra string) *consoleStatus {
	return &consoleStatus{
		w:      w,
		serial: serial,
		zebra:  zebra,
	}
}

func (s *consoleStatus) SetSerial(text string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if s.serial == text {
		return
	}
	s.serial = text
	s.renderLocked()
}

func (s *consoleStatus) SetZebra(text string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if s.zebra == text {
		return
	}
	s.zebra = text
	s.renderLocked()
}

func (s *consoleStatus) renderLocked() {
	if s.w == nil {
		return
	}
	if !s.rendered {
		fmt.Fprintln(s.w, s.serial)
		fmt.Fprintln(s.w, s.zebra)
		s.rendered = true
		return
	}
	_, _ = fmt.Fprintf(s.w, "\033[2A\033[2K\r%s\n\033[2K\r%s", s.serial, s.zebra)
}

func (s *consoleStatus) Render() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.renderLocked()
}

func waitSerialLine(device string, baud int) string {
	device = strings.TrimSpace(device)
	if device == "" {
		return "wait: serial"
	}
	if baud > 0 {
		return fmt.Sprintf("wait: serial %s @ %d", device, baud)
	}
	return "wait: serial " + device
}

func readySerialLine(device string, baud int) string {
	device = strings.TrimSpace(device)
	if device == "" {
		return "connected: serial"
	}
	if baud > 0 {
		return fmt.Sprintf("connected: serial %s @ %d", device, baud)
	}
	return "connected: serial " + device
}

func waitZebraLine(device string) string {
	device = strings.TrimSpace(device)
	if device == "" {
		return "wait: zebra"
	}
	return "wait: zebra " + device
}

func readyZebraLine(device string) string {
	device = strings.TrimSpace(device)
	if device == "" {
		return "connected: zebra"
	}
	return "connected: zebra " + device
}
