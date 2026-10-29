package ipp

// Configuration read/write over IPP (spec §8.4).
//
// Reads map Get-Printer-Attributes (RFC 8011 §4.2.5) onto the canonical
// catalogue; writes use Set-Printer-Attributes (RFC 3380 §4.1, operation
// 0x0013). Which attributes a device lets us set is taken from
// printer-settable-attributes-supported (RFC 3380 §6.1); when a device supports
// the operation but omits that list, the RFC 3380 candidates are assumed.
//
// Attribute -> key:
//
//	printer-name                       -> identity.hostname           (RFC 8011 §5.4.4; settable only if listed)
//	printer-location                   -> identity.location           (RFC 8011 §5.4.5; RFC 3380 settable)
//	printer-info                       -> identity.description        (RFC 8011 §5.4.6; RFC 3380 settable)
//	printer-organization               -> identity.organization       (PWG 5100.13 §5.6.x; settable)
//	printer-organizational-unit        -> identity.organizationalUnit (PWG 5100.13; settable)
//	copies-default                     -> print.defaultCopies         (RFC 8011 §5.2.5)
//	sides-default                      -> print.defaultDuplex         (RFC 8011 §5.2.8; bool: two-sided-long-edge / one-sided)
//	print-color-mode-default           -> print.defaultColorMode      (PWG 5100.13 §5.2.3)
//	printer-resolution-default         -> print.defaultResolution     (RFC 8011 §5.2.12; "600x600dpi")
//	document-format-default            -> print.defaultDocumentFormat (RFC 8011 §5.4.21)
//	multiple-operation-time-out        -> print.jobTimeoutSeconds     (RFC 8011 §5.4.31; read-only)
//	media-default                      -> trays.defaultMediaSize      (RFC 8011 §5.2.11)
//	media-col-default/media-source     -> trays.defaultTray           (PWG 5100.7; read-only, collection)
//	media-col-default/media-type       -> trays.defaultMediaType      (PWG 5100.7; read-only, collection)
//	natural-language-configured        -> panel.language              (RFC 8011 §5.4.19; read-only)
//	printer-uri-supported / uri-security-supported -> protocols.ipp.enabled, protocols.ipps.enabled (RFC 8011 §5.4.1-2; observed)
//
// No standard IPP attribute carries TLS versions or sleep timers, so the
// security.tls.* and energy.* keys are not produced by this driver.

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"printmarshal/internal/engine"
	"printmarshal/internal/model"
)

// ConfigAttrs is the attribute set a configuration read asks for.
var ConfigAttrs = []string{
	"printer-name", "printer-location", "printer-info", "printer-organization", "printer-organizational-unit",
	"copies-default", "copies-supported", "sides-default", "sides-supported",
	"print-color-mode-default", "print-color-mode-supported",
	"printer-resolution-default", "printer-resolution-supported",
	"document-format-default", "document-format-supported",
	"media-default", "media-supported", "media-col-default",
	"multiple-operation-time-out", "natural-language-configured",
	"printer-uri-supported", "uri-security-supported",
	"operations-supported", "printer-settable-attributes-supported",
}

// rfc3380Settable are the printer attributes RFC 3380 §3 lists as candidates
// for Set-Printer-Attributes, used when a device supports the operation but
// does not publish printer-settable-attributes-supported.
var rfc3380Settable = map[string]bool{
	"printer-location": true, "printer-info": true, "printer-organization": true, "printer-organizational-unit": true,
	"copies-default": true, "sides-default": true, "print-color-mode-default": true, "printer-resolution-default": true,
	"document-format-default": true, "media-default": true,
}

// ippSetting maps one attribute to one catalogue key.
type ippSetting struct {
	key  string
	attr string
	tag  byte
	// read extracts the canonical value; ok=false leaves the setting out.
	read func(m *Message) (any, bool)
	// encode turns a coerced canonical value into the IPP value; nil = never writable.
	encode func(m *Message, v any) (any, error)
	// reason explains a read-only setting when the device could otherwise set it.
	reason string
}

