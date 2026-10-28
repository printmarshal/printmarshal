package security

import (
	"testing"

	"printmarshal/internal/model"
)

func doc(kv map[string]any) *model.ConfigDocument {
	d := &model.ConfigDocument{}
	for k, v := range kv {
		d.Settings = append(d.Settings, model.Setting{Key: k, Value: v, Source: "test"})
	}
	return d
}

func TestOperators(t *testing.T) {
	cases := []struct {
		op      string
		cur     any
		want    any
		pass    bool
		wantErr bool
	}{
		{model.OpEq, true, true, true, false},
		{model.OpEq, "true", true, true, false}, // loose bool
		{model.OpEq, false, true, false, false},
		{model.OpEq, float64(1), 1, true, false}, // JSON number vs int
		{model.OpEq, "TLS", "tls", true, false},  // case-insensitive strings
		{model.OpNe, "1.0", "1.2", true, false},
		{model.OpGte, "1.2", "1.2", true, false},
		{model.OpGte, "1.3", "1.2", true, false},
		{model.OpGte, "1.1", "1.2", false, false},
		{model.OpGte, 45, 30, true, false},
		{model.OpLte, float64(7), 30, true, false},
		{model.OpLte, "b", "a", false, false},
		{model.OpIn, "1.2", []any{"1.2", "1.3"}, true, false},
		{model.OpIn, "1.0", []any{"1.2", "1.3"}, false, false},
		{model.OpNotIn, "1.0", []any{"1.2", "1.3"}, true, false},
		{model.OpIn, "x", "notalist", false, true},
		{"bogus", 1, 1, false, true},
		{model.OpGte, []any{1}, 1, false, true},
	}
	for _, c := range cases {
		got, err := Compare(c.op, c.cur, c.want)
		if (err != nil) != c.wantErr {
			t.Errorf("%s(%v,%v): err=%v wantErr=%v", c.op, c.cur, c.want, err, c.wantErr)
			continue
		}
		if !c.wantErr && got != c.pass {
			t.Errorf("%s(%v,%v)=%v want %v", c.op, c.cur, c.want, got, c.pass)
		}
	}
}

func TestEvaluateStatuses(t *testing.T) {
	d := doc(map[string]any{"protocols.telnet.enabled": true, "security.adminPassword": "hunter2", "security.tls.minVersion": "1.2", "network.ntp.server": ""})
	r := Evaluate(model.PolicyItem{ID: "a", Key: "protocols.telnet.enabled", Operator: model.OpEq, Value: false, Severity: "critical"}, d)
	if r.Status != model.ItemFail || r.Current != true || r.Message == "" {
		t.Errorf("fail expected: %+v", r)
	}
	r = Evaluate(model.PolicyItem{ID: "b", Key: "protocols.ftp.enabled", Operator: model.OpEq, Value: false}, d)
	if r.Status != model.ItemUnknown {
		t.Errorf("missing setting must be unknown, got %+v", r)
	}
	r = Evaluate(model.PolicyItem{ID: "c", Key: "security.adminPassword", Operator: model.OpSet}, d)
	if r.Status != model.ItemPass || r.Current != "<set>" {
		t.Errorf("secret must pass as <set>, never leak: %+v", r)
	}
	r = Evaluate(model.PolicyItem{ID: "d", Key: "network.ntp.server", Operator: model.OpSet}, d)
	if r.Status != model.ItemFail {
		t.Errorf("empty string is unset: %+v", r)
	}
	r = Evaluate(model.PolicyItem{ID: "e", Key: "protocols.ftp.enabled", Operator: model.OpUnset}, d)
	if r.Status != model.ItemPass {
		t.Errorf("absent setting is unset: %+v", r)
	}
	r = Evaluate(model.PolicyItem{ID: "f", Key: "security.tls.minVersion", Operator: model.OpGte, Value: "1.2"}, d)
	if r.Status != model.ItemPass {
		t.Errorf("tls 1.2 >= 1.2: %+v", r)
	}
	if r.Name != "Minimum TLS version" {
		t.Errorf("name must come from the catalogue, got %q", r.Name)
	}
}

