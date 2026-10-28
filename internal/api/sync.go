package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"printmarshal/internal/auth"
	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// Incremental sync (audit A15): GET /api/sync/{collection}?since=<cursor>&limit=
// returns rows ordered by (updatedAt, id) strictly after the cursor, the
// cursor of the last row and whether more remain. The cursor is opaque:
// base64url of "<stored timestamp>|<id>". A client keeps the last cursor it
// received and passes it back; it never misses or repeats a row.

const (
	syncDefaultLimit = 200
	syncMaxLimit     = 1000
)

// encodeCursor renders a cursor for clients.
func encodeCursor(c store.SyncCursor) string {
	if c.Zero() {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString([]byte(c.At + "|" + c.ID))
}

// decodeCursor parses a client cursor; "" is the beginning.
func decodeCursor(s string) (store.SyncCursor, error) {
	if s == "" {
		return store.SyncCursor{}, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
	if err != nil {
		return store.SyncCursor{}, fmt.Errorf("bad cursor")
	}
	at, id, ok := strings.Cut(string(b), "|")
	if !ok || at == "" || id == "" {
		return store.SyncCursor{}, fmt.Errorf("bad cursor")
	}
	return store.SyncCursor{At: at, ID: id}, nil
}

// syncResponse is the page shape.
type syncResponse struct {
	Collection string `json:"collection"`
	Items      []any  `json:"items"`
	Cursor     string `json:"cursor"`
	HasMore    bool   `json:"hasMore"`
}

func (s *Server) sync(w http.ResponseWriter, r *http.Request) {
	ls := s.logStore()
	if ls == nil {
		writeErr(w, http.StatusNotImplemented, "sync is not available on this datastore")
		return
	}
	coll := r.PathValue("collection")
	var fetch func() ([]store.SyncRow, bool, error)
	after, err := decodeCursor(r.URL.Query().Get("since"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	limit := intQuery(r, "limit", syncDefaultLimit)
	if limit <= 0 {
		limit = syncDefaultLimit
	}
	if limit > syncMaxLimit {
		limit = syncMaxLimit
	}
	ctx := r.Context()
	minRole := model.RoleViewer
	switch coll {
	case "devices":
		fetch = func() ([]store.SyncRow, bool, error) { return ls.SyncDevices(ctx, after, limit) }
	case "alerts":
		fetch = func() ([]store.SyncRow, bool, error) { return ls.SyncAlerts(ctx, after, limit) }
	case "jobs":
		fetch = func() ([]store.SyncRow, bool, error) { return ls.SyncJobs(ctx, after, limit) }
	case "audit":
		minRole = model.RoleOperator // same gate as GET /api/audit
		fetch = func() ([]store.SyncRow, bool, error) { return ls.SyncAudit(ctx, after, limit) }
	case "config-writes":
		fetch = func() ([]store.SyncRow, bool, error) { return ls.SyncConfigWrites(ctx, after, limit) }
	default:
		writeErr(w, http.StatusNotFound, "unknown collection; one of devices, alerts, audit, jobs, config-writes")
		return
	}
	if u := auth.UserFrom(ctx); u != nil && !u.Role.Allows(minRole) {
		s.security(ctx, r, "auth.denied", u.Username, r.Method+" "+r.URL.Path+" requires "+string(minRole), "denied")
		writeErr(w, http.StatusForbidden, fmt.Sprintf("requires %s role", minRole))
		return
	}
	rows, more, err := fetch()
	if err != nil {
		s.fail(w, err)
		return
	}
	resp := syncResponse{Collection: coll, Items: make([]any, 0, len(rows)), HasMore: more, Cursor: encodeCursor(after)}
	for _, row := range rows {
		resp.Items = append(resp.Items, row.Item)
		resp.Cursor = encodeCursor(row.Cursor)
	}
	body, err := json.Marshal(resp)
	if err != nil {
		s.fail(w, err)
		return
	}
	// Weak ETag over the page so a client re-polling the same cursor can
	// get a 304 when nothing changed.
	sum := sha256.Sum256(body)
	etag := `W/"` + hex.EncodeToString(sum[:16]) + `"`
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "no-cache")
	if match := r.Header.Get("If-None-Match"); match != "" && strings.Contains(match, etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(append(bytes.TrimRight(body, "\n"), '\n'))
}