var ippSettings = []ippSetting{
	{key: "identity.hostname", attr: "printer-name", tag: TagName, read: readString("printer-name"), encode: encodeString},
	{key: "identity.location", attr: "printer-location", tag: TagText, read: readString("printer-location"), encode: encodeString},
	{key: "identity.description", attr: "printer-info", tag: TagText, read: readString("printer-info"), encode: encodeString},
	{key: "identity.organization", attr: "printer-organization", tag: TagText, read: readString("printer-organization"), encode: encodeString},
	{key: "identity.organizationalUnit", attr: "printer-organizational-unit", tag: TagText, read: readString("printer-organizational-unit"), encode: encodeString},
	{key: "print.defaultCopies", attr: "copies-default", tag: TagInteger, read: readInt("copies-default"), encode: encodeCopies},
	{key: "print.defaultDuplex", attr: "sides-default", tag: TagKeyword, read: readDuplex, encode: encodeDuplex},
	{key: "print.defaultColorMode", attr: "print-color-mode-default", tag: TagKeyword, read: readColorMode, encode: encodeColorMode},
	{key: "print.defaultResolution", attr: "printer-resolution-default", tag: TagResolution, read: readResolution, encode: encodeResolution},
	{key: "print.defaultDocumentFormat", attr: "document-format-default", tag: TagMime, read: readString("document-format-default"), encode: encodeInSupported("document-format-supported")},
	{key: "print.jobTimeoutSeconds", attr: "multiple-operation-time-out", tag: TagInteger, read: readInt("multiple-operation-time-out"), reason: "printer description attribute; not settable through IPP"},
	{key: "trays.defaultMediaSize", attr: "media-default", tag: TagKeyword, read: readString("media-default"), encode: encodeInSupported("media-supported")},
	{key: "trays.defaultTray", attr: "media-col-default", read: readMediaCol("media-source"), reason: "member of the media-col-default collection; set it on the device"},
	{key: "trays.defaultMediaType", attr: "media-col-default", read: readMediaCol("media-type"), reason: "member of the media-col-default collection; set it on the device"},
	{key: "panel.language", attr: "natural-language-configured", tag: TagLanguage, read: readString("natural-language-configured"), reason: "not settable through IPP"},
	{key: "protocols.ipp.enabled", attr: "printer-uri-supported", read: func(m *Message) (any, bool) { return true, m.Attr("printer-uri-supported") != nil }, reason: "observed: the device answered IPP"},
	{key: "protocols.ipps.enabled", attr: "printer-uri-supported", read: readIPPS, reason: "observed from printer-uri-supported / uri-security-supported"},
}

func readString(attr string) func(*Message) (any, bool) {
	return func(m *Message) (any, bool) {
		if m.Attr(attr) == nil {
			return nil, false
		}
		return m.String(attr), true
	}
}

func readInt(attr string) func(*Message) (any, bool) {
	return func(m *Message) (any, bool) {
		n, ok := m.Int(attr)
		return n, ok
	}
}

func readDuplex(m *Message) (any, bool) {
	if m.Attr("sides-default") == nil {
		return nil, false
	}
	return strings.HasPrefix(m.String("sides-default"), "two-sided"), true
}

func readColorMode(m *Message) (any, bool) {
	if m.Attr("print-color-mode-default") == nil {
		return nil, false
	}
	switch v := m.String("print-color-mode-default"); {
	case v == "color":
		return "color", true
	case strings.HasSuffix(v, "monochrome") && v != "auto-monochrome":
		return "monochrome", true
	default: // auto, auto-monochrome, process-*, highlight
		return "auto", true
	}
}

func readResolution(m *Message) (any, bool) {
	a := m.Attr("printer-resolution-default")
	if a == nil || len(a.Values) == 0 {
		return nil, false
	}
	b, ok := a.Values[0].([]byte)
	if !ok {
		return nil, false
	}
	s, ok := decodeResolution(b)
	return s, ok
}

