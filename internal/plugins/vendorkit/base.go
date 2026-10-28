package vendorkit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gosnmp/gosnmp"

	"printmarshal/internal/driver/ipp"
	"printmarshal/internal/driver/snmp"
	"printmarshal/internal/engine"
	"printmarshal/internal/model"
)

// ErrNotVendor is returned by Identify when the device is reachable but is not
// made by (or badged for) the plugin's manufacturer.
var ErrNotVendor = errors.New("device is not this plugin's vendor")

// Standard objects the base reads beyond the ones the SNMP driver exports.
const (
	OIDSysContact = ".1.3.6.1.2.1.1.4.0"
)

// Badge is a regional / OEM brand that resolves to the plugin's canonical
// vendor (e.g. Savin -> Ricoh). Match tokens are compared case-insensitively
// as a word prefix of the device description or the IPP manufacturer.
type Badge struct {
	Name  string
	Match []string
}

// Probe is what Identify learned about an address; vendor hooks read it and
// may issue further SNMP reads through it.
type Probe struct {
	Address string
	// Session is the open SNMP session, or nil when SNMP did not answer.
	Session snmp.Session
	Variant string // "v3" | "v2c" | "" (ipp only)

	SysDescr, SysObjectID, SysName, SysLocation, SysContact string
	Enterprise                                              int
	// DeviceDescr is hrDeviceDescr of the printer device, if any.
	DeviceDescr string
	// Serial is prtGeneralSerialNumber, if any.
	Serial string
	// IPP is the Get-Printer-Attributes response, if one was fetched.
	IPP *ipp.Message
	// Badge is the matched regional brand name ("" = the vendor itself).
	Badge string
}

// Get reads one scalar; ok is false when the object is absent.
func (p *Probe) Get(ctx context.Context, oid string) (snmp.PDU, bool) {
	if p.Session == nil {
		return snmp.PDU{}, false
	}
	res, err := p.Session.Get(ctx, []string{oid})
	if err != nil || len(res) == 0 || Absent(res[0]) {
		return snmp.PDU{}, false
	}
	return res[0], true
}

// GetStr reads one scalar as text ("" when absent).
func (p *Probe) GetStr(ctx context.Context, oid string) string {
	pdu, ok := p.Get(ctx, oid)
	if !ok {
		return ""
	}
	return snmp.Str(pdu)
}

// GetInt reads one numeric scalar.
func (p *Probe) GetInt(ctx context.Context, oid string) (int64, bool) {
	pdu, ok := p.Get(ctx, oid)
	if !ok {
		return 0, false
	}
	return snmp.Int(pdu)
}

// Walk returns the bindings under root (nil on error or when SNMP is absent).
func (p *Probe) Walk(ctx context.Context, root string) []snmp.PDU {
	if p.Session == nil {
		return nil
	}
	out, err := p.Session.Walk(ctx, root)
	if err != nil {
		return nil
	}
	return out
}

// Absent reports whether a PDU carries no value (NoSuchObject etc.).
func Absent(p snmp.PDU) bool {
	return p.Type == gosnmp.NoSuchObject || p.Type == gosnmp.NoSuchInstance || p.Type == gosnmp.EndOfMibView || p.Type == gosnmp.Null
}

// HTTPDoer is the subset of *http.Client vendor web-service readers need.
type HTTPDoer interface {
	Get(ctx context.Context, url string) ([]byte, error)
}

// Options are the transports a plugin uses. Tests inject fixture-backed ones.
type Options struct {
	SNMP snmp.Dialer
	// IPP is optional; when set, Identify falls back to Get-Printer-Attributes
	// for devices whose SNMP agent gives no vendor evidence.
	IPP          ipp.Transport
	IPPEndpoints []string
	// HTTP is optional; vendor web-service readers (e.g. HP LEDM) use it.
	HTTP HTTPDoer
}

