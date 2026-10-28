package fwversion

import (
	"testing"

	"printmarshal/internal/model"
)

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"", "1.0", -1},
		{"1.0", "", 1},
		{"2.14.7", "2.14.7", 0},
		{"2.14.7", "2.14.8", -1},
		{"2.14.10", "2.14.9", 1},
		{"2.15.0", "2.14.99", 1},
		{"3.0", "3.0.0", 0},
		{"3.0.0.1", "3.0", 1},
		{"v2.14.7", "2.14.7", 0},
		{"Version 2.14.7", "2.14.7", 0},
		{"FW 1.2", "fw1.2", 0},
		{"20240115", "20231201", 1},
		{"2024.01.15", "20240115", 0},
		{"2024-01-15", "2024-02-01", -1},
		{"2.14.7a", "2.14.7", 1},
		{"2.14.7a", "2.14.7b", -1},
		{"2.14.7", "2.14.7rc1", 1},
		{"2.14.7-rc1", "2.14.7-rc2", -1},
		{"2.14.7 beta", "2.14.7", -1},
		{"1.2.3-build4", "1.2.3-build10", -1},
		{"1.2", "1.b", 1},
		{"002.014.007", "2.14.7", 0},
		{"2.14.7", "20240115", -1}, // not both dates: token-wise, 2 < 20240115
		{"ABC", "ABD", -1},
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
		if got := Compare(c.b, c.a); got != -c.want {
			t.Errorf("Compare(%q,%q) = %d, want %d (antisymmetry)", c.b, c.a, got, -c.want)
		}
	}
}

func TestCheck(t *testing.T) {
	pkg := &model.FirmwarePackage{Vendor: "Acme", Models: []string{"4200", "LaserPro 5"}, Version: "2.15.0", MinVersion: "2.10"}
	dev := func(mk, mdl, fw string) *model.Device { return &model.Device{Make: mk, Model: mdl, Firmware: fw} }
	cases := []struct {
		name          string
		pkg           *model.FirmwarePackage
		dev           *model.Device
		allowDown     bool
		wantOK        bool
		wantCurrent   bool
		wantDowngrade bool
	}{
		{"upgrade", pkg, dev("Acme", "Acme LaserPro 4200", "2.14.7"), false, true, false, false},
		{"vendor substring either way", pkg, dev("ACME Corporation", "LaserPro 5x", "2.14.7"), false, true, false, false},
		{"wrong vendor", pkg, dev("Globex", "4200", "2.14.7"), false, false, false, false},
		{"unknown vendor", pkg, dev("", "4200", "2.14.7"), false, false, false, false},
		{"wrong model", pkg, dev("Acme", "Acme InkJet 100", "2.14.7"), false, false, false, false},
		{"model case-insensitive", pkg, dev("Acme", "ACME LASERPRO 5000", "2.14.7"), false, true, false, false},
		{"below min version", pkg, dev("Acme", "4200", "2.9.9"), false, false, false, false},
		{"unknown installed with min version", pkg, dev("Acme", "4200", ""), false, false, false, false},
		{"already current", pkg, dev("Acme", "4200", "v2.15.0"), false, true, true, false},
		{"downgrade refused", pkg, dev("Acme", "4200", "2.16.0"), false, false, false, true},
		{"downgrade allowed", pkg, dev("Acme", "4200", "2.16.0"), true, true, false, true},
		{"no vendor/models/min = any device", &model.FirmwarePackage{Version: "1"}, dev("", "", ""), false, true, false, false},
		{"nil device", pkg, nil, false, false, false, false},
	}
	for _, c := range cases {
		v := Check(c.pkg, c.dev, c.allowDown)
		if v.Compatible != c.wantOK || v.Current != c.wantCurrent || v.Downgrade != c.wantDowngrade {
			t.Errorf("%s: %+v", c.name, v)
		}
		if !v.Compatible && v.Reason == "" {
			t.Errorf("%s: incompatible without a reason", c.name)
		}
	}
}
