package model

import "testing"

func TestCleanText(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"clean", "HP Color LaserJet Flow E87760", "HP Color LaserJet Flow E87760"},
		{"hp pml fd e8 header", "\xfd\xe8HP Color LaserJet Flow E87760", "HP Color LaserJet Flow E87760"},
		{"hp pml 01 15 header", "\x01\x15HP Color LaserJet MFP M283fdw", "HP Color LaserJet MFP M283fdw"},
		{"trailing nuls", "Acme LaserPrint\x00\x00", "Acme LaserPrint"},
		{"inner invalid byte", "HP\xffLaser", "HPLaser"},
		{"inner control", "HP\tLaser\x07Jet", "HPLaserJet"},
		{"surrounding space", "  HP LaserJet  ", "HP LaserJet"},
		{"valid utf-8 kept", "Drücker Büro", "Drücker Büro"},
		{"only junk", "\xfd\xe8\x00", ""},
		{"empty", "", ""},
	}
	for _, c := range cases {
		if got := CleanText(c.in); got != c.want {
			t.Errorf("%s: CleanText(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}
