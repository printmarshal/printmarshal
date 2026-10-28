package hp

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"printmarshal/internal/model"
	"printmarshal/internal/plugins/vendorkit"
)

// LEDM ("Low-End Data Model") is the XML-over-HTTP device web service HP
// LaserJet Pro / OfficeJet class devices expose under /DevMgmt. The pages
// below were fetched read-only from an HP Color LaserJet MFP M283fdw during
// this build (testdata/fixtures/hp-m283fdw-ledm/*.xml); the plugin only ever
// issues HTTP GET. The service advertises its own resource list at
// /DevMgmt/DiscoveryTree.xml, which the plugin consults before reading.
const (
	ledmDiscoveryTree    = "/DevMgmt/DiscoveryTree.xml"
	ledmProductConfig    = "/DevMgmt/ProductConfigDyn.xml"
	ledmProductUsage     = "/DevMgmt/ProductUsageDyn.xml"
	ledmProductStatus    = "/DevMgmt/ProductStatusDyn.xml"
	ledmNetApps          = "/DevMgmt/NetAppsDyn.xml"
	ledmNetAppsSecure    = "/DevMgmt/NetAppsSecureDyn.xml"
	ledmSecurity         = "/DevMgmt/SecurityDyn.xml"
	ledmAdapters         = "/IoMgmt/Adapters"
	ledmConsumableConfig = "/DevMgmt/ConsumableConfigDyn.xml"
)

// xnode is a namespace-agnostic XML tree node.
type xnode struct {
	Name     string
	Text     string
	Attrs    map[string]string
	Children []*xnode
}

// parseXML builds an xnode tree using local names only (LEDM namespaces
// differ between page revisions).
func parseXML(data []byte) (*xnode, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	root := &xnode{Name: "#root"}
	stack := []*xnode{root}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &xnode{Name: t.Name.Local, Attrs: map[string]string{}}
			for _, a := range t.Attr {
				n.Attrs[a.Name.Local] = a.Value
			}
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, n)
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			cur := stack[len(stack)-1]
			cur.Text += string(t)
		}
	}
	if len(root.Children) == 0 {
		return nil, errors.New("empty document")
	}
	return root, nil
}

// find returns the first descendant reached by the local-name path.
func (n *xnode) find(path ...string) *xnode {
	cur := n
	for _, name := range path {
		var next *xnode
		for _, c := range cur.Children {
			if c.Name == name {
				next = c
				break
			}
		}
		if next == nil {
			return nil
		}
		cur = next
	}
	return cur
}

// all returns every descendant (depth-first) with the given local name.
func (n *xnode) all(name string) []*xnode {
	var out []*xnode
	var walk func(*xnode)
	walk = func(x *xnode) {
		for _, c := range x.Children {
			if c.Name == name {
				out = append(out, c)
			}
			walk(c)
		}
	}
	walk(n)
	return out
}

// text returns the trimmed text at path ("" when absent).
func (n *xnode) text(path ...string) string {
	x := n.find(path...)
	if x == nil {
		return ""
	}
	return strings.TrimSpace(x.Text)
}

// first returns the trimmed text of the first descendant named name.
func (n *xnode) first(name string) string {
	for _, x := range n.all(name) {
		return strings.TrimSpace(x.Text)
	}
	return ""
}

// ledmClient fetches LEDM pages over the plugin's HTTP doer, trying http then
// https for the device address.
type ledmClient struct {
	http    vendorkit.HTTPDoer
	address string
}

func (c *ledmClient) get(ctx context.Context, path string) (*xnode, error) {
	if c.http == nil {
		return nil, errors.New("http transport not configured")
	}
	var errs []error
	for _, scheme := range []string{"http", "https"} {
		data, err := c.http.Get(ctx, scheme+"://"+c.address+path)
		if err != nil {
			errs = append(errs, err)
			if ctx.Err() != nil {
				break
			}
			continue
		}
		n, err := parseXML(data)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		return n, nil
	}
	return nil, errors.Join(errs...)
}

