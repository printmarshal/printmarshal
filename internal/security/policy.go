package security

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"printmarshal/internal/model"
)

// --- operators ----------------------------------------------------------------

// Evaluate assesses one policy item against a security document and returns
// the item result (spec §8.5 "assess"). A setting the device does not expose
// yields ItemUnknown; it never counts as a failure.
func Evaluate(item model.PolicyItem, doc *model.ConfigDocument) model.ItemResult {
	r := model.ItemResult{ItemID: item.ID, Key: item.Key, Name: item.Name, Severity: item.Severity, Desired: item.Value}
	if item.Name == "" {
		if def, ok := model.CatalogByKey[item.Key]; ok {
			r.Name = def.Name
		} else {
			r.Name = item.Key
		}
	}
	var cur *model.Setting
	if doc != nil {
		cur, _ = doc.Get(item.Key)
	}
	// set/unset are about presence, so they never yield unknown.
	switch item.Operator {
	case model.OpSet:
		if cur != nil && present(cur.Value) {
			r.Status, r.Current = model.ItemPass, displayValue(item.Key, cur.Value)
		} else {
			r.Status, r.Message = model.ItemFail, "setting is not set"
		}
		return r
	case model.OpUnset:
		if cur == nil || !present(cur.Value) {
			r.Status = model.ItemPass
		} else {
			r.Status, r.Current, r.Message = model.ItemFail, displayValue(item.Key, cur.Value), "setting is set"
		}
		return r
	}
	if cur == nil {
		r.Status, r.Message = model.ItemUnknown, "setting not readable on this device"
		return r
	}
	r.Current = displayValue(item.Key, cur.Value)
	pass, err := Compare(item.Operator, cur.Value, item.Value)
	if err != nil {
		r.Status, r.Message = model.ItemUnknown, err.Error()
		return r
	}
	if pass {
		r.Status = model.ItemPass
	} else {
		r.Status = model.ItemFail
		r.Message = fmt.Sprintf("is %v, want %s %v", r.Current, item.Operator, item.Value)
		if cur.Source == "probe" {
			r.Message += " (observed by network probe)"
		}
	}
	return r
}

// Compare applies op to (current, desired): eq | ne | gte | lte | in | notIn.
// Values are compared loosely: JSON numbers, bools and their string forms
// ("true", "1.2") compare equal; gte/lte compare numerically when both sides
// parse as numbers (so TLS "1.2" >= "1.1" works), else lexically.
func Compare(op string, current, desired any) (bool, error) {
	switch op {
	case model.OpEq:
		return equal(current, desired), nil
	case model.OpNe:
		return !equal(current, desired), nil
	case model.OpGte, model.OpLte:
		c, err := order(current, desired)
		if err != nil {
			return false, err
		}
		if op == model.OpGte {
			return c >= 0, nil
		}
		return c <= 0, nil
	case model.OpIn, model.OpNotIn:
		list, ok := asList(desired)
		if !ok {
			return false, fmt.Errorf("operator %s needs a list value", op)
		}
		found := false
		for _, v := range list {
			if equal(current, v) {
				found = true
				break
			}
		}
		if op == model.OpIn {
			return found, nil
		}
		return !found, nil
	}
	return false, fmt.Errorf("unknown operator %q", op)
}

// equal compares two loosely-typed values.
func equal(a, b any) bool {
	if fa, ok := asFloat(a); ok {
		if fb, ok := asFloat(b); ok {
			return fa == fb
		}
	}
	if ba, ok := asBool(a); ok {
		if bb, ok := asBool(b); ok {
			return ba == bb
		}
	}
	la, oka := asList(a)
	lb, okb := asList(b)
	if oka && okb {
		if len(la) != len(lb) {
			return false
		}
		for i := range la {
			if !equal(la[i], lb[i]) {
				return false
			}
		}
		return true
	}
	return strings.EqualFold(strings.TrimSpace(fmt.Sprint(a)), strings.TrimSpace(fmt.Sprint(b)))
}

func order(a, b any) (int, error) {
	if fa, ok := asFloat(a); ok {
		if fb, ok := asFloat(b); ok {
			switch {
			case fa < fb:
				return -1, nil
			case fa > fb:
				return 1, nil
			}
			return 0, nil
		}
	}
	if _, ok := asList(a); ok {
		return 0, fmt.Errorf("cannot order a list value")
	}
	return strings.Compare(fmt.Sprint(a), fmt.Sprint(b)), nil
}

func asFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case float32:
		return float64(x), true
	case float64:
		return x, true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f, err == nil
	}
	return 0, false
}

func asBool(v any) (bool, bool) {
	switch x := v.(type) {
	case bool:
		return x, true
	case string:
		switch strings.ToLower(strings.TrimSpace(x)) {
		case "true", "yes", "on", "enabled":
			return true, true
		case "false", "no", "off", "disabled":
			return false, true
		}
	}
	return false, false
}

func asList(v any) ([]any, bool) {
	switch x := v.(type) {
	case []any:
		return x, true
	case []string:
		out := make([]any, len(x))
		for i, s := range x {
			out[i] = s
		}
		return out, true
	}
	return nil, false
}

// present reports whether a value counts as "set": non-empty, non-false, non-zero.
func present(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return strings.TrimSpace(x) != ""
	}
	if f, ok := asFloat(v); ok {
		return f != 0
	}
	if l, ok := asList(v); ok {
		return len(l) > 0
	}
	return true
}

// displayValue redacts secret-typed settings so a current value never leaks
// into assessment results (spec §11).
func displayValue(key string, v any) any {
	if def, ok := model.CatalogByKey[key]; ok && def.Type == "secret" {
		if present(v) {
			return "<set>"
		}
		return "<unset>"
	}
	return v
}

// IsSecretKey reports whether key holds a secret in the catalogue.
func IsSecretKey(key string) bool {
	def, ok := model.CatalogByKey[key]
	return ok && def.Type == "secret"
}

// --- scoring ------------------------------------------------------------------

// Score computes the severity-weighted compliance score (0..100) and counts.
// Unknown items are counted but excluded from the denominator; a device whose
// every item is unknown scores 0 and is not compliant, because nothing was
// verified.
func Score(items []model.ItemResult) (score float64, passed, failed, unknown int, compliant bool) {
	var num, den float64
	for _, it := range items {
		w := model.SeverityWeight(it.Severity)
		switch it.Status {
		case model.ItemPass:
			passed++
			num += w
			den += w
		case model.ItemFail:
			failed++
			den += w
		default:
			unknown++
		}
	}
	if den > 0 {
		score = math.Round(num/den*1000) / 10
	}
	compliant = failed == 0 && passed > 0
	return
}

// Assess evaluates every item of a policy against a document.
func Assess(p *model.Policy, doc *model.ConfigDocument, deviceID, jobID string) *model.Assessment {
	a := &model.Assessment{DeviceID: deviceID, PolicyID: p.ID, JobID: jobID, At: time.Now().UTC(), Items: []model.ItemResult{}}
	for _, it := range p.Items {
		a.Items = append(a.Items, Evaluate(it, doc))
	}
	a.Score, a.Passed, a.Failed, a.Unknown, a.Compliant = Score(a.Items)
	return a
}

// --- validation -----------------------------------------------------------------

// ValidatePolicy checks a policy's items and fills defaults (ids, severity, names).
func ValidatePolicy(p *model.Policy) error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("name is required")
	}
	seen := map[string]bool{}
	for i := range p.Items {
		it := &p.Items[i]
		it.Key = strings.TrimSpace(it.Key)
		if it.Key == "" {
			return fmt.Errorf("item %d: key is required", i+1)
		}
		if it.ID == "" {
			it.ID = it.Key
		}
		if seen[it.ID] {
			return fmt.Errorf("item %q: duplicate id", it.ID)
		}
		seen[it.ID] = true
		if it.Operator == "" {
			it.Operator = model.OpEq
		}
		if !model.ValidOperator(it.Operator) {
			return fmt.Errorf("item %q: unknown operator %q", it.ID, it.Operator)
		}
		switch it.Operator {
		case model.OpIn, model.OpNotIn:
			if _, ok := asList(it.Value); !ok {
				return fmt.Errorf("item %q: operator %s needs a list value", it.ID, it.Operator)
			}
		case model.OpSet, model.OpUnset:
		default:
			if it.Value == nil {
				return fmt.Errorf("item %q: value is required for operator %s", it.ID, it.Operator)
			}
		}
		switch it.Severity {
		case "":
			it.Severity = model.SeverityItemMedium
		case model.SeverityItemCritical, model.SeverityItemHigh, model.SeverityItemMedium, model.SeverityItemLow:
		default:
			return fmt.Errorf("item %q: unknown severity %q", it.ID, it.Severity)
		}
		switch it.ManagementPath {
		case "", "snmp", "ipp", "http":
		default:
			return fmt.Errorf("item %q: managementPath must be snmp, ipp or http", it.ID)
		}
		if it.Name == "" {
			if def, ok := model.CatalogByKey[it.Key]; ok {
				it.Name = def.Name
			} else {
				it.Name = it.Key
			}
		}
		if it.Remediate && it.Remediation == nil && it.Operator != model.OpEq {
			return fmt.Errorf("item %q: remediate needs an explicit remediation unless the operator is eq", it.ID)
		}
	}
	return nil
}

