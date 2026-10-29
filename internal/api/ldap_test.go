package api

import (
	"strings"
	"testing"

	"printmarshal/internal/auth/ldap/ldaptest"
	"printmarshal/internal/config"
)

var testGUID = []byte{0x33, 0x22, 0x11, 0x00, 0x55, 0x44, 0x77, 0x66, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}

func ldapDirectory(s *ldaptest.Server) {
	s.Passwords["cn=svc,ou=service,dc=example,dc=com"] = "svcpass"
	s.Passwords["cn=ann,ou=people,dc=example,dc=com"] = "annpass"
	s.Passwords["cn=admin,ou=people,dc=example,dc=com"] = "ldapadminpass"
	s.Passwords["cn=vic,ou=people,dc=example,dc=com"] = "vicpass"
	s.Entries = []*ldaptest.Entry{
		{DN: "CN=ann,OU=people,DC=example,DC=com", Attrs: map[string][]string{"sAMAccountName": {"ann"}, "userPrincipalName": {"ann@example.com"},
			"memberOf": {"CN=Printer Admins,OU=Groups,DC=example,DC=com", "CN=Staff,OU=Groups,DC=example,DC=com"}}, Raw: map[string][][]byte{"objectGUID": {testGUID}}},
		// A directory user named like the local administrator.
		{DN: "CN=admin,OU=people,DC=example,DC=com", Attrs: map[string][]string{"sAMAccountName": {"admin"}, "memberOf": {"CN=Printer Admins,OU=Groups,DC=example,DC=com"}, "entryUUID": {"uuid-admin"}}},
		// A directory user whose account name collides with the local "viewer" but who signs in with the UPN.
		{DN: "CN=vic,OU=people,DC=example,DC=com", Attrs: map[string][]string{"sAMAccountName": {"viewer"}, "userPrincipalName": {"vic@example.com"}, "entryUUID": {"uuid-vic"}}},
	}
}

// Login-form sign-in against the fake directory: service-account flow,
// provisioning, role mapping and re-sync, allowed groups, indistinguishable
// failures, and the local admin staying out of the directory's reach.
func TestLDAPLogin(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	dir := ldaptest.New(t)
	ldapDirectory(dir)
	t.Setenv("PM_TEST_LDAP_BIND", "svcpass")
	s.Cfg.Auth.LDAP = config.LDAP{URL: dir.URL, BindDN: "cn=svc,ou=service,dc=example,dc=com", BindPasswordEnv: "PM_TEST_LDAP_BIND", BaseDN: "dc=example,dc=com", DisplayName: "Corp AD",
		RoleMap: map[string]string{"Printer Admins": "admin", "cn=staff,ou=groups,dc=example,dc=com": "operator"}}

	code, body := do(t, ts, "", "GET", "/api/auth/providers", "")
	if code != 200 || !strings.Contains(body, `{"kind":"ldap","name":"Corp AD","method":"password"}`) {
		t.Fatalf("providers: %d %s", code, body)
	}

	// ann signs in with her password: provisioned as admin, bound by objectGUID.
	code, body = do(t, ts, "", "POST", "/api/auth/login", `{"username":"ann","password":"annpass"}`)
	if code != 200 || !strings.Contains(body, `"username":"ann"`) || !strings.Contains(body, `"role":"admin"`) {
		t.Fatalf("ldap login: %d %s", code, body)
	}
	if _, users := do(t, ts, admin, "GET", "/api/users", ""); !strings.Contains(users, `"externalIssuer":"ldap:`+dir.URL+`"`) || !strings.Contains(users, `"externalSubject":"00112233-4455-6677-8899-aabbccddeeff"`) {
		t.Fatalf("users: %s", users)
	}
	_, audit := do(t, ts, admin, "GET", "/api/audit?limit=20", "")
	if !strings.Contains(audit, `"source":"ldap"`) || !strings.Contains(audit, "signed in via Corp AD") {
		t.Fatalf("audit: %s", audit)
	}
	// The directory sees the service bind followed by ann's bind, never a bind for a local account.
	if len(dir.Binds) != 2 || dir.Binds[1] != "CN=ann,OU=people,DC=example,DC=com" {
		t.Fatalf("binds %v", dir.Binds)
	}
	// Wrong password and unknown user: same status, same body.
	c1, b1 := do(t, ts, "", "POST", "/api/auth/login", `{"username":"ann","password":"wrong"}`)
	c2, b2 := do(t, ts, "", "POST", "/api/auth/login", `{"username":"nobody","password":"annpass"}`)
	if c1 != 401 || c2 != 401 || b1 != b2 || !strings.Contains(b1, "invalid credentials") {
		t.Fatalf("failures differ: %d %s / %d %s", c1, b1, c2, b2)
	}
	// The local admin never reaches the directory, even with the directory admin's password.
	binds := len(dir.Binds)
	if code, _ := do(t, ts, "", "POST", "/api/auth/login", `{"username":"admin","password":"ldapadminpass"}`); code != 401 {
		t.Fatalf("directory admin got the local admin account: %d", code)
	}
	if len(dir.Binds) != binds {
		t.Fatalf("directory consulted for a local account: %v", dir.Binds[binds:])
	}
	if code, _ := do(t, ts, "", "POST", "/api/auth/login", `{"username":"admin","password":"adminpass123"}`); code != 200 {
		t.Fatalf("local admin broken: %d", code)
	}
	// A directory identity whose mapped username is a local account's name is refused (H-001).
	s.Cfg.Auth.LDAP.UsernameAttr = "sAMAccountName"
	code, body = do(t, ts, "", "POST", "/api/auth/login", `{"username":"vic@example.com","password":"vicpass"}`)
	if code != 401 || !strings.Contains(body, "already exists") {
		t.Fatalf("username takeover: %d %s", code, body)
	}
	if _, users := do(t, ts, admin, "GET", "/api/users", ""); strings.Contains(users, "uuid-vic") {
		t.Fatal("vic was provisioned")
	}
	s.Cfg.Auth.LDAP.UsernameAttr = ""

	// Role re-sync: ann drops out of Printer Admins and is an operator next time.
	dir.Entries[0].Attrs["memberOf"] = []string{"CN=Staff,OU=Groups,DC=example,DC=com"}
	code, body = do(t, ts, "", "POST", "/api/auth/login", `{"username":"ANN","password":"annpass"}`)
	if code != 200 || !strings.Contains(body, `"username":"ann"`) || !strings.Contains(body, `"role":"operator"`) {
		t.Fatalf("resync: %d %s", code, body)
	}
	// Allowed groups.
	s.Cfg.Auth.LDAP.AllowedGroups = []string{"Printer Admins"}
	if code, body := do(t, ts, "", "POST", "/api/auth/login", `{"username":"ann","password":"annpass"}`); code != 401 || !strings.Contains(body, "allowed") {
		t.Fatalf("allowedGroups: %d %s", code, body)
	}
	s.Cfg.Auth.LDAP.AllowedGroups = nil

	// An empty service-account password variable is a configuration error, reported as unavailable.
	t.Setenv("PM_TEST_LDAP_BIND", "")
	if code, body := do(t, ts, "", "POST", "/api/auth/login", `{"username":"ann","password":"annpass"}`); code != 503 || !strings.Contains(body, "unavailable") {
		t.Fatalf("empty secret: %d %s", code, body)
	}
	t.Setenv("PM_TEST_LDAP_BIND", "svcpass")
	// A dead directory is unavailable too, while the local admin keeps working.
	// (ann is bound to the old URL's issuer, so a free name is used here.)
	s.Cfg.Auth.LDAP.URL = "ldap://127.0.0.1:1"
	if code, _ := do(t, ts, "", "POST", "/api/auth/login", `{"username":"zed","password":"zedpass"}`); code != 503 {
		t.Fatalf("dead directory: %d", code)
	}
	if code, _ := do(t, ts, "", "POST", "/api/auth/login", `{"username":"ann","password":"annpass"}`); code != 401 {
		t.Fatalf("account bound to another issuer offered to the new directory: %d", code)
	}
	if code, _ := do(t, ts, "", "POST", "/api/auth/login", `{"username":"admin","password":"adminpass123"}`); code != 200 {
		t.Fatal("local admin blocked by a dead directory")
	}
	_, audit = do(t, ts, admin, "GET", "/api/audit?limit=50", "")
	if !strings.Contains(audit, `"result":"error"`) || !strings.Contains(audit, `"action":"auth.failed"`) {
		t.Fatalf("errors not recorded: %s", audit)
	}
}

