package mobileapi

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const discoveryProbeV1 = "GSCALE_DISCOVER_V1"
const discoveryAnnounceInterval = 250 * time.Millisecond

type discoveryAnnouncement struct {
	Type           string `json:"type"`
	App            string `json:"app"`
	Service        string `json:"service"`
	ServerName     string `json:"server_name"`
	ServerRef      string `json:"server_ref"`
	DisplayName    string `json:"display_name"`
	Role           string `json:"role"`
	HTTPPort       int    `json:"http_port"`
	CandidatePorts []int  `json:"candidate_ports,omitempty"`
}

func (s *Server) ListenAndServeDiscovery(ctx context.Context) error {
	addr, err := net.ResolveUDPAddr("udp4", strings.TrimSpace(s.cfg.DiscoveryAddr))
	if err != nil {
		return err
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	announceConn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err == nil {
		_ = enableUDPBroadcast(announceConn)
		go func() {
			<-ctx.Done()
			_ = announceConn.Close()
		}()
		go s.broadcastDiscoveryAnnouncements(ctx, announceConn, addr.Port)
	}

	buf := make([]byte, 2048)
	for {
		n, remote, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		if strings.TrimSpace(string(buf[:n])) != discoveryProbeV1 {
			continue
		}
		payload, err := s.discoveryAnnouncementPayload()
		if err != nil {
			return err
		}
		if _, err := conn.WriteToUDP(payload, remote); err != nil && ctx.Err() != nil {
			return nil
		}
	}
}

func (s *Server) discoveryAnnouncementPayload() ([]byte, error) {
	profile := s.currentProfile()
	return json.Marshal(discoveryAnnouncement{
		Type:           "gscale_announce_v1",
		App:            "gscale-zebra",
		Service:        "mobileapi",
		ServerName:     s.cfg.ServerName,
		ServerRef:      profile.Ref,
		DisplayName:    profile.DisplayName,
		Role:           profile.Role,
		HTTPPort:       httpPortFromListenAddr(s.cfg.ListenAddr),
		CandidatePorts: append([]int(nil), s.cfg.CandidatePorts...),
	})
}

func (s *Server) broadcastDiscoveryAnnouncements(
	ctx context.Context,
	conn *net.UDPConn,
	port int,
) {
	targets := collectDiscoveryBroadcastTargets()
	send := func() {
		payload, err := s.discoveryAnnouncementPayload()
		if err != nil {
			return
		}
		for _, target := range targets {
			_, _ = conn.WriteToUDP(payload, &net.UDPAddr{IP: target, Port: port})
		}
	}

	send()
	ticker := time.NewTicker(discoveryAnnounceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			send()
		}
	}
}

func collectDiscoveryBroadcastTargets() []net.IP {
	out := map[string]net.IP{
		"255.255.255.255": net.IPv4(255, 255, 255, 255),
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return []net.IP{net.IPv4(255, 255, 255, 255)}
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP == nil {
				continue
			}
			ip := ipNet.IP.To4()
			mask := ipNet.Mask
			if ip == nil || len(mask) != net.IPv4len {
				continue
			}
			broadcast := make(net.IP, net.IPv4len)
			for i := 0; i < net.IPv4len; i++ {
				broadcast[i] = ip[i] | ^mask[i]
			}
			out[broadcast.String()] = broadcast
		}
	}

	result := make([]net.IP, 0, len(out))
	for _, ip := range out {
		result = append(result, ip)
	}
	return result
}

func enableUDPBroadcast(conn *net.UDPConn) error {
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var controlErr error
	if err := rawConn.Control(func(fd uintptr) {
		controlErr = syscall.SetsockoptInt(
			int(fd),
			syscall.SOL_SOCKET,
			syscall.SO_BROADCAST,
			1,
		)
	}); err != nil {
		return err
	}
	return controlErr
}

func httpPortFromListenAddr(addr string) int {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return defaultPrimaryMobileAPIPort()
	}
	if strings.HasPrefix(addr, ":") {
		port, err := strconv.Atoi(strings.TrimPrefix(addr, ":"))
		if err == nil && port > 0 {
			return port
		}
		return defaultPrimaryMobileAPIPort()
	}

	_, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return defaultPrimaryMobileAPIPort()
	}
	port, err := strconv.Atoi(strings.TrimSpace(portText))
	if err != nil || port <= 0 {
		return defaultPrimaryMobileAPIPort()
	}
	return port
}
