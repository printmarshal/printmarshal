package ipp

import (
	"context"
	"crypto/md5" //nolint:gosec // verifying RFC 7616 MD5 digests in a test
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"strings"
	"sync"
	"testing"

	"printmarshal/internal/model"
)

// ippServer is an httptest-backed printer that can demand authentication.
type ippServer struct {
	t  *testing.T
	mu sync.Mutex
	// challenge is the WWW-Authenticate value sent when a request needs
	// authentication and carries none that verify accepts; "" = open.
	challenge string
	// challengeOps limits the challenge to these operations; nil = every request.
	challengeOps map[uint16]bool
	// verify reports whether the Authorization header satisfies the challenge.
	verify func(r *http.Request, body []byte) bool

	sets      int
	gets      int
	setAuths  []string // Authorization header on each accepted Set
	setUsers  []string // requesting-user-name on each accepted Set
	rejected  int      // 401s issued
	lastReqID uint32
}

func (s *ippServer) handler() http.HandlerFunc {
	okResp := encodeMsg(s.t, &Message{Version: [2]byte{2, 0}, RequestID: 1, StatusCode: StatusOK,
		Groups: []Group{{Tag: TagOperation, Attributes: []Attribute{{Tag: TagCharset, Name: "attributes-charset", Values: []any{"utf-8"}}}}}})
	getResp := encodeMsg(s.t, settableResponse())
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		m, err := Decode(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		s.lastReqID = m.RequestID
		needs := s.challenge != "" && (s.challengeOps == nil || s.challengeOps[m.StatusCode])
		if needs && (r.Header.Get("Authorization") == "" || s.verify == nil || !s.verify(r, body)) {
			s.rejected++
			w.Header().Set("WWW-Authenticate", s.challenge)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/ipp")
		switch m.StatusCode {
		case OpGetPrinterAttributes:
			s.gets++
			_, _ = w.Write(getResp)
		case OpSetPrinterAttributes:
			s.sets++
			s.setAuths = append(s.setAuths, r.Header.Get("Authorization"))
			s.setUsers = append(s.setUsers, m.String("requesting-user-name"))
			_, _ = w.Write(okResp)
		default:
			http.Error(w, "unsupported", http.StatusBadRequest)
		}
	}
}

var locationChange = []model.SettingChange{{Key: "identity.location", Value: "Lobby"}}

func mustURL(s string) *neturl.URL {
	u, err := neturl.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}

func newDriverFor(urls ...string) *Driver {
	eps := make([]string, 0, len(urls))
	for _, u := range urls {
		eps = append(eps, u+"/ipp/print")
	}
	return New(NewTransport(DefaultOptions()), Options{Endpoints: eps, InsecureTLS: true})
}

// H-016: a Basic challenge is answered once with the bound admin credential,
// and the admin user is the requesting-user-name.
func TestWriteAnswersBasicChallenge(t *testing.T) {
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:s3cret"))
	s := &ippServer{t: t, challenge: `Basic realm="printer"`, verify: func(r *http.Request, _ []byte) bool {
		return r.Header.Get("Authorization") == want
	}}
	srv := httptest.NewServer(s.handler())
	defer srv.Close()
	d := newDriverFor(srv.URL)
	res, err := d.WriteConfig(context.Background(), &model.Device{ID: "d1", Address: "unused"}, model.Creds{AdminUser: "admin", AdminPass: "s3cret"}, locationChange, model.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Results[0].Status != model.SettingWritten {
		t.Fatalf("result = %+v", res.Results[0])
	}
	if s.sets != 1 || s.setAuths[0] != want {
		t.Errorf("sets=%d auths=%q", s.sets, s.setAuths)
	}
	if s.setUsers[0] != "admin" {
		t.Errorf("requesting-user-name = %q, want the bound admin user", s.setUsers[0])
	}
	// Read before, write, read back: each challenged exactly once.
	if s.rejected != 3 {
		t.Errorf("401s = %d, want one per request", s.rejected)
	}
}

// H-016: Digest (RFC 7616) with MD5 and SHA-256; the server recomputes the
// response from the Authorization parameters.
func TestWriteAnswersDigestChallenge(t *testing.T) {
	for _, algo := range []string{"MD5", "SHA-256", "SHA-256-sess", ""} {
		t.Run("algo="+algo, func(t *testing.T) {
			const realm, nonce, opaque = "HP LaserJet", "dcd98b7102dd2f0e8b11d0f600bfb0c093", "5ccc069c403ebaf9f0171e9517f40e41"
			challenge := `Digest realm="` + realm + `", qop="auth", nonce="` + nonce + `", opaque="` + opaque + `"`
			if algo != "" {
				challenge += ", algorithm=" + algo
			}
			s := &ippServer{t: t, challenge: challenge, challengeOps: map[uint16]bool{OpSetPrinterAttributes: true}}
			s.verify = func(r *http.Request, body []byte) bool {
				cs := parseChallenges([]string{r.Header.Get("Authorization")})
				if len(cs) != 1 || cs[0].scheme != "digest" {
					t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
					return false
				}
				p := cs[0].params
				if p["username"] != "admin" || p["realm"] != realm || p["nonce"] != nonce || p["opaque"] != opaque ||
					p["uri"] != "/ipp/print" || p["qop"] != "auth" || p["nc"] != "00000001" || len(p["cnonce"]) != 32 {
					t.Errorf("digest params = %v", p)
					return false
				}
				if (algo == "" && p["algorithm"] != "") || (algo != "" && p["algorithm"] != algo) {
					t.Errorf("algorithm = %q, want %q", p["algorithm"], algo)
				}
				var newHash func() hash.Hash = md5.New
				if strings.HasPrefix(strings.ToUpper(algo), "SHA-256") {
					newHash = sha256.New
				}
				h := func(parts ...string) string {
					hh := newHash()
					hh.Write([]byte(strings.Join(parts, ":")))
					return hex.EncodeToString(hh.Sum(nil))
				}
				a1 := h("admin", realm, "s3cret")
				if strings.HasSuffix(strings.ToUpper(algo), "-SESS") {
					a1 = h(a1, nonce, p["cnonce"])
				}
				want := h(a1, nonce, "00000001", p["cnonce"], "auth", h("POST", "/ipp/print"))
				return p["response"] == want
			}
			srv := httptest.NewServer(s.handler())
			defer srv.Close()
			d := newDriverFor(srv.URL)
			res, err := d.WriteConfig(context.Background(), &model.Device{ID: "d1", Address: "unused"}, model.Creds{AdminUser: "admin", AdminPass: "s3cret"}, locationChange, model.WriteOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if res.Results[0].Status != model.SettingWritten || s.sets != 1 || s.rejected != 1 {
				t.Errorf("result=%+v sets=%d rejected=%d", res.Results[0], s.sets, s.rejected)
			}
		})
	}
}

// H-016: a wrong credential is reported as rejected, not retried.
func TestWriteRejectedCredential(t *testing.T) {
	s := &ippServer{t: t, challenge: `Basic realm="printer"`, challengeOps: map[uint16]bool{OpSetPrinterAttributes: true},
		verify: func(*http.Request, []byte) bool { return false }}
	srv := httptest.NewServer(s.handler())
	defer srv.Close()
	res, err := newDriverFor(srv.URL).WriteConfig(context.Background(), &model.Device{ID: "d1"}, model.Creds{AdminUser: "admin", AdminPass: "wrong"}, locationChange, model.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r := res.Results[0]
	if r.Status != model.SettingFailed || r.Error != ErrAuthRejected.Error() || s.rejected != 2 {
		t.Errorf("result=%+v rejected=%d", r, s.rejected)
	}
}

// H-016: without a bound credential the setting fails with a clear reason
// instead of a bare "http 401".
func TestWriteWithoutCredentialExplains(t *testing.T) {
	s := &ippServer{t: t, challenge: `Basic realm="printer"`, challengeOps: map[uint16]bool{OpSetPrinterAttributes: true}}
	srv := httptest.NewServer(s.handler())
	defer srv.Close()
	res, err := newDriverFor(srv.URL).WriteConfig(context.Background(), &model.Device{ID: "d1"}, model.Creds{}, locationChange, model.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r := res.Results[0]
	if r.Status != model.SettingFailed || !strings.Contains(r.Error, "no admin credential is bound") || strings.Contains(r.Error, "http 401") {
		t.Errorf("result = %+v", r)
	}
	if s.sets != 0 || len(res.Rollback) != 0 {
		t.Errorf("sets=%d rollback=%v", s.sets, res.Rollback)
	}
	// An unsupported scheme is named too.
	s.challenge = `Negotiate`
	res, err = newDriverFor(srv.URL).WriteConfig(context.Background(), &model.Device{ID: "d1"}, model.Creds{AdminUser: "admin", AdminPass: "x"}, locationChange, model.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if r := res.Results[0]; r.Status != model.SettingFailed || !strings.Contains(r.Error, "unsupported authentication scheme") {
		t.Errorf("result = %+v", r)
	}
}

// H-017: writes go to the TLS endpoint even when the configured order lists
// plaintext first, and the presented certificate is pinned.
func TestWritePrefersTLSAndPinsCertificate(t *testing.T) {
	plain := &ippServer{t: t}
	tlsS := &ippServer{t: t}
	plainSrv := httptest.NewServer(plain.handler())
	defer plainSrv.Close()
	tlsSrv := httptest.NewTLSServer(tlsS.handler())
	defer tlsSrv.Close()

	d := newDriverFor(plainSrv.URL, tlsSrv.URL)
	dev := &model.Device{ID: "d1", Address: "unused"}
	if _, err := d.WriteConfig(context.Background(), dev, model.Creds{}, locationChange, model.WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if tlsS.sets != 1 || plain.sets != 0 || plain.gets != 0 {
		t.Fatalf("tls sets=%d plain sets=%d plain gets=%d", tlsS.sets, plain.sets, plain.gets)
	}
	want := Fingerprint(tlsSrv.Certificate())
	if fp, ok := d.PinnedCertificate(dev); !ok || fp != want || !strings.HasPrefix(fp, "sha256:") {
		t.Fatalf("pin = %q %v, want %q", fp, ok, want)
	}

	// A different certificate is refused before anything is written, and the
	// plaintext endpoint is not tried instead.
	d.pinMu.Lock()
	d.pins[pinKey(dev)] = "sha256:" + strings.Repeat("00", 32)
	d.pinMu.Unlock()
	_, err := d.WriteConfig(context.Background(), dev, model.Creds{}, locationChange, model.WriteOptions{})
	if !errors.Is(err, ErrPinMismatch) || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "0000") {
		t.Fatalf("err = %v", err)
	}
	if tlsS.sets != 1 || plain.sets != 0 || plain.gets != 0 {
		t.Errorf("after mismatch: tls sets=%d plain sets=%d plain gets=%d", tlsS.sets, plain.sets, plain.gets)
	}
	// Reads are unaffected by the pin.
	if _, err := d.ReadStatus(context.Background(), dev, model.Creds{}); err != nil {
		t.Errorf("read after pin mismatch: %v", err)
	}

	// Once pinned, a write cannot be downgraded to plaintext by the TLS
	// endpoint going away.
	d.pinMu.Lock()
	d.pins[pinKey(dev)] = want
	d.pinMu.Unlock()
	tlsSrv.Close()
	_, err = d.WriteConfig(context.Background(), dev, model.Creds{}, locationChange, model.WriteOptions{})
	if !errors.Is(err, ErrPinMismatch) || !strings.Contains(err.Error(), "plaintext") {
		t.Fatalf("downgrade err = %v", err)
	}
	if plain.sets != 0 {
		t.Errorf("plaintext write happened after a pin: %d", plain.sets)
	}
}

// H-017: ippWriteRequireTLS refuses plaintext writes with a clear error.
func TestWriteRequireTLS(t *testing.T) {
	plain := &ippServer{t: t}
	plainSrv := httptest.NewServer(plain.handler())
	defer plainSrv.Close()
	d := New(NewTransport(DefaultOptions()), Options{Endpoints: []string{plainSrv.URL + "/ipp/print"}, InsecureTLS: true, WriteRequireTLS: true})
	dev := &model.Device{ID: "d1", Address: "unused"}
	_, err := d.WriteConfig(context.Background(), dev, model.Creds{}, locationChange, model.WriteOptions{})
	if !errors.Is(err, ErrPlaintextWrite) || !strings.Contains(err.Error(), "ippWriteRequireTLS") {
		t.Fatalf("err = %v", err)
	}
	if plain.sets != 0 || plain.gets != 0 {
		t.Errorf("plaintext traffic with TLS required: sets=%d gets=%d", plain.sets, plain.gets)
	}
	// Reads still use the configured plaintext endpoint.
	if _, err := d.ReadStatus(context.Background(), dev, model.Creds{}); err != nil {
		t.Errorf("read: %v", err)
	}
	// A TLS endpoint satisfies the requirement.
	tlsS := &ippServer{t: t}
	tlsSrv := httptest.NewTLSServer(tlsS.handler())
	defer tlsSrv.Close()
	d = New(NewTransport(DefaultOptions()), Options{Endpoints: []string{plainSrv.URL + "/ipp/print", tlsSrv.URL + "/ipp/print"}, InsecureTLS: true, WriteRequireTLS: true})
	if _, err := d.WriteConfig(context.Background(), dev, model.Creds{}, locationChange, model.WriteOptions{}); err != nil {
		t.Fatal(err)
	}
	if tlsS.sets != 1 || plain.sets != 0 {
		t.Errorf("tls sets=%d plain sets=%d", tlsS.sets, plain.sets)
	}
}

func TestWriteOrder(t *testing.T) {
	got := writeOrder(defaultEndpoints)
	want := []string{
		"https://%s:631/ipp/print", "https://%s:631/ipp", "https://%s:631/ipp/printer", "https://%s:631/", "https://%s:443/ipp/print",
		"http://%s:631/ipp/print", "http://%s:631/ipp", "http://%s:631/ipp/printer", "http://%s:631/", "http://%s:80/ipp/print",
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("writeOrder =\n%v\nwant\n%v", got, want)
	}
	if got := writeOrder([]string{"http://%s:6631/ipp/print"}); strings.Join(got, " ") != "https://%s:6631/ipp/print http://%s:6631/ipp/print" {
		t.Errorf("custom = %v", got)
	}
}

func TestParseChallenges(t *testing.T) {
	cs := parseChallenges([]string{`Basic realm="a, b", charset="UTF-8", Digest realm="r\"q", nonce=abc, qop="auth,auth-int", algorithm=SHA-256`, `Negotiate`})
	if len(cs) != 3 || cs[0].scheme != "basic" || cs[1].scheme != "digest" || cs[2].scheme != "negotiate" {
		t.Fatalf("challenges = %+v", cs)
	}
	if cs[0].params["realm"] != "a, b" || cs[1].params["realm"] != `r"q` || cs[1].params["nonce"] != "abc" || cs[1].params["qop"] != "auth,auth-int" || cs[1].params["algorithm"] != "SHA-256" {
		t.Errorf("params = %+v / %+v", cs[0].params, cs[1].params)
	}
	// Digest is preferred over Basic; an unknown digest algorithm falls back to Basic.
	if h, _ := authorizationFor(cs, "POST", mustURL("https://p/ipp/print"), nil, Auth{User: "u", Password: "p"}); !strings.HasPrefix(h, "Digest ") {
		t.Errorf("preferred = %q", h)
	}
	cs[1].params["algorithm"] = "SHA-512-256"
	if h, _ := authorizationFor(cs, "POST", mustURL("https://p/ipp/print"), nil, Auth{User: "u", Password: "p"}); !strings.HasPrefix(h, "Basic ") {
		t.Errorf("fallback = %q", h)
	}
	if _, err := authorizationFor(cs[2:], "POST", mustURL("https://p/"), nil, Auth{User: "u"}); !errors.Is(err, ErrUnsupportedChallenge) {
		t.Errorf("negotiate err = %v", err)
	}
}