// resources returns the resource URIs the device advertises (nil if LEDM is absent).
func (c *ledmClient) resources(ctx context.Context) map[string]bool {
	tree, err := c.get(ctx, ledmDiscoveryTree)
	if err != nil {
		return nil
	}
	out := map[string]bool{}
	for _, r := range tree.all("ResourceURI") {
		out[strings.TrimSpace(r.Text)] = true
	}
	return out
}

// --- usage ---------------------------------------------------------------

// LEDMUsage is the PrinterSubunit block of ProductUsageDyn.xml.
type LEDMUsage struct {
	Total, Mono, Color, DuplexSheets int64
	HasMonoColor                     bool
}

// ParseLEDMUsage parses ProductUsageDyn.xml.
func ParseLEDMUsage(data []byte) (*LEDMUsage, error) {
	n, err := parseXML(data)
	if err != nil {
		return nil, err
	}
	ps := n.find("ProductUsageDyn", "PrinterSubunit")
	if ps == nil {
		return nil, errors.New("no PrinterSubunit")
	}
	u := &LEDMUsage{}
	u.Total, _ = strconv.ParseInt(ps.text("TotalImpressions"), 10, 64)
	mono, errM := strconv.ParseInt(ps.text("MonochromeImpressions"), 10, 64)
	color, errC := strconv.ParseInt(ps.text("ColorImpressions"), 10, 64)
	if errM == nil && errC == nil {
		u.Mono, u.Color, u.HasMonoColor = mono, color, true
	}
	u.DuplexSheets, _ = strconv.ParseInt(ps.text("DuplexSheets"), 10, 64)
	return u, nil
}

// --- status --------------------------------------------------------------

// statusCategories translates LEDM StatusCategory / alert ids. "ready" and
// "genuineHP" were observed on the M283fdw; the rest are the category names
// HP's own GPL client (HPLIP, ledm.py) handles and are listed for readability
// only — an unknown id is still shown, humanised.
var statusCategories = map[string]string{
	"ready":                  "Ready",
	"processing":             "Processing",
	"inPowerSave":            "Power save",
	"initializing":           "Initialising",
	"inkLow":                 "Supply low",
	"inkVeryLow":             "Supply very low",
	"inkOut":                 "Supply empty",
	"trayEmptyOrOpen":        "Tray empty or open",
	"jamInPrinter":           "Paper jam",
	"closeDoorOrCover":       "Door or cover open",
	"cartridgeMissing":       "Cartridge missing",
	"cartridgeNotRecognized": "Cartridge not recognised",
	"nonHPInk":               "Non-HP supply installed",
	"genuineHP":              "Genuine HP supply installed",
	"outputBinFull":          "Output bin full",
	"manuallyFeed":           "Manual feed required",
	"sizeMismatch":           "Media size mismatch",
	"scannerError":           "Scanner error",
	"inkSystemFailure":       "Supply system failure",
	"shuttingDown":           "Shutting down",
}

// LEDMStatus is ProductStatusDyn.xml reduced to what the engine needs.
type LEDMStatus struct {
	Category string
	Message  string
	Alerts   []model.DeviceAlert
}

// ParseLEDMStatus parses ProductStatusDyn.xml and translates alert ids.
func ParseLEDMStatus(data []byte) (*LEDMStatus, error) {
	n, err := parseXML(data)
	if err != nil {
		return nil, err
	}
	return statusFromNode(n), nil
}

func statusFromNode(n *xnode) *LEDMStatus {
	st := &LEDMStatus{Category: n.first("StatusCategory"), Message: n.first("LocString")}
	if st.Message == "" {
		st.Message = TranslateHPCode(st.Category)
	}
	for i, a := range n.all("Alert") {
		id := a.first("ProductStatusAlertID")
		da := model.DeviceAlert{Index: i + 1, Code: id, Description: TranslateHPCode(id), Severity: model.SeverityOK, Group: "hp-ledm"}
		if seq, err := strconv.Atoi(a.first("SequenceNumber")); err == nil {
			da.GroupIndex = seq
		}
		switch strings.ToLower(a.first("Severity")) {
		case "error", "critical", "fatal":
			da.Severity = model.SeverityCritical
		case "warning":
			da.Severity = model.SeverityWarning
		}
		var details []string
		if c := a.first("AlertDetailsMarkerColor"); c != "" {
			details = append(details, c)
		}
		if t := a.first("AlertDetailsConsumableTypeEnum"); t != "" {
			details = append(details, t)
		}
		if len(details) > 0 {
			da.Description += " (" + strings.Join(details, " ") + ")"
		}
		st.Alerts = append(st.Alerts, da)
	}
	return st
}

