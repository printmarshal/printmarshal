package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// Public status board (intent/public-status-dashboard). This is the only
// data surface outside require, so its boundary is spelled out here rather
// than inherited:
//   - Off by default; while off (or on the wrong path) it answers exactly
//     like an unknown /api/ path.
//   - The response is the purpose-built publicBoard below, never a
//     model.Device: a field leaves only because buildPublicBoard puts it
//     there. Alerts are a severity and a fixed category, never their text.
//   - It reads the store only (no fleet service, no engine), from a 15 s
//     in-memory snapshot built once per window, and is rate-limited per
//     peer address. No parameter selects data.

const (
	publicStatusKey      = "public.status"
	publicDefaultTitle   = "Printer status"
	publicMaxTitle       = 60  // characters
	publicMaxLabel       = 80  // characters
	publicMaxDevices     = 200 // R8
	publicMaxOrigins     = 20
	publicMaxBody        = 64 << 10
	publicCacheTTL       = 15 * time.Second
	publicRatePerMinute  = 30
	publicLimiterMaxKeys = 10000
	publicSlugBytes      = 16 // 22 base64url characters
	publicStaleIntervals = 3
	// publicAlertScan bounds the one active-alert query a build makes.
	publicAlertScan = 10000
)

// publicShow holds the per-signal switches of the board.
type publicShow struct {
	Location bool `json:"location"`
	Online   bool `json:"online"`
	State    bool `json:"state"`
	Paper    bool `json:"paper"`
	Toner    bool `json:"toner"`
	Alerts   bool `json:"alerts"`
}

func (sh publicShow) names() []string {
	var out []string
	for _, f := range []struct {
		on   bool
		name string
	}{{sh.Location, "location"}, {sh.Online, "online"}, {sh.State, "state"}, {sh.Paper, "paper"}, {sh.Toner, "toner"}, {sh.Alerts, "alerts"}} {
		if f.on {
			out = append(out, f.name)
		}
	}
	return out
}

// publicStatusDevice is one listed device and its optional public label.
type publicStatusDevice struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// publicStatusConfig is the stored configuration (settings key public.status).
type publicStatusConfig struct {
	Enabled      bool                 `json:"enabled"`
	Unlisted     bool                 `json:"unlisted"`
	Slug         string               `json:"slug"`
	Title        string               `json:"title"`
	FrameOrigins []string             `json:"frameOrigins"`
	Devices      []publicStatusDevice `json:"devices"`
	Show         publicShow           `json:"show"`
}

// publicStatusPut is the PUT body: the configuration, where the slug is
// owned by the server (a client value is ignored), plus a request to issue
// a new slug.
type publicStatusPut struct {
	publicStatusConfig
	RegenerateSlug bool `json:"regenerateSlug"`
}

func defaultPublicStatus() publicStatusConfig {
	return publicStatusConfig{Title: publicDefaultTitle, FrameOrigins: []string{}, Devices: []publicStatusDevice{}}
}

// normalized fills defaults so a stored value from any version reads cleanly.
func (c publicStatusConfig) normalized() publicStatusConfig {
	if strings.TrimSpace(c.Title) == "" {
		c.Title = publicDefaultTitle
	}
	if c.FrameOrigins == nil {
		c.FrameOrigins = []string{}
	}
	if c.Devices == nil {
		c.Devices = []publicStatusDevice{}
	}
	return c
}

func (s *Server) loadPublicStatus(ctx context.Context) (publicStatusConfig, error) {
	var c publicStatusConfig
	if err := s.Store.GetSetting(ctx, publicStatusKey, &c); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return defaultPublicStatus(), nil
		}
		return publicStatusConfig{}, err
	}
	return c.normalized(), nil
}

func (s *Server) savePublicStatus(ctx context.Context, c publicStatusConfig) error {
	return s.Store.SetSetting(ctx, publicStatusKey, c)
}

// --- response (R2): every key here is a deliberate public field -------------

