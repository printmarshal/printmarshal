// Package model holds the domain types shared across PrintMarshal: devices,
// supplies, trays, counters, alerts, jobs, groups, credentials and audit entries.
//
// The model is deliberately vendor-neutral. Nothing in here knows about any
// manufacturer; vendor-specific data is carried in free-form fields (Make,
// Model, EnterpriseID) that plugins may interpret.
package model

import "time"

// Capability names a thing a driver can do for a device (spec §6).
type Capability string

const (
	CapDiscover        Capability = "discover"
	CapIdentify        Capability = "identify"
	CapReadStatus      Capability = "read_status"
	CapReadSupplies    Capability = "read_supplies"
	CapReadTrays       Capability = "read_trays"
	CapReadCounters    Capability = "read_counters"
	CapReadConfig      Capability = "read_config"
	CapWriteConfig     Capability = "write_config"
	CapReadSecurity    Capability = "read_security"
	CapWriteSecurity   Capability = "write_security"
	CapManageCerts     Capability = "manage_certs"
	CapFirmwareRead    Capability = "firmware_read"
	CapFirmwareUpdate  Capability = "firmware_update"
	CapSubscribeAlerts Capability = "subscribe_alerts"
)

// AllCapabilities lists every capability the engine knows about.
var AllCapabilities = []Capability{
	CapDiscover, CapIdentify, CapReadStatus, CapReadSupplies, CapReadTrays,
	CapReadCounters, CapReadConfig, CapWriteConfig, CapReadSecurity,
	CapWriteSecurity, CapManageCerts, CapFirmwareRead, CapFirmwareUpdate,
	CapSubscribeAlerts,
}

// LifecycleState is where a device is in its life (spec §8.2).
type LifecycleState string

const (
	StateDiscovered     LifecycleState = "discovered"
	StateOnboarded      LifecycleState = "onboarded"
	StateActive         LifecycleState = "active"
	StateNeedsAttention LifecycleState = "needs_attention"
	StateDecommissioned LifecycleState = "decommissioned"
)

// ValidTransition reports whether a lifecycle change is allowed.
func ValidTransition(from, to LifecycleState) bool {
	switch from {
	case StateDiscovered:
		return to == StateOnboarded || to == StateActive || to == StateDecommissioned
	case StateOnboarded:
		return to == StateActive || to == StateNeedsAttention || to == StateDecommissioned
	case StateActive:
		return to == StateNeedsAttention || to == StateDecommissioned
	case StateNeedsAttention:
		return to == StateActive || to == StateDecommissioned
	case StateDecommissioned:
		return to == StateOnboarded
	}
	return false
}

// Severity of an alert or a device status.
type Severity string