func readMediaCol(member string) func(*Message) (any, bool) {
	return func(m *Message) (any, bool) {
		a := m.Attr("media-col-default")
		if a == nil || len(a.Values) == 0 {
			return nil, false
		}
		coll, ok := a.Values[0].(map[string]Attribute)
		if !ok {
			return nil, false
		}
		mem, ok := coll[member]
		if !ok || len(mem.Values) == 0 {
			return nil, false
		}
		switch v := mem.Values[0].(type) {
		case string:
			return v, true
		case []byte:
			return string(v), true
		}
		return nil, false
	}
}

func readIPPS(m *Message) (any, bool) {
	if m.Attr("printer-uri-supported") == nil {
		return nil, false
	}
	for _, u := range m.Strings("printer-uri-supported") {
		if strings.HasPrefix(u, "ipps:") {
			return true, true
		}
	}
	for _, s := range m.Strings("uri-security-supported") {
		if s == "tls" || s == "ssl3" {
			return true, true
		}
	}
	return false, true
}

func encodeString(_ *Message, v any) (any, error) { return fmt.Sprint(v), nil }

func encodeCopies(m *Message, v any) (any, error) {
	n, _ := v.(int)
	if a := m.Attr("copies-supported"); a != nil && len(a.Values) > 0 {
		if r, ok := a.Values[0].([2]int); ok && (n < r[0] || n > r[1]) {
			return nil, fmt.Errorf("copies %d outside copies-supported %d..%d", n, r[0], r[1])
		}
	}
	if n < 1 {
		return nil, fmt.Errorf("copies must be at least 1")
	}
	return n, nil
}

func encodeDuplex(m *Message, v any) (any, error) {
	want := "one-sided"
	if b, _ := v.(bool); b {
		want = "two-sided-long-edge"
	}
	if sup := m.Strings("sides-supported"); len(sup) > 0 && !contains(sup, want) {
		return nil, fmt.Errorf("%s is not in sides-supported %v", want, sup)
	}
	return want, nil
}

func encodeColorMode(m *Message, v any) (any, error) {
	want := fmt.Sprint(v)
	if sup := m.Strings("print-color-mode-supported"); len(sup) > 0 && !contains(sup, want) {
		return nil, fmt.Errorf("%s is not in print-color-mode-supported %v", want, sup)
	}
	return want, nil
}

func encodeResolution(m *Message, v any) (any, error) {
	b, err := encodeResolutionString(fmt.Sprint(v))
	if err != nil {
		return nil, err
	}
	if a := m.Attr("printer-resolution-supported"); a != nil {
		ok := false
		for _, sv := range a.Values {
			if sb, isB := sv.([]byte); isB && string(sb) == string(b) {
				ok = true
			}
		}
		if !ok {
			return nil, fmt.Errorf("%v is not in printer-resolution-supported", v)
		}
	}
	return b, nil
}

