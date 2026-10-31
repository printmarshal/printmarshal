package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"printmarshal/internal/auth"
	"printmarshal/internal/discovery"
	"printmarshal/internal/events"
	"printmarshal/internal/importcsv"
	"printmarshal/internal/jobs"
	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// Import printers from a CSV file (intent/csv-import). POST
// /api/discovery/import takes a multipart form: file (the CSV), mode
// (preview | apply) and replace (comma-separated columns whose values
// replace ones an operator set). The preview reads the store only and
// contacts no device; the apply is an administrator's and runs as a job of
// type "import" that recomputes the plan and applies it row by row. Nothing
// from the file is logged; the audit records counts and per-device changes.

const (
	importJobType = "import"
	// importFormSlack is what the multipart framing and the small fields may
	// add to the file's own limit.
	importFormSlack = 1 << 20
	// importFieldMax bounds the mode and replace fields.
	importFieldMax = 4096
	// importIdentifyTimeout bounds identifying one created row, as discovery does.
	importIdentifyTimeout = 60 * time.Second
)

// importPayload is a queued import's parsed file and the caller it runs as.
type importPayload struct {
	table   *importcsv.Table
	replace map[string]bool
	user    *model.User
	ip      string
}

func (s *Server) importRoutes(mux *http.ServeMux) {
	mux.Handle("POST /api/discovery/import", s.require(model.RoleOperator, s.importCSV))
}

func (s *Server) importCSV(w http.ResponseWriter, r *http.Request) {
	mode, replaceList, file, err := readImportForm(w, r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if mode != "preview" && mode != "apply" {
		writeErr(w, http.StatusBadRequest, `mode must be "preview" or "apply"`)
		return
	}
	replace, err := importcsv.ParseReplace(replaceList)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	u := auth.UserFrom(r.Context())
	if mode == "apply" && (u == nil || !u.Role.Allows(model.RoleAdmin)) {
		who := ""
		if u != nil {
			who = u.Username
		}
		s.security(r.Context(), r, "auth.denied", who, "POST "+r.URL.Path+" mode=apply requires "+string(model.RoleAdmin), "denied")
		writeErr(w, http.StatusForbidden, fmt.Sprintf("requires %s role", model.RoleAdmin))
		return
	}
	table, err := importcsv.Parse(bytes.NewReader(file), importcsv.MaxRows, importcsv.MaxBytes)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	scoped := u != nil && len(u.Scope) > 0

	if mode == "preview" {
		lk, err := s.importLookup(r.Context(), table)
		if err != nil {
			s.fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, importcsv.Plan(table, lk, importcsv.Options{Replace: replace, Scoped: scoped}))
		return
	}

	if s.Fleet == nil || s.Fleet.Jobs == nil {
		writeErr(w, http.StatusServiceUnavailable, "the job runner is not available")
		return
	}
	key, err := importKey()
	if err != nil {
		s.fail(w, err)
		return
	}
	s.imports.Store(key, &importPayload{table: table, replace: replace, user: u, ip: clientIP(r)})
	cols := replaceColumns(replace)
	j, err := s.Fleet.Jobs.Submit(r.Context(), &model.Job{Type: importJobType, CreatedBy: actor(r),
		Target: map[string]any{"rows": len(table.Records)},
		Params: map[string]any{"payload": key, "rows": len(table.Records), "replace": cols}})
	if err != nil {
		s.imports.Delete(key)
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, j)
}

// readImportForm reads the multipart form: the mode and replace fields and
// the file, which may be at most a byte over the import limit (Parse then
// refuses it with the limit in the message).
func readImportForm(w http.ResponseWriter, r *http.Request) (mode, replace string, file []byte, err error) {
	tooLarge := fmt.Errorf("file is over %d MB", importcsv.MaxBytes>>20)
	r.Body = http.MaxBytesReader(w, r.Body, importcsv.MaxBytes+importFormSlack)
	mr, err := r.MultipartReader()
	if err != nil {
		return "", "", nil, errors.New("send the file as multipart/form-data in a field named file")
	}
	haveFile := false
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			var mbe *http.MaxBytesError
			if errors.As(err, &mbe) {
				return "", "", nil, tooLarge
			}
			return "", "", nil, errors.New("the upload is unreadable")
		}
		var limit int64 = importFieldMax
		if part.FormName() == "file" {
			limit = importcsv.MaxBytes + 1
		}
		b, err := io.ReadAll(io.LimitReader(part, limit))
		if err != nil {
			var mbe *http.MaxBytesError
			if errors.As(err, &mbe) {
				return "", "", nil, tooLarge
			}
			return "", "", nil, errors.New("the upload is unreadable")
		}
		switch part.FormName() {
		case "file":
			file, haveFile = b, true
		case "mode":
			mode = strings.TrimSpace(string(b))
		case "replace":
			replace = string(b)
		}
	}
	if !haveFile {
		return "", "", nil, errors.New("no file: send the CSV in a field named file")
	}
	return mode, replace, file, nil
}

