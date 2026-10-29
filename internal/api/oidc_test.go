package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"printmarshal/internal/auth/oidc/oidctest"
	"printmarshal/internal/config"
)

// noRedirect returns a client that stops at the first redirect so the test
// can inspect Location and cookies.
func noRedirect() *http.Client {
	return &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// startFlow runs /start and returns the pm_oidc cookie plus the state and
// nonce the browser would carry to the IdP.
func startFlow(t *testing.T, ts *httptest.Server, is *oidctest.Issuer) (*http.Cookie, string, string) {
	t.Helper()
	resp, err := noRedirect().Get(ts.URL + "/api/auth/oidc/start")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 302 {
		t.Fatalf("start status %d", resp.StatusCode)
	}
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || !strings.HasPrefix(loc.String(), is.URL+"/authorize?") {
		t.Fatalf("start location %q", resp.Header.Get("Location"))
	}
	var flow *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "pm_oidc" {
			flow = c
		}
	}
	if flow == nil || !flow.HttpOnly || flow.SameSite != http.SameSiteLaxMode || flow.Path != "/api/auth/oidc/" {
		t.Fatalf("flow cookie %+v", flow)
	}
	q := loc.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("redirect_uri") != ts.URL+"/api/auth/oidc/callback" {
		t.Fatalf("auth request %v", q)
	}
	return flow, q.Get("state"), q.Get("nonce")
}

