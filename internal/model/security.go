package model

import "time"

// --- Security (spec §8.5) -----------------------------------------------------

// PolicyItem is one desired security setting with an assess/remediate definition.
// Key refers to a ConfigDocument setting key (security.* / protocols.* mostly).
type PolicyItem struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Operator compares the current value with Value: eq | ne | gte | lte | in | notIn | set | unset
	Operator string `json:"operator"`
	Value    any    `json:"value,omitempty"`
	// Severity weights the compliance score: critical=3, high=2, medium=1, low=0.5.
	Severity string `json:"severity"`
	// Remediate: when false the item is assess-only (e.g. requires a manual step).
	Remediate bool `json:"remediate"`
	// Remediation is the change applied when the item fails (defaults to Key=Value for eq).
	Remediation *SettingChange `json:"remediation,omitempty"`
	// ManagementPath marks items whose remediation could remove the channel we
	// manage the device through (e.g. disabling SNMP, HTTP); the engine must keep
	// at least one working path (spec §11).
	ManagementPath string `json:"managementPath,omitempty"` // snmp | ipp | http | ""
}

// Policy is a named set of policy items.
type Policy struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Items       []PolicyItem `json:"items"`
	CreatedBy   string       `json:"createdBy"`
	CreatedAt   time.Time    `json:"createdAt"`
	UpdatedAt   time.Time    `json:"updatedAt"`
}

// ItemResultStatus is a policy item's assessment outcome.
type ItemResultStatus string

const (
	ItemPass    ItemResultStatus = "pass"
	ItemFail    ItemResultStatus = "fail"
	ItemUnknown ItemResultStatus = "unknown" // setting not readable on this device
)

// ItemResult is one assessed item.
type ItemResult struct {
	ItemID   string           `json:"itemId"`
	Key      string           `json:"key"`
	Name     string           `json:"name"`
	Status   ItemResultStatus `json:"status"`
	Current  any              `json:"current,omitempty"`
	Desired  any              `json:"desired,omitempty"`
	Severity string           `json:"severity"`
	Message  string           `json:"message,omitempty"`
}

// Assessment is a device's compliance with a policy at a point in time.
type Assessment struct {
	ID        string       `json:"id"`
	DeviceID  string       `json:"deviceId"`
	PolicyID  string       `json:"policyId"`
	Items     []ItemResult `json:"items"`
	Passed    int          `json:"passed"`
	Failed    int          `json:"failed"`
	Unknown   int          `json:"unknown"`
	Score     float64      `json:"score"` // 0..100, severity-weighted
	Compliant bool         `json:"compliant"`
	JobID     string       `json:"jobId,omitempty"`
	At        time.Time    `json:"at"`
}

// Certificate describes an X.509 certificate installed on a device.
type Certificate struct {
	ID          string    `json:"id"`  // device-local identifier
	Use         string    `json:"use"` // identity | ca | other
	Subject     string    `json:"subject"`
	Issuer      string    `json:"issuer"`
	Serial      string    `json:"serial,omitempty"`
	NotBefore   time.Time `json:"notBefore"`
	NotAfter    time.Time `json:"notAfter"`
	Fingerprint string    `json:"fingerprint,omitempty"` // SHA-256 hex
	SelfSigned  bool      `json:"selfSigned"`
	KeyType     string    `json:"keyType,omitempty"`
}

// EnrollmentProtocol names a standard certificate enrollment protocol.
type EnrollmentProtocol string

const (
	EnrollSCEP EnrollmentProtocol = "scep" // RFC 8894
	EnrollEST  EnrollmentProtocol = "est"  // RFC 7030
	EnrollACME EnrollmentProtocol = "acme" // RFC 8555
)

