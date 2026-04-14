package mobileapi

import (
	"net"
	"strconv"
	"strings"
)

var defaultMobileAPICandidatePorts = []int{39117, 41257, 43391, 45533, 47681}

func defaultPrimaryMobileAPIPort() int {
	return defaultMobileAPICandidatePorts[0]
}

func defaultMobileAPIListenAddr() string {
	return net.JoinHostPort(defaultMobileAPIBindHost(), strconv.Itoa(defaultPrimaryMobileAPIPort()))
}

func defaultMobileAPIBindHost() string {
	return "0.0.0.0"
}

func parseMobileAPICandidatePorts(raw string) []int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return append([]int(nil), defaultMobileAPICandidatePorts...)
	}

	seen := make(map[int]struct{}, len(defaultMobileAPICandidatePorts))
	out := make([]int, 0, len(defaultMobileAPICandidatePorts))
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		port, err := strconv.Atoi(part)
		if err != nil || port <= 0 || port > 65535 {
			continue
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		out = append(out, port)
	}
	if len(out) == 0 {
		return append([]int(nil), defaultMobileAPICandidatePorts...)
	}
	return out
}

func selectMobileAPIListenAddr(explicitAddr, bindHost string, candidatePorts []int) string {
	explicitAddr = strings.TrimSpace(explicitAddr)
	if explicitAddr != "" {
		return explicitAddr
	}

	if len(candidatePorts) == 0 {
		return defaultMobileAPIListenAddr()
	}

	bindHost = normalizeMobileAPIBindHost(bindHost)
	for _, port := range candidatePorts {
		addr := net.JoinHostPort(bindHost, strconv.Itoa(port))
		if isTCPListenAddrAvailable(addr) {
			return addr
		}
	}

	return net.JoinHostPort(bindHost, strconv.Itoa(candidatePorts[0]))
}

func normalizeMobileAPIBindHost(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaultMobileAPIBindHost()
	}
	return raw
}

func isTCPListenAddrAvailable(addr string) bool {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}