// RemediationFor returns the change applied when an item fails, or nil when
// the item is assess-only or has no derivable change.
func RemediationFor(it model.PolicyItem) *model.SettingChange {
	if !it.Remediate {
		return nil
	}
	if it.Remediation != nil {
		return &model.SettingChange{Key: it.Remediation.Key, Value: it.Remediation.Value}
	}
	if it.Operator == model.OpEq {
		return &model.SettingChange{Key: it.Key, Value: it.Value}
	}
	return nil
}

// --- built-in policy ------------------------------------------------------------

// BaselineName is the name of the built-in policy created on first run.
const BaselineName = "Baseline"

// Baseline returns the built-in policy (spec §8.5): legacy protocols off,
// SNMPv3 on, HTTPS enforced, TLS >= 1.2, an admin password set and a valid
// identity certificate. Raw 9100 is assessed only (medium, remediate=false)
// because turning it off breaks most print queues.
func Baseline() *model.Policy {
	item := func(id, key, op string, value any, sev string, remediate bool, mgmt string) model.PolicyItem {
		name := key
		if def, ok := model.CatalogByKey[key]; ok {
			name = def.Name
		}
		return model.PolicyItem{ID: id, Key: key, Name: name, Operator: op, Value: value, Severity: sev, Remediate: remediate, ManagementPath: mgmt}
	}
	p := &model.Policy{
		Name:        BaselineName,
		Description: "Built-in baseline: legacy protocols off, SNMPv3 and HTTPS on, TLS 1.2+, admin password set, valid identity certificate.",
		CreatedBy:   "system",
		Items: []model.PolicyItem{
			item("snmp-v1v2c-off", "protocols.snmp.v1v2cEnabled", model.OpEq, false, model.SeverityItemHigh, true, "snmp"),
			item("snmp-v3-on", "protocols.snmp.v3Enabled", model.OpEq, true, model.SeverityItemHigh, true, ""),
			item("telnet-off", "protocols.telnet.enabled", model.OpEq, false, model.SeverityItemCritical, true, ""),
			item("ftp-off", "protocols.ftp.enabled", model.OpEq, false, model.SeverityItemCritical, true, ""),
			item("https-on", "protocols.https.enabled", model.OpEq, true, model.SeverityItemHigh, true, ""),
			item("https-redirect", "protocols.https.redirect", model.OpEq, true, model.SeverityItemMedium, true, "http"),
			item("tls-min-1.2", "security.tls.minVersion", model.OpGte, "1.2", model.SeverityItemHigh, true, "http"),
			item("admin-password-set", "security.adminPassword.set", model.OpEq, true, model.SeverityItemCritical, false, ""),
			item("identity-cert-present", "security.cert.identityPresent", model.OpEq, true, model.SeverityItemMedium, false, ""),
			item("identity-cert-not-expiring", "security.cert.identityDaysLeft", model.OpGte, 30, model.SeverityItemMedium, false, ""),
			item("raw-9100-off", "protocols.raw9100.enabled", model.OpEq, false, model.SeverityItemMedium, false, ""),
			item("wsd-off", "protocols.wsd.enabled", model.OpEq, false, model.SeverityItemLow, true, ""),
			item("slp-off", "protocols.slp.enabled", model.OpEq, false, model.SeverityItemLow, true, ""),
			item("llmnr-off", "protocols.llmnr.enabled", model.OpEq, false, model.SeverityItemLow, true, ""),
		},
	}
	p.Items[6].Remediation = &model.SettingChange{Key: "security.tls.minVersion", Value: "1.2"}
	p.Items[9].Description = "Renew the identity certificate (enroll) when fewer than 30 days remain."
	p.Items[10].Description = "Assess only: disabling raw port 9100 breaks most print queues; enable remediation deliberately."
	return p
}