// Spec configures a plugin. Vendor packages fill it and embed *Base.
type Spec struct {
	Name   string // registry name / slug, e.g. "ricoh"
	Vendor string // canonical make reported in Identity.Make, e.g. "Ricoh"
	// Enterprises are the IANA enterprise numbers whose sysObjectID this
	// plugin claims.
	Enterprises []int
	Badges      []Badge
	// IPPManufacturers are printer-device-id MFG values (case-insensitive).
	IPPManufacturers []string
	// Capabilities is the full declared set; Priorities overrides the defaults
	// (identify 60, read_config/read_security 20, everything else 55).
	Capabilities []model.Capability
	Priorities   map[model.Capability]int
	Evidence     []CapabilityEvidence
	Notes        string
	// Families are the model families the research inventory lists for this
	// vendor (with the catalogue group ids), for /api/drivers and the README.
	Families []string
	// Management records the documented management paths from the research
	// catalogue's management profile; it is context, not a capability claim.
	Management ManagementPaths

	// Enrich is called after the match with an open probe; it may fill
	// identity fields and append the capabilities the device answered for.
	Enrich func(ctx context.Context, p *Probe, id *model.Identity)
	// Config, when set, appends vendor settings to the base config document.
	Config func(ctx context.Context, b *Base, d *model.Device, creds model.Creds, doc *model.ConfigDocument) error
	// Family, when set, derives a product family from the model text for
	// vendor.<name>.family.
	Family func(model string) string
}

// Base implements engine.Driver, engine.Identifier, engine.ConfigReader and
// engine.SecurityReader for a vendor Spec.
type Base struct {
	Spec Spec
	Opts Options
}

// New returns a Base for spec.
func New(spec Spec, opts Options) *Base {
	return &Base{Spec: spec, Opts: opts}
}

// Name implements engine.Driver.
func (b *Base) Name() string { return b.Spec.Name }

// Capabilities implements engine.Driver.
func (b *Base) Capabilities() []model.Capability {
	out := make([]model.Capability, len(b.Spec.Capabilities))
	copy(out, b.Spec.Capabilities)
	return out
}

// Supports implements engine.Driver with the standard profile rule.
func (b *Base) Supports(d *model.Device, cap model.Capability) bool {
	return engine.ProfileSupports(d, b.Spec.Name, cap)
}

// Priority implements engine.Driver.
func (b *Base) Priority(cap model.Capability) int {
	if p, ok := b.Spec.Priorities[cap]; ok {
		return p
	}
	switch cap {
	case model.CapIdentify:
		return 60
	case model.CapReadConfig, model.CapReadSecurity:
		return 20
	}
	return 55
}

// Descriptor returns the plugin's description for /api/drivers.
func (b *Base) Descriptor() Descriptor {
	d := Descriptor{Name: b.Spec.Name, Vendor: b.Spec.Vendor, Enterprises: b.Spec.Enterprises, Notes: b.Spec.Notes,
		Families: b.Spec.Families, Management: b.Spec.Management}
	for _, bd := range b.Spec.Badges {
		d.Badges = append(d.Badges, bd.Name)
	}
	d.Capabilities = append(d.Capabilities, b.Spec.Evidence...)
	return d
}

// Dial opens an SNMP session for a device.
func (b *Base) Dial(ctx context.Context, address string, creds model.Creds) (snmp.Session, string, error) {
	if b.Opts.SNMP == nil {
		return nil, "", errors.New("snmp transport not configured")
	}
	return b.Opts.SNMP(ctx, address, creds)
}