const (
	SeverityOK       Severity = "ok"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Device is one printer or multifunction device (spec §9).
type Device struct {
	ID string `json:"id"`

	// Identity — automatic fields, refreshed by scans.
	Address        string `json:"address"`
	Hostname       string `json:"hostname,omitempty"`
	HostnameSource string `json:"hostnameSource,omitempty"` // mdns | dns | advertised | ""
	MAC            string `json:"mac,omitempty"`
	Serial         string `json:"serial,omitempty"`
	UUID           string `json:"uuid,omitempty"`
	Make           string `json:"make,omitempty"`
	Model          string `json:"model,omitempty"`
	DeviceType     string `json:"deviceType,omitempty"` // printer | mfp | scanner | unknown
	Firmware       string `json:"firmware,omitempty"`
	SysDescr       string `json:"sysDescr,omitempty"`
	SysName        string `json:"sysName,omitempty"`
	EnterpriseID   int    `json:"enterpriseId,omitempty"` // raw IANA enterprise from sysObjectID; interpreted only by plugins
	AdminURL       string `json:"adminUrl,omitempty"`

	// Resolved capability profile: capability -> driver that serves it.
	Capabilities     map[Capability]string `json:"capabilities"`
	DiscoveryMethods []string              `json:"discoveryMethods"`
	IdentifyMethods  []string              `json:"identifyMethods"`

	// Operator fields — never overwritten by a scan.
	DisplayName string `json:"displayName"`
	Location    string `json:"location"`
	AssetTag    string `json:"assetTag"`
	Notes       string `json:"notes"`

	State LifecycleState `json:"state"`

	// Last known telemetry.
	Status   *Status   `json:"status,omitempty"`
	Supplies []Supply  `json:"supplies"`
	Trays    []Tray    `json:"trays"`
	Counters *Counters `json:"counters,omitempty"`

	FirstSeen  time.Time  `json:"firstSeen"`
	LastSeen   time.Time  `json:"lastSeen"`
	LastPolled *time.Time `json:"lastPolled,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

// Name returns the operator's display name if set, else the best automatic name.
func (d *Device) Name() string {
	switch {
	case d.DisplayName != "":
		return d.DisplayName
	case d.Hostname != "":
		return d.Hostname
	case d.SysName != "":
		return d.SysName
	case d.Model != "":
		return d.Model
	}
	return d.Address
}

// Status is a device's current state as read from it.
type Status struct {
	Online     bool          `json:"online"`
	State      string        `json:"state"` // idle | printing | warmup | warning | down | unknown | offline
	Severity   Severity      `json:"severity"`
	Message    string        `json:"message,omitempty"` // console / display message
	Errors     []string      `json:"errors,omitempty"`  // hrPrinterDetectedErrorState flags, printer-state-reasons
	Alerts     []DeviceAlert `json:"alerts,omitempty"`  // prtAlertTable / printer-alert
	UptimeSecs int64         `json:"uptimeSecs,omitempty"`
	Source     string        `json:"source,omitempty"` // driver that produced it
	At         time.Time     `json:"at"`
}

// DeviceAlert is an alert as reported by the device itself (Printer MIB prtAlertTable, RFC 3805 §2.2.13).
type DeviceAlert struct {
	Index       int       `json:"index"`
	Severity    Severity  `json:"severity"`
	Group       string    `json:"group,omitempty"`
	GroupIndex  int       `json:"groupIndex,omitempty"`
	Code        string    `json:"code,omitempty"`
	Description string    `json:"description"`
	Time        time.Time `json:"time,omitempty"`
}

// Supply is one marker supply: toner, ink, drum, fuser, waste container... (RFC 3805 prtMarkerSuppliesTable).
type Supply struct {
	Index       int    `json:"index"`
	Description string `json:"description"`
	Type        string `json:"type"`  // toner | ink | opc | fuser | wasteToner | ...
	Class       string `json:"class"` // consumed | filled | other
	Colorant    string `json:"colorant,omitempty"`
	Level       int    `json:"level"`       // raw level; -1 other, -2 unknown, -3 some remaining
	MaxCapacity int    `json:"maxCapacity"` // raw; -1 other, -2 unknown
	Unit        string `json:"unit,omitempty"`
	Percent     *int   `json:"percent"` // nil when unknown
}

// Tray is an input or output tray (RFC 3805 prtInputTable / prtOutputTable).
type Tray struct {
	Index       int      `json:"index"`
	Kind        string   `json:"kind"` // input | output
	Name        string   `json:"name"`
	Type        string   `json:"type,omitempty"`
	Status      string   `json:"status"` // ok | low | empty | full | nearFull | unavailable | unknown
	Level       int      `json:"level"`  // -1 other, -2 unknown, -3 at least one
	MaxCapacity int      `json:"maxCapacity"`
	Percent     *int     `json:"percent"`
	MediaName   string   `json:"mediaName,omitempty"`
	MediaSize   string   `json:"mediaSize,omitempty"`
	MediaType   string   `json:"mediaType,omitempty"`
	Flags       []string `json:"flags,omitempty"` // decoded PrtSubUnitStatusTC bits
}

// Counters are page counters. Pointers are nil when the device does not report them.
type Counters struct {
	Total    int64            `json:"total"`
	Mono     *int64           `json:"mono,omitempty"`
	Color    *int64           `json:"color,omitempty"`
	Duplex   *int64           `json:"duplex,omitempty"`
	PowerOn  *int64           `json:"powerOn,omitempty"`
	ByMarker map[string]int64 `json:"byMarker,omitempty"`
	At       time.Time        `json:"at"`
}

// Identity is what the identify capability produces for an address.
type Identity struct {
	Address        string
	Hostname       string
	HostnameSource string
	MAC            string
	Serial         string
	UUID           string
	Make           string
	Model          string
	DeviceType     string
	Firmware       string
	SysDescr       string
	SysName        string
	SysLocation    string
	EnterpriseID   int
	AdminURL       string
	// Capabilities this driver can serve for the device it just identified.
	Capabilities []Capability
	Method       string // driver name
}

// Alert is a condition PrintMarshal raised about a device (spec §8.3).
type Alert struct {
	ID          string     `json:"id"`
	DeviceID    string     `json:"deviceId"`
	Severity    Severity   `json:"severity"`
	Source      string     `json:"source"` // poll | trap | supply | offline | consumable | system
	Key         string     `json:"key"`    // de-duplication key within a device
	Description string     `json:"description"`
	RaisedAt    time.Time  `json:"raisedAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	ClearedAt   *time.Time `json:"clearedAt,omitempty"`
	AckedAt     *time.Time `json:"ackedAt,omitempty"`
	AckedBy     string     `json:"ackedBy,omitempty"`
}

// JobStatus is the lifecycle of a job.
type JobStatus string

const (
	JobQueued    JobStatus = "queued"
	JobRunning   JobStatus = "running"
	JobSucceeded JobStatus = "succeeded"
	JobPartial   JobStatus = "partial"
	JobFailed    JobStatus = "failed"
	JobCancelled JobStatus = "cancelled"
)

// Job is a unit of fleet work (spec §9).
type Job struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"` // discovery | poll | ...
	Target     map[string]any `json:"target"`
	Params     map[string]any `json:"params"`
	Status     JobStatus      `json:"status"`
	Total      int            `json:"total"`
	Done       int            `json:"done"`
	Failed     int            `json:"failed"`
	Error      string         `json:"error,omitempty"`
	CreatedBy  string         `json:"createdBy"`
	CreatedAt  time.Time      `json:"createdAt"`
	StartedAt  *time.Time     `json:"startedAt,omitempty"`
	FinishedAt *time.Time     `json:"finishedAt,omitempty"`
}