type publicBoard struct {
	Title     string         `json:"title"`
	UpdatedAt *time.Time     `json:"updatedAt"` // newest data time of the listed devices; null when none
	Stale     bool           `json:"stale"`
	Devices   []publicDevice `json:"devices"`
}

// publicDevice: pointers with omitempty so a switched-off signal has no key
// at all while a switched-on one is always present (even empty or false).
type publicDevice struct {
	Label    string          `json:"label"`
	Location *string         `json:"location,omitempty"`
	Online   *bool           `json:"online,omitempty"`
	State    *string         `json:"state,omitempty"`
	Paper    *[]publicTray   `json:"paper,omitempty"`
	Toner    *[]publicSupply `json:"toner,omitempty"`
	Alerts   *[]publicAlert  `json:"alerts,omitempty"`
}

type publicTray struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok | low | empty | unknown
}

type publicSupply struct {
	Name     string `json:"name"`
	Colorant string `json:"colorant"`
	Percent  *int   `json:"percent"`
	Status   string `json:"status"` // ok | low | empty | unknown
}

type publicAlert struct {
	Severity string `json:"severity"`
	Category string `json:"category"`
}

// tonerTypes are the marker supplies shown as toner (RFC 3805
// PrtMarkerSuppliesTypeTC and IPP marker-types, lower case without dashes);
// drums, fusers and waste containers are not.
var tonerTypes = map[string]bool{
	"toner": true, "tonercartridge": true, "ink": true, "inkcartridge": true,
	"inkribbon": true, "solidwax": true, "ribbonwax": true,
}

func isToner(sp model.Supply) bool {
	return sp.Class != "filled" && tonerTypes[strings.ReplaceAll(strings.ToLower(sp.Type), "-", "")]
}

func trayStatus(st string) string {
	switch st {
	case "ok", "full", "nearFull":
		return "ok"
	case "low":
		return "low"
	case "empty":
		return "empty"
	}
	return "unknown"
}

// supplyStatus: unknown when the level is unknown ("some remaining" is not a
// level), empty at 0 %, low at or under the console's warning threshold.
func supplyStatus(pct *int, lowPercent int) string {
	switch {
	case pct == nil:
		return "unknown"
	case *pct <= 0:
		return "empty"
	case *pct <= lowPercent:
		return "low"
	}
	return "ok"
}

// colorantName turns a colorant ("black", "light-cyan") into a name; supply
// descriptions are not used because they carry vendor and part numbers (R3).
func colorantName(c string) string {
	c = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(c, "-", " "), "_", " "))
	if c == "" {
		return ""
	}
	words := strings.Fields(strings.ToLower(c))
	for i, w := range words {
		r, n := utf8.DecodeRuneInString(w)
		words[i] = strings.ToUpper(string(r)) + w[n:]
	}
	return strings.Join(words, " ")
}

// deviceDataTime is when the poller last had something to say about d.
func deviceDataTime(d *model.Device) time.Time {
	t := d.LastSeen
	if d.Status != nil && d.Status.At.After(t) {
		t = d.Status.At
	}
	if d.LastPolled != nil && d.LastPolled.After(t) {
		t = *d.LastPolled
	}
	return t
}

var publicSeverityRank = map[model.Severity]int{model.SeverityCritical: 0, model.SeverityWarning: 1}