// Direct bind through a DN template (no service account), including that a
// directory user cannot bind as a local account.
func TestLDAPDirectBindLogin(t *testing.T) {
	ts, s := newTestServer(t)
	dir := ldaptest.New(t)
	ldapDirectory(dir)
	s.Cfg.Auth.LDAP = config.LDAP{URL: dir.URL, UserDNTemplate: "cn=%s,ou=people,dc=example,dc=com", BaseDN: "dc=example,dc=com",
		RoleMap: map[string]string{"Printer Admins": "admin"}, DefaultRole: "viewer"}
	code, body := do(t, ts, "", "POST", "/api/auth/login", `{"username":"ann","password":"annpass"}`)
	if code != 200 || !strings.Contains(body, `"role":"admin"`) {
		t.Fatalf("direct bind: %d %s", code, body)
	}
	if dir.Binds[0] != "cn=ann,ou=people,dc=example,dc=com" {
		t.Fatalf("binds %v", dir.Binds)
	}
	if code, _ := do(t, ts, "", "POST", "/api/auth/login", `{"username":"ann","password":"bad"}`); code != 401 {
		t.Fatalf("wrong password: %d", code)
	}
	n := len(dir.Binds)
	if code, _ := do(t, ts, "", "POST", "/api/auth/login", `{"username":"admin","password":"ldapadminpass"}`); code != 401 || len(dir.Binds) != n {
		t.Fatalf("local admin offered to the directory: %d %v", code, dir.Binds[n:])
	}
	// Second sign-in resolves the same account by subject.
	code, body = do(t, ts, "", "POST", "/api/auth/login", `{"username":"ann","password":"annpass"}`)
	if code != 200 || !strings.Contains(body, `"username":"ann"`) {
		t.Fatalf("second: %d %s", code, body)
	}
	sess := login(t, ts, "admin", "adminpass123")
	if _, users := do(t, ts, sess, "GET", "/api/users", ""); strings.Count(users, `"username":"ann"`) != 1 {
		t.Fatalf("duplicate accounts: %s", users)
	}
}
