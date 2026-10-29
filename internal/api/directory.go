package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"printmarshal/internal/auth/ldap"
	"printmarshal/internal/auth/oidc"
	"printmarshal/internal/config"
	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// Directory (LDAP / Active Directory) settings editable at runtime (issue
// #8). The configuration file and the PM_LDAP_* environment win, field by
// field, over what an administrator saves in the UI, and such fields are
// reported read-only with their source so it is clear which value is in
// force. Everything saved here takes effect on the next sign-in: nothing is
// cached. The service-account password is kept sealed in the settings table
// (store.SetSecretSetting), is write-only from the API, and is overridden by
// a non-empty bindPasswordEnv variable when the file names one.

const (
	directorySettingKey = "auth.ldap"
	directorySecretKey  = "auth.ldap.bindPassword"
)

// directorySettings is the non-secret part, as saved and as served.
type directorySettings struct {
	Enabled            bool              `json:"enabled"`
	URL                string            `json:"url"`
	StartTLS           bool              `json:"startTls"`
	CAFile             string            `json:"caFile"`
	InsecureSkipVerify bool              `json:"insecureSkipVerify"`
	BindDN             string            `json:"bindDN"`
	UserDNTemplate     string            `json:"userDNTemplate"`
	BaseDN             string            `json:"baseDN"`
	UserFilter         string            `json:"userFilter"`
	UsernameAttr       string            `json:"usernameAttr"`
	SubjectAttr        string            `json:"subjectAttr"`
	GroupAttr          string            `json:"groupAttr"`
	RoleMap            map[string]string `json:"roleMap"`
	DefaultRole        string            `json:"defaultRole"`
	AllowedGroups      []string          `json:"allowedGroups"`
	Timeout            string            `json:"timeout"` // Go duration, "" = default
	DisplayName        string            `json:"displayName"`
}

// directoryState is the resolved picture: effective values, where each came
// from, what the UI saved, and the secret (never serialised).
type directoryState struct {
	Settings       directorySettings
	Sources        map[string]string // field -> file | env | ui | default
	Saved          directorySettings
	FileConfigured bool // the file or environment supplies the server URL
	bindPassword   string
	PasswordSet    bool
	PasswordSource string // env | ui | ""
}

// directoryEnv maps a settings field to the environment variable that can set it.
var directoryEnv = map[string]string{
	"url": "PM_LDAP_URL", "startTls": "PM_LDAP_START_TLS", "caFile": "PM_LDAP_CA_FILE", "bindDN": "PM_LDAP_BIND_DN",
	"userDNTemplate": "PM_LDAP_USER_DN_TEMPLATE", "baseDN": "PM_LDAP_BASE_DN", "userFilter": "PM_LDAP_USER_FILTER",
	"usernameAttr": "PM_LDAP_USERNAME_ATTR", "subjectAttr": "PM_LDAP_SUBJECT_ATTR", "groupAttr": "PM_LDAP_GROUP_ATTR",
	"defaultRole": "PM_LDAP_DEFAULT_ROLE", "timeout": "PM_LDAP_TIMEOUT", "displayName": "PM_LDAP_DISPLAY_NAME",
	"insecureSkipVerify": "PM_LDAP_INSECURE_SKIP_VERIFY", "roleMap": "PM_LDAP_ROLE_MAP", "allowedGroups": "PM_LDAP_ALLOWED_GROUPS",
}

func directorySourceOf(field string) string {
	if v, ok := directoryEnv[field]; ok && os.Getenv(v) != "" {
		return "env"
	}
	return "file"
}

