package siem

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"printmarshal/internal/events"
	"printmarshal/internal/model"
)

// sdID is the structured-data element id. 32473 is the IANA enterprise
// number reserved for documentation (RFC 5612); PrintMarshal has no PEN of
// its own, and a reserved number cannot collide with a real vendor's.
const sdID = "printmarshal@32473"

// Severity (RFC 5424 §6.2.1) for an event.
func severityOf(env events.Envelope) int {
	switch env.Type {
	case "alert.raised":
		if a, ok := env.Data.(*model.Alert); ok && a.Severity == model.SeverityCritical {
			return 2 // critical
		}
		return 4 // warning
	case "alert.cleared", "alert.acked":
		return 6 // informational
	case "auth.failed", "auth.denied":
		return 4
	case "notification.dead":
		return 3 // error
	case "config.applied", "security.remediated", "device.deleted", "settings.updated", "auth.session.revoked", "server.stopping":
		return 5 // notice
	}
	return 6
}

// summary renders a short human line for an event.
func summary(env events.Envelope) string {
	switch v := env.Data.(type) {
	case *model.Alert:
		return fmt.Sprintf("%s %s: %s", env.Type, v.DeviceID, v.Description)
	case *model.Device:
		return fmt.Sprintf("%s %s (%s)", env.Type, v.Name(), v.Address)
	case *model.Job:
		return fmt.Sprintf("%s %s %s: %d/%d done, %d failed", env.Type, v.Type, v.Status, v.Done, v.Total, v.Failed)
	case map[string]any:
		for _, k := range []string{"summary", "subject", "detail", "action"} {
			if s, _ := v[k].(string); s != "" {
				if k == "action" {
					return fmt.Sprintf("%s %s %s by %s: %s", env.Type, s, str(v["target"]), str(v["actor"]), str(v["result"]))
				}
				return env.Type + " " + s
			}
		}
	}
	if d, ok := events.Lookup(env.Type); ok {
		return env.Type + " " + d.Description
	}
	return env.Type
}

func str(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// jsonLine is the JSON-lines representation: the envelope as published.
func jsonLine(env events.Envelope) []byte {
	b, err := json.Marshal(env)
	if err != nil {
		b, _ = json.Marshal(map[string]any{"id": env.ID, "version": env.Version, "type": env.Type, "at": env.At, "origin": env.Origin, "error": "data not serialisable: " + err.Error()})
	}
	return b
}

// sdEscape escapes a structured-data param value (RFC 5424 §6.3.3).
func sdEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, `]`, `\]`)
	return r.Replace(s)
}

// header is the RFC 5424 header up to and including MSGID.
func header(env events.Envelope, facility int, hostname string) string {
	if facility < 0 || facility > 23 {
		facility = 16
	}
	pri := facility*8 + severityOf(env)
	msgid := env.Type
	if len(msgid) > 32 {
		msgid = msgid[:32]
	}
	return fmt.Sprintf("<%d>1 %s %s printmarshal %d %s", pri, env.At.UTC().Format(time.RFC3339Nano), hostname, os.Getpid(), msgid)
}

// structuredData renders the envelope fields as one SD element.
func structuredData(env events.Envelope) string {
	params := []string{"id", env.ID, "type", env.Type, "source", env.Origin.Source}
	if env.Origin.Actor != "" {
		params = append(params, "actor", env.Origin.Actor)
	}
	if env.Origin.IP != "" {
		params = append(params, "ip", env.Origin.IP)
	}
	if env.Origin.RequestID != "" {
		params = append(params, "requestId", env.Origin.RequestID)
	}
	if id := deviceID(env); id != "" {
		params = append(params, "deviceId", id)
	}
	var b strings.Builder
	b.WriteString("[" + sdID)
	for i := 0; i < len(params); i += 2 {
		fmt.Fprintf(&b, ` %s="%s"`, params[i], sdEscape(params[i+1]))
	}
	b.WriteString("]")
	return b.String()
}

// deviceID finds the device an event is about, if any.
func deviceID(env events.Envelope) string {
	if env.Origin.DeviceID != "" {
		return env.Origin.DeviceID
	}
	switch v := env.Data.(type) {
	case *model.Alert:
		return v.DeviceID
	case *model.Device:
		return v.ID
	case map[string]any:
		if s, _ := v["deviceId"].(string); s != "" {
			return s
		}
	}
	return ""
}