func importKey() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func replaceColumns(replace map[string]bool) []string {
	cols := make([]string, 0, len(replace))
	for c := range replace {
		cols = append(cols, c)
	}
	sort.Strings(cols)
	return cols
}

// storeLookup answers the planner from one read of the inventory, the
// caller's scope and hostnames resolved ahead of the plan.
type storeLookup struct {
	byAddr   map[string]*model.Device
	bySerial map[string]*model.Device
	groups   map[string]*model.Group
	hosts    map[string]string
	devScope map[string]bool // nil: unscoped
	grpScope map[string]bool // nil: unscoped
}

func (l *storeLookup) DeviceByAddress(a string) *model.Device { return l.byAddr[a] }
func (l *storeLookup) DeviceBySerial(s string) *model.Device {
	return l.bySerial[strings.ToUpper(s)]
}
func (l *storeLookup) GroupByName(n string) *model.Group { return l.groups[n] }
func (l *storeLookup) ResolveHostname(h string) (string, bool) {
	a, ok := l.hosts[h]
	return a, ok
}
func (l *storeLookup) DeviceInScope(id string) bool { return l.devScope == nil || l.devScope[id] }
func (l *storeLookup) GroupInScope(id string) bool  { return l.grpScope == nil || l.grpScope[id] }

// importLookup reads what the plan needs for the principal on ctx. Hostnames
// are resolved with forward lookups (eight at a time, each bounded by the
// reverse-DNS timeout) when the fleet is available; without it they stay
// unresolved. No device is contacted.
func (s *Server) importLookup(ctx context.Context, t *importcsv.Table) (*storeLookup, error) {
	devs, err := s.Store.ListDevices(ctx, store.DeviceFilter{IncludeDecommissioned: true})
	if err != nil {
		return nil, err
	}
	groups, err := s.Store.ListGroups(ctx)
	if err != nil {
		return nil, err
	}
	l := &storeLookup{byAddr: map[string]*model.Device{}, bySerial: map[string]*model.Device{}, groups: map[string]*model.Group{}, hosts: map[string]string{}}
	newer := func(have, d *model.Device) bool {
		return have == nil || d.LastSeen.After(have.LastSeen) || (d.LastSeen.Equal(have.LastSeen) && d.ID < have.ID)
	}
	for _, d := range devs {
		if d.Address != "" && newer(l.byAddr[d.Address], d) {
			l.byAddr[d.Address] = d
		}
		if sk := strings.ToUpper(strings.TrimSpace(d.Serial)); sk != "" && newer(l.bySerial[sk], d) {
			l.bySerial[sk] = d
		}
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].ID < groups[j].ID })
	for _, g := range groups {
		if _, dup := l.groups[g.Name]; !dup {
			l.groups[g.Name] = g
		}
	}
	if u := auth.UserFrom(ctx); u != nil && len(u.Scope) > 0 {
		l.devScope = s.scopeDeviceIDs(ctx)
		l.grpScope = map[string]bool{}
		for _, gid := range u.Scope {
			l.grpScope[gid] = true
		}
	}
	if s.Fleet != nil {
		var mu sync.Mutex
		var wg sync.WaitGroup
		sem := make(chan struct{}, dnsLookupConcurrency)
		for _, h := range importcsv.Hostnames(t) {
			wg.Add(1)
			sem <- struct{}{}
			go func(h string) {
				defer wg.Done()
				defer func() { <-sem }()
				if a, ok := s.Fleet.LookupHostAddress(ctx, h); ok {
					mu.Lock()
					l.hosts[h] = a
					mu.Unlock()
				}
			}(h)
		}
		wg.Wait()
	}
	return l, nil
}

