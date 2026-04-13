package mobileapi

import (
	"os"
	"strings"

	"core/runtimecfg"
)

const (
	defaultListenAddr      = ":8081"
	defaultDiscoveryAddr   = ":18081"
	defaultBridgeStateFile = "/tmp/gscale-zebra/bridge_state.json"
	defaultProfileFile     = "/tmp/gscale-zebra/mobile_profile.json"
	defaultSetupFile       = "config/core.env"
	defaultPolygonURL      = "http://127.0.0.1:18000"
)

type Config struct {
	ListenAddr      string
	DiscoveryAddr   string
	BridgeStateFile string
	ProfileFile     string
	SetupFile       string
	PolygonURL      string
	ERPURL          string
	ERPReadURL      string
	ERPAPIKey       string
	ERPAPISecret    string
	ServerName      string
	LoginPhone      string
	LoginCode       string
	Profile         SessionProfile
}

func LoadConfig() Config {
	role := strings.ToLower(strings.TrimSpace(firstNonEmpty(
		os.Getenv("MOBILE_API_ROLE"),
		"admin",
	)))
	if role == "" {
		role = "admin"
	}

	phone := firstNonEmpty(os.Getenv("MOBILE_API_PHONE"), "998900000000")
	displayName := firstNonEmpty(os.Getenv("MOBILE_API_DISPLAY_NAME"), "Polygon Operator")
	legalName := firstNonEmpty(os.Getenv("MOBILE_API_LEGAL_NAME"), displayName)
	ref := firstNonEmpty(os.Getenv("MOBILE_API_REF"), "dev-operator")
	avatarURL := strings.TrimSpace(os.Getenv("MOBILE_API_AVATAR_URL"))
	hostname, err := os.Hostname()
	if err != nil {
		hostname = ""
	}
	setupFile := firstNonEmpty(
		os.Getenv("MOBILE_API_SETUP_FILE"),
		defaultSetupFile,
	)
	coreCfg, _ := runtimecfg.Load(setupFile)

	return Config{
		ListenAddr:      firstNonEmpty(os.Getenv("MOBILE_API_ADDR"), defaultListenAddr),
		DiscoveryAddr:   firstNonEmpty(os.Getenv("MOBILE_API_DISCOVERY_ADDR"), defaultDiscoveryAddr),
		BridgeStateFile: firstNonEmpty(os.Getenv("BRIDGE_STATE_FILE"), defaultBridgeStateFile),
		ProfileFile:     firstNonEmpty(os.Getenv("MOBILE_API_PROFILE_FILE"), defaultProfileFile),
		SetupFile:       setupFile,
		PolygonURL:      firstNonEmpty(os.Getenv("POLYGON_URL"), defaultPolygonURL),
		ERPURL:          firstNonEmpty(os.Getenv("ERP_URL"), coreCfg.ERPURL),
		ERPReadURL:      firstNonEmpty(os.Getenv("ERP_READ_URL"), coreCfg.ERPReadURL),
		ERPAPIKey:       firstNonEmpty(os.Getenv("ERP_API_KEY"), coreCfg.ERPAPIKey),
		ERPAPISecret:    firstNonEmpty(os.Getenv("ERP_API_SECRET"), coreCfg.ERPAPISecret),
		ServerName:      firstNonEmpty(os.Getenv("MOBILE_API_SERVER_NAME"), hostname, "gscale-zebra"),
		LoginPhone:      phone,
		LoginCode:       firstNonEmpty(os.Getenv("MOBILE_API_CODE"), "1234"),
		Profile: SessionProfile{
			Role:        role,
			DisplayName: displayName,
			LegalName:   legalName,
			Ref:         ref,
			Phone:       phone,
			AvatarURL:   avatarURL,
		},
	}
}

func (c Config) HasERPWriteConfig() bool {
	return strings.TrimSpace(c.ERPURL) != "" &&
		strings.TrimSpace(c.ERPAPIKey) != "" &&
		strings.TrimSpace(c.ERPAPISecret) != ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
