package ipp

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"printmarshal/internal/driver/snmp"
	"printmarshal/internal/engine"
	"printmarshal/internal/model"
)

// Name is the driver's registry name.
const Name = "ipp"

// Transport posts an IPP request body to an HTTP(S) URL and returns the response body.
type Transport interface {
	Do(ctx context.Context, url string, body []byte) ([]byte, error)
}

// Options tune the driver.
type Options struct {
	Timeout time.Duration
	// InsecureTLS accepts printers' self-signed certificates. Reads only.
	InsecureTLS bool
	// Endpoints are candidate paths tried in order; "" means the default list.
	Endpoints []string
}

// DefaultOptions are suitable for LAN printers.
func DefaultOptions() Options {
	return Options{Timeout: 4 * time.Second, InsecureTLS: true}
}

var defaultEndpoints = []string{
	"http://%s:631/ipp/print",
	"http://%s:631/ipp",
	"http://%s:631/ipp/printer",
	"http://%s:631/",
	"https://%s:443/ipp/print",
	"http://%s:80/ipp/print",
}

type httpTransport struct{ c *http.Client }

func (t *httpTransport) Do(ctx context.Context, url string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/ipp")
	req.Header.Set("Accept", "application/ipp")
	resp, err := t.c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/ipp") {
		return nil, fmt.Errorf("unexpected content-type %q", ct)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
}

// NewTransport returns the production HTTP transport.
func NewTransport(opts Options) Transport {
	tr := &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: opts.InsecureTLS}, //nolint:gosec // printers use self-signed certs; reads only
		DisableKeepAlives: true,
		DialContext:       (&net.Dialer{Timeout: opts.Timeout}).DialContext,
	}
	return &httpTransport{c: &http.Client{Transport: tr, Timeout: opts.Timeout}}
}

// Driver implements engine.Driver over IPP.
type Driver struct {
	tr        Transport
	endpoints []string
	reqID     atomic.Uint32
}

// New returns a driver using the given transport.
func New(tr Transport, opts Options) *Driver {
	eps := opts.Endpoints
	if len(eps) == 0 {
		eps = defaultEndpoints
	}
	return &Driver{tr: tr, endpoints: eps}
}

func (d *Driver) Name() string { return Name }

func (d *Driver) Capabilities() []model.Capability {
	return []model.Capability{
		model.CapIdentify, model.CapReadStatus, model.CapReadSupplies,
		model.CapReadTrays, model.CapReadCounters, model.CapFirmwareRead,
	}
}

func (d *Driver) Supports(dev *model.Device, cap model.Capability) bool {
	return engine.ProfileSupports(dev, Name, cap)
}

func (d *Driver) Priority(cap model.Capability) int {
	switch cap {
	case model.CapFirmwareRead:
		return 50
	case model.CapIdentify, model.CapReadStatus:
		return 45
	case model.CapReadSupplies:
		return 40
	}
	return 30
}

// RequestedAttrs is the attribute set the driver asks printers for.
var RequestedAttrs = []string{
	"printer-make-and-model", "printer-name", "printer-info", "printer-location", "printer-uuid",
	"printer-device-id", "printer-serial-number", "printer-firmware-name", "printer-firmware-string-version",
	"printer-more-info", "printer-dns-sd-name", "printer-state", "printer-state-reasons",
	"printer-state-message", "printer-up-time", "printer-alert", "printer-alert-description",
	"marker-names", "marker-types", "marker-colors", "marker-levels", "marker-high-levels", "marker-low-levels",
	"printer-supply", "printer-supply-description", "printer-input-tray", "printer-output-tray",
	"printer-impressions-completed", "media-ready", "printer-kind",
}

