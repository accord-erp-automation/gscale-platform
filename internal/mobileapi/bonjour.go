package mobileapi

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/mdns"
)

const bonjourServiceType = "_gscale-mobileapi._tcp"

func (s *Server) ListenAndServeBonjour(ctx context.Context) error {
	if s == nil {
		return nil
	}

	profile := s.currentProfile()
	instance := strings.TrimSpace(s.cfg.ServerName)
	if instance == "" {
		instance = "gscale-zebra"
	}

	service, err := mdns.NewMDNSService(
		instance,
		bonjourServiceType,
		"local.",
		fmt.Sprintf("%s.local.", trimBonjourHostName(s.cfg.ServerName)),
		httpPortFromListenAddr(s.cfg.ListenAddr),
		nil,
		[]string{
			"service=mobileapi",
			"app=gscale-zebra",
			"server_name=" + sanitizeBonjourTXT(s.cfg.ServerName),
			"server_ref=" + sanitizeBonjourTXT(profile.Ref),
			"display_name=" + sanitizeBonjourTXT(profile.DisplayName),
			"role=" + sanitizeBonjourTXT(profile.Role),
		},
	)
	if err != nil {
		return err
	}
	server, err := mdns.NewServer(&mdns.Config{Zone: service})
	if err != nil {
		return err
	}
	defer server.Shutdown()

	<-ctx.Done()
	return nil
}

func sanitizeBonjourTXT(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	return value
}

func trimBonjourHostName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, ".local")
	value = strings.TrimSuffix(value, ".local.")
	value = strings.Trim(value, ".")
	if value == "" {
		return "gscale-zebra"
	}
	return value
}
