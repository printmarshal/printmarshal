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
	Database string `json:"database"` // "sqlite" (default) | "postgres"
	DSN      string `json:"dsn"`      // SQLite file path (defaults to <dataDir>/printmarshal.db) or a postgres:// URL

	Poll struct {
		SNMPPort       int      `json:"snmpPort"`     // 161; override for simulators
		IPPEndpoints   []string `json:"ippEndpoints"` // printf templates with %s for the host; empty = defaults
		Interval       Duration `json:"interval"`     // scheduled poll cadence
		Concurrency    int      `json:"concurrency"`  // max simultaneous device reads
		SNMPTimeout    Duration `json:"snmpTimeout"`
		SNMPRetries    int      `json:"snmpRetries"`
		IPPTimeout     Duration `json:"ippTimeout"`
		IPPInsecureTLS bool     `json:"ippInsecureTls"`
		// IPPWriteRequireTLS refuses to send Set-Printer-Attributes over
		// plaintext; writes then need an IPPS (or HTTPS) endpoint on the device.
		IPPWriteRequireTLS bool `json:"ippWriteRequireTLS"`
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
		// LDAP lets the username/password form authenticate against an LDAP or
		// Active Directory server (docs/auth.md). Off while URL is empty.
		LDAP LDAP `json:"ldap"`
		// RADIUS lets the username/password form authenticate against RADIUS
		// servers with PAP (docs/auth.md). Off while Servers is empty.
		RADIUS RADIUS `json:"radius"`
		// SAML enables single sign-on as a SAML 2.0 service provider
		// (docs/auth.md). Off while no IdP metadata source is set.
		SAML SAML `json:"saml"`
	} `json:"auth"`

	Telemetry struct {
		Retention Duration `json:"retention"` // samples older than this are down-sampled/pruned
	} `json:"telemetry"`

	// Export is the SIEM / log export and the printer syslog receiver (docs/export.md).
	Export Export `json:"export"`
}