// runImport is the "import" job: it recomputes the plan from the file the
// request left behind (R8) and applies it row by row, one result per row.
func (s *Server) runImport(ctx context.Context, jc *jobs.Context) error {
	key, _ := jc.Job.Params["payload"].(string)
	v, ok := s.imports.LoadAndDelete(key)
	if !ok {
		return errors.New("the import file is no longer available; upload it again")
	}
	p := v.(*importPayload)
	uctx := auth.WithUser(ctx, p.user)
	lk, err := s.importLookup(uctx, p.table)
	if err != nil {
		return err
	}
	plan := importcsv.Plan(p.table, lk, importcsv.Options{Replace: p.replace, Scoped: len(p.user.Scope) > 0})
	jc.SetTotal(len(plan.Rows))
	// Writes run to completion once a row starts, and the summary is written
	// even when the job is stopped.
	wctx := context.WithoutCancel(uctx)
	counts := map[string]int{}
	cancelled := false
	for _, row := range plan.Rows {
		if ctx.Err() != nil {
			cancelled = true
			jc.Result("", "", "skipped", fmt.Sprintf("stopped at row %d: the job was cancelled; this row and the rest were not applied", row.Row),
				map[string]any{"row": row.Row, "outcome": "cancelled"})
			break
		}
		status, deviceID, msg := s.applyImportRow(ctx, wctx, p, row)
		counts[status]++
		jc.Result(deviceID, row.Address, status, msg, map[string]any{"row": row.Row, "outcome": row.Outcome})
	}
	s.auditAs(wctx, p.user.Username, p.ip, "devices.import", jc.Job.ID, nil, map[string]any{
		"rows": len(plan.Rows), "created": counts["created"], "updated": counts["updated"], "skipped": counts["skipped"],
		"failed": counts["failed"], "replace": replaceColumns(p.replace), "cancelled": cancelled,
	}, "ok")
	return nil
}

// applyImportRow applies one planned row and returns the result's status
// (created | updated | skipped | failed), device and message.
func (s *Server) applyImportRow(ctx, wctx context.Context, p *importPayload, row importcsv.Row) (status, deviceID, msg string) {
	switch row.Outcome {
	case importcsv.OutcomeUpdate:
		dev, err := s.Store.GetDevice(wctx, row.DeviceID)
		if err != nil {
			return "failed", row.DeviceID, "not updated: " + err.Error()
		}
		return s.applyImportFields(wctx, p, dev, row.Fields, row.GroupID, false)
	case importcsv.OutcomeCreate:
		ictx, cancel := context.WithTimeout(ctx, importIdentifyTimeout)
		dev, created, err := s.identifyForImport(ictx, row.Address, row.Hostname)
		cancel()
		unreachable := false
		if err != nil {
			if ctx.Err() != nil {
				return "failed", "", "not created: " + err.Error()
			}
			// The printer did not answer, the case the intent names: record
			// it now with the file's fields. A poll or a discovery identifies
			// the record when it answers (fleet.Service.PollDevice).
			if dev, created, err = s.recordUnidentified(wctx, row.Address, row.Hostname); err != nil {
				return "failed", "", "not created: " + err.Error()
			}
			unreachable = created
		}
		fields := row.Fields
		if !created {
			// Identification found the printer already recorded (by its
			// serial, at another address): the row updates it, by the same
			// rules as any update.
			if !s.allowedDevice(wctx, dev.ID) {
				return "skipped", dev.ID, importcsv.ReasonOutOfScope
			}
			member := false
			if row.GroupID != "" {
				if g, err := s.Store.GetGroup(wctx, row.GroupID); err == nil {
					member = containsID(g.MemberIDs, dev.ID)
				}
			}
			fields, _ = importcsv.Diff(dev, row.Values, member, p.replace)
			if len(fields) == 0 {
				return "skipped", dev.ID, "already recorded; unchanged"
			}
		}
		status, deviceID, msg = s.applyImportFields(wctx, p, dev, fields, row.GroupID, created)
		if unreachable && status == "created" {
			msg += "; not reachable yet, so it will be identified when it answers"
		}
		return status, deviceID, msg
	}
	return "skipped", row.DeviceID, row.Reason
}

// recordUnidentified creates the address-only record of a printer that did
// not answer. The empty capability map stores as an object, which the device
// page reads; nil would store as null.
func (s *Server) recordUnidentified(ctx context.Context, address, hostname string) (*model.Device, bool, error) {
	return s.Store.UpsertDevice(ctx, &model.Identity{Address: address, Hostname: hostname, Method: "import"}, map[model.Capability]string{}, []string{"import"})
}

// importFieldOrder is the order fields are applied and named in messages.
var importFieldOrder = []string{importcsv.ColDisplayName, importcsv.ColLocation, importcsv.ColAssetTag, importcsv.ColNotes, importcsv.ColWebURL, importcsv.ColDNSSync, importcsv.ColState, importcsv.ColGroup}

