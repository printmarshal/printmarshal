package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"printmarshal/internal/auth"
	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// Group scope (audit A11): a principal whose Scope lists group ids may only
// read or act on devices in those groups. Session users have no scope unless
// one is set on them; API tokens carry theirs from creation. The checks live
// in require (every route passes through it), so feature files need no edits.

const scopeDenied = "outside your group scope"

// authorize applies the principal-level rules that a role rank cannot
// express. It writes the 403 itself and returns false on a violation.
//   - API tokens cannot use /api/auth/* (sessions, passwords, SSO), cannot
//     mint or revoke other tokens and cannot create or delete users: a
//     service account must not be able to turn itself into a human login
//     that outlives its own expiry or revocation (review H-003).
//   - A scoped principal cannot reach /api/devices/<id>... for a device
//     outside its groups, nor name one in a JSON body (deviceId / deviceIds /
//     devices / groupId / target{...}) of a bulk endpoint, nor act on a
//     resource (alert, write record, campaign, replacement) that belongs to
//     such a device. It also cannot change groups, users or tokens, because
//     groups are what define its boundary and the other two would let it
//     mint an unscoped principal (review H-002).
func (s *Server) authorize(w http.ResponseWriter, r *http.Request) bool {
	u := auth.UserFrom(r.Context())
	if u == nil {
		return true
	}
	path := r.URL.Path
	mutating := r.Method != http.MethodGet && r.Method != http.MethodHead
	if auth.IsToken(u) {
		// Sign-in provider settings are authentication too: a token that can
		// point LDAP at a directory it controls can sign in as a human admin
		// and outlive its own expiry (audit NV-05).
		// Dashboard layouts belong to people, not service accounts.
		// Publishing the public status board is a person's decision too.
		if strings.HasPrefix(path, "/api/auth/") || strings.HasPrefix(path, "/api/settings/directory") || strings.HasPrefix(path, "/api/settings/public-status") || strings.HasPrefix(path, "/api/dashboard/layout") || (mutating && (strings.HasPrefix(path, "/api/tokens") || strings.HasPrefix(path, "/api/users"))) {
			s.security(r.Context(), r, "auth.denied", u.Username, r.Method+" "+path+" is not available to api tokens", "denied")
			writeErr(w, http.StatusForbidden, "api tokens cannot manage authentication, sign-in providers or users")
			return false
		}
	}
	if len(u.Scope) == 0 {
		return true
	}
	if mutating && (strings.HasPrefix(path, "/api/groups") || strings.HasPrefix(path, "/api/users") || strings.HasPrefix(path, "/api/tokens")) {
		s.security(r.Context(), r, "auth.denied", u.Username, r.Method+" "+path+" is not available to a group-scoped principal", "denied")
		writeErr(w, http.StatusForbidden, "a group-scoped principal cannot manage groups, users or tokens")
		return false
	}
	if id := devicePathID(path); id != "" && !s.allowedDevice(r.Context(), id) {
		return s.denyScope(w, r, u, path)
	}
	if mutating && strings.HasPrefix(path, "/api/credentials") {
		// A credential is bound to a device, a group or every device; a
		// scoped principal may only touch ones bound inside its groups
		// (audit NV-07).
		if ok, what := s.credentialScopeAllowed(r); !ok {
			return s.denyScope(w, r, u, path+" ("+what+")")
		}
	}
	ids, groups := bodyTargets(r)
	rids, rgroups, err := s.resourceTargets(r)
	if err != nil {
		// The resource is missing or unreadable: let the handler answer
		// (404/500) rather than leak whether it exists through a 403.
		return true
	}
	ids, groups = append(ids, rids...), append(groups, rgroups...)
	for _, gid := range groups {
		gids, err := s.groupDeviceIDs(r.Context(), gid)
		if err != nil {
			return true
		}
		ids = append(ids, gids...)
	}
	for _, id := range ids {
		if !s.allowedDevice(r.Context(), id) {
			return s.denyScope(w, r, u, path+" (device "+id+")")
		}
	}
	return true
}