func encodeInSupported(supported string) func(*Message, any) (any, error) {
	return func(m *Message, v any) (any, error) {
		want := fmt.Sprint(v)
		if sup := m.Strings(supported); len(sup) > 0 && !contains(sup, want) {
			return nil, fmt.Errorf("%s is not in %s", want, supported)
		}
		return want, nil
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// decodeResolution renders an IPP resolution octet string (RFC 8010 §3.5.2:
// int32 cross-feed, int32 feed, int8 units 3=dpi 4=dpcm) as "600x600dpi".
func decodeResolution(b []byte) (string, bool) {
	if len(b) != 9 {
		return "", false
	}
	x := int32(binary.BigEndian.Uint32(b[0:4]))
	y := int32(binary.BigEndian.Uint32(b[4:8]))
	unit := "dpi"
	if b[8] == 4 {
		unit = "dpcm"
	}
	return fmt.Sprintf("%dx%d%s", x, y, unit), true
}

func encodeResolutionString(s string) ([]byte, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	unit := byte(3)
	switch {
	case strings.HasSuffix(s, "dpcm"):
		unit, s = 4, strings.TrimSuffix(s, "dpcm")
	case strings.HasSuffix(s, "dpi"):
		s = strings.TrimSuffix(s, "dpi")
	}
	xs, ys, ok := strings.Cut(s, "x")
	if !ok {
		xs, ys = s, s
	}
	x, err1 := strconv.Atoi(strings.TrimSpace(xs))
	y, err2 := strconv.Atoi(strings.TrimSpace(ys))
	if err1 != nil || err2 != nil || x <= 0 || y <= 0 {
		return nil, fmt.Errorf("resolution %q: expected e.g. 600x600dpi", s)
	}
	b := make([]byte, 9)
	binary.BigEndian.PutUint32(b[0:4], uint32(x))
	binary.BigEndian.PutUint32(b[4:8], uint32(y))
	b[8] = unit
	return b, nil
}

// supportsOp reports whether operations-supported lists op.
func supportsOp(m *Message, op uint16) bool {
	for _, n := range m.Ints("operations-supported") {
		if n == int(op) {
			return true
		}
	}
	return false
}

// settable reports whether attr may be written on this device, and why not.
func settable(m *Message, s ippSetting) (bool, string) {
	if s.encode == nil {
		return false, s.reason
	}
	if !supportsOp(m, OpSetPrinterAttributes) {
		return false, "device does not support Set-Printer-Attributes (RFC 3380)"
	}
	list := m.Strings("printer-settable-attributes-supported")
	if len(list) == 0 {
		if rfc3380Settable[s.attr] {
			return true, ""
		}
		return false, "not an RFC 3380 settable attribute and the device publishes no printer-settable-attributes-supported"
	}
	if contains(list, s.attr) {
		return true, ""
	}
	return false, "not in printer-settable-attributes-supported"
}

// ConfigFrom maps a Get-Printer-Attributes response to catalogue settings.
func ConfigFrom(m *Message) []model.Setting {
	out := []model.Setting{}
	for _, s := range ippSettings {
		v, ok := s.read(m)
		if !ok {
			continue
		}
		def := model.CatalogByKey[s.key]
		w, reason := settable(m, s)
		out = append(out, model.Setting{Key: s.key, Area: def.Area, Name: def.Name, Type: def.Type, Options: def.Options,
			Value: v, Writable: w, Source: Name, ReadOnlyReason: reason})
	}
	return out
}

// ReadConfig implements engine.ConfigReader.
func (d *Driver) ReadConfig(ctx context.Context, dev *model.Device, creds model.Creds, sel model.ConfigSelector) (*model.ConfigDocument, error) {
	m, _, err := d.fetchAttrs(ctx, dev.Address, ConfigAttrs)
	if err != nil {
		return nil, err
	}
	doc := &model.ConfigDocument{DeviceID: dev.ID, Settings: filterSettings(ConfigFrom(m), sel), ReadAt: time.Now().UTC(), Sources: []string{Name}}
	return doc, nil
}

func filterSettings(in []model.Setting, sel model.ConfigSelector) []model.Setting {
	if len(sel.Areas) == 0 && len(sel.Keys) == 0 {
		return in
	}
	out := make([]model.Setting, 0, len(in))
	for _, s := range in {
		if contains(sel.Areas, s.Area) || contains(sel.Keys, s.Key) {
			out = append(out, s)
		}
	}
	return out
}

// NewSetPrinterAttributes builds a Set-Printer-Attributes request (RFC 3380 §4.1):
// the operation group names the printer and the requesting user, the printer
// attributes group carries the attributes to set.
func NewSetPrinterAttributes(uri string, requestID uint32, user string, attrs []Attribute) *Message {
	ops := []Attribute{
		{Tag: TagCharset, Name: "attributes-charset", Values: []any{"utf-8"}},
		{Tag: TagLanguage, Name: "attributes-natural-language", Values: []any{"en"}},
		{Tag: TagURI, Name: "printer-uri", Values: []any{uri}},
		{Tag: TagName, Name: "requesting-user-name", Values: []any{user}},
	}
	return &Message{Version: [2]byte{2, 0}, StatusCode: OpSetPrinterAttributes, RequestID: requestID,
		Groups: []Group{{Tag: TagOperation, Attributes: ops}, {Tag: TagPrinter, Attributes: attrs}}}
}

// RequestingUser is the requesting-user-name sent with writes when no admin
// user is bound to the device.
const RequestingUser = "printmarshal"

// fetchForWrite is fetchAttrs over the write path: TLS endpoints first, the
// bound credential presented on challenge, the certificate pin enforced. The
// URL it returns is the one the Set-Printer-Attributes requests then use, so
// a write never goes to an endpoint other than the one that was read.
func (d *Driver) fetchForWrite(ctx context.Context, dev *model.Device, auth Auth, requested []string) (*Message, string, error) {
	var errs []error
	for _, ep := range d.writeEndpoints {
		url := endpointURL(ep, dev.Address)
		req := NewGetPrinterAttributes(printerURI(url), d.reqID.Add(1), requested)
		body, err := Encode(req)
		if err != nil {
			return nil, "", err
		}
		resp, err := d.writeCall(ctx, dev, url, body, auth)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", url, err))
			// A pin mismatch must not be side-stepped by the next (plaintext)
			// endpoint, and once TLS is required every remaining endpoint is
			// plaintext.
			if ctx.Err() != nil || errors.Is(err, ErrPinMismatch) || errors.Is(err, ErrPlaintextWrite) {
				break
			}
			continue
		}
		msg, err := Decode(resp)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", url, err))
			continue
		}
		if msg.StatusCode >= 0x0400 {
			errs = append(errs, fmt.Errorf("%s: ipp status 0x%04x", url, msg.StatusCode))
			continue
		}
		return msg, url, nil
	}
	return nil, "", errors.Join(errs...)
}

