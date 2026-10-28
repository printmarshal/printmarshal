package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
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
//   - API tokens cannot use /api/auth/* (sessions, passwords, SSO) and cannot
//     mint or revoke other tokens.
//   - A scoped principal cannot reach /api/devices/<id>... for a device
//     outside its groups, nor name one in a JSON body (deviceId / deviceIds /
//     devices) of a bulk endpoint.
func (s *Server) authorize(w http.ResponseWriter, r *http.Request) bool {
	u := auth.UserFrom(r.Context())
	if u == nil {
		return true
	}
	path := r.URL.Path
	if auth.IsToken(u) {
		if strings.HasPrefix(path, "/api/auth/") || (strings.HasPrefix(path, "/api/tokens") && r.Method != http.MethodGet) {
			s.security(r.Context(), r, "auth.denied", u.Username, r.Method+" "+path+" is not available to api tokens", "denied")
			writeErr(w, http.StatusForbidden, "api tokens cannot manage authentication")
			return false
		}
	}
	if len(u.Scope) == 0 {
		return true
	}
	if id := devicePathID(path); id != "" && !s.allowedDevice(r.Context(), id) {
		return s.denyScope(w, r, u, path)
	}
	if ids := bodyDeviceIDs(r); len(ids) > 0 {
		for _, id := range ids {
			if !s.allowedDevice(r.Context(), id) {
				return s.denyScope(w, r, u, path+" (device "+id+")")
			}
		}
	}
	return true
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
	id, _, _ := strings.Cut(rest, "/")
	return id
}

// bodyDeviceIDs peeks at a JSON request body for device references and puts
// the body back for the handler. Non-JSON bodies (uploads) are left alone.
func bodyDeviceIDs(r *http.Request) []string {
	if r.Body == nil || r.Body == http.NoBody || (r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch) {
		return nil
	}
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		return nil
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, 1<<20+1))
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(b))
	if err != nil || len(b) == 0 {
		return nil
	}
	var probe struct {
		DeviceID  string          `json:"deviceId"`
		DeviceIDs []string        `json:"deviceIds"`
		Devices   json.RawMessage `json:"devices"`
	}
	if json.Unmarshal(b, &probe) != nil {
		return nil
	}
	ids := append([]string(nil), probe.DeviceIDs...)
	if probe.DeviceID != "" {
		ids = append(ids, probe.DeviceID)
	}
	var list []string
	if len(probe.Devices) > 0 && json.Unmarshal(probe.Devices, &list) == nil {
		ids = append(ids, list...)
	}
	return ids
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