// directoryState resolves the effective directory configuration.
func (s *Server) directoryState(ctx context.Context) directoryState {
	file := s.Cfg.Auth.LDAP
	st := directoryState{Sources: map[string]string{}}
	if err := s.Store.GetSetting(ctx, directorySettingKey, &st.Saved); err != nil && !errors.Is(err, store.ErrNotFound) {
		s.Log.Error("directory settings", "err", err)
	}
	saved := st.Saved
	eff := &st.Settings
	str := func(field string, dst *string, fileV, savedV string) {
		switch {
		case strings.TrimSpace(fileV) != "":
			*dst, st.Sources[field] = fileV, directorySourceOf(field)
		case strings.TrimSpace(savedV) != "":
			*dst, st.Sources[field] = savedV, "ui"
		default:
			st.Sources[field] = "default"
		}
	}
	boolean := func(field string, dst *bool, fileV, savedV bool) {
		switch {
		case fileV:
			*dst, st.Sources[field] = true, directorySourceOf(field)
		case savedV:
			*dst, st.Sources[field] = true, "ui"
		default:
			st.Sources[field] = "default"
		}
	}
	str("url", &eff.URL, file.URL, saved.URL)
	st.FileConfigured = strings.TrimSpace(file.URL) != ""
	boolean("startTls", &eff.StartTLS, file.StartTLS, saved.StartTLS)
	str("caFile", &eff.CAFile, file.CAFile, saved.CAFile)
	boolean("insecureSkipVerify", &eff.InsecureSkipVerify, file.InsecureSkipVerify, saved.InsecureSkipVerify)
	str("bindDN", &eff.BindDN, file.BindDN, saved.BindDN)
	str("userDNTemplate", &eff.UserDNTemplate, file.UserDNTemplate, saved.UserDNTemplate)
	str("baseDN", &eff.BaseDN, file.BaseDN, saved.BaseDN)
	str("userFilter", &eff.UserFilter, file.UserFilter, saved.UserFilter)
	str("usernameAttr", &eff.UsernameAttr, file.UsernameAttr, saved.UsernameAttr)
	str("subjectAttr", &eff.SubjectAttr, file.SubjectAttr, saved.SubjectAttr)
	str("groupAttr", &eff.GroupAttr, file.GroupAttr, saved.GroupAttr)
	str("defaultRole", &eff.DefaultRole, file.DefaultRole, saved.DefaultRole)
	str("displayName", &eff.DisplayName, file.DisplayName, saved.DisplayName)
	fileTimeout := ""
	if file.Timeout > 0 {
		fileTimeout = file.Timeout.D().String()
	}
	str("timeout", &eff.Timeout, fileTimeout, saved.Timeout)
	switch {
	case len(file.RoleMap) > 0:
		eff.RoleMap, st.Sources["roleMap"] = file.RoleMap, directorySourceOf("roleMap")
	case len(saved.RoleMap) > 0:
		eff.RoleMap, st.Sources["roleMap"] = saved.RoleMap, "ui"
	default:
		eff.RoleMap, st.Sources["roleMap"] = map[string]string{}, "default"
	}
	switch {
	case len(file.AllowedGroups) > 0:
		eff.AllowedGroups, st.Sources["allowedGroups"] = file.AllowedGroups, directorySourceOf("allowedGroups")
	case len(saved.AllowedGroups) > 0:
		eff.AllowedGroups, st.Sources["allowedGroups"] = saved.AllowedGroups, "ui"
	default:
		eff.AllowedGroups, st.Sources["allowedGroups"] = []string{}, "default"
	}
	// Enabled: a server named in the file is always in force; otherwise the
	// administrator's switch decides, and nothing is enabled without a URL.
	switch {
	case st.FileConfigured:
		eff.Enabled, st.Sources["enabled"] = true, st.Sources["url"]
	case saved.Enabled && eff.URL != "":
		eff.Enabled, st.Sources["enabled"] = true, "ui"
	default:
		st.Sources["enabled"] = "default"
	}
	// The service-account password: the environment variable the file names
	// wins when it is non-empty; otherwise the sealed setting.
	if file.BindPasswordEnv != "" {
		if v := os.Getenv(file.BindPasswordEnv); v != "" {
			st.bindPassword, st.PasswordSet, st.PasswordSource = v, true, "env"
			return st
		}
	}
	if v, err := s.Store.GetSecretSetting(ctx, directorySecretKey); err == nil {
		st.bindPassword, st.PasswordSet, st.PasswordSource = v, true, "ui"
	} else if !errors.Is(err, store.ErrNotFound) {
		s.Log.Error("directory bind password", "err", err)
	}
	return st
}

