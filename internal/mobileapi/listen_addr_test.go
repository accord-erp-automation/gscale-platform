package mobileapi

import (
	"fmt"
	"net"
	"testing"
)

func TestLoadConfigChoosesFirstFreeCandidatePort(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen busy port: %v", err)
	}
	defer busy.Close()

	freePort := reserveFreePort(t)

	t.Setenv("MOBILE_API_ADDR", "")
	t.Setenv("MOBILE_API_BIND_HOST", "127.0.0.1")
	t.Setenv("MOBILE_API_CANDIDATE_PORTS", fmt.Sprintf("%d,%d", busy.Addr().(*net.TCPAddr).Port, freePort))

	cfg := LoadConfig()
	want := fmt.Sprintf("127.0.0.1:%d", freePort)
	if cfg.ListenAddr != want {
		t.Fatalf("ListenAddr = %q want %q", cfg.ListenAddr, want)
	}
	if len(cfg.CandidatePorts) != 2 {
		t.Fatalf("CandidatePorts len = %d", len(cfg.CandidatePorts))
	}
}

func TestLoadConfigExplicitAddrWins(t *testing.T) {
	t.Setenv("MOBILE_API_ADDR", "0.0.0.0:8081")
	t.Setenv("MOBILE_API_BIND_HOST", "127.0.0.1")
	t.Setenv("MOBILE_API_CANDIDATE_PORTS", "39117,41257,43391")

	cfg := LoadConfig()
	if cfg.ListenAddr != "0.0.0.0:8081" {
		t.Fatalf("ListenAddr = %q", cfg.ListenAddr)
	}
}

func reserveFreePort(t *testing.T) int {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve free port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}