// resourceTargets resolves the devices behind a resource named in the path
// (an alert, a configuration write record, a firmware campaign, a stock
// replacement event) so a scoped principal cannot reach a device through
// another object's id. It returns nothing for paths that name no such
// resource.
func (s *Server) resourceTargets(r *http.Request) (deviceIDs []string, groupIDs []string, err error) {
	ctx := r.Context()
	path := r.URL.Path
	seg := func(prefix string) (string, bool) {
		rest, ok := strings.CutPrefix(path, prefix)
		if !ok {
			return "", false
		}
		id, _, _ := strings.Cut(rest, "/")
		return id, id != ""
	}
	if id, ok := seg("/api/alerts/"); ok {
		a, err := s.Store.GetAlert(ctx, id)
		if err != nil {
			return nil, nil, err
		}
		return []string{a.DeviceID}, nil, nil
	}
	if id, ok := seg("/api/config/writes/"); ok {
		rec, err := s.Store.GetWriteRecord(ctx, id)
		if err != nil {
			return nil, nil, err
		}
		return []string{rec.Result.DeviceID}, nil, nil
	}
	if id, ok := seg("/api/firmware/campaigns/"); ok {
		c, err := s.Store.GetCampaign(ctx, id)
		if err != nil {
			return nil, nil, err
		}
		if c.GroupID != "" {
			return c.DeviceIDs, []string{c.GroupID}, nil
		}
		return c.DeviceIDs, nil, nil
	}
	if id, ok := seg("/api/consumables/replacements/"); ok {
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			return nil, nil, err
		}
		ev, err := s.Store.GetStockEvent(ctx, n)
		if err != nil {
			return nil, nil, err
		}
		if ev.DeviceID != "" {
			return []string{ev.DeviceID}, nil, nil
		}
	}
	return nil, nil, nil
}

// groupDeviceIDs resolves a group (static members or rule matches) to the
// ids of the devices it currently contains.
func (s *Server) groupDeviceIDs(ctx context.Context, gid string) ([]string, error) {
	g, err := s.Store.GetGroup(ctx, gid)
	if err != nil {
		return nil, err
	}
	if g.Kind != "rule" {
		return g.MemberIDs, nil
	}
	all, err := s.Store.ListDevices(ctx, store.DeviceFilter{IncludeDecommissioned: true})
	if err != nil {
		return nil, err
	}
	var out []string
	for _, d := range all {
		if store.MatchRule(g.Rule, d) {
			out = append(out, d.ID)
		}
	}
	return out, nil
}

func (s *Server) denyScope(w http.ResponseWriter, r *http.Request, u *model.User, what string) bool {
	s.security(r.Context(), r, "auth.denied", u.Username, r.Method+" "+what+" is "+scopeDenied, "denied")
	writeErr(w, http.StatusForbidden, scopeDenied)
	return false
}

// devicePathID returns the <id> of /api/devices/<id>[/...], else "".
func devicePathID(path string) string {
	rest, ok := strings.CutPrefix(path, "/api/devices/")
	if !ok {
		return ""
	}
	id, _, hasSlash := strings.Cut(rest, "/")
	if !hasSlash && id == "rename" {
		return "" // POST /api/devices/rename names its devices in the body
	}
	return id
}

// peekBody returns the body of a POST/PUT/PATCH request when it looks like a
// JSON value, and puts it back for the handler. The handlers decode bodies
// without looking at Content-Type, so the scope check must not either
// (audit NV-04); uploads and other non-JSON bodies are left alone. The body
// is put back whole: what was peeked, then the rest unread, so an upload
// larger than the peek (the CSV import) reaches its handler intact.
func peekBody(r *http.Request) []byte {
	if r.Body == nil || r.Body == http.NoBody || (r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch) {
		return nil
	}
	orig := r.Body
	b, err := io.ReadAll(io.LimitReader(orig, 1<<20+1))
	r.Body = rejoinedBody{io.MultiReader(bytes.NewReader(b), orig), orig}
	if err != nil {
		return nil
	}
	t := bytes.TrimLeft(b, " \t\r\n")
	if len(t) == 0 || (t[0] != '{' && t[0] != '[') {
		return nil
	}
	return b
}

// rejoinedBody reads the peeked bytes and then the rest of the original body.
type rejoinedBody struct {
	io.Reader
	io.Closer
}

// bodyTargets peeks at a JSON request body for device and group references
// (top level and under "target").
func bodyTargets(r *http.Request) (deviceIDs []string, groupIDs []string) {
	b := peekBody(r)
	if len(b) == 0 {
		return nil, nil
	}
	type refs struct {
		DeviceID  string          `json:"deviceId"`
		DeviceIDs []string        `json:"deviceIds"`
		Devices   json.RawMessage `json:"devices"`
		GroupID   string          `json:"groupId"`
		GroupIDs  []string        `json:"groupIds"`
	}
	var probe struct {
		refs
		Target refs `json:"target"`
	}
	if json.Unmarshal(b, &probe) != nil {
		return nil, nil
	}
	for _, p := range []refs{probe.refs, probe.Target} {
		deviceIDs = append(deviceIDs, p.DeviceIDs...)
		if p.DeviceID != "" {
			deviceIDs = append(deviceIDs, p.DeviceID)
		}
		var list []string
		if len(p.Devices) > 0 && json.Unmarshal(p.Devices, &list) == nil {
			deviceIDs = append(deviceIDs, list...)
		}
		if p.GroupID != "" {
			groupIDs = append(groupIDs, p.GroupID)
		}
		groupIDs = append(groupIDs, p.GroupIDs...)
	}
	return deviceIDs, groupIDs
}

