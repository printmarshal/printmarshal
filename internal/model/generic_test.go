package model

import "testing"

func TestIsGenericModel(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"HP ETHERNET MULTI-ENVIRONMENT", true},
		{"  hp ethernet multi-environment  ", true},
		{"HP JetDirect 635n", true},
		{"JETDIRECT", true},
		{"HP Color LaserJet Flow E87760", false},
		{"HP Color LaserJet MFP M283fdw", false},
		{"Acme LaserPrint 4200dn", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsGenericModel(c.in); got != c.want {
			t.Errorf("IsGenericModel(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