// buildPublicBoard is the projection (R2 to R4): a pure function from loaded
// records to the public structure. devices may come in any order and may
// miss ids (deleted devices); the board follows the configured order.
func buildPublicBoard(cfg publicStatusConfig, devices []*model.Device, alerts []*model.Alert, now time.Time, pollInterval time.Duration, lowPercent int) publicBoard {
	cfg = cfg.normalized()
	byID := make(map[string]*model.Device, len(devices))
	for _, d := range devices {
		byID[d.ID] = d
	}
	type alertKey struct{ sev, cat string }
	alertsBy := map[string][]publicAlert{}
	seen := map[string]map[alertKey]bool{}
	sorted := append([]*model.Alert(nil), alerts...)
	sort.SliceStable(sorted, func(i, j int) bool {
		ri, iok := publicSeverityRank[sorted[i].Severity]
		rj, jok := publicSeverityRank[sorted[j].Severity]
		if !iok {
			ri = 9
		}
		if !jok {
			rj = 9
		}
		return ri < rj
	})
	for _, a := range sorted {
		if a.ClearedAt != nil || a.AckedAt != nil || byID[a.DeviceID] == nil {
			continue
		}
		if _, ok := publicSeverityRank[a.Severity]; !ok {
			continue
		}
		var cat string
		if a.Source == "supply" {
			cat = supplyAlertCategory(byID[a.DeviceID], a.Key)
		} else {
			cat = alertCategory(a.Source, a.Key)
		}
		if cat == "" {
			continue
		}
		k := alertKey{string(a.Severity), cat}
		if seen[a.DeviceID] == nil {
			seen[a.DeviceID] = map[alertKey]bool{}
		}
		if seen[a.DeviceID][k] {
			continue
		}
		seen[a.DeviceID][k] = true
		alertsBy[a.DeviceID] = append(alertsBy[a.DeviceID], publicAlert{Severity: k.sev, Category: k.cat})
	}

	board := publicBoard{Title: cfg.Title, Devices: []publicDevice{}}
	var newest time.Time
	for _, entry := range cfg.Devices {
		d := byID[entry.ID]
		if d == nil {
			continue // deleted since it was listed
		}
		if t := deviceDataTime(d); t.After(newest) {
			newest = t
		}
		pd := publicDevice{Label: entry.Label}
		if pd.Label == "" {
			// Never d.Name(): it falls back to sysName, hostname, model and address.
			pd.Label = d.DisplayName
		}
		if pd.Label == "" {
			pd.Label = fmt.Sprintf("Printer %d", len(board.Devices)+1)
		}
		if cfg.Show.Location {
			loc := d.Location
			pd.Location = &loc
		}
		if cfg.Show.Online {
			on := d.Status != nil && d.Status.Online
			pd.Online = &on
		}
		if cfg.Show.State {
			st := "unknown"
			if d.Status != nil && d.Status.State != "" {
				st = d.Status.State
			}
			pd.State = &st
		}
		if cfg.Show.Paper {
			trays := []publicTray{}
			for _, t := range d.Trays {
				if t.Kind != "input" || t.Type == "sheetFeedManual" {
					continue // as the paper alert rule: the bypass tray is usually empty
				}
				name := t.Name
				if name == "" {
					name = fmt.Sprintf("Tray %d", t.Index)
				}
				trays = append(trays, publicTray{Name: name, Status: trayStatus(t.Status)})
			}
			pd.Paper = &trays
		}
		if cfg.Show.Toner {
			toner := []publicSupply{}
			for _, sp := range d.Supplies {
				if !isToner(sp) {
					continue
				}
				name := colorantName(sp.Colorant)
				if name == "" {
					name = fmt.Sprintf("Toner %d", len(toner)+1)
				}
				var pct *int
				if sp.Percent != nil {
					v := *sp.Percent
					pct = &v
				}
				toner = append(toner, publicSupply{Name: name, Colorant: strings.ToLower(sp.Colorant), Percent: pct, Status: supplyStatus(pct, lowPercent)})
			}
			pd.Toner = &toner
		}
		if cfg.Show.Alerts {
			as := alertsBy[d.ID]
			if as == nil {
				as = []publicAlert{}
			}
			pd.Alerts = &as
		}
		board.Devices = append(board.Devices, pd)
	}
	switch {
	case len(board.Devices) == 0:
		t := now.UTC()
		board.UpdatedAt = &t
	case newest.IsZero():
		board.Stale = true
	default:
		t := newest.UTC()
		board.UpdatedAt = &t
		if pollInterval > 0 && now.Sub(newest) > publicStaleIntervals*pollInterval {
			board.Stale = true
		}
	}
	return board
}

// --- alert categories (R4) ---------------------------------------------------

