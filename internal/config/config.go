// Package config loads PrintMarshal settings from a JSON file with environment
// overrides (spec §12). Secrets come from the environment or the platform's
// secret mechanism, never from the image.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the full runtime configuration.
type Config struct {
	Listen   string `json:"listen"`   // HTTP listen address
	DataDir  string `json:"dataDir"`  // SQLite DB, key file
	Database string `json:"database"` // "sqlite" (default) | "postgres" (reserved)
	DSN      string `json:"dsn"`      // datastore DSN; defaults to <dataDir>/printmarshal.db

	Poll struct {
		SNMPPort       int      `json:"snmpPort"`     // 161; override for simulators
		IPPEndpoints   []string `json:"ippEndpoints"` // printf templates with %s for the host; empty = defaults
		Interval       Duration `json:"interval"`     // scheduled poll cadence
		Concurrency    int      `json:"concurrency"`  // max simultaneous device reads
		SNMPTimeout    Duration `json:"snmpTimeout"`
		SNMPRetries    int      `json:"snmpRetries"`
		IPPTimeout     Duration `json:"ippTimeout"`
		IPPInsecureTLS bool     `json:"ippInsecureTls"`
	} `json:"poll"`

	Discovery struct {
		Enabled      bool     `json:"enabled"` // run zero-config discovery on a schedule
		Interval     Duration `json:"interval"`
		MDNSWait     Duration `json:"mdnsWait"`
		WSDWait      Duration `json:"wsdWait"`
		ScanWorkers  int      `json:"scanWorkers"`
		MaxAddresses int      `json:"maxAddresses"` // cap for active scans unless allowLarge
		Targets      []string `json:"targets"`      // default active-scan targets
	} `json:"discovery"`

	Trap struct {
		Enabled bool   `json:"enabled"`
		Listen  string `json:"listen"` // e.g. "0.0.0.0:1162"
		// AllowedSources are CIDRs (or single addresses) traps are accepted from.
		// Empty = any source, but a trap is only acted on when it comes from
		// the address of a known device.
		AllowedSources []string `json:"allowedSources,omitempty"`
		// RatePerMinute caps traps handled per source address (0 = default 60).
		RatePerMinute int `json:"ratePerMinute,omitempty"`
	} `json:"trap"`

	Alerts struct {
		SupplyWarningPercent  int `json:"supplyWarningPercent"`
		SupplyCriticalPercent int `json:"supplyCriticalPercent"`
		OfflineAfterFailures  int `json:"offlineAfterFailures"`
	} `json:"alerts"`

	Notifiers []Notifier `json:"notifiers"`

	Jobs struct {
		Workers int `json:"workers"`
	} `json:"jobs"`

	Auth struct {
		SessionTTL    Duration `json:"sessionTtl"`
		SecureCookies bool     `json:"secureCookies"`
		// AllowedOrigins are extra Origin host patterns accepted on the WebSocket
		// stream (e.g. "console.example.com", "*.corp.example"). Same-origin is
		// always accepted; nothing else is unless listed here.
		AllowedOrigins []string `json:"allowedOrigins,omitempty"`
		// LoginFailuresPerMinute per source IP before sign-in is rate limited (0 = default 10).
		LoginFailuresPerMinute int `json:"loginFailuresPerMinute,omitempty"`
		// OIDC enables single sign-on through an OpenID Connect provider
		// (authorization-code flow with PKCE; docs/auth.md). Off while Issuer is empty.
		OIDC OIDC `json:"oidc"`
	} `json:"auth"`

	Telemetry struct {
		Retention Duration `json:"retention"` // samples older than this are down-sampled/pruned
	} `json:"telemetry"`
}

// OIDC configures the optional OpenID Connect sign-in (docs/auth.md).
type OIDC struct {
	Issuer          string `json:"issuer"` // e.g. https://login.microsoftonline.com/<tenant>/v2.0
	ClientID        string `json:"clientId"`
	ClientSecretEnv string `json:"clientSecretEnv,omitempty"` // env var holding the client secret (never the secret itself)
	RedirectURL     string `json:"redirectUrl"`               // https://<printmarshal>/api/auth/oidc/callback
	DisplayName     string `json:"displayName,omitempty"`     // login button label; defaults to the issuer host
	// Scopes requested besides "openid" (default: profile, email).
	Scopes []string `json:"scopes,omitempty"`
	// RoleClaim names the ID-token claim (dot path allowed, e.g. "realm_access.roles")
	// whose values are looked up in RoleMap; the highest mapped role wins.
	RoleClaim string            `json:"roleClaim,omitempty"`
	RoleMap   map[string]string `json:"roleMap,omitempty"` // claim value -> viewer | operator | admin
	// DefaultRole is used when no RoleMap entry matches (default viewer).
	DefaultRole string `json:"defaultRole,omitempty"`
	// AllowedDomains restricts sign-in to these email domains (empty = any).
	AllowedDomains []string `json:"allowedDomains,omitempty"`
}

