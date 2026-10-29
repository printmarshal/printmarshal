package model

import "testing"

func TestResolveWebURL(t *testing.T) {
	cases := []struct {
		name string
		d    Device
		want string
	}{
		{"nothing", Device{}, ""},
		{"override wins", Device{WebURLOverride: "https://printer-7/admin", AdminURL: "http://10.0.0.7/", WebURLProbed: "http://10.0.0.7/"}, "https://printer-7/admin"},
		{"advertised web ui", Device{AdminURL: "http://10.0.0.7/", WebURLProbed: "https://10.0.0.7/"}, "http://10.0.0.7/"},
		{"advertised ipp port is skipped", Device{AdminURL: "http://10.0.0.7:631/", WebURLProbed: "https://10.0.0.7/"}, "https://10.0.0.7/"},
		{"advertised ipp path is skipped", Device{AdminURL: "http://10.0.0.7/ipp/print", WebURLProbed: "https://10.0.0.7/"}, "https://10.0.0.7/"},
		{"probed only", Device{WebURLProbed: "http://10.0.0.7/"}, "http://10.0.0.7/"},
		{"probe found none", Device{WebURLProbed: WebURLNone}, ""},
		{"advertised with credentials is skipped", Device{AdminURL: "http://admin:secret@10.0.0.7/"}, ""},
	}
	for _, c := range cases {
		if got := ResolveWebURL(&c.d); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestValidWebURL(t *testing.T) {
	for _, ok := range []string{"", "  ", "http://10.0.0.7/", "https://printer-7.example.net:8443/admin?x=1"} {
		if err := ValidWebURL(ok); err != nil {
			t.Errorf("%q: unexpected %v", ok, err)
		}
	}
	for _, bad := range []string{"ftp://10.0.0.7/", "10.0.0.7", "http://", "http://admin:pw@10.0.0.7/", "javascript:alert(1)"} {
		if err := ValidWebURL(bad); err == nil {
			t.Errorf("%q: expected an error", bad)
		}
	}
}

func TestWebHost(t *testing.T) {
	cases := []struct{ host, addr, want string }{
		{"", "10.0.0.7", "10.0.0.7"},
		{"printer-7.example.net.", "10.0.0.7", "printer-7.example.net"},
		{"", "fe80::1", "[fe80::1]"},
	}
	for _, c := range cases {
		if got := WebHost(&Device{Hostname: c.host, Address: c.addr}); got != c.want {
			t.Errorf("host=%q addr=%q: got %q want %q", c.host, c.addr, got, c.want)
		}
	}
}