// applyImportFields writes a row's fields to dev with "import" as their
// source, sets its lifecycle state and adds the group membership, and audits
// the change.
func (s *Server) applyImportFields(ctx context.Context, p *importPayload, dev *model.Device, fields map[string]any, groupID string, created bool) (status, deviceID, msg string) {
	before := operatorFields(dev)
	now := time.Now().UTC()
	patch := store.DevicePatch{FieldSources: map[string]model.FieldSource{}}
	var set []string
	var state model.LifecycleState
	joinGroup := false
	for _, col := range importFieldOrder {
		v, ok := fields[col]
		if !ok {
			continue
		}
		set = append(set, col)
		if col != importcsv.ColGroup {
			patch.FieldSources[col] = model.FieldSource{Source: model.SourceImport, At: now}
		}
		switch col {
		case importcsv.ColDisplayName:
			x := v.(string)
			patch.DisplayName = &x
		case importcsv.ColLocation:
			x := v.(string)
			patch.Location = &x
		case importcsv.ColAssetTag:
			x := v.(string)
			patch.AssetTag = &x
		case importcsv.ColNotes:
			x := v.(string)
			patch.Notes = &x
		case importcsv.ColWebURL:
			x := v.(string)
			patch.WebURLOverride = &x
		case importcsv.ColDNSSync:
			x := v.(bool)
			patch.DNSSync = &x
		case importcsv.ColState:
			state = model.LifecycleState(v.(string))
		case importcsv.ColGroup:
			joinGroup = groupID != ""
		}
	}
	upd := dev
	auditCreated := func(d *model.Device) {
		after := operatorFields(d)
		after["address"] = d.Address
		s.auditAs(ctx, p.user.Username, p.ip, "device.created", dev.ID, nil, after, "ok")
	}
	fail := func(what string, err error) (string, string, string) {
		if created {
			auditCreated(upd) // the record exists whatever else went wrong
			return "failed", dev.ID, fmt.Sprintf("created, but %s: %v", what, err)
		}
		return "failed", dev.ID, fmt.Sprintf("not updated: %s: %v", what, err)
	}
	if state != "" {
		next, err := s.Store.SetDeviceState(ctx, dev.ID, state)
		if err != nil {
			return fail("state not set", err)
		}
		upd = next
	}
	if !patch.Empty() {
		next, err := s.Store.PatchDevice(ctx, dev.ID, patch)
		if err != nil {
			return fail("fields not set", err)
		}
		upd = next
	}
	if joinGroup {
		g, err := s.Store.GetGroup(ctx, groupID)
		if err == nil && !containsID(g.MemberIDs, dev.ID) {
			err = s.Store.SetGroupMembers(ctx, g.ID, append(append([]string{}, g.MemberIDs...), dev.ID))
			if err == nil {
				s.auditAs(ctx, p.user.Username, p.ip, "group.members", g.ID, nil, map[string]any{"count": len(g.MemberIDs) + 1, "added": dev.ID}, "ok")
			}
		}
		if err != nil {
			return fail("group not joined", err)
		}
	}
	if created {
		auditCreated(upd)
	} else {
		s.auditAs(ctx, p.user.Username, p.ip, "device.patch", dev.ID, before, operatorFields(upd), "ok")
	}
	s.publishAs(model.Event{Type: "device.updated", Data: upd}, p.user.Username, p.ip)
	status, msg = "updated", "set "+strings.Join(set, ", ")
	if created {
		status, msg = "created", "created"
		if len(set) > 0 {
			msg += "; set " + strings.Join(set, ", ")
		}
	}
	return status, dev.ID, msg
}

// identifyForImport records a created row's printer the way discovery does:
// the read-only identification, then a read-only first poll so the record
// has status straight away.
func (s *Server) identifyForImport(ctx context.Context, address, hostname string) (*model.Device, bool, error) {
	if s.importIdentify != nil {
		return s.importIdentify(ctx, address, hostname)
	}
	if s.Fleet == nil {
		return nil, false, errors.New("identification is not available")
	}
	methods := []string{"import"}
	dev, created, err := s.Fleet.IdentifyAndUpsert(ctx, address, methods, discovery.Candidate{Address: address, Hostname: hostname, Methods: methods})
	if err != nil {
		return nil, false, err
	}
	if upd, _, perr := s.Fleet.PollDevice(ctx, dev); perr == nil && upd != nil {
		dev = upd
	}
	return dev, created, nil
}

// publishAs publishes an event on behalf of the person who queued a job.
func (s *Server) publishAs(e model.Event, who, ip string) {
	if s.Bus != nil {
		s.Bus.PublishFrom(e, events.Origin{Source: "operator", Actor: who, IP: ip})
		return
	}
	if s.Hub != nil {
		s.Hub.Publish(e)
	}
}

func containsID(list []string, id string) bool {
	for _, x := range list {
		if x == id {
			return true
		}
	}
	return false
}
