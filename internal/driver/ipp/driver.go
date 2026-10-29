package ipp

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"printmarshal/internal/driver/snmp"
	"printmarshal/internal/engine"
	"printmarshal/internal/model"
)

// Name is the driver's registry name.
const Name = "ipp"

// Transport posts an IPP request body to an HTTP(S) URL and returns the response body.
// It is the read path: no credential is ever presented through Do.
type Transport interface {
	Do(ctx context.Context, url string, body []byte) ([]byte, error)
}

// Auth is the admin credential an authenticated call presents when the device
// challenges the request (HTTP 401). An empty User means none is bound.
type Auth struct {
	User     string
	Password string
}

// Response is the result of an authenticated call.
type Response struct {
	Body []byte
	// TLS reports whether the exchange ran over TLS.
	TLS bool
	// Fingerprint is "sha256:" + the hex SHA-256 of the leaf certificate the
	// device presented; empty over plaintext.
	Fingerprint string
	// Authenticated reports that the device challenged and accepted the credential.
	Authenticated bool
}

// AuthTransport is a Transport that can also answer an HTTP authentication
// challenge (Basic or Digest, RFC 7617 / RFC 7616) and report the TLS
// certificate the device presented. Writes require it; the challenge is
// answered at most once, to the host that issued it, and no redirect is followed.
type AuthTransport interface {
	Transport
	DoAuth(ctx context.Context, url string, body []byte, auth Auth) (*Response, error)
}

