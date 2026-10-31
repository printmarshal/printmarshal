package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"printmarshal/internal/auth"
	"printmarshal/internal/config"
	"printmarshal/internal/model"
	"printmarshal/internal/secrets"
	"printmarshal/internal/store"
)

// The public status board is the first data route outside require; these
// tests pin its boundary (spec R1 to R10 and decision 3).

var allSignals = map[string]any{"location": true, "online": true, "state": true, "paper": true, "toner": true, "alerts": true}

// putBoard saves the board configuration as the given principal.
func putBoard(t *testing.T, ts *httptest.Server, token string, cfg map[string]any) (int, string) {
	t.Helper()
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return do(t, ts, token, "PUT", "/api/settings/public-status", string(b))
}

// getPublic requests a public path with no session, no token and no cookie.
func getPublic(t *testing.T, ts *httptest.Server, path string, hdr map[string]string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest("GET", ts.URL+path, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

// noSuchEndpoint is the mux's generic 404 body, which a disabled board must match.
func noSuchEndpoint(t *testing.T, ts *httptest.Server) string {
	t.Helper()
	resp, body := getPublic(t, ts, "/api/definitely-not-a-route", nil)
	if resp.StatusCode != 404 || !strings.Contains(body, "no such endpoint") {
		t.Fatalf("fallback 404 = %d %s", resp.StatusCode, body)
	}
	return body
}

// seedBoardDevice stores a device with full identity, operator fields, a
// fresh poll (status, trays, supplies) and returns it.
func seedBoardDevice(t *testing.T, st store.Store, n int, displayName string) *model.Device {
	t.Helper()
	ctx := context.Background()
	d, _, err := st.UpsertDevice(ctx, &model.Identity{
		Address: fmt.Sprintf("10.9.8.%d", n), Hostname: fmt.Sprintf("secret-host-%d.corp.example", n),
		MAC: fmt.Sprintf("00:11:22:33:44:%02d", n), Serial: fmt.Sprintf("SERIALXYZ%d", n), UUID: fmt.Sprintf("uuid-secret-%d", n),
		Make: "Acmecorp", Model: "LaserMax 9000", Firmware: "FW-7.7.7", SysName: fmt.Sprintf("sysname-secret-%d", n),
		Method: "snmp",
	}, map[model.Capability]string{model.CapReadStatus: "snmp"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	loc, tag, note := "Library, ground floor", "ASSET-SECRET-77", "note-secret: key under the mat"
	p := store.DevicePatch{Location: &loc, AssetTag: &tag, Notes: &note}
	if displayName != "" {
		p.DisplayName = &displayName
	}
	if _, err := st.PatchDevice(ctx, d.ID, p); err != nil {
		t.Fatal(err)
	}
	pct := func(v int) *int { return &v }
	d, err = st.ApplyPoll(ctx, d.ID, store.PollUpdate{
		Reachable: true, At: time.Now().UTC(),
		Status: &model.Status{Online: true, State: "idle", Severity: model.SeverityOK, Message: "console-secret", At: time.Now().UTC()},
		Trays: []model.Tray{
			{Index: 1, Kind: "input", Name: "Tray 1", Status: "ok", Level: 200, MaxCapacity: 250, Percent: pct(80)},
			{Index: 2, Kind: "input", Name: "Tray 2", Status: "empty", Level: 0, MaxCapacity: 500, Percent: pct(0)},
			{Index: 1, Kind: "output", Name: "Output bin", Status: "ok"},
		},
		Supplies: []model.Supply{
			{Index: 1, Description: "Acmecorp TN-999 Black Toner", Type: "toner", Class: "consumed", Colorant: "black", Level: 80, MaxCapacity: 100, Percent: pct(80)},
			{Index: 2, Description: "Acmecorp TN-999 Cyan Toner", Type: "toner", Class: "consumed", Colorant: "cyan", Level: 3, MaxCapacity: 100, Percent: pct(3)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// publicServer builds a Server with no fleet service (and so no engine): the
// board must be served from the store alone (R5). wrap may decorate the store.
func publicServer(t *testing.T, wrap func(store.Store) store.Store) (*Server, http.Handler, store.Store) {
	t.Helper()
	box, _ := secrets.New(make([]byte, 32))
	raw, err := store.OpenSQLite(t.TempDir()+"/p.db", box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { raw.Close() })
	var st store.Store = raw
	if wrap != nil {
		st = wrap(raw)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := &Server{Store: st, Fleet: nil, Auth: auth.New(raw, time.Hour, false, log), Hub: NewHub(), Cfg: config.Default(), Log: log, Version: "test"}
	return s, s.Handler(), raw
}

func keySet(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func wantKeys(t *testing.T, what string, m map[string]any, want ...string) {
	t.Helper()
	sort.Strings(want)
	if got := keySet(m); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("%s keys = %v, want exactly %v", what, got, want)
	}
}

func decodeBoard(t *testing.T, body string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("board is not a JSON object: %v\n%s", err, body)
	}
	return m
}

func boardDevices(t *testing.T, board map[string]any) []map[string]any {
	t.Helper()
	list, ok := board["devices"].([]any)
	if !ok {
		t.Fatalf("devices is not a list: %#v", board["devices"])
	}
	out := make([]map[string]any, 0, len(list))
	for _, v := range list {
		m, ok := v.(map[string]any)
		if !ok {
			t.Fatalf("device entry is not an object: %#v", v)
		}
		out = append(out, m)
	}
	return out
}

func objects(t *testing.T, v any, what string) []map[string]any {
	t.Helper()
	list, ok := v.([]any)
	if !ok {
		t.Fatalf("%s is not a list: %#v", what, v)
	}
	out := make([]map[string]any, 0, len(list))
	for _, e := range list {
		m, ok := e.(map[string]any)
		if !ok {
			t.Fatalf("%s entry is not an object: %#v", what, e)
		}
		out = append(out, m)
	}
	return out
}

// R1: off by default and absent when off; a PUT that disables the board
// takes effect on the very next request (the cache is invalidated).
func TestPublicStatusIs404WhenOff(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	generic := noSuchEndpoint(t, ts)
	for _, p := range []string{"/api/public/status", "/api/public/status/AAAAAAAAAAAAAAAAAAAAAA"} {
		if resp, body := getPublic(t, ts, p, nil); resp.StatusCode != 404 || body != generic {
			t.Errorf("fresh instance %s = %d %q, want 404 %q", p, resp.StatusCode, body, generic)
		}
	}
	d := seedBoardDevice(t, s.Store, 1, "Lobby")
	if code, body := putBoard(t, ts, admin, map[string]any{"enabled": true, "devices": []map[string]any{{"id": d.ID}}, "show": allSignals}); code != 200 {
		t.Fatalf("enable: %d %s", code, body)
	}
	resp, body := getPublic(t, ts, "/api/public/status", nil)
	if resp.StatusCode != 200 || !strings.Contains(body, `"label":"Lobby"`) {
		t.Fatalf("enabled board without a session = %d %s", resp.StatusCode, body)
	}
	if code, body := putBoard(t, ts, admin, map[string]any{"enabled": false, "devices": []map[string]any{{"id": d.ID}}, "show": allSignals}); code != 200 {
		t.Fatalf("disable: %d %s", code, body)
	}
	if resp, body := getPublic(t, ts, "/api/public/status", nil); resp.StatusCode != 404 || body != generic {
		t.Errorf("right after disabling = %d %q, want 404 %q", resp.StatusCode, body, generic)
	}
}

// R1: while unlisted only the current slug answers; regenerating retires the
// old one at once; a slug sent by the client is ignored.
func TestPublicStatusUnlistedSlugOnly(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	generic := noSuchEndpoint(t, ts)
	d := seedBoardDevice(t, s.Store, 1, "Lobby")
	base := func(extra map[string]any) map[string]any {
		m := map[string]any{"enabled": true, "unlisted": true, "devices": []map[string]any{{"id": d.ID}}, "show": allSignals}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	slugRe := regexp.MustCompile(`^[A-Za-z0-9_-]{22}$`)
	readSlug := func() string {
		t.Helper()
		code, body := do(t, ts, admin, "GET", "/api/settings/public-status", "")
		var cfg struct {
			Slug string `json:"slug"`
		}
		if code != 200 || json.Unmarshal([]byte(body), &cfg) != nil {
			t.Fatalf("read settings: %d %s", code, body)
		}
		return cfg.Slug
	}
	if code, body := putBoard(t, ts, admin, base(map[string]any{"slug": "chosen-by-the-client"})); code != 200 {
		t.Fatalf("unlisted: %d %s", code, body)
	}
	slug := readSlug()
	if !slugRe.MatchString(slug) {
		t.Fatalf("slug %q is not 22 base64url characters", slug)
	}
	if resp, body := getPublic(t, ts, "/api/public/status", nil); resp.StatusCode != 404 || body != generic {
		t.Errorf("plain path while unlisted = %d %q", resp.StatusCode, body)
	}
	if resp, body := getPublic(t, ts, "/api/public/status/"+slug, nil); resp.StatusCode != 200 || !strings.Contains(body, `"label":"Lobby"`) {
		t.Errorf("slug path = %d %s", resp.StatusCode, body)
	}
	wrong := strings.Repeat("A", 22)
	if resp, body := getPublic(t, ts, "/api/public/status/"+wrong, nil); resp.StatusCode != 404 || body != generic {
		t.Errorf("wrong slug = %d %q", resp.StatusCode, body)
	}
	// Saving again keeps the slug; regenerating replaces it.
	if code, body := putBoard(t, ts, admin, base(nil)); code != 200 || readSlug() != slug {
		t.Fatalf("resave changed the slug: %d %s", code, body)
	}
	if code, body := putBoard(t, ts, admin, base(map[string]any{"regenerateSlug": true})); code != 200 {
		t.Fatalf("regenerate: %d %s", code, body)
	}
	fresh := readSlug()
	if fresh == slug || !slugRe.MatchString(fresh) {
		t.Fatalf("regenerated slug %q (old %q)", fresh, slug)
	}
	if resp, body := getPublic(t, ts, "/api/public/status/"+slug, nil); resp.StatusCode != 404 || body != generic {
		t.Errorf("old slug after regenerating = %d %q", resp.StatusCode, body)
	}
	if resp, _ := getPublic(t, ts, "/api/public/status/"+fresh, nil); resp.StatusCode != 200 {
		t.Errorf("new slug = %d", resp.StatusCode)
	}
	// Listed again: the plain path answers and the slug does not.
	if code, body := putBoard(t, ts, admin, base(map[string]any{"unlisted": false})); code != 200 {
		t.Fatalf("listed: %d %s", code, body)
	}
	if resp, _ := getPublic(t, ts, "/api/public/status", nil); resp.StatusCode != 200 {
		t.Errorf("plain path when listed = %d", resp.StatusCode)
	}
	if resp, body := getPublic(t, ts, "/api/public/status/"+fresh, nil); resp.StatusCode != 404 || body != generic {
		t.Errorf("slug path when listed = %d %q", resp.StatusCode, body)
	}
}

// R2: the served JSON has exactly the specified keys per object, decoded as
// maps so that no struct tag, omitempty or embedded field can hide a leak.
func TestPublicBoardKeySet(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	d := seedBoardDevice(t, s.Store, 1, "Lobby")
	if _, _, err := s.Store.RaiseAlert(context.Background(), d.ID, "supply:2", "supply", model.SeverityWarning, "Acmecorp TN-999 Cyan Toner low (3%)"); err != nil {
		t.Fatal(err)
	}
	devs := []map[string]any{{"id": d.ID, "label": "Library ground floor, colour"}}
	if code, body := putBoard(t, ts, admin, map[string]any{"enabled": true, "title": "Library printers", "devices": devs, "show": allSignals}); code != 200 {
		t.Fatalf("enable: %d %s", code, body)
	}
	resp, body := getPublic(t, ts, "/api/public/status", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("board = %d %s", resp.StatusCode, body)
	}
	board := decodeBoard(t, body)
	wantKeys(t, "board", board, "title", "updatedAt", "stale", "devices")
	if board["title"] != "Library printers" {
		t.Errorf("title = %v", board["title"])
	}
	list := boardDevices(t, board)
	if len(list) != 1 {
		t.Fatalf("devices = %d, want 1", len(list))
	}
	dev := list[0]
	wantKeys(t, "device (all switches on)", dev, "label", "location", "online", "state", "paper", "toner", "alerts")
	if dev["label"] != "Library ground floor, colour" || dev["location"] != "Library, ground floor" || dev["online"] != true || dev["state"] != "idle" {
		t.Errorf("device values = %v", dev)
	}
	paper := objects(t, dev["paper"], "paper")
	if len(paper) != 2 { // input trays only
		t.Fatalf("paper = %v, want the two input trays", dev["paper"])
	}
	for _, p := range paper {
		wantKeys(t, "paper entry", p, "name", "status")
	}
	if paper[0]["status"] != "ok" || paper[1]["status"] != "empty" {
		t.Errorf("paper statuses = %v", paper)
	}
	toner := objects(t, dev["toner"], "toner")
	if len(toner) != 2 {
		t.Fatalf("toner = %v", dev["toner"])
	}
	for _, s := range toner {
		wantKeys(t, "toner entry", s, "name", "colorant", "percent", "status")
	}
	if toner[0]["colorant"] != "black" || toner[0]["percent"] != float64(80) || toner[0]["status"] != "ok" || toner[1]["status"] != "low" {
		t.Errorf("toner values = %v", toner)
	}
	alerts := objects(t, dev["alerts"], "alerts")
	if len(alerts) != 1 {
		t.Fatalf("alerts = %v", dev["alerts"])
	}
	wantKeys(t, "alert entry", alerts[0], "severity", "category")
	if alerts[0]["severity"] != "warning" || alerts[0]["category"] != "toner" {
		t.Errorf("alert = %v", alerts[0])
	}

	// Every switch off: a device is its label and nothing else.
	if code, body := putBoard(t, ts, admin, map[string]any{"enabled": true, "devices": devs, "show": map[string]any{}}); code != 200 {
		t.Fatalf("switches off: %d %s", code, body)
	}
	_, body = getPublic(t, ts, "/api/public/status", nil)
	board = decodeBoard(t, body)
	wantKeys(t, "board", board, "title", "updatedAt", "stale", "devices")
	if board["title"] != "Printer status" {
		t.Errorf("default title = %v", board["title"])
	}
	list = boardDevices(t, board)
	if len(list) != 1 {
		t.Fatalf("devices = %d, want 1", len(list))
	}
	wantKeys(t, "device (all switches off)", list[0], "label")
}

// R3, R4: identity, operator notes, alert text, ids and security findings
// never reach the public JSON, whatever the switches say.
func TestPublicBoardNeverCarriesIdentity(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	ctx := context.Background()
	anon := seedBoardDevice(t, s.Store, 1, "") // no display name: must not fall back to hostname, model or address
	named := seedBoardDevice(t, s.Store, 2, "Copy room")
	raise := func(key, source string, sev model.Severity, desc string) *model.Alert {
		a, _, err := s.Store.RaiseAlert(ctx, anon.ID, key, source, sev, desc)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	raise("error:doorOpen", "poll", model.SeverityCritical, "Door open by the desk of SECRETDESC")
	raise("policy:snmp-v1:item", "security", model.SeverityCritical, "SNMPv1 enabled with community SECRETFINDING")
	acked := raise("error:jammed", "poll", model.SeverityCritical, "Jam ACKEDSECRET")
	if _, err := s.Store.AckAlert(ctx, acked.ID, "admin"); err != nil {
		t.Fatal(err)
	}
	cleared := raise("offline", "offline", model.SeverityCritical, "Device unreachable CLEAREDSECRET")
	if _, err := s.Store.ClearAlert(ctx, cleared.ID); err != nil {
		t.Fatal(err)
	}
	devs := []map[string]any{{"id": anon.ID}, {"id": named.ID}}
	if code, body := putBoard(t, ts, admin, map[string]any{"enabled": true, "devices": devs, "show": allSignals}); code != 200 {
		t.Fatalf("enable: %d %s", code, body)
	}
	resp, body := getPublic(t, ts, "/api/public/status", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("board = %d %s", resp.StatusCode, body)
	}
	for _, d := range []*model.Device{anon, named} {
		for _, secret := range []string{d.ID, d.Address, d.Hostname, d.MAC, d.Serial, d.UUID, d.SysName} {
			if strings.Contains(body, secret) {
				t.Errorf("public board carries %q:\n%s", secret, body)
			}
		}
	}
	for _, secret := range []string{"Acmecorp", "LaserMax", "FW-7.7.7", "TN-999", "ASSET-SECRET", "note-secret", "console-secret",
		"SECRETDESC", "SECRETFINDING", "ACKEDSECRET", "CLEAREDSECRET", "security", "jam", "offline", `"id"`} {
		if strings.Contains(body, secret) {
			t.Errorf("public board carries %q:\n%s", secret, body)
		}
	}
	list := boardDevices(t, decodeBoard(t, body))
	if len(list) != 2 {
		t.Fatalf("devices = %d, want 2", len(list))
	}
	if list[0]["label"] != "Printer 1" || list[1]["label"] != "Copy room" {
		t.Errorf("labels = %v, %v; want the configured order with a neutral fallback", list[0]["label"], list[1]["label"])
	}
	alerts := objects(t, list[0]["alerts"], "alerts")
	if len(alerts) != 1 || alerts[0]["severity"] != "critical" || alerts[0]["category"] != "door" {
		t.Errorf("alerts = %v, want only the active, unacknowledged door alert", alerts)
	}
}

// R4: the fixed source/key to category mapping; security findings and
// fleet-level stock alerts are never board alerts ("").
func TestAlertCategoryMapping(t *testing.T) {
	for _, c := range []struct{ source, key, want string }{
		{"offline", "offline", "offline"},
		{"poll", "state:down", "offline"},
		{"poll", "error:offline", "offline"},
		{"trap", "trap:link-down", "offline"},
		{"poll", "paper:A4", "paper"},
		{"poll", "paper:*", "paper"},
		{"poll", "error:lowPaper", "paper"},
		{"poll", "error:media-low", "paper"},
		{"poll", "error:inputTrayMissing", "paper"},
		{"poll", "device:input:inputMediaSupplyEmpty:2", "paper"},
		{"supply", "supply:1", "toner"},
		{"poll", "error:noToner", "toner"},
		{"poll", "error:marker-supply-empty-error", "toner"},
		{"poll", "error:markerSupplyMissing", "toner"},
		{"poll", "device:markerSupplies:markerTonerEmpty:1", "toner"},
		{"poll", "error:doorOpen", "door"},
		{"poll", "error:cover-open-error", "door"},
		{"poll", "device:cover:coverOpen:1", "door"},
		{"poll", "device:generalPrinter:doorOpen:1", "door"},
		{"poll", "error:jammed", "jam"},
		{"poll", "error:media-jam-error", "jam"},
		{"poll", "device:mediaPath:jam:1", "jam"},
		{"poll", "error:serviceRequested", "maintenance"},
		{"poll", "error:overduePreventMaint", "maintenance"},
		{"poll", "device:marker:markerFuserOverTemperature:1", "maintenance"},
		{"poll", "device:markerSupplies:markerWasteTonerReceptacleFull:1", "maintenance"},
		{"poll", "error:outputFull", "other"},
		{"poll", "error:some-new-error", "other"},
		{"poll", "device:paper low in tray 2:4", "other"},
		{"trap", "trap:device-cold-start", "other"},
		{"system", "anything", "other"},
		{"made-up-source", "x", "other"},
		{"security", "policy:snmp-v1:item", ""},
		{"consumable", "consumable:abc", ""},
	} {
		if got := alertCategory(c.source, c.key); got != c.want {
			t.Errorf("alertCategory(%q, %q) = %q, want %q", c.source, c.key, got, c.want)
		}
	}
}

// R5: the board is served from the store with no fleet service and no
// engine at all: a request can never reach a device.
func TestPublicStatusReadsStoreOnly(t *testing.T) {
	s, h, raw := publicServer(t, nil)
	if s.Fleet != nil {
		t.Fatal("the fixture must have no fleet service")
	}
	d := seedBoardDevice(t, raw, 1, "Lobby")
	if err := raw.SetSetting(context.Background(), publicStatusKey, publicStatusConfig{
		Enabled: true, Devices: []publicStatusDevice{{ID: d.ID}}, Show: publicShow{Online: true, State: true, Paper: true, Toner: true, Alerts: true, Location: true},
	}); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	resp, body := getPublic(t, ts, "/api/public/status", nil)
	if resp.StatusCode != 200 || !strings.Contains(body, `"label":"Lobby"`) || !strings.Contains(body, `"online":true`) {
		t.Fatalf("board without a fleet service = %d %s", resp.StatusCode, body)
	}
}

// countingStore counts device loads and makes each one slow, so concurrent
// requests overlap a build.
type countingStore struct {
	store.Store
	loads atomic.Int32
	delay time.Duration
}

func (c *countingStore) ListDevices(ctx context.Context, f store.DeviceFilter) ([]*model.Device, error) {
	c.loads.Add(1)
	time.Sleep(c.delay)
	return c.Store.ListDevices(ctx, f)
}

func (c *countingStore) GetDevice(ctx context.Context, id string) (*model.Device, error) {
	c.loads.Add(1)
	time.Sleep(c.delay)
	return c.Store.GetDevice(ctx, id)
}

// R6: concurrent requests share one build, later ones are served from the
// cache for 15 seconds, and query parameters select nothing.
func TestPublicStatusCacheSharesBuilds(t *testing.T) {
	var cs *countingStore
	s, h, raw := publicServer(t, func(st store.Store) store.Store {
		cs = &countingStore{Store: st, delay: 150 * time.Millisecond}
		return cs
	})
	var skew atomic.Int64
	s.pub.cache.now = func() time.Time { return time.Now().Add(time.Duration(skew.Load())) }
	d := seedBoardDevice(t, raw, 1, "Lobby")
	if err := raw.SetSetting(context.Background(), publicStatusKey, publicStatusConfig{Enabled: true, Devices: []publicStatusDevice{{ID: d.ID}}, Show: publicShow{Online: true}}); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	const n = 10
	var wg sync.WaitGroup
	bodies := make([]string, n)
	codes := make([]int, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req, _ := http.NewRequest("GET", ts.URL+"/api/public/status", nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Error(err)
				return
			}
			defer resp.Body.Close()
			b, _ := io.ReadAll(resp.Body)
			codes[i], bodies[i] = resp.StatusCode, string(b)
		}(i)
	}
	wg.Wait()
	for i := range codes {
		if codes[i] != 200 || bodies[i] != bodies[0] {
			t.Fatalf("request %d = %d %s", i, codes[i], bodies[i])
		}
	}
	if got := cs.loads.Load(); got != 1 {
		t.Fatalf("%d concurrent requests made %d device loads, want 1 shared build", n, got)
	}
	_, again := getPublic(t, ts, "/api/public/status?device="+d.ID+"&filter=all&limit=1", nil)
	if got := cs.loads.Load(); got != 1 || again != bodies[0] {
		t.Fatalf("cached request: loads=%d body=%s (want 1 and the same board)", got, again)
	}
	skew.Store(int64(16 * time.Second))
	if resp, _ := getPublic(t, ts, "/api/public/status", nil); resp.StatusCode != 200 {
		t.Fatalf("after expiry = %d", resp.StatusCode)
	}
	if got := cs.loads.Load(); got != 2 {
		t.Fatalf("after 16 s: loads=%d, want a rebuild (2)", got)
	}
}

// R7: more than 30 requests a minute from one address answer 429 with
// Retry-After: 60, and a forwarded header does not buy a fresh budget.
func TestPublicStatusRateLimit(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	d := seedBoardDevice(t, s.Store, 1, "Lobby")
	if code, body := putBoard(t, ts, admin, map[string]any{"enabled": true, "devices": []map[string]any{{"id": d.ID}}}); code != 200 {
		t.Fatalf("enable: %d %s", code, body)
	}
	for i := 1; i <= 30; i++ {
		if resp, body := getPublic(t, ts, "/api/public/status", nil); resp.StatusCode != 200 {
			t.Fatalf("request %d = %d %s", i, resp.StatusCode, body)
		}
	}
	resp, _ := getPublic(t, ts, "/api/public/status", nil)
	if resp.StatusCode != 429 || resp.Header.Get("Retry-After") != "60" {
		t.Fatalf("31st request = %d Retry-After=%q, want 429 and 60", resp.StatusCode, resp.Header.Get("Retry-After"))
	}
	resp, _ = getPublic(t, ts, "/api/public/status", map[string]string{"X-Forwarded-For": "203.0.113.9", "X-Real-IP": "203.0.113.9"})
	if resp.StatusCode != 429 {
		t.Errorf("forwarded header reset the budget: %d", resp.StatusCode)
	}
}

// R9: admin sessions only; API tokens are refused as for the directory
// settings; every change is in the security log, enabling once more.
func TestPublicStatusSettingsRefuseTokens(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	viewer := login(t, ts, "viewer", "viewerpass1")
	tok := mkToken(t, ts, admin, `{"name":"automation","role":"admin"}`).Secret
	d := seedBoardDevice(t, s.Store, 1, "Lobby")
	body := `{"enabled":true,"devices":[{"id":"` + d.ID + `"}]}`
	for _, c := range []struct{ method, body string }{{"GET", ""}, {"PUT", body}} {
		if code, out := do(t, ts, tok, c.method, "/api/settings/public-status", c.body); code != 403 {
			t.Errorf("%s with an admin api token = %d %s, want 403", c.method, code, out)
		}
		if code, out := do(t, ts, viewer, c.method, "/api/settings/public-status", c.body); code != 403 {
			t.Errorf("%s as viewer = %d %s, want 403", c.method, code, out)
		}
	}
	if resp, _ := getPublic(t, ts, "/api/public/status", nil); resp.StatusCode != 404 {
		t.Fatalf("a refused PUT enabled the board: %d", resp.StatusCode)
	}
	code, out := do(t, ts, admin, "GET", "/api/settings/public-status", "")
	if code != 200 || !strings.Contains(out, `"enabled":false`) || !strings.Contains(out, `"title":"Printer status"`) {
		t.Fatalf("admin default settings: %d %s", code, out)
	}
	if code, out := do(t, ts, admin, "PUT", "/api/settings/public-status", body); code != 200 {
		t.Fatalf("admin enable: %d %s", code, out)
	}
	if code, out := do(t, ts, admin, "PUT", "/api/settings/public-status", `{"enabled":true,"title":"Lobby board","devices":[{"id":"`+d.ID+`"}]}`); code != 200 {
		t.Fatalf("admin change: %d %s", code, out)
	}
	_, audit := do(t, ts, admin, "GET", "/api/audit", "")
	if n := strings.Count(audit, `"action":"public.status.changed"`); n != 2 {
		t.Errorf("public.status.changed entries = %d, want 2:\n%s", n, audit)
	}
	if n := strings.Count(audit, `"action":"public.status.enabled"`); n != 1 {
		t.Errorf("public.status.enabled entries = %d, want 1:\n%s", n, audit)
	}
	var entries []model.AuditEntry
	_ = json.Unmarshal([]byte(audit), &entries)
	for _, e := range entries {
		if strings.HasPrefix(e.Action, "public.status.") && (e.Actor != "admin" || !strings.Contains(e.Target, "devices=1") || !strings.Contains(e.Target, "enabled=true") || !strings.Contains(e.Target, "unlisted=false")) {
			t.Errorf("security entry lacks who/what: %+v", e)
		}
	}
}

// R8, R10: every listed device must exist and be in the caller's group
// scope, at most 200 are listed, and a device deleted later just leaves.
func TestPublicStatusSettingsScope(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	g, in, out := scopedFixture(t, s)

	if code, body := putBoard(t, ts, admin, map[string]any{"enabled": true, "devices": []map[string]any{{"id": "no-such-device"}}}); code != 400 {
		t.Errorf("unknown device = %d %s, want 400", code, body)
	}
	many := make([]map[string]any, 201)
	for i := range many {
		many[i] = map[string]any{"id": fmt.Sprintf("dev-%d", i)}
	}
	if code, body := putBoard(t, ts, admin, map[string]any{"enabled": true, "devices": many}); code != 400 || !strings.Contains(body, "200") {
		t.Errorf("201 devices = %d %s, want 400 naming the limit", code, body)
	}

	// A group-scoped administrator (sessions carry no scope today and tokens
	// are refused, so the handler is called with the principal directly).
	scoped := &model.User{ID: "u-scoped", Username: "site-admin", Role: model.RoleAdmin, Scope: []string{g.ID}}
	put := func(ids ...string) (int, string) {
		devs := make([]map[string]any, 0, len(ids))
		for _, id := range ids {
			devs = append(devs, map[string]any{"id": id})
		}
		b, _ := json.Marshal(map[string]any{"enabled": true, "devices": devs})
		req := httptest.NewRequest("PUT", "/api/settings/public-status", strings.NewReader(string(b)))
		req = req.WithContext(auth.WithUser(req.Context(), scoped))
		rec := httptest.NewRecorder()
		s.putPublicStatusSettings(rec, req)
		return rec.Code, rec.Body.String()
	}
	if code, body := put(in.ID, out.ID); code != 403 {
		t.Errorf("scoped admin publishing a device outside its groups = %d %s, want 403", code, body)
	}
	if _, body := do(t, ts, admin, "GET", "/api/settings/public-status", ""); strings.Contains(body, out.ID) {
		t.Errorf("a refused PUT was saved: %s", body)
	}
	if code, body := put(in.ID); code != 200 {
		t.Errorf("scoped admin publishing its own device = %d %s", code, body)
	}

	if code, body := putBoard(t, ts, admin, map[string]any{"enabled": true, "devices": []map[string]any{{"id": out.ID, "label": "Gone soon"}, {"id": in.ID, "label": "Stays"}}}); code != 200 {
		t.Fatalf("admin publish: %d %s", code, body)
	}
	if err := s.Store.DeleteDevice(context.Background(), out.ID); err != nil {
		t.Fatal(err)
	}
	resp, body := getPublic(t, ts, "/api/public/status", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("board after a delete = %d %s", resp.StatusCode, body)
	}
	list := boardDevices(t, decodeBoard(t, body))
	if len(list) != 1 || list[0]["label"] != "Stays" {
		t.Errorf("devices after a delete = %v", list)
	}
}

// Decision 3: framing is allowed only on the board's page paths and only
// for listed origins; every other path keeps X-Frame-Options: DENY.
func TestPublicStatusFrameHeaders(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	d := seedBoardDevice(t, s.Store, 1, "Lobby")
	devs := []map[string]any{{"id": d.ID}}
	for _, bad := range []string{"https://signage.example.com/path", "javascript:alert(1)", "https://a.example.com; script-src *", "https://user:pw@a.example.com", "ftp://a.example.com", "https://*.example.com", "signage.example.com"} {
		if code, body := putBoard(t, ts, admin, map[string]any{"enabled": true, "devices": devs, "frameOrigins": []string{bad}}); code != 400 {
			t.Errorf("frame origin %q accepted: %d %s", bad, code, body)
		}
	}
	if code, body := putBoard(t, ts, admin, map[string]any{"enabled": true, "devices": devs, "frameOrigins": []string{"https://signage.example.com", "http://wall.local:8080"}}); code != 200 {
		t.Fatalf("enable with origins: %d %s", code, body)
	}
	const want = "frame-ancestors https://signage.example.com http://wall.local:8080"
	for _, p := range []string{"/status", "/status/", "/status/AbCdEfGhIjKlMnOpQrStUv", "/status?wall=1"} {
		resp, _ := getPublic(t, ts, p, nil)
		if got := resp.Header.Get("Content-Security-Policy"); got != want || resp.Header.Get("X-Frame-Options") != "" {
			t.Errorf("%s: CSP=%q XFO=%q, want %q and no X-Frame-Options", p, got, resp.Header.Get("X-Frame-Options"), want)
		}
		if resp.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s lost nosniff", p)
		}
	}
	for _, p := range []string{"/", "/devices", "/settings", "/statusx", "/status-not-the-board", "/api/public/status", "/api/settings/public-status", "/x/status"} {
		resp, _ := getPublic(t, ts, p, nil)
		if resp.Header.Get("X-Frame-Options") != "DENY" || strings.Contains(resp.Header.Get("Content-Security-Policy"), "frame-ancestors") {
			t.Errorf("%s: XFO=%q CSP=%q, want DENY and no frame-ancestors", p, resp.Header.Get("X-Frame-Options"), resp.Header.Get("Content-Security-Policy"))
		}
	}
	// Disabled, or no origins listed: the board's page is DENY again at once.
	if code, body := putBoard(t, ts, admin, map[string]any{"enabled": false, "devices": devs, "frameOrigins": []string{"https://signage.example.com"}}); code != 200 {
		t.Fatalf("disable: %d %s", code, body)
	}
	if resp, _ := getPublic(t, ts, "/status", nil); resp.Header.Get("X-Frame-Options") != "DENY" {
		t.Errorf("disabled board still frameable: XFO=%q", resp.Header.Get("X-Frame-Options"))
	}
	if code, body := putBoard(t, ts, admin, map[string]any{"enabled": true, "devices": devs}); code != 200 {
		t.Fatalf("no origins: %d %s", code, body)
	}
	if resp, _ := getPublic(t, ts, "/status", nil); resp.Header.Get("X-Frame-Options") != "DENY" || resp.Header.Get("Content-Security-Policy") != "" {
		t.Errorf("no origins: XFO=%q CSP=%q", resp.Header.Get("X-Frame-Options"), resp.Header.Get("Content-Security-Policy"))
	}
}
