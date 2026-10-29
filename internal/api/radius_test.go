package api

import (
	"strings"
	"testing"
	"time"

	"printmarshal/internal/auth/ldap/ldaptest"
	"printmarshal/internal/auth/radius/radiustest"
	"printmarshal/internal/config"
)

// Login-form sign-in against the in-process RADIUS server, role mapping
// from Filter-Id / Class, and the ordering with LDAP.
func TestRADIUSLogin(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	rs := radiustest.New(t, "s3cret")
	rs.Users["bob"], rs.Users["Bob"] = "bobpass", "bobpass" // the fake compares names exactly, like many servers
	rs.Users["admin"] = "radiusadmin"
	rs.Reply["bob"] = []radiustest.Attr{{Type: radiustest.AttrFilterID, Value: "printer-admins"}, {Type: radiustest.AttrClass, Value: "ops"}}
	rs.Reply["Bob"] = rs.Reply["bob"]
	t.Setenv("PM_TEST_RADIUS_SECRET", "s3cret")
	s.Cfg.Auth.RADIUS = config.RADIUS{Servers: []string{rs.Addr}, SecretEnv: "PM_TEST_RADIUS_SECRET", Timeout: config.Duration(500 * time.Millisecond), Retries: 1,
		RoleMap: map[string]string{"printer-admins": "admin", "ops": "operator"}, DisplayName: "Corp RADIUS"}

	if code, body := do(t, ts, "", "GET", "/api/auth/providers", ""); code != 200 || !strings.Contains(body, `{"kind":"radius","name":"Corp RADIUS","method":"password"}`) {
		t.Fatalf("providers: %d %s", code, body)
	}
	code, body := do(t, ts, "", "POST", "/api/auth/login", `{"username":"bob","password":"bobpass"}`)
	if code != 200 || !strings.Contains(body, `"username":"bob"`) || !strings.Contains(body, `"role":"admin"`) {
		t.Fatalf("radius login: %d %s", code, body)
	}
	if _, users := do(t, ts, admin, "GET", "/api/users", ""); !strings.Contains(users, `"externalIssuer":"radius:`+rs.Addr+`"`) || !strings.Contains(users, `"externalSubject":"bob"`) {
		t.Fatalf("users: %s", users)
	}
	// Class instead of Filter-Id; a case variant of the name resolves to the same account.
	s.Cfg.Auth.RADIUS.RoleAttribute = "Class"
	code, body = do(t, ts, "", "POST", "/api/auth/login", `{"username":"Bob","password":"bobpass"}`)
	if code != 200 || !strings.Contains(body, `"username":"bob"`) || !strings.Contains(body, `"role":"operator"`) {
		t.Fatalf("class role: %d %s", code, body)
	}
	if _, users := do(t, ts, admin, "GET", "/api/users", ""); strings.Count(users, `"externalSubject":"bob"`) != 1 {
		t.Fatalf("case variants made two accounts: %s", users)
	}
	c1, b1 := do(t, ts, "", "POST", "/api/auth/login", `{"username":"bob","password":"nope"}`)
	c2, b2 := do(t, ts, "", "POST", "/api/auth/login", `{"username":"zed","password":"bobpass"}`)
	if c1 != 401 || c2 != 401 || b1 != b2 {
		t.Fatalf("failures differ: %s / %s", b1, b2)
	}
	// The local admin is never sent to RADIUS.
	n := rs.Requests
	if code, _ := do(t, ts, "", "POST", "/api/auth/login", `{"username":"admin","password":"radiusadmin"}`); code != 401 || rs.Requests != n {
		t.Fatalf("local admin offered to radius: %d (%d requests)", code, rs.Requests-n)
	}
	// Missing secret → unavailable.
	t.Setenv("PM_TEST_RADIUS_SECRET", "")
	if code, _ := do(t, ts, "", "POST", "/api/auth/login", `{"username":"bob","password":"bobpass"}`); code != 503 {
		t.Fatalf("empty secret: %d", code)
	}
	t.Setenv("PM_TEST_RADIUS_SECRET", "s3cret")
	_, audit := do(t, ts, admin, "GET", "/api/audit?limit=30", "")
	if !strings.Contains(audit, `"source":"radius"`) || !strings.Contains(audit, "signed in via Corp RADIUS") {
		t.Fatalf("audit: %s", audit)
	}
}

// With LDAP and RADIUS both on, a free name goes to LDAP first and falls
// back to RADIUS; a bound account only ever talks to its own provider.
func TestLDAPThenRADIUS(t *testing.T) {
	ts, s := newTestServer(t)
	dir := ldaptest.New(t)
	ldapDirectory(dir)
	rs := radiustest.New(t, "s3cret")
	rs.Users["bob"] = "bobpass"
	rs.Users["ann"] = "annradius"
	t.Setenv("PM_TEST_LDAP_BIND", "svcpass")
	t.Setenv("PM_TEST_RADIUS_SECRET", "s3cret")
	s.Cfg.Auth.LDAP = config.LDAP{URL: dir.URL, BindDN: "cn=svc,ou=service,dc=example,dc=com", BindPasswordEnv: "PM_TEST_LDAP_BIND", BaseDN: "dc=example,dc=com"}
	s.Cfg.Auth.RADIUS = config.RADIUS{Servers: []string{rs.Addr}, SecretEnv: "PM_TEST_RADIUS_SECRET", Timeout: config.Duration(500 * time.Millisecond), Retries: 0}

	// bob is unknown to the directory and accepted by RADIUS.
	if code, body := do(t, ts, "", "POST", "/api/auth/login", `{"username":"bob","password":"bobpass"}`); code != 200 || !strings.Contains(body, `"username":"bob"`) {
		t.Fatalf("fallback: %d %s", code, body)
	}
	if rs.Requests != 1 || len(dir.Searches) != 1 {
		t.Fatalf("order: radius=%d ldap searches=%d", rs.Requests, len(dir.Searches))
	}
	// ann is an LDAP account; her RADIUS password must not work and RADIUS is not asked.
	if code, _ := do(t, ts, "", "POST", "/api/auth/login", `{"username":"ann","password":"annpass"}`); code != 200 {
		t.Fatalf("ann via ldap: %d", code)
	}
	n := rs.Requests
	if code, _ := do(t, ts, "", "POST", "/api/auth/login", `{"username":"ann","password":"annradius"}`); code != 401 || rs.Requests != n {
		t.Fatalf("bound account offered to radius: %d (%d requests)", code, rs.Requests-n)
	}
	// Both providers reject → one generic 401.
	if code, body := do(t, ts, "", "POST", "/api/auth/login", `{"username":"zed","password":"x"}`); code != 401 || !strings.Contains(body, "invalid credentials") {
		t.Fatalf("both reject: %d %s", code, body)
	}
	if code, body := do(t, ts, "", "GET", "/api/auth/providers", ""); code != 200 || !strings.Contains(body, `"kind":"ldap"`) || !strings.Contains(body, `"kind":"radius"`) {
		t.Fatalf("providers: %s", body)
	}
}