// Options tune the driver.
type Options struct {
	Timeout time.Duration
	// InsecureTLS accepts printers' self-signed certificates: no CA validation
	// on reads or writes. Writes pin the presented certificate instead (see
	// WriteRequireTLS and docs/security.md).
	InsecureTLS bool
	// Endpoints are candidate paths tried in order; "" means the default list.
	// Writes try the TLS form of each endpoint before its plaintext form.
	Endpoints []string
	// WriteRequireTLS refuses to send Set-Printer-Attributes over plaintext.
	WriteRequireTLS bool
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

// FingerprintPrefix prefixes certificate fingerprints reported by the driver.
const FingerprintPrefix = "sha256:"

// ErrPinMismatch is wrapped by the error a write gets when the device presents
// a certificate other than the one pinned on first TLS contact.
var ErrPinMismatch = errors.New("certificate pin mismatch")

// ErrPlaintextWrite is wrapped by the error a write gets when it would have to
// go over plaintext and that is not allowed.
var ErrPlaintextWrite = errors.New("refusing to send Set-Printer-Attributes over plaintext")

type httpTransport struct {
	c *http.Client
	// noRedirect is c without redirect following, used by DoAuth so a
	// credential can only ever reach the host that was asked.
	noRedirect *http.Client
}

func (t *httpTransport) Do(ctx context.Context, url string, body []byte) ([]byte, error) {
	resp, err := t.post(ctx, t.c, url, body, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	r, err := readResponse(resp)
	if err != nil {
		return nil, err
	}
	return r.Body, nil
}

// DoAuth implements AuthTransport.
func (t *httpTransport) DoAuth(ctx context.Context, rawURL string, body []byte, auth Auth) (*Response, error) {
	target, err := neturl.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	resp, err := t.post(ctx, t.noRedirect, rawURL, body, "")
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusUnauthorized {
		defer resp.Body.Close()
		return readResponse(resp)
	}
	challenges := parseChallenges(resp.Header.Values("WWW-Authenticate"))
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
	if auth.User == "" {
		return nil, ErrAuthRequired
	}
	if len(challenges) == 0 {
		return nil, fmt.Errorf("%w: 401 without a WWW-Authenticate challenge", ErrUnsupportedChallenge)
	}
	header, err := authorizationFor(challenges, http.MethodPost, target, body, auth)
	if err != nil {
		return nil, err
	}
	resp2, err := t.post(ctx, t.noRedirect, rawURL, body, header)
	if err != nil {
		return nil, err
	}
	defer resp2.Body.Close()
	if resp2.Request != nil && resp2.Request.URL.Host != target.Host {
		return nil, fmt.Errorf("authenticated request was answered by %s, not %s", resp2.Request.URL.Host, target.Host)
	}
	if resp2.StatusCode == http.StatusUnauthorized {
		return nil, ErrAuthRejected
	}
	r, err := readResponse(resp2)
	if err != nil {
		return nil, err
	}
	r.Authenticated = true
	return r, nil
}

func (t *httpTransport) post(ctx context.Context, c *http.Client, url string, body []byte, authorization string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/ipp")
	req.Header.Set("Accept", "application/ipp")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	return c.Do(req)
}

// readResponse validates an HTTP response, reads the body and records the TLS
// leaf certificate. The caller closes the body.
func readResponse(resp *http.Response) (*Response, error) {
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/ipp") {
		return nil, fmt.Errorf("unexpected content-type %q", ct)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	r := &Response{Body: b}
	if resp.TLS != nil {
		r.TLS = true
		if len(resp.TLS.PeerCertificates) > 0 {
			r.Fingerprint = Fingerprint(resp.TLS.PeerCertificates[0])
		}
	}
	return r, nil
}

// Fingerprint renders a certificate's SHA-256 fingerprint as the driver pins it.
func Fingerprint(c *x509.Certificate) string {
	sum := sha256.Sum256(c.Raw)
	return FingerprintPrefix + hex.EncodeToString(sum[:])
}

// NewTransport returns the production HTTP transport (an AuthTransport).
func NewTransport(opts Options) Transport {
	tr := &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: opts.InsecureTLS}, //nolint:gosec // printers use self-signed certs; writes pin the leaf certificate instead
		DisableKeepAlives: true,
		DialContext:       (&net.Dialer{Timeout: opts.Timeout}).DialContext,
	}
	c := &http.Client{Transport: tr, Timeout: opts.Timeout}
	noRedirect := &http.Client{Transport: tr, Timeout: opts.Timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &httpTransport{c: c, noRedirect: noRedirect}
}

// Driver implements engine.Driver over IPP.
type Driver struct {
	tr        Transport
	endpoints []string
	// writeEndpoints is endpoints reordered so TLS forms come first.
	writeEndpoints  []string
	writeRequireTLS bool
	reqID           atomic.Uint32

	// pins holds the certificate pinned per device on its first TLS contact
	// during a write (trust on first use). It lives for the process lifetime:
	// the device model has no field for it and persisting one would need a
	// schema change.
	pinMu sync.Mutex
	pins  map[string]string
}

// New returns a driver using the given transport.
func New(tr Transport, opts Options) *Driver {
	eps := opts.Endpoints
	if len(eps) == 0 {
		eps = defaultEndpoints
	}
	return &Driver{tr: tr, endpoints: eps, writeEndpoints: writeOrder(eps), writeRequireTLS: opts.WriteRequireTLS, pins: map[string]string{}}
}

// writeOrder returns the endpoints a write tries: the https:// form of every
// endpoint first (an http:// template on port 80 has none; on every other
// port the printer's IPPS listener is expected there), then the plaintext
// forms, each list deduplicated and in the configured order.
func writeOrder(eps []string) []string {
	var tlsEps, plainEps []string
	seen := map[string]bool{}
	add := func(list *[]string, ep string) {
		if !seen[ep] {
			seen[ep] = true
			*list = append(*list, ep)
		}
	}
	for _, ep := range eps {
		switch {
		case strings.HasPrefix(ep, "https://"):
			add(&tlsEps, ep)
		case strings.HasPrefix(ep, "http://"):
			if !onPort80(ep) {
				add(&tlsEps, "https://"+strings.TrimPrefix(ep, "http://"))
			}
			add(&plainEps, ep)
		default:
			add(&plainEps, ep)
		}
	}
	return append(tlsEps, plainEps...)
}

func onPort80(ep string) bool {
	rest := strings.TrimPrefix(ep, "http://")
	hostport, _, _ := strings.Cut(rest, "/")
	return strings.HasSuffix(hostport, ":80")
}

// endpointURL renders an endpoint template for a device address; a template
// without %s is used as is.
func endpointURL(ep, address string) string {
	if strings.Contains(ep, "%s") {
		return fmt.Sprintf(ep, address)
	}
	return ep
}

// pinKey identifies the device a certificate pin belongs to.
func pinKey(dev *model.Device) string {
	if dev.ID != "" {
		return dev.ID
	}
	return dev.Address
}

// PinnedCertificate reports the fingerprint pinned for a device, if any.
func (d *Driver) PinnedCertificate(dev *model.Device) (string, bool) {
	d.pinMu.Lock()
	defer d.pinMu.Unlock()
	fp, ok := d.pins[pinKey(dev)]
	return fp, ok
}

// checkPin records fp on first TLS contact and afterwards refuses any other
// certificate. A plaintext exchange (fp == "") is refused once a pin exists so
// the pin cannot be side-stepped by blocking the TLS port.
func (d *Driver) checkPin(dev *model.Device, url, fp string) error {
	d.pinMu.Lock()
	defer d.pinMu.Unlock()
	key := pinKey(dev)
	pinned, ok := d.pins[key]
	switch {
	case fp == "" && !ok:
		return nil
	case fp == "":
		return fmt.Errorf("%w for %s: %s was pinned on an earlier TLS contact and %s is plaintext; refusing to downgrade the write", ErrPinMismatch, dev.Address, pinned, url)
	case !ok:
		d.pins[key] = fp
		return nil
	case pinned != fp:
		return fmt.Errorf("%w for %s: the device at %s presented %s but %s was pinned on first contact; refusing the write (restart PrintMarshal to re-pin after a planned certificate change)", ErrPinMismatch, dev.Address, url, fp, pinned)
	}
	return nil
}

// writeCall sends one request over the write path: TLS required when
// configured, certificate pin enforced, credential presented on challenge.
func (d *Driver) writeCall(ctx context.Context, dev *model.Device, url string, body []byte, auth Auth) ([]byte, error) {
	at, ok := d.tr.(AuthTransport)
	if !ok {
		return nil, errors.New("ipp transport cannot authenticate writes")
	}
	plaintext := !strings.HasPrefix(url, "https://")
	if plaintext && d.writeRequireTLS {
		return nil, fmt.Errorf("%w: ippWriteRequireTLS is set and %s is not a TLS endpoint", ErrPlaintextWrite, url)
	}
	resp, err := at.DoAuth(ctx, url, body, auth)
	if err != nil {
		return nil, err
	}
	if err := d.checkPin(dev, url, resp.Fingerprint); err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (d *Driver) Name() string { return Name }

func (d *Driver) Capabilities() []model.Capability {
	return []model.Capability{
		model.CapIdentify, model.CapReadStatus, model.CapReadSupplies,
		model.CapReadTrays, model.CapReadCounters, model.CapFirmwareRead,
		model.CapReadConfig, model.CapWriteConfig,
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
	case model.CapReadSupplies, model.CapReadConfig, model.CapWriteConfig:
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
	"printer-impressions-completed", "media-ready", "printer-kind", "operations-supported",
}

// fetch tries each endpoint and returns the first successful response.
func (d *Driver) fetch(ctx context.Context, address string) (*Message, string, error) {
	return d.fetchAttrs(ctx, address, RequestedAttrs)
}

// fetchAttrs is fetch with an explicit requested-attributes list.
func (d *Driver) fetchAttrs(ctx context.Context, address string, requested []string) (*Message, string, error) {
	var errs []error
	for _, ep := range d.endpoints {
		url := endpointURL(ep, address)
		uri := printerURI(url)
		req := NewGetPrinterAttributes(uri, d.reqID.Add(1), requested)
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

// printerURI derives the printer-uri operation attribute from the HTTP endpoint.
func printerURI(url string) string {
	return "ipp://" + strings.TrimPrefix(strings.TrimPrefix(url, "http://"), "https://")
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
	// Get-Printer-Attributes always yields a configuration document; writes need
	// Set-Printer-Attributes (RFC 3380) in operations-supported.
	id.Capabilities = append(id.Capabilities, model.CapReadConfig)
	if supportsOp(m, OpSetPrinterAttributes) {
		id.Capabilities = append(id.Capabilities, model.CapWriteConfig)
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