// JobResult is one per-device (or per-address) result of a job.
type JobResult struct {
	ID       int64          `json:"id"`
	JobID    string         `json:"jobId"`
	DeviceID string         `json:"deviceId,omitempty"`
	Address  string         `json:"address,omitempty"`
	Status   string         `json:"status"` // ok | error | skipped
	Message  string         `json:"message,omitempty"`
	Data     map[string]any `json:"data,omitempty"`
	At       time.Time      `json:"at"`
}

// TelemetrySample is one time-series point.
type TelemetrySample struct {
	DeviceID string    `json:"deviceId"`
	Metric   string    `json:"metric"` // status.online | supply.percent | counter.total | tray.percent ...
	Label    string    `json:"label,omitempty"`
	Value    float64   `json:"value"`
	At       time.Time `json:"at"`
}

// Group of devices: static membership or a rule.
type Group struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Kind        string     `json:"kind"` // static | rule
	Rule        *GroupRule `json:"rule,omitempty"`
	MemberIDs   []string   `json:"memberIds"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

// GroupRule matches devices dynamically. All non-empty fields must match.
type GroupRule struct {
	MakeContains     string     `json:"makeContains,omitempty"`
	ModelContains    string     `json:"modelContains,omitempty"`
	LocationContains string     `json:"locationContains,omitempty"`
	Subnet           string     `json:"subnet,omitempty"` // CIDR
	DeviceType       string     `json:"deviceType,omitempty"`
	State            string     `json:"state,omitempty"`
	HasCapability    Capability `json:"hasCapability,omitempty"`
}

// CredentialType enumerates secret kinds (spec §8.5).
type CredentialType string

const (
	CredSNMPCommunity CredentialType = "snmp_community"
	CredSNMPv3        CredentialType = "snmpv3"
	CredAdminPassword CredentialType = "admin_password"
	CredCertificate   CredentialType = "certificate"
)

// CredentialPurpose binds a credential to the kind of device access it may be
// used for (audit A19). Read credentials serve discovery, identification,
// polling, configuration/security reads and dry runs; only write credentials
// are handed to configuration applies, security remediation and rotation,
// certificate installation and firmware delivery. A device with no
// write-capable credential bound cannot be written to.
type CredentialPurpose string

const (
	PurposeRead  CredentialPurpose = "read"
	PurposeWrite CredentialPurpose = "write"
	PurposeBoth  CredentialPurpose = "both"
)

// Valid reports whether p is one of the known purposes.
func (p CredentialPurpose) Valid() bool {
	switch p {
	case PurposeRead, PurposeWrite, PurposeBoth:
		return true
	}
	return false
}

// CanRead reports whether a credential of this purpose may serve reads. The
// empty purpose is the pre-purpose default, read.
func (p CredentialPurpose) CanRead() bool { return p == PurposeRead || p == PurposeBoth || p == "" }

// CanWrite reports whether a credential of this purpose may serve writes.
func (p CredentialPurpose) CanWrite() bool { return p == PurposeWrite || p == PurposeBoth }

// Credential metadata. The secret itself is never part of this struct; it lives
// encrypted in the store and is resolved into a Creds value only inside the engine.
type Credential struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Type      CredentialType    `json:"type"`
	Scope     string            `json:"scope"`              // "" = global, "device:<id>", "group:<id>"
	Username  string            `json:"username,omitempty"` // non-secret part (SNMPv3 user / admin user)
	Priority  int               `json:"priority"`
	Purpose   CredentialPurpose `json:"purpose"` // read (default) | write | both
	CreatedAt time.Time         `json:"createdAt"`
	UpdatedAt time.Time         `json:"updatedAt"`
}

// Secret is the secret half of a credential. It is encrypted at rest and must
// never be serialised into API responses or logs.
type Secret struct {
	Community    string `json:"community,omitempty"`
	AuthProtocol string `json:"authProtocol,omitempty"` // MD5 | SHA | SHA256 ...
	AuthPassword string `json:"authPassword,omitempty"`
	PrivProtocol string `json:"privProtocol,omitempty"` // DES | AES | AES256 ...
	PrivPassword string `json:"privPassword,omitempty"`
	Password     string `json:"password,omitempty"`
	PEM          string `json:"pem,omitempty"`
}

// Creds is the resolved set of credentials the engine hands to drivers for one device.
type Creds struct {
	Communities []string
	V3          *V3Creds
	AdminUser   string
	AdminPass   string
}

// V3Creds is an SNMPv3 USM user.
type V3Creds struct {
	Username     string
	AuthProtocol string
	AuthPassword string
	PrivProtocol string
	PrivPassword string
}

// AuditEntry records a write (spec §9, §11).
type AuditEntry struct {
	ID     int64          `json:"id"`
	Actor  string         `json:"actor"`
	Action string         `json:"action"`
	Target string         `json:"target"`
	Before map[string]any `json:"before,omitempty"`
	After  map[string]any `json:"after,omitempty"`
	Result string         `json:"result"`
	At     time.Time      `json:"at"`
}

// Role is an access-control role (spec §11).
type Role string

const (
	RoleViewer   Role = "viewer"
	RoleOperator Role = "operator"
	RoleAdmin    Role = "admin"
)

// Allows reports whether role r satisfies the minimum role.
func (r Role) Allows(min Role) bool {
	rank := map[Role]int{RoleViewer: 1, RoleOperator: 2, RoleAdmin: 3}
	return rank[r] >= rank[min]
}

// User of the console.
type User struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Role      Role      `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
	Kind      string    `json:"kind,omitempty"`  // "" = session user (local or external); "token" = API token principal
	Scope     []string  `json:"scope,omitempty"` // group ids the principal is limited to; empty = every device
	// ExternalIssuer / ExternalSubject bind an account to one identity at one
	// identity provider ("" for local accounts). External sign-ins resolve the
	// account by this pair, never by the username, so a provider cannot hand a
	// local administrator's name to someone else.
	ExternalIssuer  string `json:"externalIssuer,omitempty"`
	ExternalSubject string `json:"externalSubject,omitempty"`
}

