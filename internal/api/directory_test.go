package api

import (
	"encoding/json"
	"strings"
	"testing"

	"printmarshal/internal/auth/ldap/ldaptest"
	"printmarshal/internal/config"
)

// The directory settings page (issue #8): an administrator configures LDAP
// at runtime, the service-account password is stored sealed and never
// returned, the test endpoint reports each step, the configuration is in
// force for the next sign-in without a restart, and fields set by the file
// or environment are read-only.
func TestDirectorySettings(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	dir := ldaptest.New(t)
	ldapDirectory(dir)

	// Nothing configured: no directory provider, the page says so.
	code, body := do(t, ts, admin, "GET", "/api/settings/directory", "")
	if code != 200 || !strings.Contains(body, `"enabled":false`) || !strings.Contains(body, `"fileConfigured":false`) || !strings.Contains(body, `"set":false`) {
		t.Fatalf("empty: %d %s", code, body)
	}
	if _, body := do(t, ts, "", "GET", "/api/auth/providers", ""); strings.Contains(body, `"ldap"`) {
		t.Fatalf("provider listed before configuration: %s", body)
	}
	// Viewers and operators cannot read it: it names the directory's internals.
	viewer := login(t, ts, "viewer", "viewerpass1")
	if code, _ := do(t, ts, viewer, "GET", "/api/settings/directory", ""); code != 403 {
		t.Fatalf("viewer read: %d", code)
	}

	// Validation.
	for body, want := range map[string]string{
		`{"enabled":true,"url":"http://dc1"}`:                          "ldap://",
		`{"enabled":true,"url":"ldap://dc1"}`:                          "bindDN",
		`{"url":"ldap://dc1","bindDN":"cn=svc","defaultRole":"root"}`:  "defaultRole",
		`{"url":"ldap://dc1","bindDN":"cn=svc","roleMap":{"g":"god"}}`: "roleMap",
		`{"url":"ldap://dc1","bindDN":"cn=svc","timeout":"2h"}`:        "timeout",
		`{"url":"ldap://dc1","userDNTemplate":"cn=x"}`:                 "%s",
	} {
		if code, resp := do(t, ts, admin, "PUT", "/api/settings/directory", body); code != 400 || !strings.Contains(resp, want) {
			t.Fatalf("validation %s: %d %s", body, code, resp)
		}
	}

	// A test before saving: candidate settings and password, wrong password first.
	cand := `"settings":{"enabled":true,"url":"` + dir.URL + `","bindDN":"cn=svc,ou=service,dc=example,dc=com","baseDN":"dc=example,dc=com","roleMap":{"Printer Admins":"admin"},"displayName":"Corp AD"}`
	code, body = do(t, ts, admin, "POST", "/api/settings/directory/test", `{`+cand+`,"bindPassword":"wrong"}`)
	if code != 200 || !strings.Contains(body, `"ok":false`) || !strings.Contains(body, "service account bind") {
		t.Fatalf("test with wrong service password: %d %s", code, body)
	}
	code, body = do(t, ts, admin, "POST", "/api/settings/directory/test", `{`+cand+`,"bindPassword":"svcpass","username":"ann","password":"annpass"}`)
	if code != 200 || strings.Contains(body, `"ok":false`) || !strings.Contains(body, `"role":"admin"`) || !strings.Contains(body, `"allowed":true`) || !strings.Contains(body, "Printer Admins") {
		t.Fatalf("test sign-in: %d %s", code, body)
	}
	if strings.Contains(body, "annpass") || strings.Contains(body, "svcpass") {
		t.Fatalf("a password leaked into the test result: %s", body)
	}
	code, body = do(t, ts, admin, "POST", "/api/settings/directory/test", `{`+cand+`,"bindPassword":"svcpass","username":"ann","password":"nope"}`)
	if code != 200 || !strings.Contains(body, "rejected the user name or password") {
		t.Fatalf("test wrong user password: %d %s", code, body)
	}

	// Save with the password: it is stored sealed, reported as set, never returned.
	code, body = do(t, ts, admin, "PUT", "/api/settings/directory", `{"enabled":true,"url":"`+dir.URL+`","bindDN":"cn=svc,ou=service,dc=example,dc=com","baseDN":"dc=example,dc=com","roleMap":{"Printer Admins":"admin","cn=staff,ou=groups,dc=example,dc=com":"operator"},"displayName":"Corp AD","bindPassword":"svcpass"}`)
	if code != 200 || !strings.Contains(body, `"enabled":true`) || !strings.Contains(body, `"set":true`) || !strings.Contains(body, `"source":"ui"`) {
		t.Fatalf("save: %d %s", code, body)
	}
	if strings.Contains(body, "svcpass") {
		t.Fatalf("password returned: %s", body)
	}
	var page struct {
		Sources map[string]string `json:"sources"`
	}
	_ = json.Unmarshal([]byte(body), &page)
	if page.Sources["url"] != "ui" || page.Sources["userFilter"] != "default" {
		t.Fatalf("sources: %v", page.Sources)
	}
	if v, err := s.Store.GetSecretSetting(t.Context(), directorySecretKey); err != nil || v != "svcpass" {
		t.Fatalf("sealed password: %q %v", v, err)
	}
	var raw map[string]any
	if err := s.Store.GetSetting(t.Context(), directorySecretKey, &raw); err != nil || strings.Contains(raw["sealed"].(string), "svcpass") {
		t.Fatalf("password stored in clear: %v %v", raw, err)
	}

	// In force at once: the provider is listed and ann signs in as admin.
	if _, body := do(t, ts, "", "GET", "/api/auth/providers", ""); !strings.Contains(body, `{"kind":"ldap","name":"Corp AD","method":"password"}`) {
		t.Fatalf("provider after save: %s", body)
	}
	code, body = do(t, ts, "", "POST", "/api/auth/login", `{"username":"ann","password":"annpass"}`)
	if code != 200 || !strings.Contains(body, `"role":"admin"`) {
		t.Fatalf("ldap login via saved settings: %d %s", code, body)
	}
	// Audit: the change is recorded without the password.
	_, audit := do(t, ts, admin, "GET", "/api/audit?limit=30", "")
	if !strings.Contains(audit, "settings.directory.update") || !strings.Contains(audit, `"bindPassword":"set"`) || strings.Contains(audit, "svcpass") || !strings.Contains(audit, "settings.directory.test") {
		t.Fatalf("audit: %s", audit)
	}

	// Switching it off from the UI stops sign-ins without touching the password.
	code, body = do(t, ts, admin, "PUT", "/api/settings/directory", `{"enabled":false,"url":"`+dir.URL+`","bindDN":"cn=svc,ou=service,dc=example,dc=com","baseDN":"dc=example,dc=com"}`)
	if code != 200 || !strings.Contains(body, `"enabled":false`) || !strings.Contains(body, `"set":true`) {
		t.Fatalf("disable: %d %s", code, body)
	}
	if code, _ := do(t, ts, "", "POST", "/api/auth/login", `{"username":"ann","password":"annpass"}`); code != 401 {
		t.Fatalf("login while disabled: %d", code)
	}
	// Clearing the password.
	if code, body := do(t, ts, admin, "PUT", "/api/settings/directory", `{"enabled":false,"url":"`+dir.URL+`","bindDN":"cn=svc,ou=service,dc=example,dc=com","baseDN":"dc=example,dc=com","bindPassword":""}`); code != 200 || !strings.Contains(body, `"set":false`) {
		t.Fatalf("clear password: %d %s", code, body)
	}

	// The file wins: a URL in the configuration is reported as such and cannot be changed here.
	s.Cfg.Auth.LDAP = config.LDAP{URL: "ldaps://dc-from-file.example.com:636", BindDN: "cn=file,dc=example,dc=com", BindPasswordEnv: "PM_TEST_DIR_BIND"}
	t.Setenv("PM_TEST_DIR_BIND", "filepass")
	t.Setenv("PM_LDAP_BIND_DN", "cn=file,dc=example,dc=com")
	code, body = do(t, ts, admin, "GET", "/api/settings/directory", "")
	_ = json.Unmarshal([]byte(body), &page)
	if code != 200 || !strings.Contains(body, `"fileConfigured":true`) || !strings.Contains(body, `"enabled":true`) || page.Sources["url"] != "file" || page.Sources["bindDN"] != "env" || !strings.Contains(body, `"source":"env"`) {
		t.Fatalf("file-configured: %d %s", code, body)
	}
	code, body = do(t, ts, admin, "PUT", "/api/settings/directory", `{"enabled":true,"url":"ldap://elsewhere","bindDN":"cn=file,dc=example,dc=com"}`)
	if code != 400 || !strings.Contains(body, "cannot be changed here: url") {
		t.Fatalf("locked field: %d %s", code, body)
	}
	if code, body := do(t, ts, admin, "PUT", "/api/settings/directory", `{"enabled":true,"url":"ldaps://dc-from-file.example.com:636","bindDN":"cn=file,dc=example,dc=com","bindPassword":"x"}`); code != 400 || !strings.Contains(body, "PM_TEST_DIR_BIND") {
		t.Fatalf("env password locked: %d %s", code, body)
	}
	// Editable fields can still be saved alongside the file's values.
	if code, body := do(t, ts, admin, "PUT", "/api/settings/directory", `{"enabled":true,"url":"ldaps://dc-from-file.example.com:636","bindDN":"cn=file,dc=example,dc=com","displayName":"Corp","roleMap":{"ops":"operator"}}`); code != 200 || !strings.Contains(body, `"displayName":"Corp"`) {
		t.Fatalf("save editable fields: %d %s", code, body)
	}
}