func TestScoreIsSeverityWeightedAndExcludesUnknown(t *testing.T) {
	items := []model.ItemResult{
		{ItemID: "1", Severity: "critical", Status: model.ItemPass}, // 3
		{ItemID: "2", Severity: "high", Status: model.ItemFail},     // 2
		{ItemID: "3", Severity: "medium", Status: model.ItemPass},   // 1
		{ItemID: "4", Severity: "low", Status: model.ItemFail},      // .5
		{ItemID: "5", Severity: "critical", Status: model.ItemUnknown},
	}
	score, passed, failed, unknown, compliant := Score(items)
	if passed != 2 || failed != 2 || unknown != 1 || compliant {
		t.Errorf("counts: %d %d %d %v", passed, failed, unknown, compliant)
	}
	// (3+1)/(3+2+1+0.5) = 61.5%
	if score != 61.5 {
		t.Errorf("score = %v want 61.5", score)
	}
	score, _, _, _, compliant = Score([]model.ItemResult{{Severity: "low", Status: model.ItemPass}, {Severity: "critical", Status: model.ItemUnknown}})
	if score != 100 || !compliant {
		t.Errorf("all-known-pass must be 100/compliant: %v %v", score, compliant)
	}
	score, _, _, _, compliant = Score([]model.ItemResult{{Severity: "critical", Status: model.ItemUnknown}})
	if score != 0 || compliant {
		t.Errorf("nothing verified must be 0/non-compliant: %v %v", score, compliant)
	}
}

func TestBaselineValidatesAndCoversSpec(t *testing.T) {
	p := Baseline()
	if err := ValidatePolicy(p); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"protocols.snmp.v1v2cEnabled": true, "protocols.snmp.v3Enabled": true, "protocols.telnet.enabled": true, "protocols.ftp.enabled": true,
		"protocols.https.enabled": true, "protocols.https.redirect": true, "security.tls.minVersion": true, "security.adminPassword.set": true,
		"security.cert.identityPresent": true, "security.cert.identityDaysLeft": true, "protocols.raw9100.enabled": true, "protocols.wsd.enabled": true,
		"protocols.slp.enabled": true, "protocols.llmnr.enabled": true}
	for _, it := range p.Items {
		delete(want, it.Key)
		if it.Key == "protocols.raw9100.enabled" && (it.Remediate || it.Severity != "medium") {
			t.Errorf("raw 9100 must be medium and assess-only: %+v", it)
		}
		if _, ok := model.CatalogByKey[it.Key]; !ok {
			t.Errorf("%s is not a catalogue key", it.Key)
		}
	}
	if len(want) > 0 {
		t.Errorf("baseline misses %v", want)
	}
}

func TestValidatePolicy(t *testing.T) {
	bad := []model.Policy{
		{Name: "", Items: nil},
		{Name: "x", Items: []model.PolicyItem{{Key: "a", Operator: "weird", Value: 1}}},
		{Name: "x", Items: []model.PolicyItem{{Key: "a", Operator: model.OpIn, Value: "notalist"}}},
		{Name: "x", Items: []model.PolicyItem{{Key: "a", Operator: model.OpEq}}},
		{Name: "x", Items: []model.PolicyItem{{Key: "a", Operator: model.OpEq, Value: 1, Severity: "fatal"}}},
		{Name: "x", Items: []model.PolicyItem{{ID: "d", Key: "a", Operator: model.OpEq, Value: 1}, {ID: "d", Key: "b", Operator: model.OpEq, Value: 1}}},
		{Name: "x", Items: []model.PolicyItem{{Key: "a", Operator: model.OpGte, Value: 1, Remediate: true}}},
		{Name: "x", Items: []model.PolicyItem{{Key: "a", Operator: model.OpEq, Value: 1, ManagementPath: "usb"}}},
	}
	for i, p := range bad {
		if err := ValidatePolicy(&p); err == nil {
			t.Errorf("case %d should fail", i)
		}
	}
	p := model.Policy{Name: "ok", Items: []model.PolicyItem{{Key: "protocols.ftp.enabled", Value: false}}}
	if err := ValidatePolicy(&p); err != nil {
		t.Fatal(err)
	}
	it := p.Items[0]
	if it.ID != it.Key || it.Operator != model.OpEq || it.Severity != "medium" || it.Name != "FTP enabled" {
		t.Errorf("defaults not filled: %+v", it)
	}
}

func TestManagementPaths(t *testing.T) {
	d := &model.Device{Capabilities: map[model.Capability]string{model.CapReadStatus: "snmp", model.CapReadSupplies: "ipp", model.CapWriteSecurity: "acmeplugin"}}
	paths := ManagementPaths(d)
	if !paths["snmp"] || !paths["ipp"] || !paths["http"] || len(paths) != 3 {
		t.Errorf("paths = %v", paths)
	}
	if p := ManagementPaths(&model.Device{IdentifyMethods: []string{"snmp"}}); !p["snmp"] || len(p) != 1 {
		t.Errorf("identify fallback = %v", p)
	}
}