// formatSyslog renders "<PRI>1 TS HOST APP PID MSGID [SD] MSG" with the
// event data as a JSON MSG. format "json" puts the whole envelope in MSG
// with a NILVALUE SD; format "cef" puts a CEF record in MSG.
func formatSyslog(env events.Envelope, format string, facility int, hostname, version string) string {
	h := header(env, facility, hostname)
	switch format {
	case "json":
		return h + " - " + string(jsonLine(env))
	case "cef":
		return h + " - " + formatCEF(env, version)
	}
	data, err := json.Marshal(env.Data)
	if err != nil || env.Data == nil {
		data = []byte(`{}`)
	}
	return h + " " + structuredData(env) + " " + summary(env) + " " + string(data)
}

// cefHeaderEscape escapes a CEF header field (| and \).
func cefHeaderEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `|`, `\|`, "\n", " ", "\r", " ").Replace(s)
}

// cefExtEscape escapes a CEF extension value (= \ and newlines).
func cefExtEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `=`, `\=`, "\n", `\n`, "\r", `\r`).Replace(s)
}

// cefSeverity maps syslog severity to the CEF 0-10 scale.
func cefSeverity(env events.Envelope) int {
	switch severityOf(env) {
	case 0, 1, 2:
		return 9
	case 3:
		return 7
	case 4:
		return 5
	case 5:
		return 3
	}
	return 1
}

// formatCEF renders an ArcSight Common Event Format record. Field mapping is
// documented in docs/export.md.
func formatCEF(env events.Envelope, version string) string {
	if version == "" {
		version = "dev"
	}
	name := summary(env)
	if d, ok := events.Lookup(env.Type); ok {
		name = d.Description
	}
	var ext []string
	add := func(k, v string) {
		if v != "" {
			ext = append(ext, k+"="+cefExtEscape(v))
		}
	}
	add("rt", strconv.FormatInt(env.At.UTC().UnixMilli(), 10))
	add("externalId", env.ID)
	add("cat", env.Type)
	add("suser", env.Origin.Actor)
	add("src", env.Origin.IP)
	add("cs1Label", "source")
	add("cs1", env.Origin.Source)
	if id := deviceID(env); id != "" {
		add("cs2Label", "deviceId")
		add("cs2", id)
	}
	if env.Origin.RequestID != "" {
		add("cs3Label", "requestId")
		add("cs3", env.Origin.RequestID)
	}
	switch v := env.Data.(type) {
	case *model.Alert:
		add("dvchost", v.DeviceID)
		add("cs4Label", "alertSeverity")
		add("cs4", string(v.Severity))
		add("cs5Label", "alertSource")
		add("cs5", v.Source)
		add("msg", v.Description)
	case *model.Device:
		add("dvc", v.Address)
		add("dvchost", v.Name())
		add("msg", summary(env))
	case *model.Job:
		add("cs4Label", "jobType")
		add("cs4", v.Type)
		add("outcome", string(v.Status))
		add("msg", summary(env))
	case map[string]any:
		add("act", str(v["action"]))
		add("outcome", str(v["result"]))
		if u := str(v["user"]); u != "" && env.Origin.Actor == "" {
			add("suser", u)
		}
		add("msg", summary(env))
		if t := str(v["target"]); t != "" {
			add("cs6Label", "target")
			add("cs6", t)
		}
	default:
		add("msg", summary(env))
	}
	if env.Data != nil {
		if b, err := json.Marshal(env.Data); err == nil {
			s := string(b)
			if len(s) > 4000 {
				s = s[:4000]
			}
			add("flexString1Label", "data")
			add("flexString1", s)
		}
	}
	return fmt.Sprintf("CEF:0|PrintMarshal|PrintMarshal|%s|%s|%s|%d|%s",
		cefHeaderEscape(version), cefHeaderEscape(env.Type), cefHeaderEscape(name), cefSeverity(env), strings.Join(ext, " "))
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "-"
	}
	if len(h) > 255 {
		h = h[:255]
	}
	return h
}