// Identify implements engine.Identifier. It returns ErrNotVendor for devices
// of another manufacturer so the engine never records this plugin against them.
func (b *Base) Identify(ctx context.Context, address string, creds model.Creds) (*model.Identity, error) {
	p := &Probe{Address: address}
	var dialErr error
	if b.Opts.SNMP != nil {
		s, variant, err := b.Opts.SNMP(ctx, address, creds)
		if err == nil {
			p.Session, p.Variant = s, variant
			defer s.Close()
			b.readSystem(ctx, p)
		} else {
			dialErr = err
		}
	}

	matched := false
	if p.Session != nil {
		matched = b.matchSNMP(p)
		if !matched && p.Enterprise != 0 {
			return nil, fmt.Errorf("%w: sysObjectID enterprise %d is not %s", ErrNotVendor, p.Enterprise, b.Spec.Vendor)
		}
	}
	if !matched && b.Opts.IPP != nil {
		if m := b.fetchIPP(ctx, address); m != nil {
			p.IPP = m
			matched = b.matchIPP(p)
		}
	}
	if !matched {
		if p.Session == nil && p.IPP == nil {
			if dialErr != nil {
				return nil, fmt.Errorf("no snmp/ipp answer: %w", dialErr)
			}
			return nil, errors.New("no snmp/ipp answer")
		}
		return nil, fmt.Errorf("%w: no %s evidence in sysDescr/hrDeviceDescr/IPP", ErrNotVendor, b.Spec.Vendor)
	}

	id := &model.Identity{Address: address, Method: b.Spec.Name, Make: b.Spec.Vendor, DeviceType: "printer"}
	if p.Variant != "" {
		id.Method += "/" + p.Variant
	} else if p.IPP != nil {
		id.Method += "/ipp"
	}
	b.fillFromProbe(ctx, p, id)
	id.Capabilities = []model.Capability{model.CapIdentify, model.CapReadConfig, model.CapReadSecurity}
	if b.Spec.Enrich != nil {
		b.Spec.Enrich(ctx, p, id)
	}
	id.Capabilities = intersect(id.Capabilities, b.Spec.Capabilities)
	return id, nil
}

func (b *Base) readSystem(ctx context.Context, p *Probe) {
	res, err := p.Session.Get(ctx, []string{snmp.OIDSysDescr, snmp.OIDSysObjectID, snmp.OIDSysName, snmp.OIDSysLocation, OIDSysContact})
	if err != nil {
		return
	}
	for _, pdu := range res {
		if Absent(pdu) {
			continue
		}
		switch pdu.OID {
		case snmp.OIDSysDescr:
			p.SysDescr = snmp.Str(pdu)
		case snmp.OIDSysObjectID:
			p.SysObjectID = snmp.Str(pdu)
			p.Enterprise = snmp.EnterpriseID(p.SysObjectID)
		case snmp.OIDSysName:
			p.SysName = snmp.Str(pdu)
		case snmp.OIDSysLocation:
			p.SysLocation = snmp.Str(pdu)
		case OIDSysContact:
			p.SysContact = snmp.Str(pdu)
		}
	}
	types := p.Walk(ctx, snmp.OIDHrDeviceType)
	for _, t := range types {
		if snmp.Str(t) == snmp.OIDHrDevicePrinterType {
			idx := strings.TrimPrefix(t.OID, snmp.OIDHrDeviceType+".")
			p.DeviceDescr = p.GetStr(ctx, snmp.OIDHrDeviceDescr+"."+idx)
			break
		}
	}
	for _, pdu := range p.Walk(ctx, snmp.OIDPrtGeneralSerialNumber) {
		if v := snmp.Str(pdu); v != "" {
			p.Serial = v
			break
		}
	}
}

// matchSNMP decides from the enterprise number first, then from badge text
// when the agent reports no enterprise at all.
func (b *Base) matchSNMP(p *Probe) bool {
	for _, e := range b.Spec.Enterprises {
		if p.Enterprise == e {
			p.Badge = b.badgeFor(p.DeviceDescr, p.SysDescr)
			return true
		}
	}
	if p.Enterprise != 0 {
		return false
	}
	if bd, ok := b.badgeMatch(p.DeviceDescr, p.SysDescr); ok {
		p.Badge = bd
		return true
	}
	return false
}

