package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	csaml "github.com/crewjam/saml"

	"printmarshal/internal/auth/saml/samltest"
	"printmarshal/internal/config"
)

// samlStart runs /start and returns the flow cookie and the IdP redirect.
func samlStart(t *testing.T, ts *httptest.Server, idp *samltest.IdP) (*http.Cookie, string) {
	t.Helper()
	resp, err := noRedirect().Get(ts.URL + "/api/auth/saml/start")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	if resp.StatusCode != 302 || !strings.HasPrefix(loc, idp.URL+"/sso?SAMLRequest=") {
		t.Fatalf("start: %d %q", resp.StatusCode, loc)
	}
	if strings.Contains(loc, "RelayState=") {
		t.Fatalf("relay state carried: %q", loc)
	}
	var flow *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "pm_saml" {
			flow = c
		}
	}
	if flow == nil || !flow.HttpOnly || flow.Path != "/api/auth/saml/" || flow.Value == "" {
		t.Fatalf("flow cookie %+v", flow)
	}
	return flow, loc
}

// samlPost delivers the IdP response to the ACS like the browser would.
func samlPost(t *testing.T, ts *httptest.Server, flow *http.Cookie, r samltest.Response) *http.Response {
	t.Helper()
	form := url.Values{"SAMLResponse": {r.SAMLResponse}, "RelayState": {r.RelayState}}
	req, _ := http.NewRequest("POST", ts.URL+"/api/auth/saml/acs", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
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

func ssoError(resp *http.Response) string {
	u, _ := url.Parse(resp.Header.Get("Location"))
	if u == nil {
		return ""
	}
	return u.Query().Get("sso_error")
}

// A full SP-initiated SAML sign-in against the in-process IdP, then the
// failure paths: tampered, expired and replayed assertions, wrong or
// missing request binding, unstable NameIDs, role mapping and re-sync, and
// the local administrator staying unreachable.
func TestSAMLSignIn(t *testing.T) {
	ts, s := newTestServer(t)
	admin := login(t, ts, "admin", "adminpass123")
	idp := samltest.New(t)
	s.Cfg.DataDir = t.TempDir()
	s.Cfg.Auth.SAML = config.SAML{IDPMetadataURL: idp.URL + "/metadata", BaseURL: ts.URL, DisplayName: "Corp SAML",
		RoleAttr: "eduPersonAffiliation", RoleMap: map[string]string{"printer-admins": "admin", "helpdesk": "operator"}}
	idp.Session.NameID, idp.Session.UserName, idp.Session.Groups = "ann", "ann", []string{"printer-admins"}

	if code, body := do(t, ts, "", "GET", "/api/auth/providers", ""); code != 200 || !strings.Contains(body, `{"kind":"saml","name":"Corp SAML","method":"redirect","start":"/api/auth/saml/start"}`) {
		t.Fatalf("providers: %d %s", code, body)
	}
	// SP metadata is served and the signing pair was generated under the data dir.
	code, meta := do(t, ts, "", "GET", "/api/auth/saml/metadata", "")
	if code != 200 || !strings.Contains(meta, `entityID="`+ts.URL+`/api/auth/saml/metadata"`) || !strings.Contains(meta, ts.URL+"/api/auth/saml/acs") {
		t.Fatalf("metadata: %d %s", code, meta)
	}
	if st, err := os.Stat(filepath.Join(s.Cfg.DataDir, "saml-sp.key")); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("sp key: %v %v", err, st)
	}

	// Happy path.
	flow, loc := samlStart(t, ts, idp)
	resp := samlPost(t, ts, flow, idp.Respond(t, loc))
	if resp.StatusCode != 303 || resp.Header.Get("Location") != "/" {
		t.Fatalf("acs: %d %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	sess := sessionCookie(resp)
	if sess == "" {
		t.Fatal("no session")
	}
	if code, body := do(t, ts, sess, "GET", "/api/auth/me", ""); code != 200 || !strings.Contains(body, `"username":"ann"`) || !strings.Contains(body, `"role":"admin"`) {
		t.Fatalf("me: %d %s", code, body)
	}
	if _, users := do(t, ts, admin, "GET", "/api/users", ""); !strings.Contains(users, `"externalIssuer":"saml:`+idp.EntityID()+`"`) || !strings.Contains(users, `"externalSubject":"ann"`) {
		t.Fatalf("users: %s", users)
	}
	_, audit := do(t, ts, admin, "GET", "/api/audit?limit=20", "")
	if !strings.Contains(audit, `"source":"saml"`) || !strings.Contains(audit, "signed in via Corp SAML") {
		t.Fatalf("audit: %s", audit)
	}

	// An IdP that encrypts the assertion to the SP certificate works too.
	idp.Encrypt = true
	flow, loc = samlStart(t, ts, idp)
	if resp := samlPost(t, ts, flow, idp.Respond(t, loc)); sessionCookie(resp) == "" {
		t.Fatalf("encrypted assertion: %q", resp.Header.Get("Location"))
	}
	idp.Encrypt = false

	// Replay: the same response (with the same cookie) is refused.
	flow, loc = samlStart(t, ts, idp)
	r := idp.Respond(t, loc)
	if resp := samlPost(t, ts, flow, r); sessionCookie(resp) == "" {
		t.Fatalf("first use failed: %q", resp.Header.Get("Location"))
	}
	if resp := samlPost(t, ts, flow, r); sessionCookie(resp) != "" || !strings.Contains(ssoError(resp), "already used") {
		t.Fatalf("replay accepted: %q", resp.Header.Get("Location"))
	}
	// Tampered: the signed NameID is changed after signing.
	flow, loc = samlStart(t, ts, idp)
	tampered, err := samltest.Tamper(idp.Respond(t, loc), ">ann<", ">admin<")
	if err != nil {
		t.Fatal(err)
	}
	if resp := samlPost(t, ts, flow, tampered); sessionCookie(resp) != "" || !strings.Contains(ssoError(resp), "could not be verified") {
		t.Fatalf("tampered accepted: %q", resp.Header.Get("Location"))
	}
	// Expired.
	idp.Mutate = func(a *csaml.Assertion) {
		past := time.Now().Add(-time.Hour)
		a.Conditions.NotOnOrAfter = past
		a.Subject.SubjectConfirmations[0].SubjectConfirmationData.NotOnOrAfter = past
	}
	flow, loc = samlStart(t, ts, idp)
	if resp := samlPost(t, ts, flow, idp.Respond(t, loc)); sessionCookie(resp) != "" || !strings.Contains(ssoError(resp), "could not be verified") {
		t.Fatalf("expired accepted: %q", resp.Header.Get("Location"))
	}
	idp.Mutate = nil
	// No flow cookie (unsolicited / IdP-initiated) and a cookie from another flow.
	_, loc = samlStart(t, ts, idp)
	if resp := samlPost(t, ts, nil, idp.Respond(t, loc)); sessionCookie(resp) != "" {
		t.Fatalf("unsolicited accepted: %q", resp.Header.Get("Location"))
	}
	other, _ := samlStart(t, ts, idp)
	_, loc = samlStart(t, ts, idp)
	if resp := samlPost(t, ts, other, idp.Respond(t, loc)); sessionCookie(resp) != "" {
		t.Fatalf("InResponseTo mismatch accepted: %q", resp.Header.Get("Location"))
	}
	// Garbage.
	flow, _ = samlStart(t, ts, idp)
	if resp := samlPost(t, ts, flow, samltest.Response{SAMLResponse: "bm90IHhtbA=="}); sessionCookie(resp) != "" {
		t.Fatal("garbage accepted")
	}

	// Role re-sync: ann moves to helpdesk.
	idp.Session.Groups = []string{"helpdesk"}
	flow, loc = samlStart(t, ts, idp)
	resp = samlPost(t, ts, flow, idp.Respond(t, loc))
	if s2 := sessionCookie(resp); s2 == "" {
		t.Fatalf("resync sign-in: %q", resp.Header.Get("Location"))
	} else if code, body := do(t, ts, s2, "GET", "/api/auth/me", ""); code != 200 || !strings.Contains(body, `"role":"operator"`) {
		t.Fatalf("role not synced: %d %s", code, body)
	}
	// Allowed groups.
	s.Cfg.Auth.SAML.AllowedGroups = []string{"printer-admins"}
	flow, loc = samlStart(t, ts, idp)
	if resp := samlPost(t, ts, flow, idp.Respond(t, loc)); sessionCookie(resp) != "" || !strings.Contains(ssoError(resp), "allowed") {
		t.Fatalf("allowedGroups: %q", resp.Header.Get("Location"))
	}
	s.Cfg.Auth.SAML.AllowedGroups = nil

	// A transient NameID cannot be the subject unless subjectAttr names an attribute.
	idp.Session.NameIDFormat, idp.Session.NameID, idp.Session.SubjectID = string(csaml.TransientNameIDFormat), "_tmp123", "stable-ann"
	flow, loc = samlStart(t, ts, idp)
	if resp := samlPost(t, ts, flow, idp.Respond(t, loc)); sessionCookie(resp) != "" || !strings.Contains(ssoError(resp), "stable identifier") {
		t.Fatalf("transient NameID accepted: %q", resp.Header.Get("Location"))
	}
	// With subjectAttr and usernameAttr the attributes are used instead. The
	// subject "stable-ann" is a new identity, so it cannot reuse ann's name
	// (that would be a takeover); as ann2 it gets its own account.
	s.Cfg.Auth.SAML.SubjectAttr = "urn:oasis:names:tc:SAML:attribute:subject-id"
	s.Cfg.Auth.SAML.UsernameAttr = "uid"
	flow, loc = samlStart(t, ts, idp)
	if resp := samlPost(t, ts, flow, idp.Respond(t, loc)); sessionCookie(resp) != "" || !strings.Contains(ssoError(resp), "already exists") {
		t.Fatalf("new subject took ann's account: %q", resp.Header.Get("Location"))
	}
	idp.Session.UserName = "ann2"
	flow, loc = samlStart(t, ts, idp)
	resp = samlPost(t, ts, flow, idp.Respond(t, loc))
	if s3 := sessionCookie(resp); s3 == "" {
		t.Fatalf("attribute subject: %q", resp.Header.Get("Location"))
	} else if code, body := do(t, ts, s3, "GET", "/api/auth/me", ""); code != 200 || !strings.Contains(body, `"username":"ann2"`) {
		t.Fatalf("me: %d %s", code, body)
	}
	if _, users := do(t, ts, admin, "GET", "/api/users", ""); !strings.Contains(users, `"externalSubject":"stable-ann"`) || strings.Count(users, `"username":"ann"`) != 1 {
		t.Fatalf("accounts: %s", users)
	}
	s.Cfg.Auth.SAML.SubjectAttr, s.Cfg.Auth.SAML.UsernameAttr = "", ""
	idp.Session.NameIDFormat, idp.Session.SubjectID, idp.Session.UserName = string(csaml.PersistentNameIDFormat), "", "ann"

	// H-001: an IdP subject named "admin" cannot take the local administrator.
	idp.Session.NameID, idp.Session.UserName = "admin", "admin"
	flow, loc = samlStart(t, ts, idp)
	if resp := samlPost(t, ts, flow, idp.Respond(t, loc)); sessionCookie(resp) != "" || !strings.Contains(ssoError(resp), "already exists") {
		t.Fatalf("local admin adopted: %q", resp.Header.Get("Location"))
	}
	_, audit = do(t, ts, admin, "GET", "/api/audit?limit=80", "")
	if strings.Count(audit, `"action":"auth.failed"`) < 6 {
		t.Fatalf("failures not recorded: %s", audit)
	}

	// Off: providers no longer lists it and /start reports it.
	s.Cfg.Auth.SAML = config.SAML{}
	if _, body := do(t, ts, "", "GET", "/api/auth/providers", ""); strings.Contains(body, `"kind":"saml"`) {
		t.Fatalf("providers: %s", body)
	}
	if resp, _ := noRedirect().Get(ts.URL + "/api/auth/saml/start"); resp == nil || !strings.Contains(resp.Header.Get("Location"), "sso_error=") {
		t.Fatal("start while off")
	}
}

// IdP metadata from a file and a configured SP key pair.
func TestSAMLMetadataFileAndKeys(t *testing.T) {
	ts, s := newTestServer(t)
	idp := samltest.New(t)
	dir := t.TempDir()
	resp, err := http.Get(idp.URL + "/metadata")
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	metaFile := filepath.Join(dir, "idp.xml")
	if err := os.WriteFile(metaFile, b, 0o600); err != nil {
		t.Fatal(err)
	}
	// Generate a pair once through the package, then point the config at the files.
	s.Cfg.DataDir = dir
	s.Cfg.Auth.SAML = config.SAML{IDPMetadataFile: metaFile, BaseURL: ts.URL}
	if code, _ := do(t, ts, "", "GET", "/api/auth/saml/metadata", ""); code != 200 {
		t.Fatalf("metadata: %d", code)
	}
	s.saml = nil
	s.Cfg.DataDir = t.TempDir()
	s.Cfg.Auth.SAML.SPCertFile, s.Cfg.Auth.SAML.SPKeyFile = filepath.Join(dir, "saml-sp.crt"), filepath.Join(dir, "saml-sp.key")
	if code, body := do(t, ts, "", "GET", "/api/auth/saml/metadata", ""); code != 200 || !strings.Contains(body, "X509Certificate") {
		t.Fatalf("metadata with configured keys: %d %s", code, body)
	}
	if _, err := os.Stat(filepath.Join(s.Cfg.DataDir, "saml-sp.key")); err == nil {
		t.Fatal("a pair was generated although one was configured")
	}
	flow, loc := samlStart(t, ts, idp)
	if r := samlPost(t, ts, flow, idp.Respond(t, loc)); sessionCookie(r) == "" {
		t.Fatalf("sign-in: %q", r.Header.Get("Location"))
	}
}