// TranslateHPCode turns an HP LEDM status/alert id into readable text.
func TranslateHPCode(code string) string {
	if t, ok := statusCategories[code]; ok {
		return t
	}
	if code == "" {
		return "Unknown"
	}
	var b strings.Builder
	for i, r := range code {
		if i > 0 && unicode.IsUpper(r) {
			b.WriteRune(' ')
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		if i == 0 {
			b.WriteRune(unicode.ToUpper(r))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// --- configuration ---------------------------------------------------------

var durationRe = regexp.MustCompile(`^(\d+)\s*(minute|minutes|min|hour|hours|hr)$`)

// ledmMinutes converts "1minute" / "15minutes" / "4hours" to minutes.
func ledmMinutes(s string) (int, bool) {
	m := durationRe.FindStringSubmatch(strings.ToLower(strings.TrimSpace(s)))
	if m == nil {
		return 0, false
	}
	n, _ := strconv.Atoi(m[1])
	if strings.HasPrefix(m[2], "h") {
		n *= 60
	}
	return n, true
}

func enabled(s string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "enabled", "on", "true", "yes":
		return true, true
	case "disabled", "off", "false", "no":
		return false, true
	}
	return false, false
}

// LEDMPages are the parsed documents the config reader consumes.
type LEDMPages struct {
	ProductConfig, NetApps, NetAppsSecure, Security, Adapters *xnode
}

// ConfigFromLEDM maps the LEDM pages onto canonical catalogue keys plus
// vendor.hp.* facts. Everything is reported read-only: the LEDM PUT path is
// not documented in the research and is not implemented.
func ConfigFromLEDM(pages LEDMPages, doc *model.ConfigDocument) {
	const ro = "LEDM write path not documented; read-only"
	add := func(key string, v any) { vendorkit.AddSetting(doc, Name, key, v, false, ro) }
	addBool := func(key, raw string) {
		if b, ok := enabled(raw); ok {
			add(key, b)
		}
	}
	if pc := pages.ProductConfig; pc != nil {
		if v := pc.first("AssetNumber"); v != "" {
			add("identity.assetNumber", v)
		}
		if v := pc.first("PasswordStatus"); v != "" {
			add("security.adminPassword.set", !strings.EqualFold(v, "notSet"))
		}
		if m, ok := ledmMinutes(pc.first("PowerSaveTimeout")); ok {
			add("energy.sleepDelayMinutes", m)
		}
		if apply, ok := enabled(pc.first("ShutDownDelayApply")); ok && apply {
			if m, ok := ledmMinutes(pc.first("ShutDownDelay")); ok {
				add("energy.autoOffMinutes", m)
			}
		}
		if v := pc.first("DeviceLanguage"); v != "" {
			add("panel.language", v)
		}
		addBool("print.defaultDuplex", pc.first("Duplex"))
		if v := pc.first("ProductNumber"); v != "" {
			add("vendor.hp.productNumber", v)
		}
		if v := pc.first("ServiceID"); v != "" {
			add("vendor.hp.serviceId", v)
		}
		if v := pc.text("ProductConfigDyn", "ProductInformation", "Version", "Date"); v != "" {
			add("vendor.hp.firmwareDate", v)
		}
		if v := pc.first("MakeAndModelFamily"); v != "" {
			add("vendor.hp.modelFamily", v)
		}
		addBool("vendor.hp.powerSave", pc.first("PowerSave"))
		if v := pc.first("DuplexUnit"); v != "" {
			add("vendor.hp.duplexUnit", v)
		}
		if v := pc.first("CountryAndRegionName"); v != "" {
			add("vendor.hp.countryRegion", v)
		}
	}
	if na := pages.NetApps; na != nil {
		if sn := na.find("NetAppsDyn", "SNMPConfigWithVersion"); sn != nil {
			if on, ok := enabled(sn.text("SNMP")); ok {
				add("protocols.snmp.v1v2cEnabled", on)
			}
			if v := sn.text("AccessOption"); v != "" {
				add("protocols.snmp.v2cWriteEnabled", strings.EqualFold(v, "readWrite"))
			}
			if v := sn.text("GetCommunityNameConfig"); v != "" {
				add("vendor.hp.snmpGetCommunityPolicy", v)
			}
		}
		addBool("protocols.mdns.enabled", na.first("MDNSSupport"))
		addBool("protocols.wsd.enabled", na.first("WSDiscovery"))
		addBool("protocols.lpd.enabled", na.first("LPD"))
		addBool("protocols.llmnr.enabled", na.first("LLMNR"))
		addBool("protocols.slp.enabled", na.first("SLPSupport"))
		addBool("protocols.https.redirect", na.first("HTTPSRedirection"))
		addBool("protocols.raw9100.enabled", na.first("Port9100PrintingSupport"))
		addBool("protocols.ipp.enabled", na.first("IPPSupport"))
		if dhcp, ok := enabled(na.first("DHCPv4")); ok {
			if dhcp {
				add("network.ipv4.mode", "dhcp")
			} else {
				add("network.ipv4.mode", "static")
			}
		}
		if v := na.first("DNSSDPrintPreferredService"); v != "" {
			add("vendor.hp.dnssdPreferredService", v)
		}
	}
	if ns := pages.NetAppsSecure; ns != nil {
		addBool("network.wifiDirect.enabled", ns.first("WifiDirectDefault"))
	}
	if sec := pages.Security; sec != nil {
		if v := sec.first("EncryptionStrength"); v != "" {
			add("vendor.hp.encryptionStrength", v)
		}
	}
	if ad := pages.Adapters; ad != nil {
		for _, hw := range ad.all("HardwareConfig") {
			name := hw.text("Name")
			port := hw.text("DeviceConnectivityPortType")
			if port == "wifiEmbedded" && (name == "wifi0" || strings.HasPrefix(name, "wlan")) {
				if on, ok := enabled(hw.text("Power")); ok {
					add("network.wifi.enabled", on)
				}
				if enc := hw.first("EncryptionType"); enc != "" {
					add("vendor.hp.wifi.encryption", enc)
				}
			}
			if port == "ethernet" {
				if c, ok := enabled(hw.text("IsConnected")); ok {
					add("vendor.hp.ethernet.connected", c)
				}
			}
		}
	}
}

// readLEDMConfig fetches the LEDM pages a config read needs. Missing pages
// are tolerated; an unreachable web service is an error.
func readLEDMConfig(ctx context.Context, c *ledmClient) (LEDMPages, error) {
	res := c.resources(ctx)
	if res == nil {
		return LEDMPages{}, fmt.Errorf("LEDM discovery tree not available on %s", c.address)
	}
	var pages LEDMPages
	fetch := func(path string) *xnode {
		if !res[path] {
			return nil
		}
		n, err := c.get(ctx, path)
		if err != nil {
			return nil
		}
		return n
	}
	pages.ProductConfig = fetch(ledmProductConfig)
	pages.NetApps = fetch(ledmNetApps)
	pages.Security = fetch(ledmSecurity)
	// NetAppsSecureDyn and /IoMgmt/Adapters are not listed in the discovery
	// tree but answered on the M283fdw; a 404 is simply tolerated.
	if n, err := c.get(ctx, ledmNetAppsSecure); err == nil {
		pages.NetAppsSecure = n
	}
	if n, err := c.get(ctx, ledmAdapters); err == nil {
		pages.Adapters = n
	}
	return pages, nil
}