func (b *Base) matchIPP(p *Probe) bool {
	devID := ParseDeviceID(p.IPP.String("printer-device-id"))
	mfg := devID["MFG"]
	mm := p.IPP.String("printer-make-and-model")
	for _, m := range b.Spec.IPPManufacturers {
		if strings.EqualFold(strings.TrimSpace(mfg), m) {
			p.Badge = b.badgeFor(mm, mfg)
			return true
		}
	}
	if bd, ok := b.badgeMatch(mfg, mm); ok {
		p.Badge = bd
		return true
	}
	return false
}

// badgeMatch returns the matching badge name for any of the texts, or the
// vendor's own name when the vendor name itself leads the text.
func (b *Base) badgeMatch(texts ...string) (string, bool) {
	for _, t := range texts {
		if wordPrefix(t, b.Spec.Vendor) {
			return "", true
		}
		for _, bd := range b.Spec.Badges {
			for _, m := range bd.Match {
				if wordPrefix(t, m) {
					return bd.Name, true
				}
			}
		}
	}
	return "", false
}

// badgeFor returns the regional badge visible in the texts ("" = vendor name).
func (b *Base) badgeFor(texts ...string) string {
	bd, _ := b.badgeMatch(texts...)
	return bd
}

// wordPrefix reports whether text starts with token as a whole word, ignoring case.
func wordPrefix(text, token string) bool {
	t := strings.TrimSpace(strings.ToLower(text))
	tok := strings.ToLower(token)
	if tok == "" || !strings.HasPrefix(t, tok) {
		return false
	}
	if len(t) == len(tok) {
		return true
	}
	c := t[len(tok)]
	return c == ' ' || c == '-' || c == '_' || c == ',' || c == ';' || c == '/' || c == '('
}

func (b *Base) fillFromProbe(ctx context.Context, p *Probe, id *model.Identity) {
	id.SysDescr = p.SysDescr
	id.SysName = p.SysName
	id.SysLocation = p.SysLocation
	id.EnterpriseID = p.Enterprise
	id.Serial = p.Serial
	_, m := snmp.MakeModel(p.SysDescr)
	id.Model = firstNonEmpty(p.DeviceDescr, m)
	if p.Session != nil {
		for _, pdu := range p.Walk(ctx, snmp.OIDIfPhysAddress) {
			if mac := MAC(pdu); mac != "" {
				id.MAC = mac
				break
			}
		}
	}
	if p.IPP != nil {
		devID := ParseDeviceID(p.IPP.String("printer-device-id"))
		id.Model = firstNonEmpty(id.Model, p.IPP.String("printer-make-and-model"), devID["MDL"])
		id.Serial = firstNonEmpty(id.Serial, p.IPP.String("printer-serial-number"), devID["SN"], devID["SERN"])
		id.UUID = strings.TrimPrefix(p.IPP.String("printer-uuid"), "urn:uuid:")
		id.Firmware = firstNonEmpty(p.IPP.String("printer-firmware-string-version"), p.IPP.String("printer-firmware-name"))
		id.SysName = firstNonEmpty(id.SysName, p.IPP.String("printer-dns-sd-name"), p.IPP.String("printer-name"))
		id.SysLocation = firstNonEmpty(id.SysLocation, p.IPP.String("printer-location"))
		id.AdminURL = p.IPP.String("printer-more-info")
		if strings.Contains(strings.ToLower(devID["CMD"]), "scan") {
			id.DeviceType = "mfp"
		}
	}
	if looksLikeMFP(id.Model) || looksLikeMFP(p.SysDescr) {
		id.DeviceType = "mfp"
	}
}

func looksLikeMFP(s string) bool {
	u := strings.ToUpper(s)
	for _, k := range []string{"MFP", "MULTIFUNCTION", " MFC", "SCANNER", "FACSIMILE", "COPIER"} {
		if strings.Contains(u, k) {
			return true
		}
	}
	return false
}