func callback(t *testing.T, ts *httptest.Server, flow *http.Cookie, code, state string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("GET", ts.URL+"/api/auth/oidc/callback?code="+url.QueryEscape(code)+"&state="+url.QueryEscape(state), nil)
	if flow != nil {
		req.AddCookie(flow)
	}
	resp, err := noRedirect().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

func sessionCookie(resp *http.Response) string {
	for _, c := range resp.Cookies() {
		if c.Name == "pm_session" && c.Value != "" {
			return c.Value
		}
	}
	return ""
}

// A full OIDC sign-in against a fake issuer: discovery, PKCE authorization
// request, callback with code exchange, ID-token verification, first-login
// provisioning with a mapped role, role re-sync, and the failure paths.
func TestOIDCSignIn(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	is := oidctest.New(t, "pm-client")
	t.Setenv("PM_TEST_OIDC_SECRET", "s3cret")
	s.Cfg.Auth.OIDC = config.OIDC{Issuer: is.URL, ClientID: "pm-client", ClientSecretEnv: "PM_TEST_OIDC_SECRET", RedirectURL: ts.URL + "/api/auth/oidc/callback",
		DisplayName: "Corp SSO", RoleClaim: "groups", RoleMap: map[string]string{"printer-admins": "admin", "helpdesk": "operator"}, AllowedDomains: []string{"example.com"}}

	code, body := do(t, ts, "", "GET", "/api/auth/providers", "")
	if code != 200 || !strings.Contains(body, `"name":"Corp SSO"`) || !strings.Contains(body, `"enabled":true`) {
		t.Fatalf("providers: %d %s", code, body)
	}

	// Happy path: ann is in printer-admins and gets an admin session.
	flow, state, nonce := startFlow(t, ts, is)
	is.Nonce, is.WantCode = nonce, "code-1"
	is.Claims["email"], is.Claims["preferred_username"], is.Claims["groups"] = "ann@example.com", "ann", []string{"printer-admins"}
	resp := callback(t, ts, flow, "code-1", state)
	if resp.StatusCode != 303 || resp.Header.Get("Location") != "/" {
		t.Fatalf("callback: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	sess := sessionCookie(resp)
	if sess == "" {
		t.Fatal("no session cookie")
	}
	if is.LastTokenForm.Get("client_secret") != "s3cret" {
		t.Fatalf("client secret not read from env: %v", is.LastTokenForm)
	}
	code, body = do(t, ts, sess, "GET", "/api/auth/me", "")
	if code != 200 || !strings.Contains(body, `"username":"ann"`) || !strings.Contains(body, `"role":"admin"`) {
		t.Fatalf("me: %d %s", code, body)
	}
	_, audit := do(t, ts, sess, "GET", "/api/audit?limit=20", "")
	if !strings.Contains(audit, `"action":"user.create"`) || !strings.Contains(audit, `"action":"auth.login"`) || !strings.Contains(audit, "Corp SSO") {
		t.Fatalf("audit: %s", audit)
	}

	// Replaying the callback (same state, cookie gone) fails; the flow cookie is single use.
	if resp := callback(t, ts, nil, "code-1", state); resp.StatusCode != 303 || !strings.Contains(resp.Header.Get("Location"), "oidc_error=") || sessionCookie(resp) != "" {
		t.Fatalf("replay: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	// A forged state with a valid cookie fails.
	flow, _, nonce = startFlow(t, ts, is)
	is.Nonce = nonce
	if resp := callback(t, ts, flow, "code-1", "forged"); sessionCookie(resp) != "" || !strings.Contains(resp.Header.Get("Location"), "oidc_error=") {
		t.Fatal("forged state accepted")
	}

	// Second sign-in: ann left printer-admins, so her role is re-synced down.
	flow, state, nonce = startFlow(t, ts, is)
	is.Nonce, is.Claims["groups"] = nonce, []string{"staff"}
	resp = callback(t, ts, flow, "code-1", state)
	sess2 := sessionCookie(resp)
	if sess2 == "" {
		t.Fatalf("second sign-in: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	if code, body := do(t, ts, sess2, "GET", "/api/auth/me", ""); code != 200 || !strings.Contains(body, `"role":"viewer"`) {
		t.Fatalf("role not synced: %d %s", code, body)
	}
	if code, _ := do(t, ts, sess2, "GET", "/api/users", ""); code != 403 {
		t.Fatalf("demoted user still admin: %d", code)
	}

	// H-001: an IdP identity whose preferred_username is "admin" must not be
	// signed in as the local administrator (accounts bind by issuer+subject).
	flow, state, nonce = startFlow(t, ts, is)
	is.Nonce, is.Claims["sub"], is.Claims["email"], is.Claims["preferred_username"], is.Claims["groups"] = nonce, "subject-2", "mallory@example.com", "admin", []string{"staff"}
	resp = callback(t, ts, flow, "code-1", state)
	if sessionCookie(resp) != "" || !strings.Contains(resp.Header.Get("Location"), "already+exists") {
		t.Fatalf("local admin adopted by an IdP identity: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	// The same subject renamed to "ann" at the provider must not take over ann's account either.
	flow, state, nonce = startFlow(t, ts, is)
	is.Nonce, is.Claims["preferred_username"] = nonce, "ann"
	resp = callback(t, ts, flow, "code-1", state)
	if sessionCookie(resp) != "" || !strings.Contains(resp.Header.Get("Location"), "already+exists") {
		t.Fatalf("ann's account transferred to another subject: %q", resp.Header.Get("Location"))
	}
	// With a free username the new subject gets its own account, and ann's is untouched.
	flow, state, nonce = startFlow(t, ts, is)
	is.Nonce, is.Claims["preferred_username"] = nonce, "mallory"
	resp = callback(t, ts, flow, "code-1", state)
	if sess3 := sessionCookie(resp); sess3 == "" {
		t.Fatalf("fresh subject refused: %q", resp.Header.Get("Location"))
	} else if code, body := do(t, ts, sess3, "GET", "/api/auth/me", ""); code != 200 || !strings.Contains(body, `"username":"mallory"`) || !strings.Contains(body, `"role":"viewer"`) {
		t.Fatalf("me: %d %s", code, body)
	}
	if _, body := do(t, ts, admin, "GET", "/api/users", ""); !strings.Contains(body, `"username":"ann","role":"viewer"`) || !strings.Contains(body, `"externalSubject":"subject-1"`) {
		t.Fatalf("users: %s", body)
	}
	// An explicitly unverified email cannot pass the domain boundary.
	flow, state, nonce = startFlow(t, ts, is)
	is.Nonce, is.Claims["sub"], is.Claims["email_verified"], is.Claims["preferred_username"] = nonce, "subject-3", false, "carol"
	resp = callback(t, ts, flow, "code-1", state)
	if sessionCookie(resp) != "" || !strings.Contains(resp.Header.Get("Location"), "verified") {
		t.Fatalf("unverified email accepted: %q", resp.Header.Get("Location"))
	}
	delete(is.Claims, "email_verified")
	is.Claims["sub"] = "subject-1"

	// Domain restriction.
	flow, state, nonce = startFlow(t, ts, is)
	is.Nonce, is.Claims["email"], is.Claims["preferred_username"] = nonce, "bob@elsewhere.net", "bob"
	resp = callback(t, ts, flow, "code-1", state)
	if sessionCookie(resp) != "" || !strings.Contains(resp.Header.Get("Location"), "domain") {
		t.Fatalf("foreign domain accepted: %q", resp.Header.Get("Location"))
	}
	if code, body := do(t, ts, admin, "GET", "/api/users", ""); strings.Contains(body, "bob") {
		t.Fatalf("bob was provisioned: %d %s", code, body)
	}

	// The IdP refusing the request lands on the login page with a message, never a session.
	flow, state, _ = startFlow(t, ts, is)
	req, _ := http.NewRequest("GET", ts.URL+"/api/auth/oidc/callback?error=access_denied&state="+state, nil)
	req.AddCookie(flow)
	r2, err := noRedirect().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	r2.Body.Close()
	if sessionCookie(r2) != "" || !strings.Contains(r2.Header.Get("Location"), "access_denied") {
		t.Fatalf("idp error: %q", r2.Header.Get("Location"))
	}
	_, audit = do(t, ts, admin, "GET", "/api/audit?limit=50", "")
	if !strings.Contains(audit, `"action":"auth.failed"`) {
		t.Fatalf("failures not recorded: %s", audit)
	}

	// Not configured: providers says so and /start reports it.
	s.Cfg.Auth.OIDC = config.OIDC{}
	if code, body := do(t, ts, "", "GET", "/api/auth/providers", ""); code != 200 || !strings.Contains(body, `"enabled":false`) {
		t.Fatalf("providers off: %d %s", code, body)
	}
}