// fetch tries each endpoint and returns the first successful response.
func (d *Driver) fetch(ctx context.Context, address string) (*Message, string, error) {
	var errs []error
	for _, ep := range d.endpoints {
		url := fmt.Sprintf(ep, address)
		uri := "ipp://" + strings.TrimPrefix(strings.TrimPrefix(url, "http://"), "https://")
		req := NewGetPrinterAttributes(uri, d.reqID.Add(1), RequestedAttrs)
		body, err := Encode(req)
		if err != nil {
			return nil, "", err
		}
		resp, err := d.tr.Do(ctx, url, body)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", url, err))
			if ctx.Err() != nil {
				break
			}
			continue
		}
		msg, err := Decode(resp)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", url, err))
			continue
		}
		if msg.StatusCode >= 0x0400 { // client/server error status
			errs = append(errs, fmt.Errorf("%s: ipp status 0x%04x", url, msg.StatusCode))
			continue
		}
		return msg, url, nil
	}
	return nil, "", errors.Join(errs...)
}

// Identify implements engine.Identifier.
func (d *Driver) Identify(ctx context.Context, address string, creds model.Creds) (*model.Identity, error) {
	m, url, err := d.fetch(ctx, address)
	if err != nil {
		return nil, err
	}
	id := &model.Identity{Address: address, Method: Name, DeviceType: "printer"}
	id.Model = m.String("printer-make-and-model")
	devID := parseDeviceID(m.String("printer-device-id"))
	id.Make = devID["MFG"]
	if id.Make == "" {
		id.Make, _ = snmp.MakeModel(id.Model)
	}
	if id.Model == "" {
		id.Model = strings.TrimSpace(devID["MFG"] + " " + devID["MDL"])
	}
	id.Serial = firstNonEmpty(m.String("printer-serial-number"), devID["SN"], devID["SERN"])
	id.UUID = strings.TrimPrefix(m.String("printer-uuid"), "urn:uuid:")
	id.Firmware = firstNonEmpty(m.String("printer-firmware-string-version"), m.String("printer-firmware-name"))
	id.SysName = firstNonEmpty(m.String("printer-dns-sd-name"), m.String("printer-name"))
	id.SysLocation = m.String("printer-location")
	id.AdminURL = m.String("printer-more-info")
	id.Hostname = ""
	_ = url
	for _, k := range m.Strings("printer-kind") {
		if k == "document" || k == "photo" || k == "label" {
			continue
		}
	}
	if strings.Contains(strings.ToLower(devID["CMD"]), "scan") || strings.Contains(strings.ToLower(id.Model), "mfp") {
		id.DeviceType = "mfp"
	}

	id.Capabilities = []model.Capability{model.CapIdentify, model.CapReadStatus}
	if m.Attr("printer-supply") != nil || m.Attr("marker-levels") != nil {
		id.Capabilities = append(id.Capabilities, model.CapReadSupplies)
	}
	if m.Attr("printer-input-tray") != nil {
		id.Capabilities = append(id.Capabilities, model.CapReadTrays)
	}
	if m.Attr("printer-impressions-completed") != nil {
		id.Capabilities = append(id.Capabilities, model.CapReadCounters)
	}
	if id.Firmware != "" {
		id.Capabilities = append(id.Capabilities, model.CapFirmwareRead)
	}
	return id, nil
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// parseDeviceID parses an IEEE 1284 device id ("MFG:X;MDL:Y;CMD:..;SN:..").
func parseDeviceID(s string) map[string]string {
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

// ReadStatus implements engine.StatusReader.
func (d *Driver) ReadStatus(ctx context.Context, dev *model.Device, creds model.Creds) (*model.Status, error) {
	m, _, err := d.fetch(ctx, dev.Address)
	if err != nil {
		return nil, err
	}
	return StatusFrom(m), nil
}

// StatusFrom derives a Status from a Get-Printer-Attributes response.
func StatusFrom(m *Message) *model.Status {
	st := &model.Status{Online: true, State: "unknown", Severity: model.SeverityOK, At: time.Now().UTC()}
	if n, ok := m.Int("printer-state"); ok {
		switch n {
		case 3:
			st.State = "idle"
		case 4:
			st.State = "printing"
		case 5:
			st.State = "down"
			st.Severity = model.SeverityCritical
		}
	}
	if n, ok := m.Int("printer-up-time"); ok {
		st.UptimeSecs = int64(n)
	}
	st.Message = m.String("printer-state-message")
	for _, r := range m.Strings("printer-state-reasons") {
		if r == "none" || r == "" {
			continue
		}
		st.Errors = append(st.Errors, r)
		switch {
		case strings.HasSuffix(r, "-error"):
			st.Severity = model.SeverityCritical
		case strings.HasSuffix(r, "-warning"):
			if st.Severity == model.SeverityOK {
				st.Severity = model.SeverityWarning
			}
		}
	}
	descs := m.Strings("printer-alert-description")
	for i, a := range m.Strings("printer-alert") {
		kv := parseKV(a)
		da := model.DeviceAlert{Index: i + 1, Code: kv["code"], Group: kv["group"], Severity: model.SeverityOK}
		if gi, err := strconv.Atoi(kv["index"]); err == nil {
			da.GroupIndex = gi
		}
		switch kv["severity"] {
		case "critical":
			da.Severity = model.SeverityCritical
		case "warning":
			da.Severity = model.SeverityWarning
		}
		if i < len(descs) {
			da.Description = descs[i]
		}
		if da.Description == "" {
			da.Description = da.Code
		}
		st.Alerts = append(st.Alerts, da)
		if da.Severity == model.SeverityCritical {
			st.Severity = model.SeverityCritical
		} else if da.Severity == model.SeverityWarning && st.Severity == model.SeverityOK {
			st.Severity = model.SeverityWarning
		}
	}
	if st.State == "unknown" {
		st.State = "idle"
	}
	return st
}

// parseKV parses PWG "key=value;key=value;" strings.
func parseKV(s string) map[string]string {
	out := map[string]string{}
	for _, kv := range strings.Split(s, ";") {
		k, v, ok := strings.Cut(kv, "=")
		if ok {
			out[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return out
}

// ReadSupplies implements engine.SupplyReader.
func (d *Driver) ReadSupplies(ctx context.Context, dev *model.Device, creds model.Creds) ([]model.Supply, error) {
	m, _, err := d.fetch(ctx, dev.Address)
	if err != nil {
		return nil, err
	}
	s := SuppliesFrom(m)
	if len(s) == 0 {
		return nil, errors.New("no printer-supply or marker-* attributes")
	}
	return s, nil
}

// SuppliesFrom parses printer-supply (PWG 5100.13) with a marker-* (CUPS style) fallback.
func SuppliesFrom(m *Message) []model.Supply {
	var out []model.Supply
	descs := m.Strings("printer-supply-description")
	if sup := m.Strings("printer-supply"); len(sup) > 0 {
		for i, s := range sup {
			kv := parseKV(s)
			idx, _ := strconv.Atoi(kv["index"])
			if idx == 0 {
				idx = i + 1
			}
			level := atoiDef(kv["level"], -2)
			max := atoiDef(kv["maxcapacity"], -2)
			sp := model.Supply{
				Index: idx, Type: kv["type"], Unit: kv["unit"], Level: level, MaxCapacity: max,
				Colorant: kv["colorantname"],
			}
			switch kv["class"] {
			case "supplyThatIsConsumed":
				sp.Class = "consumed"
			case "receptacleThatIsFilled":
				sp.Class = "filled"
			default:
				sp.Class = "other"
			}
			if i < len(descs) {
				sp.Description = descs[i]
			}
			if sp.Description == "" {
				sp.Description = strings.TrimSpace(sp.Colorant + " " + sp.Type)
			}
			if sp.Unit == "percent" && max <= 0 {
				max = 100
				sp.MaxCapacity = 100
			}
			sp.Percent = model.Percent(level, max)
			out = append(out, sp)
		}
		return out
	}
	names := m.Strings("marker-names")
	types := m.Strings("marker-types")
	colors := m.Strings("marker-colors")
	levels := m.Ints("marker-levels")
	highs := m.Ints("marker-high-levels")
	for i, lvl := range levels {
		sp := model.Supply{Index: i + 1, Level: lvl, MaxCapacity: 100, Class: "consumed", Unit: "percent"}
		if i < len(names) {
			sp.Description = names[i]
		}
		if i < len(types) {
			sp.Type = types[i]
			if strings.HasPrefix(sp.Type, "waste") {
				sp.Class = "filled"
			}
		}
		if i < len(colors) {
			sp.Colorant = colors[i]
		}
		if i < len(highs) && highs[i] > 0 {
			sp.MaxCapacity = highs[i]
		}
		sp.Percent = model.Percent(lvl, sp.MaxCapacity)
		out = append(out, sp)
	}
	return out
}

func atoiDef(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// ReadTrays implements engine.TrayReader.
func (d *Driver) ReadTrays(ctx context.Context, dev *model.Device, creds model.Creds) ([]model.Tray, error) {
	m, _, err := d.fetch(ctx, dev.Address)
	if err != nil {
		return nil, err
	}
	t := TraysFrom(m)
	if len(t) == 0 {
		return nil, errors.New("no printer-input-tray attribute")
	}
	return t, nil
}

// TraysFrom parses printer-input-tray / printer-output-tray (PWG 5100.13 §5.4).
func TraysFrom(m *Message) []model.Tray {
	var out []model.Tray
	for i, s := range m.Strings("printer-input-tray") {
		kv := parseKV(s)
		t := model.Tray{Index: i + 1, Kind: "input", Name: kv["name"], Type: kv["type"],
			Level: atoiDef(kv["level"], -2), MaxCapacity: atoiDef(kv["maxcapacity"], -2),
			MediaName: kv["medianame"], MediaType: kv["mediatype"]}
		if t.Name == "" {
			t.Name = fmt.Sprintf("Tray %d", t.Index)
		}
		if x, y := atoiDef(kv["mediaxfeed"], 0), atoiDef(kv["mediafeed"], 0); x > 0 && y > 0 {
			t.MediaSize = fmt.Sprintf("%dx%dmm", x/100, y/100)
		}
		t.Percent = model.Percent(t.Level, t.MaxCapacity)
		summary, flags := snmp.SubunitStatus(int64(atoiDef(kv["status"], 0)))
		t.Flags = flags
		t.Status = snmp.TrayStatus(summary, t.Level, t.Percent, flags)
		out = append(out, t)
	}
	for i, s := range m.Strings("printer-output-tray") {
		kv := parseKV(s)
		t := model.Tray{Index: i + 1, Kind: "output", Name: kv["name"], Type: kv["type"],
			Level: atoiDef(kv["remaining"], -2), MaxCapacity: atoiDef(kv["maxcapacity"], -2)}
		if t.Name == "" {
			t.Name = fmt.Sprintf("Output %d", t.Index)
		}
		t.Percent = model.Percent(t.Level, t.MaxCapacity)
		summary, flags := snmp.SubunitStatus(int64(atoiDef(kv["status"], 0)))
		t.Flags = flags
		t.Status = summary
		if summary == "ok" && t.Level == 0 {
			t.Status = "full"
		}
		out = append(out, t)
	}
	return out
}

// ReadCounters implements engine.CounterReader.
func (d *Driver) ReadCounters(ctx context.Context, dev *model.Device, creds model.Creds) (*model.Counters, error) {
	m, _, err := d.fetch(ctx, dev.Address)
	if err != nil {
		return nil, err
	}
	n, ok := m.Int("printer-impressions-completed")
	if !ok {
		return nil, errors.New("printer-impressions-completed not reported")
	}
	return &model.Counters{Total: int64(n), At: time.Now().UTC()}, nil
}

// ReadFirmware implements engine.FirmwareReader.
func (d *Driver) ReadFirmware(ctx context.Context, dev *model.Device, creds model.Creds) (string, error) {
	m, _, err := d.fetch(ctx, dev.Address)
	if err != nil {
		return "", err
	}
	fw := firstNonEmpty(m.String("printer-firmware-string-version"), m.String("printer-firmware-name"))
	if fw == "" {
		return "", errors.New("no firmware attribute")
	}
	return fw, nil
}

var _ engine.Identifier = (*Driver)(nil)
var _ engine.StatusReader = (*Driver)(nil)
var _ engine.SupplyReader = (*Driver)(nil)
var _ engine.TrayReader = (*Driver)(nil)
var _ engine.CounterReader = (*Driver)(nil)
var _ engine.FirmwareReader = (*Driver)(nil)