// MAC renders an ifPhysAddress binding as aa:bb:cc:dd:ee:ff ("" if not a usable MAC).
func MAC(p snmp.PDU) string {
	switch v := p.Value.(type) {
	case []byte:
		if len(v) == 6 && !allZero(v) {
			parts := make([]string, 6)
			for i, x := range v {
				parts[i] = fmt.Sprintf("%02x", x)
			}
			return strings.Join(parts, ":")
		}
	case string:
		// Some agents (and net-snmp text) render the MAC as text already.
		parts := strings.Split(strings.TrimSpace(v), ":")
		if len(parts) == 6 {
			var out []string
			for _, x := range parts {
				if len(x) == 0 || len(x) > 2 {
					return ""
				}
				if len(x) == 1 {
					x = "0" + x
				}
				out = append(out, strings.ToLower(x))
			}
			mac := strings.Join(out, ":")
			if mac != "00:00:00:00:00:00" {
				return mac
			}
		}
	}
	return ""
}

func allZero(b []byte) bool {
	for _, x := range b {
		if x != 0 {
			return false
		}
	}
	return true
}

// ParseDeviceID parses an IEEE 1284 device id ("MFG:X;MDL:Y;CMD:..;SN:..").
func ParseDeviceID(s string) map[string]string {
	out := map[string]string{}
	for _, kv := range strings.Split(s, ";") {
		k, v, ok := strings.Cut(kv, ":")
		if !ok {
			continue
		}
		k = strings.ToUpper(strings.TrimSpace(k))
		switch k {
		case "MANUFACTURER":
			k = "MFG"
		case "MODEL":
			k = "MDL"
		case "COMMAND SET":
			k = "CMD"
		}
		out[k] = strings.TrimSpace(v)
	}
	return out
}

// identifyAttrs is the small attribute set the base asks for over IPP.
var identifyAttrs = []string{
	"printer-make-and-model", "printer-device-id", "printer-uuid", "printer-serial-number",
	"printer-firmware-name", "printer-firmware-string-version", "printer-name", "printer-dns-sd-name",
	"printer-location", "printer-more-info",
}

var defaultIPPEndpoints = []string{
	"http://%s:631/ipp/print",
	"http://%s:631/ipp",
	"http://%s:631/",
}

var ippRequestID atomic.Uint32

func (b *Base) fetchIPP(ctx context.Context, address string) *ipp.Message {
	eps := b.Opts.IPPEndpoints
	if len(eps) == 0 {
		eps = defaultIPPEndpoints
	}
	for _, ep := range eps {
		url := fmt.Sprintf(ep, address)
		uri := "ipp://" + strings.TrimPrefix(strings.TrimPrefix(url, "http://"), "https://")
		req := ipp.NewGetPrinterAttributes(uri, ippRequestID.Add(1), identifyAttrs)
		body, err := ipp.Encode(req)
		if err != nil {
			return nil
		}
		resp, err := b.Opts.IPP.Do(ctx, url, body)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			continue
		}
		msg, err := ipp.Decode(resp)
		if err != nil || msg.StatusCode >= 0x0400 {
			continue
		}
		return msg
	}
	return nil
}

// --- config documents ----------------------------------------------------