// CAProfile is a configured certificate authority endpoint.
type CAProfile struct {
	ID       string             `json:"id"`
	Name     string             `json:"name"`
	Protocol EnrollmentProtocol `json:"protocol"`
	URL      string             `json:"url"`
	// CredentialID references a credential holding the challenge/password/account key.
	CredentialID string `json:"credentialId,omitempty"`
	// CAFingerprint pins the enrollment server's TLS certificate (SHA-256 hex of
	// the leaf or any chain certificate); empty = system trust roots.
	CAFingerprint string `json:"caFingerprint,omitempty"`
	ValidityDays  int    `json:"validityDays,omitempty"`
	// KeyType of the keys generated for devices: rsa2048 (default, widest
	// printer support) | rsa3072 | ecdsa-p256.
	KeyType string `json:"keyType,omitempty"`
	// Options are protocol-specific, non-secret settings: ACME "email",
	// "dnsProvider" (manual) and "dnsWait" (e.g. "5m"); EST "label" (RFC 7030
	// §3.2.2 path segment); SCEP "caIdent" (GetCACert message).
	Options   map[string]string `json:"options,omitempty"`
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

// CertRequest asks a driver to install or renew a certificate.
type CertRequest struct {
	Use        string   `json:"use"` // identity | ca
	CommonName string   `json:"commonName,omitempty"`
	SANs       []string `json:"sans,omitempty"`
	// PEM is a ready certificate (and key for identity, if the device accepts it).
	CertPEM  string `json:"-"`
	KeyPEM   string `json:"-"`
	ChainPEM string `json:"-"`
}

// CertResult is the outcome of a certificate operation.
type CertResult struct {
	DeviceID  string       `json:"deviceId"`
	DryRun    bool         `json:"dryRun"`
	Installed *Certificate `json:"installed,omitempty"`
	Message   string       `json:"message,omitempty"`
	At        time.Time    `json:"at"`
}

// --- Additions for the security feature (phase 5) ----------------------------

// Policy item severities and their score weights (spec §8.5 "optional risk weighting").
const (
	SeverityItemCritical = "critical"
	SeverityItemHigh     = "high"
	SeverityItemMedium   = "medium"
	SeverityItemLow      = "low"
)

// SeverityWeight returns the compliance-score weight of an item severity:
// critical 3, high 2, medium 1, low 0.5. Unknown severities weigh as medium.
func SeverityWeight(sev string) float64 {
	switch sev {
	case SeverityItemCritical:
		return 3
	case SeverityItemHigh:
		return 2
	case SeverityItemLow:
		return 0.5
	}
	return 1
}

// Policy item operators.
const (
	OpEq    = "eq"
	OpNe    = "ne"
	OpGte   = "gte"
	OpLte   = "lte"
	OpIn    = "in"
	OpNotIn = "notIn"
	OpSet   = "set"
	OpUnset = "unset"
)

// Operators lists every policy operator.
var Operators = []string{OpEq, OpNe, OpGte, OpLte, OpIn, OpNotIn, OpSet, OpUnset}

// ValidOperator reports whether op is a known operator.
func ValidOperator(op string) bool {
	for _, o := range Operators {
		if o == op {
			return true
		}
	}
	return false
}

// RemediationStatus is the per-item outcome of a remediation.
type RemediationStatus string

const (
	RemediationWritten    RemediationStatus = "written"
	RemediationWouldWrite RemediationStatus = "would_write" // dry run
	RemediationUnchanged  RemediationStatus = "unchanged"   // already compliant
	RemediationSkipped    RemediationStatus = "skipped"     // assess-only, no writer, or management-path rule
	RemediationFailed     RemediationStatus = "failed"
)

// RemediationItem is one policy item's remediation outcome. Values of secret
// settings are never carried here.
type RemediationItem struct {
	ItemID  string            `json:"itemId"`
	Key     string            `json:"key"`
	Name    string            `json:"name"`
	Status  RemediationStatus `json:"status"`
	Before  any               `json:"before,omitempty"`
	After   any               `json:"after,omitempty"`
	Driver  string            `json:"driver,omitempty"`
	Message string            `json:"message,omitempty"`
}

// RemediationResult is a device's remediation outcome.
type RemediationResult struct {
	DeviceID string            `json:"deviceId"`
	PolicyID string            `json:"policyId"`
	DryRun   bool              `json:"dryRun"`
	Items    []RemediationItem `json:"items"`
	Written  int               `json:"written"`
	Skipped  int               `json:"skipped"`
	Failed   int               `json:"failed"`
	At       time.Time         `json:"at"`
}

// IssuedCertificate records a certificate PrintMarshal obtained from a CA
// profile on a device's behalf. The certificate and chain are public and are
// kept so an operator can install them by hand when no driver can; the
// private key is stored encrypted and is never part of this struct.
type IssuedCertificate struct {
	ID          string     `json:"id"`
	DeviceID    string     `json:"deviceId"`
	CAProfileID string     `json:"caProfileId"`
	Protocol    string     `json:"protocol"`
	Use         string     `json:"use"` // identity | ca
	Subject     string     `json:"subject"`
	Issuer      string     `json:"issuer"`
	Serial      string     `json:"serial"`
	NotBefore   time.Time  `json:"notBefore"`
	NotAfter    time.Time  `json:"notAfter"`
	Fingerprint string     `json:"fingerprint"` // SHA-256 hex
	KeyType     string     `json:"keyType"`
	CertPEM     string     `json:"certPem"`
	ChainPEM    string     `json:"chainPem,omitempty"`
	Installed   bool       `json:"installed"`
	InstalledBy string     `json:"installedBy,omitempty"` // driver that installed it
	InstalledAt *time.Time `json:"installedAt,omitempty"`
	JobID       string     `json:"jobId,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// DeviceCertificate is a certificate observed on a device, by a driver
// (source = driver name) or by the TLS probe (source "probe").
type DeviceCertificate struct {
	DeviceID string    `json:"deviceId"`
	Source   string    `json:"source"`
	SeenAt   time.Time `json:"seenAt"`
	Certificate
}

// Credential rotation kinds.
const (
	RotateSNMPCommunity = "snmpCommunity"
	RotateAdminPassword = "adminPassword"
)

// CompliancePoint is one day of a policy's compliance history.
type CompliancePoint struct {
	Date      string  `json:"date"` // YYYY-MM-DD
	Assessed  int     `json:"assessed"`
	Compliant int     `json:"compliant"`
	AvgScore  float64 `json:"avgScore"`
}