// The board's alert categories come from the alert's source and key through
// the fixed tables below; the free-text description is never read. Keys
// matched, from where PrintMarshal raises them:
//
//	source "offline"  key "offline"                       alerts/rules.go Evaluate
//	source "supply"   key "supply:<index>"                alerts/rules.go Evaluate
//	                      (categorised by the device's supply at that index,
//	                      see supplyAlertCategory)
//	source "poll"     key "state:down"                    alerts/rules.go Evaluate
//	                  key "paper:<media size>|*"          alerts/rules.go Evaluate
//	                  key "error:<flag>"                  alerts/rules.go errorSeverity
//	                      (hrPrinterDetectedErrorState / IPP printer-state-reasons;
//	                      the flags are listed in errorCategories)
//	                  key "device:<group>:<code>:<n>"     alerts/rules.go Evaluate
//	                      (prtAlertTable / printer-alert; codes from
//	                      driver/snmp alertCodes, listed in deviceCodeCategories)
//	source "trap"     key "trap:link-down"                fleet/service.go trapKey,
//	                      other "trap:<description>" keys  trap/listener.go
//	source "security" (assessment findings)               never on the board
//	source "consumable" (stock, no device)                never on the board
//
// Anything else (new flags, unknown codes, other sources) is "other".
var errorCategories = map[string]string{
	"offline":  "offline",
	"noToner":  "toner",
	"jammed":   "jam",
	"doorOpen": "door",
	// IPP printer-state-reasons and the remaining hrPrinterDetectedErrorState bits
	"toner-empty-error": "toner", "marker-supply-empty-error": "toner",
	"markerSupplyMissing": "toner", "marker-supply-missing-error": "toner",
	"door-open-error": "door", "cover-open-error": "door", "cover-open": "door",
	"media-jam-error": "jam", "media-jam": "jam",
	"noPaper": "paper", "media-empty-error": "paper", "media-empty": "paper",
	"lowPaper": "paper", "media-low-warning": "paper", "media-low": "paper",
	"inputTrayMissing": "paper", "input-tray-missing-error": "paper", "inputTrayEmpty": "paper",
	"serviceRequested": "maintenance", "overduePreventMaint": "maintenance",
}

var deviceCodeCategories = map[string]string{
	"coverOpen": "door", "interlockOpen": "door", "doorOpen": "door", "subunitOpened": "door",
	"jam":              "jam",
	"markerTonerEmpty": "toner", "markerInkEmpty": "toner", "markerPrintRibbonEmpty": "toner",
	"markerTonerAlmostEmpty": "toner", "markerInkAlmostEmpty": "toner", "markerPrintRibbonAlmostEmpty": "toner",
	"inputMediaSupplyLow": "paper", "inputMediaSupplyEmpty": "paper", "inputMediaTrayMissing": "paper",
	"inputMediaChangeRequest": "paper", "inputManualInputRequest": "paper", "inputCannotFeedSizeSelected": "paper",
	"subunitLifeAlmostOver": "maintenance", "subunitLifeOver": "maintenance",
	"markerFuserUnderTemperature": "maintenance", "markerFuserOverTemperature": "maintenance",
	"markerFuserTimingFailure": "maintenance", "markerFuserThermistorFailure": "maintenance",
	"markerWasteTonerReceptacleAlmostFull": "maintenance", "markerWasteInkReceptacleAlmostFull": "maintenance",
	"markerWasteTonerReceptacleFull": "maintenance", "markerWasteInkReceptacleFull": "maintenance",
}

// supplyAlertCategory categorises a supply alert by the part it concerns. The
// key only names the supply's index, so the part comes from the device's own
// supply data, never from the alert's free text: marker supplies (isToner) are
// toner, waste containers and other named parts (drum, developer, fuser, ...)
// are maintenance. A supply the device no longer reports, or one whose type it
// will not name, is "other": broader, and still true.
func supplyAlertCategory(d *model.Device, key string) string {
	idx, err := strconv.Atoi(strings.TrimPrefix(key, "supply:"))
	if err != nil || d == nil {
		return "other"
	}
	for _, sp := range d.Supplies {
		if sp.Index != idx {
			continue
		}
		switch {
		case isToner(sp):
			return "toner"
		case sp.Class == "filled":
			return "maintenance"
		}
		switch strings.ToLower(strings.TrimSpace(sp.Type)) {
		case "", "other", "unknown":
			return "other"
		}
		return "maintenance"
	}
	return "other"
}