// Export configures the security-event export sinks and the printer syslog
// receiver (audit A12). Every sink is optional; an empty address disables it.
type Export struct {
	// Buffer is the number of events queued for the export worker before
	// new ones are dropped (and counted) so the event bus never blocks.
	Buffer int `json:"buffer,omitempty"`

	Syslog struct {
		// Address is udp://host:514, tcp://host:6514 or tls://host:6514.
		Address string `json:"address"`
		// Format is "syslog" (RFC 5424 with structured data), "cef" or "json".
		Format string `json:"format"`
		// Facility is the syslog facility number (default 16 = local0).
		Facility int `json:"facility,omitempty"`
		// TLSInsecure skips server certificate verification for tls://.
		TLSInsecure bool `json:"tlsInsecure,omitempty"`
		// CAFile is a PEM bundle that verifies the tls:// server.
		CAFile string `json:"caFile,omitempty"`
	} `json:"syslog"`

	File struct {
		Path      string `json:"path"`                // JSON lines, appended
		MaxSizeMB int    `json:"maxSizeMb,omitempty"` // rotate above this size (default 50)
		Keep      int    `json:"keep,omitempty"`      // rotated files to keep (default 5)
	} `json:"file"`

	HTTP struct {
		URL           string   `json:"url"`                     // POST target for JSON-lines batches
		TokenEnv      string   `json:"tokenEnv,omitempty"`      // env var holding a bearer token
		BatchSize     int      `json:"batchSize,omitempty"`     // events per POST (default 100)
		FlushInterval Duration `json:"flushInterval,omitempty"` // send a partial batch after this (default 5s)
		Timeout       Duration `json:"timeout,omitempty"`       // per attempt (default 10s)
	} `json:"http"`

	// IngestListen is the UDP address the printer syslog receiver binds
	// (e.g. "0.0.0.0:1514"); empty = off.
	IngestListen string `json:"ingestListen"`
	// IngestRetention prunes device log lines older than this (default 30 days).
	IngestRetention Duration `json:"ingestRetention,omitempty"`
	// IngestRatePerMinute caps lines handled per source address (default 120).
	IngestRatePerMinute int `json:"ingestRatePerMinute,omitempty"`
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

// LDAP configures the optional LDAP / Active Directory sign-in (docs/auth.md).
// Accounts are bound by (issuer "ldap:<url>", SubjectAttr value), never by name.
type LDAP struct {
	URL                string `json:"url"`                          // ldap://host:389 or ldaps://host:636
	StartTLS           bool   `json:"startTls,omitempty"`           // upgrade an ldap:// connection with StartTLS before binding
	CAFile             string `json:"caFile,omitempty"`             // PEM bundle that verifies the server (system roots otherwise)
	InsecureSkipVerify bool   `json:"insecureSkipVerify,omitempty"` // skip server certificate verification (lab use only)
	// BindDN / BindPasswordEnv are the optional read-only service account
	// used to find the user's entry. Without them PrintMarshal binds directly
	// as the user, using UserDNTemplate to build the bind name.
	BindDN          string `json:"bindDN,omitempty"`
	BindPasswordEnv string `json:"bindPasswordEnv,omitempty"` // env var holding the service-account password (never the password itself)
	// UserDNTemplate is used when BindDN is empty: "uid=%s,ou=people,dc=example,dc=com"
	// or an AD UPN such as "%s@corp.example.com".
	UserDNTemplate string `json:"userDNTemplate,omitempty"`
	BaseDN         string `json:"baseDN"` // search base for the user entry
	// UserFilter finds the entry; every %s is replaced by the RFC 4515-escaped
	// login name. Default (|(sAMAccountName=%s)(uid=%s)(userPrincipalName=%s)(mail=%s)).
	UserFilter string `json:"userFilter,omitempty"`
	// UsernameAttr names the attribute that becomes the local username
	// (default: the attribute that matched, then sAMAccountName, uid, the DN).
	UsernameAttr string `json:"usernameAttr,omitempty"`
	// SubjectAttr is the stable identifier the account is bound to
	// (default objectGUID, then entryUUID, then the DN).
	SubjectAttr string `json:"subjectAttr,omitempty"`
	// GroupAttr lists the user's groups (default memberOf).
	GroupAttr string `json:"groupAttr,omitempty"`
	// RoleMap maps a group DN or CN to viewer | operator | admin; the highest
	// mapped role wins and is re-applied on every sign-in.
	RoleMap     map[string]string `json:"roleMap,omitempty"`
	DefaultRole string            `json:"defaultRole,omitempty"` // when no RoleMap entry matches (default viewer)
	// AllowedGroups, when set, refuses users in none of these groups (DN or CN).
	AllowedGroups []string `json:"allowedGroups,omitempty"`
	Timeout       Duration `json:"timeout,omitempty"`     // per connection (default 10s)
	DisplayName   string   `json:"displayName,omitempty"` // shown on the login page; defaults to the server host
}

// RADIUS configures the optional RADIUS (RFC 2865 PAP) sign-in (docs/auth.md).
// RADIUS has no stable user identifier, so the subject is the lowercased
// username and the issuer "radius:<first server>".
type RADIUS struct {
	Servers       []string `json:"servers"`                 // host:port, tried in order
	SecretEnv     string   `json:"secretEnv"`               // env var holding the shared secret (never the secret itself)
	Timeout       Duration `json:"timeout,omitempty"`       // per attempt (default 3s)
	Retries       int      `json:"retries,omitempty"`       // retransmissions per server (default 2)
	NASIdentifier string   `json:"nasIdentifier,omitempty"` // NAS-Identifier attribute (default "printmarshal")
	// RoleAttribute is the reply attribute whose values are looked up in
	// RoleMap: "Filter-Id" (default) or "Class".
	RoleAttribute string            `json:"roleAttribute,omitempty"`
	RoleMap       map[string]string `json:"roleMap,omitempty"`
	DefaultRole   string            `json:"defaultRole,omitempty"`
	DisplayName   string            `json:"displayName,omitempty"`
}

// SAML configures the optional SAML 2.0 service provider (docs/auth.md).
type SAML struct {
	IDPMetadataURL  string `json:"idpMetadataUrl,omitempty"`  // https URL of the IdP metadata (fetched at first use)
	IDPMetadataFile string `json:"idpMetadataFile,omitempty"` // or a local copy of it
	// BaseURL is PrintMarshal's public origin, e.g. https://printmarshal.example.com;
	// EntityID and ACSURL default to <BaseURL>/api/auth/saml/metadata and /acs.
	BaseURL  string `json:"baseUrl"`
	EntityID string `json:"entityId,omitempty"`
	ACSURL   string `json:"acsUrl,omitempty"`
	// SPCertFile / SPKeyFile hold the SP signing pair (PEM). When absent a
	// self-signed pair is generated under the data directory on first start.
	SPCertFile   string `json:"spCertFile,omitempty"`
	SPKeyFile    string `json:"spKeyFile,omitempty"`
	SignRequests bool   `json:"signRequests,omitempty"` // sign AuthnRequests (RSA-SHA256)
	// UsernameAttr names the assertion attribute used as the local username
	// (default: the NameID).
	UsernameAttr string `json:"usernameAttr,omitempty"`
	// SubjectAttr is the stable identifier the account is bound to. Default:
	// the NameID when its format is persistent or unspecified; a transient
	// NameID requires an attribute here (Entra ID: objectidentifier).
	SubjectAttr string `json:"subjectAttr,omitempty"`
	// RoleAttr is the attribute carrying groups / roles looked up in RoleMap.
	RoleAttr      string            `json:"roleAttr,omitempty"`
	RoleMap       map[string]string `json:"roleMap,omitempty"`
	DefaultRole   string            `json:"defaultRole,omitempty"`
	AllowedGroups []string          `json:"allowedGroups,omitempty"` // refuse users with none of these RoleAttr values
	DisplayName   string            `json:"displayName,omitempty"`
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
	// AuthMethod selects the SASL mechanism: "plain" (default), "login" (Exchange/
	// older relays) or "xoauth2" (Microsoft 365, Google Workspace). For xoauth2
	// the bearer token comes from TokenEnv, or is fetched with the OAuth 2.0
	// client-credentials grant when TokenURL is set.
	AuthMethod      string `json:"authMethod,omitempty"`
	TokenEnv        string `json:"tokenEnv,omitempty"` // env var holding a ready bearer token
	TokenURL        string `json:"tokenUrl,omitempty"` // e.g. https://login.microsoftonline.com/<tenant>/oauth2/v2.0/token
	ClientID        string `json:"clientId,omitempty"`
	ClientSecretEnv string `json:"clientSecretEnv,omitempty"` // env var holding the OAuth client secret
	Scope           string `json:"scope,omitempty"`           // e.g. https://outlook.office365.com/.default
	// Legacy switches the Teams payload back to the retired Office 365 connector
	// MessageCard format; the default is an Adaptive Card for Workflows webhooks.
	Legacy bool `json:"legacy,omitempty"`
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
	c.Export.Buffer = 4096
	c.Export.Syslog.Format = "syslog"
	c.Export.Syslog.Facility = 16
	c.Export.File.MaxSizeMB = 50
	c.Export.File.Keep = 5
	c.Export.HTTP.BatchSize = 100
	c.Export.HTTP.FlushInterval = Duration(5 * time.Second)
	c.Export.HTTP.Timeout = Duration(10 * time.Second)
	c.Export.IngestRetention = Duration(30 * 24 * time.Hour)
	c.Export.IngestRatePerMinute = 120
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
	envBool("PM_IPP_WRITE_REQUIRE_TLS", &c.Poll.IPPWriteRequireTLS)
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
	envStr("PM_LDAP_URL", &c.Auth.LDAP.URL)
	envBool("PM_LDAP_START_TLS", &c.Auth.LDAP.StartTLS)
	envStr("PM_LDAP_CA_FILE", &c.Auth.LDAP.CAFile)
	envStr("PM_LDAP_BIND_DN", &c.Auth.LDAP.BindDN)
	envStr("PM_LDAP_BIND_PASSWORD_ENV", &c.Auth.LDAP.BindPasswordEnv)
	envStr("PM_LDAP_USER_DN_TEMPLATE", &c.Auth.LDAP.UserDNTemplate)
	envStr("PM_LDAP_BASE_DN", &c.Auth.LDAP.BaseDN)
	envStr("PM_LDAP_USER_FILTER", &c.Auth.LDAP.UserFilter)
	if v := os.Getenv("PM_RADIUS_SERVERS"); v != "" {
		c.Auth.RADIUS.Servers = strings.Split(v, ",")
	}
	envStr("PM_RADIUS_SECRET_ENV", &c.Auth.RADIUS.SecretEnv)
	envStr("PM_RADIUS_NAS_IDENTIFIER", &c.Auth.RADIUS.NASIdentifier)
	envStr("PM_SAML_IDP_METADATA_URL", &c.Auth.SAML.IDPMetadataURL)
	envStr("PM_SAML_IDP_METADATA_FILE", &c.Auth.SAML.IDPMetadataFile)
	envStr("PM_SAML_BASE_URL", &c.Auth.SAML.BaseURL)
	envStr("PM_SAML_ENTITY_ID", &c.Auth.SAML.EntityID)
	envStr("PM_SAML_ACS_URL", &c.Auth.SAML.ACSURL)
	envStr("PM_SAML_SP_CERT_FILE", &c.Auth.SAML.SPCertFile)
	envStr("PM_SAML_SP_KEY_FILE", &c.Auth.SAML.SPKeyFile)
	envStr("PM_EXPORT_SYSLOG", &c.Export.Syslog.Address)
	envStr("PM_EXPORT_SYSLOG_FORMAT", &c.Export.Syslog.Format)
	envInt("PM_EXPORT_SYSLOG_FACILITY", &c.Export.Syslog.Facility)
	envBool("PM_EXPORT_SYSLOG_TLS_INSECURE", &c.Export.Syslog.TLSInsecure)
	envStr("PM_EXPORT_SYSLOG_CA_FILE", &c.Export.Syslog.CAFile)
	envStr("PM_EXPORT_FILE", &c.Export.File.Path)
	envInt("PM_EXPORT_FILE_MAX_MB", &c.Export.File.MaxSizeMB)
	envInt("PM_EXPORT_FILE_KEEP", &c.Export.File.Keep)
	envStr("PM_EXPORT_HTTP", &c.Export.HTTP.URL)
	envStr("PM_EXPORT_HTTP_TOKEN_ENV", &c.Export.HTTP.TokenEnv)
	envInt("PM_EXPORT_HTTP_BATCH", &c.Export.HTTP.BatchSize)
	envDur("PM_EXPORT_HTTP_FLUSH", &c.Export.HTTP.FlushInterval)
	envInt("PM_EXPORT_BUFFER", &c.Export.Buffer)
	envStr("PM_EXPORT_INGEST_LISTEN", &c.Export.IngestListen)
	envDur("PM_EXPORT_INGEST_RETENTION", &c.Export.IngestRetention)
	envInt("PM_EXPORT_INGEST_RATE", &c.Export.IngestRatePerMinute)
	if c.DSN == "" {
		c.DSN = c.DataDir + "/printmarshal.db"
	}
	return c, nil
}
