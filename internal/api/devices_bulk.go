package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// Bulk edit of operator fields (intent/bulk-device-edit): one explicit,
// audited action over many devices. Scans never touch these fields; a
// bulk edit is the operator speaking for all of them at once.

const (
	bulkMaxDevices = 2000
)

type bulkPatchBody struct {
	DeviceIDs     []string `json:"deviceIds"`
	RenameFromDNS bool     `json:"renameFromDns"`
	Set           struct {
		DNSSync    *bool   `json:"dnsSync"`
		Location   *string `json:"location"`
		AppendNote *string `json:"appendNote"`
	} `json:"set"`
}

type bulkResult struct {
	ID      string   `json:"id"`
	Changed []string `json:"changed"`
	Skipped string   `json:"skipped,omitempty"`
}

// appendNote adds text to a device's notes on its own paragraph; an empty
// existing note gets the text alone.
func appendNote(existing, text string) string {
	existing = strings.TrimRight(existing, "\r\n\t ")
	text = strings.TrimSpace(text)
	if existing == "" {
		return text
	}
	return existing + "\n\n" + text
}

func (s *Server) bulkPatchDevices(w http.ResponseWriter, r *http.Request) {
	var body bulkPatchBody
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(body.DeviceIDs) == 0 {
		writeErr(w, http.StatusBadRequest, "deviceIds is required")
		return
	}
	if len(body.DeviceIDs) > bulkMaxDevices {
		writeErr(w, http.StatusBadRequest, fmt.Sprintf("at most %d devices per request", bulkMaxDevices))
		return
	}
	if !body.RenameFromDNS && body.Set.DNSSync == nil && body.Set.Location == nil && body.Set.AppendNote == nil {
		writeErr(w, http.StatusBadRequest, "nothing to change: give renameFromDns or one of set.dnsSync, set.location, set.appendNote")
		return
	}
	if body.Set.Location != nil {
		loc := strings.TrimSpace(*body.Set.Location)
		if len(loc) > model.MaxLocation {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("location is over %d characters", model.MaxLocation))
			return
		}
		body.Set.Location = &loc
	}
	if body.Set.AppendNote != nil {
		note := strings.TrimSpace(*body.Set.AppendNote)
		if note == "" {
			writeErr(w, http.StatusBadRequest, "appendNote is empty")
			return
		}
		if len(note) > model.MaxNote {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("appendNote is over %d characters", model.MaxNote))
			return
		}
		body.Set.AppendNote = &note
	}
	if !s.allowedAll(w, r, body.DeviceIDs) {
		return
	}

	// Load every device first (nil marks one not found), then look up the
	// DNS names that are not stored yet before renaming from them.
	loaded := make([]*model.Device, len(body.DeviceIDs))
	for i, id := range body.DeviceIDs {
		d, err := s.Store.GetDevice(r.Context(), id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				continue
			}
			s.fail(w, err)
			return
		}
		loaded[i] = d
	}
	// before keeps each record as it was ahead of the lookup: a device kept in
	// sync with DNS is renamed by the lookup itself, and that rename is still
	// this edit's doing.
	before := append([]*model.Device(nil), loaded...)
	if body.RenameFromDNS {
		s.ensureDNSNames(r.Context(), loaded)
	}

	results := make([]bulkResult, 0, len(body.DeviceIDs))
	changedDevices, skipped := 0, 0
	for i, id := range body.DeviceIDs {
		d, orig := loaded[i], before[i]
		if d == nil {
			results = append(results, bulkResult{ID: id, Changed: []string{}, Skipped: "not found"})
			skipped++
			continue
		}
		patch := store.DevicePatch{}
		var changed []string
		var notes []string
		if body.RenameFromDNS {
			if to, reason := dnsRenameTarget(d); reason != "" {
				notes = append(notes, "rename skipped: "+reason)
			} else if to != d.Name() {
				src := "dns"
				patch.DisplayName, patch.DisplayNameSource = &to, &src
				changed = append(changed, "displayName")
			} else if d.Name() != orig.Name() {
				changed = append(changed, "displayName")
			}
		}
		if body.Set.DNSSync != nil && *body.Set.DNSSync != d.DNSSync {
			v := *body.Set.DNSSync
			patch.DNSSync = &v
			changed = append(changed, "dnsSync")
		}
		if body.Set.Location != nil && *body.Set.Location != d.Location {
			v := *body.Set.Location
			patch.Location = &v
			changed = append(changed, "location")
		}
		if body.Set.AppendNote != nil {
			v := appendNote(d.Notes, *body.Set.AppendNote)
			patch.Notes = &v
			changed = append(changed, "notes")
		}
		res := bulkResult{ID: d.ID, Changed: changed}
		if len(changed) == 0 {
			res.Changed = []string{}
			res.Skipped = "unchanged"
			if len(notes) > 0 {
				res.Skipped = strings.Join(notes, "; ")
			}
			skipped++
			results = append(results, res)
			continue
		}
		// A rename done by the lookup alone leaves nothing to patch.
		upd := d
		if patch != (store.DevicePatch{}) {
			var err error
			if upd, err = s.Store.PatchDevice(r.Context(), d.ID, patch); err != nil {
				s.fail(w, err)
				return
			}
		}
		s.audit(r.Context(), r, "device.patch", d.ID, operatorFields(orig), operatorFields(upd), "ok")
		s.publish(r, model.Event{Type: "device.updated", Data: upd})
		if len(notes) > 0 {
			res.Skipped = strings.Join(notes, "; ")
		}
		changedDevices++
		results = append(results, res)
	}
	s.audit(r.Context(), r, "devices.bulk", fmt.Sprintf("%d devices", len(body.DeviceIDs)), nil, map[string]any{
		"renameFromDns": body.RenameFromDNS, "dnsSync": body.Set.DNSSync, "location": body.Set.Location,
		"appendNote": body.Set.AppendNote != nil, "changed": changedDevices, "skipped": skipped,
	}, "ok")
	writeJSON(w, http.StatusOK, map[string]any{"changed": changedDevices, "skipped": skipped, "results": results})
}