// ldapConfigOf turns the effective settings into the package's Config.
func (st directoryState) ldapConfig() ldap.Config {
	e := st.Settings
	cfg := ldap.Config{URL: strings.TrimSpace(e.URL), StartTLS: e.StartTLS, CAFile: e.CAFile, InsecureSkipVerify: e.InsecureSkipVerify,
		BindDN: e.BindDN, BindPassword: st.bindPassword, UserDNTemplate: e.UserDNTemplate, BaseDN: e.BaseDN, UserFilter: e.UserFilter,
		UsernameAttr: e.UsernameAttr, SubjectAttr: e.SubjectAttr, GroupAttr: e.GroupAttr}
	if d, err := time.ParseDuration(e.Timeout); err == nil {
		cfg.Timeout = d
	}
	return cfg
}

func (st directoryState) name() string {
	if n := st.Settings.DisplayName; n != "" {
		return n
	}
	return hostOf(st.Settings.URL)
}

// validateDirectory checks what an administrator submits.
func validateDirectory(d directorySettings) error {
	if u := strings.TrimSpace(d.URL); u != "" {
		p, err := url.Parse(u)
		if err != nil || (p.Scheme != "ldap" && p.Scheme != "ldaps") || p.Host == "" {
			return errors.New("url must be ldap://host[:port] or ldaps://host[:port]")
		}
	}
	if d.Enabled && strings.TrimSpace(d.URL) == "" {
		return errors.New("directory sign-in cannot be enabled without a server url")
	}
	if d.Enabled && d.BindDN == "" && d.UserDNTemplate == "" {
		return errors.New("either a service account (bindDN) or a userDNTemplate is required")
	}
	if d.UserDNTemplate != "" && !strings.Contains(d.UserDNTemplate, "%s") {
		return errors.New("userDNTemplate must contain %s where the login name goes")
	}
	if d.UserFilter != "" && !strings.Contains(d.UserFilter, "%s") {
		return errors.New("userFilter must contain %s where the login name goes")
	}
	if d.DefaultRole != "" && !model.Role(d.DefaultRole).Allows(model.RoleViewer) {
		return fmt.Errorf("defaultRole %q is not viewer, operator or admin", d.DefaultRole)
	}
	for g, r := range d.RoleMap {
		if strings.TrimSpace(g) == "" || !model.Role(r).Allows(model.RoleViewer) {
			return fmt.Errorf("roleMap: %q must map a group to viewer, operator or admin", g)
		}
	}
	if d.Timeout != "" {
		if t, err := time.ParseDuration(d.Timeout); err != nil || t <= 0 || t > time.Minute {
			return errors.New("timeout must be a duration between 1s and 1m, such as 10s")
		}
	}
	return nil
}

// lockedChanges lists fields the submission would change although the file
// or environment sets them.
func (st directoryState) lockedChanges(in directorySettings) []string {
	var locked []string
	chk := func(field string, same bool) {
		if src := st.Sources[field]; (src == "file" || src == "env") && !same {
			locked = append(locked, field)
		}
	}
	e := st.Settings
	chk("url", in.URL == e.URL)
	chk("startTls", in.StartTLS == e.StartTLS)
	chk("caFile", in.CAFile == e.CAFile)
	chk("insecureSkipVerify", in.InsecureSkipVerify == e.InsecureSkipVerify)
	chk("bindDN", in.BindDN == e.BindDN)
	chk("userDNTemplate", in.UserDNTemplate == e.UserDNTemplate)
	chk("baseDN", in.BaseDN == e.BaseDN)
	chk("userFilter", in.UserFilter == e.UserFilter)
	chk("usernameAttr", in.UsernameAttr == e.UsernameAttr)
	chk("subjectAttr", in.SubjectAttr == e.SubjectAttr)
	chk("groupAttr", in.GroupAttr == e.GroupAttr)
	chk("defaultRole", in.DefaultRole == e.DefaultRole)
	chk("displayName", in.DisplayName == e.DisplayName)
	chk("timeout", in.Timeout == e.Timeout)
	chk("roleMap", sameMap(in.RoleMap, e.RoleMap))
	chk("allowedGroups", sameList(in.AllowedGroups, e.AllowedGroups))
	chk("enabled", in.Enabled == e.Enabled)
	sort.Strings(locked)
	return locked
}