// alertCategory maps an alert to offline, paper, toner, door, jam,
// maintenance or other, or "" for alerts that never appear on the board. It
// sees the source and key only, which cannot tell a toner from a drum, so the
// board sends source "supply" through supplyAlertCategory, and any supply alert
// that reached this function would be "other".
func alertCategory(source, key string) string {
	switch source {
	case "security", "consumable":
		return ""
	case "offline":
		return "offline"
	case "trap":
		if key == "trap:link-down" {
			return "offline"
		}
		return "other"
	case "poll":
		switch {
		case key == "state:down":
			return "offline"
		case strings.HasPrefix(key, "paper:"):
			return "paper"
		case strings.HasPrefix(key, "error:"):
			if c, ok := errorCategories[strings.TrimPrefix(key, "error:")]; ok {
				return c
			}
		case strings.HasPrefix(key, "device:"):
			// device:<group>:<code>:<n>; a row without a code is keyed by its
			// description instead and stays "other".
			rest := strings.TrimPrefix(key, "device:")
			if i := strings.LastIndexByte(rest, ':'); i >= 0 {
				if _, code, ok := strings.Cut(rest[:i], ":"); ok {
					if c, ok := deviceCodeCategories[code]; ok {
						return c
					}
				}
			}
		}
	}
	return "other"
}

// --- snapshot cache (R6) ------------------------------------------------------

// boardSnapshot is one build: the configuration it was built from and, when
// the board is enabled, the encoded response.
type boardSnapshot struct {
	cfg  publicStatusConfig
	body []byte
}

type buildCall struct {
	done chan struct{}
	snap *boardSnapshot
	err  error
}

// boardCache holds the snapshot for publicCacheTTL; concurrent requests
// during a rebuild wait for the one build in flight.
type boardCache struct {
	now func() time.Time

	mu       sync.Mutex
	snap     *boardSnapshot
	builtAt  time.Time
	inflight *buildCall
	gen      uint64 // bumped by invalidate; a build from an older generation is not kept
}