// IsExternal reports whether the account is bound to an identity provider.
func (u *User) IsExternal() bool { return u != nil && u.ExternalIssuer != "" }

// ConsumableType classifies a stock-kept consumable (consumables spec §2).
type ConsumableType string

const (
	ConsumableToner          ConsumableType = "toner"
	ConsumableInk            ConsumableType = "ink"
	ConsumableDrum           ConsumableType = "drum"
	ConsumableFuser          ConsumableType = "fuser"
	ConsumableMaintenanceKit ConsumableType = "maintenance_kit"
	ConsumableWaste          ConsumableType = "waste"
	ConsumableStaples        ConsumableType = "staples"
	ConsumableOther          ConsumableType = "other"
)

// ConsumableTypes lists every consumable type in display order.
var ConsumableTypes = []ConsumableType{
	ConsumableToner, ConsumableInk, ConsumableDrum, ConsumableFuser,
	ConsumableMaintenanceKit, ConsumableWaste, ConsumableStaples, ConsumableOther,
}

// ValidConsumableType reports whether t is a known type.
func ValidConsumableType(t ConsumableType) bool {
	for _, k := range ConsumableTypes {
		if k == t {
			return true
		}
	}
	return false
}

// Consumable is one type of spare consumable kept in stock (consumables spec §2).
// Every field is operator-owned: nothing here is ever written by a scan or poll.
// Cost and reorder fields are optional references only — there is no pricing or
// billing logic anywhere in PrintMarshal.
type Consumable struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	Type             ConsumableType `json:"type"`
	Color            string         `json:"color"`      // "" | cyan | magenta | yellow | black | <named>
	PartNumber       string         `json:"partNumber"` // SKU; the primary match key against installed supplies
	CompatibleModels []string       `json:"compatibleModels"`
	Unit             string         `json:"unit"` // each | pack | ...
	OnHand           int            `json:"onHand"`
	WarningAt        int            `json:"warningAt"`
	CriticalAt       int            `json:"criticalAt"`
	Location         string         `json:"location"`
	Notes            string         `json:"notes"`
	ReorderURL       string         `json:"reorderUrl"`
	UnitCost         *float64       `json:"unitCost,omitempty"` // reference only
	CreatedAt        time.Time      `json:"createdAt"`
	UpdatedAt        time.Time      `json:"updatedAt"`
}