// WriteConfig implements engine.ConfigWriter. Each setting is sent in its own
// Set-Printer-Attributes request so the outcome is per setting; a dry run
// performs the read only and issues no Set-Printer-Attributes at all.
//
// The read and the writes go over the write path (driver.go): the TLS form of
// each endpoint before its plaintext form, plaintext refused altogether when
// WriteRequireTLS is set, the device's certificate pinned on first TLS contact,
// and the bound admin credential (model.Creds.AdminUser / AdminPass) presented
// when the device answers 401.
func (d *Driver) WriteConfig(ctx context.Context, dev *model.Device, creds model.Creds, changes []model.SettingChange, opts model.WriteOptions) (*model.WriteResult, error) {
	auth := Auth{User: creds.AdminUser, Password: creds.AdminPass}
	m, url, err := d.fetchForWrite(ctx, dev, auth, ConfigAttrs)
	if err != nil {
		return nil, err
	}
	current := map[string]model.Setting{}
	for _, s := range ConfigFrom(m) {
		current[s.Key] = s
	}
	byKey := map[string]ippSetting{}
	for _, s := range ippSettings {
		byKey[s.key] = s
	}
	res := &model.WriteResult{DeviceID: dev.ID, DryRun: opts.DryRun, Results: []model.SettingResult{}, At: time.Now().UTC()}
	for _, ch := range changes {
		r := model.SettingResult{Key: ch.Key, Driver: Name}
		s, ok := byKey[ch.Key]
		cur, present := current[ch.Key]
		switch {
		case !ok:
			r.Status, r.Error = model.SettingSkipped, "not an IPP setting"
		case !present:
			r.Status, r.Error = model.SettingSkipped, "the device does not report "+s.attr
		case !cur.Writable:
			r.Status, r.Error = model.SettingSkipped, cur.ReadOnlyReason
		}
		if r.Status != "" {
			res.Results = append(res.Results, r)
			continue
		}
		desired, err := model.CoerceValue(ch.Key, ch.Value)
		if err != nil {
			r.Status, r.Error = model.SettingFailed, err.Error()
			res.Results = append(res.Results, r)
			continue
		}
		r.Before = cur.Value
		if model.ValuesEqual(cur.Value, desired) {
			r.Status, r.After = model.SettingUnchanged, cur.Value
			res.Results = append(res.Results, r)
			continue
		}
		ippVal, err := s.encode(m, desired)
		if err != nil {
			r.Status, r.Error = model.SettingFailed, err.Error()
			res.Results = append(res.Results, r)
			continue
		}
		r.After = desired
		if opts.DryRun {
			r.Status = model.SettingWouldWrite
			res.Results = append(res.Results, r)
			continue
		}
		if err := d.setAttribute(ctx, dev, url, auth, Attribute{Tag: s.tag, Name: s.attr, Values: []any{ippVal}}); err != nil {
			r.Status, r.Error, r.After = model.SettingFailed, err.Error(), nil
			res.Results = append(res.Results, r)
			continue
		}
		r.Status = model.SettingWritten
		res.Rollback = append(res.Rollback, model.SettingChange{Key: ch.Key, Value: r.Before})
		res.Results = append(res.Results, r)
	}
	// Read back once so After reflects what the device now holds.
	if !opts.DryRun && len(res.Rollback) > 0 {
		if m2, err := d.getAttributesAt(ctx, dev, url, auth, ConfigAttrs); err == nil {
			after := map[string]any{}
			for _, s := range ConfigFrom(m2) {
				after[s.Key] = s.Value
			}
			for i := range res.Results {
				if res.Results[i].Status == model.SettingWritten {
					if v, ok := after[res.Results[i].Key]; ok {
						res.Results[i].After = v
					}
				}
			}
		}
	}
	return res, nil
}