func (c *boardCache) get(ctx context.Context, build func(context.Context) (*boardSnapshot, error)) (*boardSnapshot, error) {
	c.mu.Lock()
	if c.snap != nil && c.now().Sub(c.builtAt) < publicCacheTTL {
		snap := c.snap
		c.mu.Unlock()
		return snap, nil
	}
	if call := c.inflight; call != nil {
		c.mu.Unlock()
		select {
		case <-call.done:
			return call.snap, call.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	call := &buildCall{done: make(chan struct{})}
	c.inflight = call
	gen := c.gen
	c.mu.Unlock()

	defer func() {
		// Runs on panic too, so a failed build never strands the waiters.
		c.mu.Lock()
		if c.inflight == call {
			c.inflight = nil
		}
		if call.err == nil && call.snap != nil && gen == c.gen {
			c.snap, c.builtAt = call.snap, c.now()
		}
		c.mu.Unlock()
		if call.snap == nil && call.err == nil {
			call.err = errors.New("public board build failed")
		}
		close(call.done)
	}()
	// The build outlives the first requester's cancellation: others wait on it.
	call.snap, call.err = build(context.WithoutCancel(ctx))
	return call.snap, call.err
}

// invalidate drops the snapshot and detaches any build in flight, so the
// next request reads the configuration as it is now.
func (c *boardCache) invalidate() {
	c.mu.Lock()
	c.snap, c.inflight = nil, nil
	c.gen++
	c.mu.Unlock()
}

// --- rate limiter (R7) ---------------------------------------------------------

// publicLimiter allows `limit` requests per address per sliding minute, in
// the shape of loginLimiter. Entries older than a minute are pruned on each
// hit (and the whole map at most once a minute); the map holds at most
// `maxKeys` addresses, dropping the least recently seen.
type publicLimiter struct {
	now     func() time.Time
	limit   int
	maxKeys int

	mu        sync.Mutex
	hits      map[string][]time.Time
	lastSweep time.Time
}

func newPublicLimiter(limit, maxKeys int) *publicLimiter {
	return &publicLimiter{now: time.Now, limit: limit, maxKeys: maxKeys, hits: map[string][]time.Time{}}
}

func (l *publicLimiter) prune(ip string, now time.Time) []time.Time {
	ts := l.hits[ip]
	keep := ts[:0]
	for _, t := range ts {
		if now.Sub(t) < time.Minute {
			keep = append(keep, t)
		}
	}
	if len(keep) == 0 {
		delete(l.hits, ip)
		return nil
	}
	l.hits[ip] = keep
	return keep
}

func (l *publicLimiter) sweep(now time.Time) {
	for ip := range l.hits {
		l.prune(ip, now)
	}
	l.lastSweep = now
}

// Allow records a request from ip and reports whether it is within budget.
// Refused requests are not recorded, so a client that backs off recovers.
func (l *publicLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.Sub(l.lastSweep) >= time.Minute {
		l.sweep(now)
	}
	ts := l.prune(ip, now)
	if len(ts) >= l.limit {
		return false
	}
	if ts == nil && len(l.hits) >= l.maxKeys {
		l.sweep(now)
		for len(l.hits) >= l.maxKeys {
			oldest, at, found := "", now, false
			for k, v := range l.hits {
				if last := v[len(v)-1]; !found || last.Before(at) {
					oldest, at, found = k, last, true
				}
			}
			delete(l.hits, oldest)
		}
	}
	l.hits[ip] = append(ts, now)
	return true
}

// --- server state and handlers -----------------------------------------------

type publicState struct {
	cache   *boardCache
	limiter *publicLimiter
}

func newPublicState() *publicState {
	return &publicState{
		cache:   &boardCache{now: time.Now},
		limiter: newPublicLimiter(publicRatePerMinute, publicLimiterMaxKeys),
	}
}

// buildPublicSnapshot loads the configuration and, when the board is on, the
// listed devices and their active alerts: store reads only (R5).
func (s *Server) buildPublicSnapshot(ctx context.Context) (*boardSnapshot, error) {
	cfg, err := s.loadPublicStatus(ctx)
	if err != nil {
		return nil, err
	}
	snap := &boardSnapshot{cfg: cfg}
	if !cfg.Enabled {
		return snap, nil
	}
	var devices []*model.Device
	if len(cfg.Devices) > 0 {
		ids := make([]string, 0, len(cfg.Devices))
		for _, d := range cfg.Devices {
			ids = append(ids, d.ID)
		}
		devices, err = s.Store.ListDevices(ctx, store.DeviceFilter{IDs: ids, IncludeDecommissioned: true})
		if err != nil {
			return nil, err
		}
	}
	var alerts []*model.Alert
	if cfg.Show.Alerts && len(devices) > 0 {
		if alerts, err = s.Store.ListAlerts(ctx, store.AlertFilter{ActiveOnly: true, Limit: publicAlertScan}); err != nil {
			return nil, err
		}
	}
	var poll time.Duration
	low := 15
	if s.Cfg != nil {
		poll = time.Duration(s.Cfg.Poll.Interval)
		if s.Cfg.Alerts.SupplyWarningPercent > 0 {
			low = s.Cfg.Alerts.SupplyWarningPercent
		}
	}
	board := buildPublicBoard(cfg, devices, alerts, time.Now(), poll, low)
	if snap.body, err = json.Marshal(board); err != nil {
		return nil, err
	}
	snap.body = append(snap.body, '\n')
	return snap, nil
}

// getPublicStatus serves GET /api/public/status and /api/public/status/{slug}.
func (s *Server) getPublicStatus(w http.ResponseWriter, r *http.Request) {
	if !s.pub.limiter.Allow(clientIP(r)) {
		w.Header().Set("Retry-After", "60")
		writeErr(w, http.StatusTooManyRequests, "too many requests; try again in a minute")
		return
	}
	snap, err := s.pub.cache.get(r.Context(), s.buildPublicSnapshot)
	if err != nil {
		s.Log.Error("public status board", "err", err)
		writeErr(w, http.StatusInternalServerError, "status unavailable")
		return
	}
	cfg := snap.cfg
	slug := r.PathValue("slug")
	switch {
	case !cfg.Enabled:
	case slug == "" && !cfg.Unlisted:
	case slug != "" && cfg.Unlisted && cfg.Slug != "" && subtle.ConstantTimeCompare([]byte(slug), []byte(cfg.Slug)) == 1:
	default:
		cfg.Enabled = false
	}
	if !cfg.Enabled {
		writeErr(w, http.StatusNotFound, "no such endpoint") // same as the mux fallback (R1)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(snap.body)
}

// boardFrameOrigins returns the origins allowed to frame the board's page,
// or nil (secureHeaders then sends DENY). It shares the board's snapshot.
func (s *Server) boardFrameOrigins(r *http.Request) []string {
	if s.pub == nil {
		return nil
	}
	snap, err := s.pub.cache.get(r.Context(), s.buildPublicSnapshot)
	if err != nil || !snap.cfg.Enabled {
		return nil
	}
	return snap.cfg.FrameOrigins
}

func (s *Server) getPublicStatusSettings(w http.ResponseWriter, r *http.Request) {
	cfg, err := s.loadPublicStatus(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func newPublicSlug() (string, error) {
	b := make([]byte, publicSlugBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// validOrigin accepts scheme://host[:port] for http and https and returns
// it in canonical form; anything else could widen or break the CSP header.
func validOrigin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery || (u.Path != "" && u.Path != "/") {
		return "", fmt.Errorf("frame origin %q must be scheme://host[:port]", raw)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("frame origin %q must use http or https", raw)
	}
	host := strings.ToLower(u.Host)
	if host == "" {
		return "", fmt.Errorf("frame origin %q has no host", raw)
	}
	for _, c := range host {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '.' || c == '-' || c == ':' || c == '[' || c == ']') {
			return "", fmt.Errorf("frame origin %q has an invalid host", raw)
		}
	}
	return scheme + "://" + host, nil
}

// validate checks and normalises a submitted configuration (R8, R13).
func (c *publicStatusConfig) validate() error {
	c.Title = strings.TrimSpace(c.Title)
	if c.Title == "" {
		c.Title = publicDefaultTitle
	}
	if utf8.RuneCountInString(c.Title) > publicMaxTitle {
		return fmt.Errorf("title is over %d characters", publicMaxTitle)
	}
	if len(c.Devices) > publicMaxDevices {
		return fmt.Errorf("a board lists at most %d devices", publicMaxDevices)
	}
	seen := map[string]bool{}
	for i := range c.Devices {
		d := &c.Devices[i]
		d.ID = strings.TrimSpace(d.ID)
		d.Label = strings.TrimSpace(d.Label)
		if d.ID == "" {
			return errors.New("every device needs an id")
		}
		if seen[d.ID] {
			return fmt.Errorf("device %s is listed twice", d.ID)
		}
		seen[d.ID] = true
		if utf8.RuneCountInString(d.Label) > publicMaxLabel {
			return fmt.Errorf("public label of device %s is over %d characters", d.ID, publicMaxLabel)
		}
	}
	origins := []string{}
	have := map[string]bool{}
	for _, o := range c.FrameOrigins {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		norm, err := validOrigin(o)
		if err != nil {
			return err
		}
		if !have[norm] {
			have[norm] = true
			origins = append(origins, norm)
		}
	}
	if len(origins) > publicMaxOrigins {
		return fmt.Errorf("at most %d frame origins", publicMaxOrigins)
	}
	c.FrameOrigins = origins
	if c.Devices == nil {
		c.Devices = []publicStatusDevice{}
	}
	return nil
}

// publicStatusAudit is one side of the audit row for a save: flat keys, so the
// trail lists each changed setting on its own. It is built field by field and
// has no slug field on purpose: operators read the audit log, and on an
// unlisted board the slug is the only thing keeping the address private. Only
// whether this save issued a new one is recorded (always false on the before
// side, so the row shows a change only when one was issued).
func publicStatusAudit(c publicStatusConfig, slugRegenerated bool) map[string]any {
	ids := make([]string, 0, len(c.Devices))
	labels := map[string]string{}
	for _, d := range c.Devices {
		ids = append(ids, d.ID)
		if d.Label != "" {
			labels[d.ID] = d.Label
		}
	}
	origins := c.FrameOrigins
	if origins == nil {
		origins = []string{}
	}
	return map[string]any{
		"enabled":         c.Enabled,
		"unlisted":        c.Unlisted,
		"title":           c.Title,
		"frameOrigins":    origins,
		"deviceIds":       ids, // in board order
		"deviceLabels":    labels,
		"show.location":   c.Show.Location,
		"show.online":     c.Show.Online,
		"show.state":      c.Show.State,
		"show.paper":      c.Show.Paper,
		"show.toner":      c.Show.Toner,
		"show.alerts":     c.Show.Alerts,
		"slugRegenerated": slugRegenerated,
	}
}

func (s *Server) putPublicStatusSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	r.Body = http.MaxBytesReader(w, r.Body, publicMaxBody)
	var body publicStatusPut
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	next := body.publicStatusConfig
	if err := next.validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ids := make([]string, 0, len(next.Devices))
	for _, d := range next.Devices {
		ids = append(ids, d.ID)
	}
	// Group scope first, so a scoped caller learns nothing about devices
	// outside its groups; an empty board names no device and is allowed.
	if len(ids) > 0 && !s.allowedAll(w, r, ids) {
		return
	}
	if len(ids) > 0 {
		found, err := s.Store.ListDevices(ctx, store.DeviceFilter{IDs: ids, IncludeDecommissioned: true})
		if err != nil {
			s.fail(w, err)
			return
		}
		have := make(map[string]bool, len(found))
		for _, d := range found {
			have[d.ID] = true
		}
		for _, id := range ids {
			if !have[id] {
				writeErr(w, http.StatusBadRequest, "no such device: "+id)
				return
			}
		}
	}
	prev, err := s.loadPublicStatus(ctx)
	if err != nil {
		s.fail(w, err)
		return
	}
	next.Slug = prev.Slug // server-owned
	regenerated := false
	if body.RegenerateSlug || (next.Unlisted && next.Slug == "") {
		if next.Slug, err = newPublicSlug(); err != nil {
			s.fail(w, err)
			return
		}
		regenerated = true
	}
	if err := s.savePublicStatus(ctx, next); err != nil {
		s.fail(w, err)
		return
	}
	s.pub.cache.invalidate() // board and frame origins change on the next request

	show := strings.Join(next.Show.names(), ",")
	if show == "" {
		show = "none"
	}
	// The slug is never logged: the audit log is readable by operators.
	detail := fmt.Sprintf("public status board: enabled=%t unlisted=%t devices=%d show=%s frameOrigins=%d slugRegenerated=%t",
		next.Enabled, next.Unlisted, len(next.Devices), show, len(next.FrameOrigins), regenerated)
	who := actor(r)
	s.securityChange(ctx, r, "public.status.changed", who, detail, publicStatusAudit(prev, false), publicStatusAudit(next, regenerated), "ok")
	if next.Enabled && !prev.Enabled {
		s.securityChange(ctx, r, "public.status.enabled", who, detail, map[string]any{"enabled": false}, map[string]any{"enabled": true}, "ok")
	}
	writeJSON(w, http.StatusOK, next)
}