func sameMap(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func sameList(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (st directoryState) payload() map[string]any {
	issuer := ""
	if st.Settings.URL != "" {
		issuer = st.ldapConfig().Issuer()
	}
	return map[string]any{
		"enabled":        st.Settings.Enabled,
		"fileConfigured": st.FileConfigured,
		"settings":       st.Settings,
		"sources":        st.Sources,
		"saved":          st.Saved,
		"bindPassword":   map[string]any{"set": st.PasswordSet, "source": st.PasswordSource},
		"issuer":         issuer,
		"name":           st.name(),
		"defaults":       map[string]any{"userFilter": ldap.DefaultUserFilter, "timeout": ldap.DefaultTimeout.String(), "groupAttr": "memberOf", "defaultRole": "viewer"},
	}
}

func (s *Server) getDirectorySettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.directoryState(r.Context()).payload())
}

// directoryUpdate is the PUT body: the settings plus an optional password.
// A nil password leaves the stored one alone; "" clears it.
type directoryUpdate struct {
	directorySettings
	BindPassword *string `json:"bindPassword"`
}

func (s *Server) putDirectorySettings(w http.ResponseWriter, r *http.Request) {
	var body directoryUpdate
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	in := normaliseDirectory(body.directorySettings)
	if err := validateDirectory(in); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx := r.Context()
	st := s.directoryState(ctx)
	if locked := st.lockedChanges(in); len(locked) > 0 {
		writeErr(w, http.StatusBadRequest, "set by the configuration file or environment and cannot be changed here: "+strings.Join(locked, ", "))
		return
	}
	if body.BindPassword != nil && st.PasswordSource == "env" {
		writeErr(w, http.StatusBadRequest, "the service-account password comes from "+s.Cfg.Auth.LDAP.BindPasswordEnv+" and cannot be changed here")
		return
	}
	// Fields the file sets are not persisted: the file keeps winning and the
	// saved copy stays what the administrator can actually edit.
	saved := in
	for field, src := range st.Sources {
		if src == "file" || src == "env" {
			clearDirectoryField(&saved, field)
		}
	}
	if err := s.Store.SetSetting(ctx, directorySettingKey, saved); err != nil {
		s.fail(w, err)
		return
	}
	pw := "unchanged"
	if body.BindPassword != nil {
		if err := s.Store.SetSecretSetting(ctx, directorySecretKey, *body.BindPassword); err != nil {
			s.fail(w, err)
			return
		}
		pw = "set"
		if *body.BindPassword == "" {
			pw = "cleared"
		}
	}
	after := s.directoryState(ctx)
	s.audit(ctx, r, "settings.directory.update", "ldap", map[string]any{"settings": st.Settings}, map[string]any{"settings": after.Settings, "bindPassword": pw}, "ok")
	writeJSON(w, http.StatusOK, after.payload())
}

func normaliseDirectory(d directorySettings) directorySettings {
	d.URL = strings.TrimSpace(d.URL)
	d.BindDN = strings.TrimSpace(d.BindDN)
	d.BaseDN = strings.TrimSpace(d.BaseDN)
	d.DisplayName = strings.TrimSpace(d.DisplayName)
	d.Timeout = strings.TrimSpace(d.Timeout)
	// An empty group row (the editor's "Add group" before it is filled in) is
	// dropped rather than refused.
	roles := map[string]string{}
	for g, r := range d.RoleMap {
		if g = strings.TrimSpace(g); g != "" {
			roles[g] = strings.TrimSpace(r)
		}
	}
	d.RoleMap = roles
	groups := d.AllowedGroups[:0:0]
	for _, g := range d.AllowedGroups {
		if g = strings.TrimSpace(g); g != "" {
			groups = append(groups, g)
		}
	}
	d.AllowedGroups = groups
	return d
}