// getAttributesAt reads the configuration attributes from one write-path URL.
func (d *Driver) getAttributesAt(ctx context.Context, dev *model.Device, url string, auth Auth, requested []string) (*Message, error) {
	body, err := Encode(NewGetPrinterAttributes(printerURI(url), d.reqID.Add(1), requested))
	if err != nil {
		return nil, err
	}
	raw, err := d.writeCall(ctx, dev, url, body, auth)
	if err != nil {
		return nil, err
	}
	return Decode(raw)
}

// setAttribute sends one Set-Printer-Attributes request and interprets the
// response (RFC 3380 §4.1.2): a status at or above 0x0400 is a failure, and an
// attribute echoed in the unsupported-attributes group was not applied. The
// bound admin user is sent as requesting-user-name and presented when the
// device challenges; without one a challenge yields ErrAuthRequired.
func (d *Driver) setAttribute(ctx context.Context, dev *model.Device, url string, auth Auth, a Attribute) error {
	user := auth.User
	if user == "" {
		user = RequestingUser
	}
	req := NewSetPrinterAttributes(printerURI(url), d.reqID.Add(1), user, []Attribute{a})
	body, err := Encode(req)
	if err != nil {
		return err
	}
	raw, err := d.writeCall(ctx, dev, url, body, auth)
	if err != nil {
		return err
	}
	resp, err := Decode(raw)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 0x0400 {
		switch resp.StatusCode {
		case StatusClientErrorAttrsNotSettable:
			return errors.New("device rejected the attribute: client-error-attributes-not-settable (0x0413)")
		case StatusServerErrorOpNotSupported:
			return errors.New("device does not support Set-Printer-Attributes (0x0501)")
		}
		return fmt.Errorf("ipp status 0x%04x", resp.StatusCode)
	}
	for _, g := range resp.Groups {
		if g.Tag != TagUnsupported {
			continue
		}
		for _, u := range g.Attributes {
			if u.Name == a.Name {
				return fmt.Errorf("device did not apply %s (listed as unsupported, status 0x%04x)", a.Name, resp.StatusCode)
			}
		}
	}
	return nil
}

var _ engine.ConfigReader = (*Driver)(nil)
var _ engine.ConfigWriter = (*Driver)(nil)