// Notifier is one webhook or SMTP destination (spec §8.3).
type Notifier struct {
	Name   string   `json:"name"`
	Type   string   `json:"type"`   // webhook | slack | teams | discord | smtp
	URL    string   `json:"url"`    // webhook URL
	Events []string `json:"events"` // alert.raised, alert.cleared, job.finished, device.discovered; empty = all alert events
	// SMTP
	Host        string   `json:"host,omitempty"`
	Port        int      `json:"port,omitempty"`
	From        string   `json:"from,omitempty"`
	To          []string `json:"to,omitempty"`
	Username    string   `json:"username,omitempty"`
	PasswordEnv string   `json:"passwordEnv,omitempty"` // name of env var holding the SMTP password
	StartTLS    bool     `json:"startTls,omitempty"`
}

// Duration marshals as a Go duration string.
type Duration time.Duration

func (d Duration) D() time.Duration { return time.Duration(d) }

func (d Duration) MarshalJSON() ([]byte, error) { return json.Marshal(time.Duration(d).String()) }

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		v, err := time.ParseDuration(s)
		if err != nil {
			return err
		}
		*d = Duration(v)
		return nil
	}
	var n float64
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*d = Duration(time.Duration(n) * time.Second)
	return nil
}

// Default returns sensible single-container defaults.
func Default() *Config {
	c := &Config{Listen: ":8080", DataDir: "./data", Database: "sqlite"}
	c.Poll.Interval = Duration(5 * time.Minute)
	c.Poll.Concurrency = 32
	c.Poll.SNMPPort = 161
	c.Poll.SNMPTimeout = Duration(2 * time.Second)
	c.Poll.SNMPRetries = 1
	c.Poll.IPPTimeout = Duration(4 * time.Second)
	c.Poll.IPPInsecureTLS = true
	c.Discovery.Enabled = false
	c.Discovery.Interval = Duration(1 * time.Hour)
	c.Discovery.MDNSWait = Duration(5 * time.Second)
	c.Discovery.WSDWait = Duration(3 * time.Second)
	c.Discovery.ScanWorkers = 64
	c.Discovery.MaxAddresses = 4096
	c.Trap.Enabled = true
	c.Trap.Listen = "0.0.0.0:1162"
	c.Alerts.SupplyWarningPercent = 15
	c.Alerts.SupplyCriticalPercent = 5
	c.Alerts.OfflineAfterFailures = 2
	c.Jobs.Workers = 4
	c.Auth.SessionTTL = Duration(30 * 24 * time.Hour)
	c.Telemetry.Retention = Duration(90 * 24 * time.Hour)
	return c
}

// Load reads path (if non-empty) over the defaults, then applies PM_* env overrides.
func Load(path string) (*Config, error) {
	c := Default()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("config: %w", err)
		}
		if err := json.Unmarshal(b, c); err != nil {
			return nil, fmt.Errorf("config %s: %w", path, err)
		}
	}
	envStr := func(key string, dst *string) {
		if v := os.Getenv(key); v != "" {
			*dst = v
		}
	}
	envBool := func(key string, dst *bool) {
		if v := os.Getenv(key); v != "" {
			*dst = v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
		}
	}
	envInt := func(key string, dst *int) {
		if v := os.Getenv(key); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				*dst = n
			}
		}
	}
	envDur := func(key string, dst *Duration) {
		if v := os.Getenv(key); v != "" {
			if d, err := time.ParseDuration(v); err == nil {
				*dst = Duration(d)
			}
		}
	}
	envStr("PM_LISTEN", &c.Listen)
	envStr("PM_DATA_DIR", &c.DataDir)
	envStr("PM_DATABASE", &c.Database)
	envStr("PM_DSN", &c.DSN)
	envDur("PM_POLL_INTERVAL", &c.Poll.Interval)
	envInt("PM_POLL_CONCURRENCY", &c.Poll.Concurrency)
	envInt("PM_SNMP_PORT", &c.Poll.SNMPPort)
	if v := os.Getenv("PM_IPP_ENDPOINTS"); v != "" {
		c.Poll.IPPEndpoints = strings.Split(v, ",")
	}
	envBool("PM_DISCOVERY_ENABLED", &c.Discovery.Enabled)
	envDur("PM_DISCOVERY_INTERVAL", &c.Discovery.Interval)
	if v := os.Getenv("PM_DISCOVERY_TARGETS"); v != "" {
		c.Discovery.Targets = strings.Split(v, ",")
	}
	envBool("PM_TRAP_ENABLED", &c.Trap.Enabled)
	envStr("PM_TRAP_LISTEN", &c.Trap.Listen)
	envInt("PM_JOB_WORKERS", &c.Jobs.Workers)
	envBool("PM_SECURE_COOKIES", &c.Auth.SecureCookies)
	envStr("PM_OIDC_ISSUER", &c.Auth.OIDC.Issuer)
	envStr("PM_OIDC_CLIENT_ID", &c.Auth.OIDC.ClientID)
	envStr("PM_OIDC_CLIENT_SECRET_ENV", &c.Auth.OIDC.ClientSecretEnv)
	envStr("PM_OIDC_REDIRECT_URL", &c.Auth.OIDC.RedirectURL)
	if c.DSN == "" {
		c.DSN = c.DataDir + "/printmarshal.db"
	}
	return c, nil
}