// scopeDeviceIDs resolves the principal's groups (static members or rule
// matches) to the set of device ids it may touch. nil means "unscoped".
func (s *Server) scopeDeviceIDs(ctx context.Context) map[string]bool {
	u := auth.UserFrom(ctx)
	if u == nil || len(u.Scope) == 0 {
		return nil
	}
	set := map[string]bool{}
	var all []*model.Device
	for _, gid := range u.Scope {
		g, err := s.Store.GetGroup(ctx, gid)
		if err != nil {
			continue // a deleted group grants nothing
		}
		if g.Kind != "rule" {
			for _, id := range g.MemberIDs {
				set[id] = true
			}
			continue
		}
		if all == nil {
			all, _ = s.Store.ListDevices(ctx, store.DeviceFilter{IncludeDecommissioned: true})
		}
		for _, d := range all {
			if store.MatchRule(g.Rule, d) {
				set[d.ID] = true
			}
		}
	}
	return set
}

// allowedDevice reports whether the principal on ctx may act on deviceID.
func (s *Server) allowedDevice(ctx context.Context, deviceID string) bool {
	set := s.scopeDeviceIDs(ctx)
	return set == nil || set[deviceID]
}

// allowedAll denies a scoped principal an operation that names devices
// outside its groups, or none at all (a whole-fleet action). It writes the
// 403 itself and returns false on a violation; unscoped principals pass.
func (s *Server) allowedAll(w http.ResponseWriter, r *http.Request, ids []string) bool {
	u := auth.UserFrom(r.Context())
	if u == nil || len(u.Scope) == 0 {
		return true
	}
	if len(ids) == 0 {
		return s.denyScope(w, r, u, r.URL.Path+" (whole fleet)")
	}
	set := s.scopeDeviceIDs(r.Context())
	for _, id := range ids {
		if !set[id] {
			return s.denyScope(w, r, u, r.URL.Path+" (device "+id+")")
		}
	}
	return true
}

// filterAlerts drops alerts of devices outside the principal's scope.
func (s *Server) filterAlerts(ctx context.Context, as []*model.Alert) []*model.Alert {
	set := s.scopeDeviceIDs(ctx)
	if set == nil {
		return as
	}
	out := []*model.Alert{}
	for _, a := range as {
		if set[a.DeviceID] {
			out = append(out, a)
		}
	}
	return out
}

// filterDevices drops devices outside the principal's scope (no-op when unscoped).
func (s *Server) filterDevices(ctx context.Context, devs []*model.Device) []*model.Device {
	set := s.scopeDeviceIDs(ctx)
	if set == nil {
		return devs
	}
	out := []*model.Device{}
	for _, d := range devs {
		if set[d.ID] {
			out = append(out, d)
		}
	}
	return out
}

// credentialScopeString reports whether a scoped principal may act on a
// credential with the given scope string ("" = every device, "device:<id>",
// "group:<id>"). A global credential applies to devices outside the
// principal's groups, so it is never allowed.
func (s *Server) credentialScopeString(ctx context.Context, scope string) bool {
	u := auth.UserFrom(ctx)
	if u == nil || len(u.Scope) == 0 {
		return true
	}
	switch {
	case strings.HasPrefix(scope, "device:"):
		return s.allowedDevice(ctx, strings.TrimPrefix(scope, "device:"))
	case strings.HasPrefix(scope, "group:"):
		gid := strings.TrimPrefix(scope, "group:")
		for _, g := range u.Scope {
			if g == gid {
				return true
			}
		}
		return false
	}
	return false
}

// credentialScopeAllowed checks a mutating /api/credentials request: the
// scope named in a POST body, or the stored scope of the credential named in
// the path. A credential that does not exist is left to the handler's 404.
func (s *Server) credentialScopeAllowed(r *http.Request) (bool, string) {
	u := auth.UserFrom(r.Context())
	if u == nil || len(u.Scope) == 0 {
		return true, ""
	}
	if r.URL.Path == "/api/credentials" {
		var body struct {
			Scope string `json:"scope"`
		}
		if b := peekBody(r); len(b) > 0 {
			_ = json.Unmarshal(b, &body)
		}
		if !s.credentialScopeString(r.Context(), body.Scope) {
			return false, "credential scope " + strconv.Quote(body.Scope)
		}
		return true, ""
	}
	id, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/api/credentials/"), "/")
	cs, err := s.Store.ListCredentials(r.Context())
	if err != nil {
		return true, ""
	}
	for _, c := range cs {
		if c.ID == id && !s.credentialScopeString(r.Context(), c.Scope) {
			return false, "credential " + id
		}
	}
	return true, ""
}

// filterCredentials drops credentials a scoped principal may not act on.
func (s *Server) filterCredentials(ctx context.Context, cs []*model.Credential) []*model.Credential {
	u := auth.UserFrom(ctx)
	if u == nil || len(u.Scope) == 0 {
		return cs
	}
	out := []*model.Credential{}
	for _, c := range cs {
		if s.credentialScopeString(ctx, c.Scope) {
			out = append(out, c)
		}
	}
	return out
}