func clearDirectoryField(d *directorySettings, field string) {
	switch field {
	case "url":
		d.URL = ""
	case "startTls":
		d.StartTLS = false
	case "caFile":
		d.CAFile = ""
	case "insecureSkipVerify":
		d.InsecureSkipVerify = false
	case "bindDN":
		d.BindDN = ""
	case "userDNTemplate":
		d.UserDNTemplate = ""
	case "baseDN":
		d.BaseDN = ""
	case "userFilter":
		d.UserFilter = ""
	case "usernameAttr":
		d.UsernameAttr = ""
	case "subjectAttr":
		d.SubjectAttr = ""
	case "groupAttr":
		d.GroupAttr = ""
	case "defaultRole":
		d.DefaultRole = ""
	case "displayName":
		d.DisplayName = ""
	case "timeout":
		d.Timeout = ""
	case "roleMap":
		d.RoleMap = map[string]string{}
	case "allowedGroups":
		d.AllowedGroups = []string{}
	case "enabled":
		d.Enabled = false
	}
}

// directoryTest is the POST body of the test endpoint: optional candidate
// settings and password (so a configuration can be tried before it is
// saved) and an optional user to sign in as. The user's password is used for
// one bind and never stored or logged.
type directoryTest struct {
	Settings     *directorySettings `json:"settings"`
	BindPassword *string            `json:"bindPassword"`
	Username     string             `json:"username"`
	Password     string             `json:"password"`
}

type directoryStep struct {
	Name  string `json:"name"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func (s *Server) testDirectory(w http.ResponseWriter, r *http.Request) {
	var body directoryTest
	if err := readJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx := r.Context()
	st := s.directoryState(ctx)
	if body.Settings != nil {
		in := normaliseDirectory(*body.Settings)
		if err := validateDirectory(in); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if locked := st.lockedChanges(in); len(locked) > 0 {
			writeErr(w, http.StatusBadRequest, "set by the configuration file or environment and cannot be changed here: "+strings.Join(locked, ", "))
			return
		}
		st.Settings = in
	}
	if body.BindPassword != nil && *body.BindPassword != "" && st.PasswordSource != "env" {
		st.bindPassword = *body.BindPassword
	}
	cfg := st.ldapConfig()
	if cfg.BindDN != "" && cfg.BindPassword == "" {
		writeErr(w, http.StatusBadRequest, "a service account is configured but no password is set; enter one to test")
		return
	}
	tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out := map[string]any{}
	steps := []directoryStep{}
	if err := ldap.TestConnection(tctx, cfg); err != nil {
		steps = append(steps, directoryStep{Name: "connect and service bind", Error: err.Error()})
		out["steps"] = steps
		s.audit(ctx, r, "settings.directory.test", "ldap", nil, map[string]any{"url": cfg.URL, "user": body.Username}, "failed")
		writeJSON(w, http.StatusOK, out)
		return
	}
	steps = append(steps, directoryStep{Name: "connect and service bind", OK: true})
	result := "ok"
	if strings.TrimSpace(body.Username) != "" {
		id, err := ldap.Authenticate(tctx, cfg, body.Username, body.Password)
		if err != nil {
			msg := err.Error()
			if errors.Is(err, ldap.ErrInvalidCredentials) {
				msg = "the directory rejected the user name or password"
			}
			steps = append(steps, directoryStep{Name: "sign in as " + body.Username, Error: msg})
			result = "failed"
		} else {
			role := oidc.MapRole(lowerAll(id.Candidates()), lowerKeys(st.Settings.RoleMap), st.Settings.DefaultRole)
			allowed := len(st.Settings.AllowedGroups) == 0 || id.InGroup(st.Settings.AllowedGroups)
			steps = append(steps, directoryStep{Name: "sign in as " + body.Username, OK: true})
			out["signIn"] = map[string]any{"dn": id.DN, "username": id.Username, "subject": id.Subject, "groups": id.Candidates(), "role": role, "allowed": allowed}
			if !allowed {
				steps = append(steps, directoryStep{Name: "allowed groups", Error: "the user is in none of the allowed groups"})
			} else {
				steps = append(steps, directoryStep{Name: "allowed groups", OK: true})
			}
		}
	}
	out["steps"] = steps
	s.audit(ctx, r, "settings.directory.test", "ldap", nil, map[string]any{"url": cfg.URL, "user": body.Username}, result)
	writeJSON(w, http.StatusOK, out)
}

// fileLDAP exposes the file section for the read-only "configured by" hints.
func (s *Server) fileLDAP() config.LDAP { return s.Cfg.Auth.LDAP }