// StockStatus classifies on-hand stock against the item's thresholds.
type StockStatus string

const (
	StockOK       StockStatus = "ok"
	StockLow      StockStatus = "low"
	StockCritical StockStatus = "critical"
	StockOut      StockStatus = "out"
)

// Status derives the stock status: out at zero, critical/low at the thresholds.
func (c *Consumable) Status() StockStatus {
	switch {
	case c.OnHand <= 0:
		return StockOut
	case c.OnHand <= c.CriticalAt:
		return StockCritical
	case c.OnHand <= c.WarningAt:
		return StockLow
	}
	return StockOK
}

// Stock event kinds.
const (
	StockEventSet         = "set"         // operator set the quantity
	StockEventAdd         = "add"         // operator added units
	StockEventRemove      = "remove"      // operator removed units
	StockEventReplacement = "replacement" // a device's installed supply was replaced
	StockEventImport      = "import"      // CSV import
)

// Stock event statuses (replacement events only; others are always "applied").
const (
	StockEventApplied   = "applied"   // quantity changed
	StockEventPending   = "pending"   // detected; waiting for an operator to confirm the decrement
	StockEventLogged    = "logged"    // detected; auto-decrement is off so nothing changed
	StockEventDismissed = "dismissed" // operator declined the decrement
	StockEventConfirmed = "confirmed" // operator confirmed; the decrement is a separate applied event
)