// ReadConfig implements engine.ConfigReader: identity.* from the SNMP system
// group plus vendor.<name>.* facts, then whatever Spec.Config adds.
func (b *Base) ReadConfig(ctx context.Context, d *model.Device, creds model.Creds, sel model.ConfigSelector) (*model.ConfigDocument, error) {
	doc := &model.ConfigDocument{DeviceID: d.ID, ReadAt: time.Now().UTC(), Sources: []string{b.Spec.Name}}
	s, _, err := b.Dial(ctx, d.Address, creds)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	p := &Probe{Address: d.Address, Session: s}
	b.readSystem(ctx, p)
	if !b.matchSNMP(p) {
		return nil, fmt.Errorf("%w: refusing to read config", ErrNotVendor)
	}
	ro := "vendor write path not documented; read-only"
	add := func(key string, value any) {
		def, ok := model.CatalogByKey[key]
		s := model.Setting{Key: key, Value: value, Source: b.Spec.Name, Writable: false, ReadOnlyReason: ro}
		if ok {
			s.Area, s.Name, s.Type, s.Options = def.Area, def.Name, def.Type, def.Options
		} else {
			s.Area, s.Name, s.Type = "other", key, typeOf(value)
		}
		doc.Settings = append(doc.Settings, s)
	}
	add("identity.hostname", p.SysName)
	add("identity.location", p.SysLocation)
	add("identity.contact", p.SysContact)
	prefix := "vendor." + b.Spec.Name + "."
	badge := p.Badge
	if badge == "" {
		badge = b.Spec.Vendor
	}
	add(prefix+"badge", badge)
	add(prefix+"sysObjectId", p.SysObjectID)
	if b.Spec.Family != nil {
		if fam := b.Spec.Family(firstNonEmpty(p.DeviceDescr, d.Model)); fam != "" {
			add(prefix+"family", fam)
		}
	}
	if b.Spec.Config != nil {
		if err := b.Spec.Config(ctx, b, d, creds, doc); err != nil {
			return nil, err
		}
	}
	doc.Settings = Filter(doc.Settings, sel)
	return doc, nil
}

// ReadSecurity implements engine.SecurityReader: the config document
// restricted to security-relevant keys.
func (b *Base) ReadSecurity(ctx context.Context, d *model.Device, creds model.Creds) (*model.ConfigDocument, error) {
	doc, err := b.ReadConfig(ctx, d, creds, model.ConfigSelector{})
	if err != nil {
		return nil, err
	}
	var out []model.Setting
	for _, s := range doc.Settings {
		if def, ok := model.CatalogByKey[s.Key]; ok && def.Security {
			out = append(out, s)
			continue
		}
		if s.Area == "security" || s.Area == "protocols" {
			out = append(out, s)
		}
	}
	doc.Settings = out
	return doc, nil
}

// Filter applies a ConfigSelector to settings.
func Filter(in []model.Setting, sel model.ConfigSelector) []model.Setting {
	if len(sel.Areas) == 0 && len(sel.Keys) == 0 {
		return in
	}
	var out []model.Setting
	for _, s := range in {
		keep := false
		for _, a := range sel.Areas {
			if s.Area == a {
				keep = true
			}
		}
		for _, k := range sel.Keys {
			if s.Key == k {
				keep = true
			}
		}
		if keep {
			out = append(out, s)
		}
	}
	return out
}

// AddSetting appends a canonical-catalogue setting (or a vendor.* key) to doc.
func AddSetting(doc *model.ConfigDocument, source, key string, value any, writable bool, roReason string) {
	def, ok := model.CatalogByKey[key]
	s := model.Setting{Key: key, Value: value, Source: source, Writable: writable, ReadOnlyReason: roReason}
	if ok {
		s.Area, s.Name, s.Type, s.Options = def.Area, def.Name, def.Type, def.Options
	} else {
		s.Area, s.Name, s.Type = "other", key, typeOf(value)
		if strings.HasPrefix(key, "vendor.") {
			s.Name = key
		}
	}
	if writable {
		s.ReadOnlyReason = ""
	}
	doc.Settings = append(doc.Settings, s)
}

func typeOf(v any) string {
	switch v.(type) {
	case bool:
		return "bool"
	case int, int64, int32:
		return "int"
	case []string, []any:
		return "list"
	}
	return "string"
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func intersect(have, declared []model.Capability) []model.Capability {
	var out []model.Capability
	for _, h := range have {
		for _, d := range declared {
			if h == d {
				out = append(out, h)
				break
			}
		}
	}
	return out
}

var _ engine.Driver = (*Base)(nil)
var _ engine.Identifier = (*Base)(nil)
var _ engine.ConfigReader = (*Base)(nil)
var _ engine.SecurityReader = (*Base)(nil)
