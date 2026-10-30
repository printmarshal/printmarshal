package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"printmarshal/internal/model"
)

// scopedFixture creates one device inside a static group and one outside it,
// and returns the group plus the ids.
func scopedFixture(t *testing.T, s *Server) (g *model.Group, in, out *model.Device) {
	t.Helper()
	ctx := context.Background()
	var err error
	in, _, err = s.Store.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.10", Serial: "IN1", Make: "Acme", Model: "Acme 100", Method: "snmp"}, map[model.Capability]string{model.CapReadStatus: "snmp"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, _, err = s.Store.UpsertDevice(ctx, &model.Identity{Address: "10.0.0.11", Serial: "OUT1", Make: "Acme", Model: "Acme 100", Method: "snmp"}, map[model.Capability]string{model.CapReadStatus: "snmp"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	g = &model.Group{Name: "site-a", Kind: "static"}
	if err := s.Store.CreateGroup(ctx, g); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.SetGroupMembers(ctx, g.ID, []string{in.ID}); err != nil {
		t.Fatal(err)
	}
	return g, in, out
}

// Security audit run-1, NV-03: the sync API is an alternate reader of the
// device, alert, job and config-write collections and must apply the same
// group scope as the primary lists.
func TestSyncIsGroupScoped(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	g, in, out := scopedFixture(t, s)
	tok := mkToken(t, ts, admin, `{"name":"bot","role":"operator","scope":["`+g.ID+`"]}`).Secret
	code, body := do(t, ts, tok, "GET", "/api/sync/devices", "")
	if code != 200 || !strings.Contains(body, in.ID) {
		t.Fatalf("sync devices in scope: %d %s", code, body)
	}
	if strings.Contains(body, out.ID) || strings.Contains(body, "OUT1") {
		t.Fatalf("sync devices leaked an out-of-scope device: %s", body)
	}
	if _, body := do(t, ts, admin, "GET", "/api/sync/devices", ""); !strings.Contains(body, out.ID) {
		t.Fatalf("admin sync filtered: %s", body)
	}
}

// NV-06: the live stream is a reader too; a scoped subscriber must not
// receive events about devices outside its groups.
func TestStreamIsGroupScoped(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	g, in, out := scopedFixture(t, s)
	tok := mkToken(t, ts, admin, `{"name":"bot","role":"viewer","scope":["`+g.ID+`"]}`).Secret
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer "+tok)
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/api/stream", &websocket.DialOptions{HTTPHeader: hdr})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	time.Sleep(100 * time.Millisecond) // let the subscriber register
	s.Hub.Publish(model.Event{Type: "device.updated", Data: out})
	s.Hub.Publish(model.Event{Type: "alert.raised", Data: &model.Alert{DeviceID: out.ID, Description: "toner"}})
	s.Hub.Publish(model.Event{Type: "device.updated", Data: in})
	_, msg, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(msg), in.ID) || strings.Contains(string(msg), out.ID) {
		t.Fatalf("first message for a scoped subscriber must be the in-scope device, got %s", msg)
	}
}

// NV-05: an API token must not be able to reconfigure a human sign-in
// provider, which would let it mint a session that outlives the token.
func TestTokensCannotManageDirectorySettings(t *testing.T) {
	ts, _ := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	tok := mkToken(t, ts, admin, `{"name":"automation","role":"admin"}`).Secret
	for _, c := range []struct{ method, path, body string }{
		{"PUT", "/api/settings/directory", `{"enabled":false}`},
		{"POST", "/api/settings/directory/test", `{}`},
		{"GET", "/api/settings/directory", ""},
	} {
		if code, body := do(t, ts, tok, c.method, c.path, c.body); code != 403 {
			t.Errorf("%s %s with an api token = %d %s, want 403", c.method, c.path, code, body)
		}
	}
	if code, _ := do(t, ts, admin, "GET", "/api/settings/directory", ""); code != 200 {
		t.Errorf("admin session blocked: %d", code)
	}
}

// NV-07: credentials carry a device or group scope; a group-scoped admin
// token may only create, change or delete credentials bound inside its scope,
// and never a global one that would apply to every device.
func TestScopedAdminTokenCredentialBoundary(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	g, in, out := scopedFixture(t, s)
	tok := mkToken(t, ts, admin, `{"name":"site-admin","role":"admin","scope":["`+g.ID+`"]}`).Secret
	code, body := do(t, ts, admin, "POST", "/api/credentials", `{"name":"foreign","type":"snmp_community","scope":"device:`+out.ID+`","secret":{"community":"x"}}`)
	if code != 201 {
		t.Fatalf("create foreign credential: %d %s", code, body)
	}
	foreignID := strings.SplitN(strings.SplitN(body, `"id":"`, 2)[1], `"`, 2)[0]
	if code, _ := do(t, ts, tok, "PATCH", "/api/credentials/"+foreignID, `{"purpose":"write"}`); code != 403 {
		t.Errorf("scoped token changed a foreign credential's purpose: %d", code)
	}
	if code, _ := do(t, ts, tok, "DELETE", "/api/credentials/"+foreignID, ""); code != 403 {
		t.Errorf("scoped token deleted a foreign credential: %d", code)
	}
	if code, _ := do(t, ts, tok, "POST", "/api/credentials", `{"name":"f2","type":"snmp_community","scope":"device:`+out.ID+`","secret":{"community":"x"}}`); code != 403 {
		t.Errorf("scoped token created a credential for a foreign device: %d", code)
	}
	if code, _ := do(t, ts, tok, "POST", "/api/credentials", `{"name":"global","type":"snmp_community","secret":{"community":"x"}}`); code != 403 {
		t.Errorf("scoped token created a global credential: %d", code)
	}
	if code, body := do(t, ts, tok, "POST", "/api/credentials", `{"name":"mine","type":"snmp_community","scope":"device:`+in.ID+`","secret":{"community":"x"}}`); code != 201 {
		t.Errorf("scoped token refused an in-scope credential: %d %s", code, body)
	}
	if _, body := do(t, ts, tok, "GET", "/api/credentials", ""); strings.Contains(body, "foreign") {
		t.Errorf("scoped token lists a foreign credential: %s", body)
	}
}

// NV-04: the scope check peeks at JSON bodies, but only when the request
// says application/json; the handlers decode the body regardless. A scoped
// principal must not be able to name foreign targets by changing the
// Content-Type or by appending data after the JSON value.
func TestBodyTargetsAreCheckedRegardlessOfContentType(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	g, _, out := scopedFixture(t, s)
	tok := mkToken(t, ts, admin, `{"name":"bot","role":"operator","scope":["`+g.ID+`"]}`).Secret
	send := func(ct, body string) int {
		req, _ := http.NewRequest("POST", ts.URL+"/api/profiles/nonexistent/apply", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		if ct != "" {
			req.Header.Set("Content-Type", ct)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	target := `{"target":{"deviceIds":["` + out.ID + `"]},"dryRun":true}`
	if code := send("application/json", target); code != 403 {
		t.Fatalf("baseline json: %d, want 403", code)
	}
	if code := send("text/plain", target); code != 403 {
		t.Errorf("text/plain body bypassed the scope check: %d", code)
	}
	if code := send("", target); code != 403 {
		t.Errorf("missing content type bypassed the scope check: %d", code)
	}
	if code := send("application/json", target+" trailing"); code != 403 && code != 400 {
		t.Errorf("trailing data after the JSON value reached the handler: %d", code)
	}
}

var _ = httptest.NewServer