// StockEvent is one entry in a consumable's stock history (consumables spec §2).
type StockEvent struct {
	ID           int64     `json:"id"`
	ConsumableID string    `json:"consumableId"`
	Actor        string    `json:"actor"`
	Kind         string    `json:"kind"`
	Delta        int       `json:"delta"`
	Resulting    int       `json:"resulting"`
	Reason       string    `json:"reason,omitempty"`
	DeviceID     string    `json:"deviceId,omitempty"`
	SupplyIndex  int       `json:"supplyIndex,omitempty"`
	Status       string    `json:"status"`
	At           time.Time `json:"at"`
}

// SupplyLink resolves one installed supply (a device slot) to a consumable
// stock item. Auto links are recomputed after every poll; manual links are
// operator-owned and never overwritten.
type SupplyLink struct {
	DeviceID     string    `json:"deviceId"`
	SupplyIndex  int       `json:"supplyIndex"`
	ConsumableID string    `json:"consumableId"`
	Source       string    `json:"source"` // auto | manual
	UpdatedAt    time.Time `json:"updatedAt"`
}

// AutoDecrement modes for inferred replacements (consumables spec §3).
const (
	AutoDecrementOff       = "off"
	AutoDecrementPrompt    = "prompt"
	AutoDecrementAutomatic = "automatic"
)

// ConsumableSettings are the operator-set defaults for the consumables feature.
type ConsumableSettings struct {
	DefaultWarningAt     int    `json:"defaultWarningAt"`
	DefaultCriticalAt    int    `json:"defaultCriticalAt"`
	AutoDecrement        string `json:"autoDecrement"`        // off | prompt | automatic
	ReplacementJumpPct   int    `json:"replacementJumpPct"`   // level rise that counts as a replacement
	ForecastMinDays      int    `json:"forecastMinDays"`      // minimum history before a forecast is given
	ReorderLeadDays      int    `json:"reorderLeadDays"`      // "reorder soon" when stock lasts no longer than this
	AutoSuggestFromScans bool   `json:"autoSuggestFromScans"` // offer unmatched installed supplies as new items
}

// DefaultConsumableSettings returns the defaults.
func DefaultConsumableSettings() ConsumableSettings {
	return ConsumableSettings{DefaultWarningAt: 2, DefaultCriticalAt: 1, AutoDecrement: AutoDecrementPrompt,
		ReplacementJumpPct: 30, ForecastMinDays: 3, ReorderLeadDays: 14, AutoSuggestFromScans: true}
}

// NotificationLog records one delivery attempt.
type NotificationLog struct {
	ID       int64     `json:"id"`
	Notifier string    `json:"notifier"`
	Event    string    `json:"event"`
	Target   string    `json:"target"`
	OK       bool      `json:"ok"`
	Error    string    `json:"error,omitempty"`
	At       time.Time `json:"at"`
}

// Event is published on the live stream (WebSocket) and to notifiers.
type Event struct {
	Type string    `json:"type"` // device.updated | device.discovered | job.updated | alert.raised | alert.cleared | alert.acked
	At   time.Time `json:"at"`
	Data any       `json:"data"`
}

// Percent computes a 0..100 percentage from a level and capacity following the
// Printer MIB conventions, returning nil when either is unknown.
func Percent(level, max int) *int {
	if level < 0 || max <= 0 {
		return nil
	}
	p := int(float64(level) * 100 / float64(max))
	if p > 100 {
		p = 100
	}
	return &p
}

// Outbox is one durable, retried notification delivery (one event × one
// destination). Deliveries survive restarts; the notifier worker drains them.
type Outbox struct {
	ID        int64      `json:"id"`
	EventID   string     `json:"eventId"`
	Event     string     `json:"event"`
	Notifier  string     `json:"notifier"`
	Subject   string     `json:"subject"`
	Body      string     `json:"body"`
	Payload   any        `json:"payload,omitempty"`
	State     string     `json:"state"` // pending | delivered | dead
	Attempts  int        `json:"attempts"`
	LastError string     `json:"lastError,omitempty"`
	NextAt    time.Time  `json:"nextAt"`
	CreatedAt time.Time  `json:"createdAt"`
	DoneAt    *time.Time `json:"doneAt,omitempty"`
}
