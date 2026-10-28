package alerts

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"printmarshal/internal/config"
)

func TestTeamsPayloadIsAdaptiveCardByDefault(t *testing.T) {
	p := teamsPayload(config.Notifier{}, "Subject", "Body")
	b, _ := json.Marshal(p)
	if !strings.Contains(string(b), `"AdaptiveCard"`) || !strings.Contains(string(b), `"Subject"`) {
		t.Fatalf("workflows payload = %s", b)
	}
	if l := teamsPayload(config.Notifier{Legacy: true}, "S", "B"); l["@type"] != "MessageCard" {
		t.Errorf("legacy payload = %v", l)
	}
}

func TestClientCredentialsTokenIsCached(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("client_id") != "app" || r.Form.Get("client_secret") != "s3" || r.Form.Get("scope") != "mail" {
			t.Errorf("form = %v", r.Form)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok-1", "expires_in": 3600})
	}))
	defer srv.Close()
	t.Setenv("PM_TEST_SECRET", "s3")
	ts := newTokenSource(srv.Client())
	dest := config.Notifier{Name: "m365", TokenURL: srv.URL, ClientID: "app", ClientSecretEnv: "PM_TEST_SECRET", Scope: "mail"}
	for i := 0; i < 3; i++ {
		tok, err := ts.Token(context.Background(), dest)
		if err != nil || tok != "tok-1" {
			t.Fatalf("token = %q, %v", tok, err)
		}
	}
	if calls != 1 {
		t.Errorf("token endpoint called %d times; want 1 (cached)", calls)
	}
	t.Setenv("PM_TEST_TOKEN", "env-tok")
	if tok, _ := ts.Token(context.Background(), config.Notifier{Name: "x", TokenEnv: "PM_TEST_TOKEN"}); tok != "env-tok" {
		t.Errorf("tokenEnv = %q", tok)
	}
	n := &Notifier{tokens: ts}
	if _, err := n.smtpAuth(context.Background(), config.Notifier{AuthMethod: "bogus"}); err == nil {
		t.Error("unknown auth method must fail")
	}
	a, err := n.smtpAuth(context.Background(), config.Notifier{AuthMethod: "xoauth2", Username: "svc@example.com", TokenEnv: "PM_TEST_TOKEN"})
	if err != nil {
		t.Fatal(err)
	}
	mech, initial, _ := a.Start(nil)
	if mech != "XOAUTH2" || string(initial) != "user=svc@example.com\x01auth=Bearer env-tok\x01\x01" {
		t.Errorf("xoauth2 initial = %q %q", mech, initial)
	}
	la, _ := n.smtpAuth(context.Background(), config.Notifier{AuthMethod: "login", Username: "u"})
	if resp, _ := la.Next([]byte("Username:"), true); string(resp) != "u" {
		t.Errorf("login username = %q", resp)
	}
}
