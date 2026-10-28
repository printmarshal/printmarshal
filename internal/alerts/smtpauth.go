package alerts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"printmarshal/internal/config"
)

// SMTP authentication beyond PLAIN (audit A08): LOGIN for relays that insist
// on it, and XOAUTH2 for Microsoft 365 / Google Workspace where basic
// authentication is disabled. Tokens are read from an environment variable or
// fetched with the OAuth 2.0 client-credentials grant and cached until shortly
// before they expire.

// loginAuth implements the (non-standard but ubiquitous) LOGIN mechanism.
type loginAuth struct{ username, password string }

func (a *loginAuth) Start(*smtp.ServerInfo) (string, []byte, error) { return "LOGIN", nil, nil }

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	switch strings.ToLower(strings.TrimSpace(string(fromServer))) {
	case "username:":
		return []byte(a.username), nil
	case "password:":
		return []byte(a.password), nil
	}
	return nil, errors.New("smtp login: unexpected server challenge")
}

// xoauth2Auth implements SASL XOAUTH2.
type xoauth2Auth struct{ username, token string }

func (a *xoauth2Auth) Start(*smtp.ServerInfo) (string, []byte, error) {
	return "XOAUTH2", []byte("user=" + a.username + "\x01auth=Bearer " + a.token + "\x01\x01"), nil
}

func (a *xoauth2Auth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		// the server sent a JSON error; an empty line makes it finish the exchange
		return []byte(""), fmt.Errorf("smtp xoauth2 rejected: %s", strings.TrimSpace(string(fromServer)))
	}
	return nil, nil
}

type cachedToken struct {
	value   string
	expires time.Time
}

// tokenSource fetches and caches client-credentials tokens per notifier.
type tokenSource struct {
	mu    sync.Mutex
	cache map[string]cachedToken
	http  *http.Client
}

func newTokenSource(c *http.Client) *tokenSource {
	return &tokenSource{cache: map[string]cachedToken{}, http: c}
}

// Token returns a bearer token for dest: TokenEnv wins, else the client
// credentials grant at TokenURL.
func (t *tokenSource) Token(ctx context.Context, dest config.Notifier) (string, error) {
	if dest.TokenEnv != "" {
		if v := os.Getenv(dest.TokenEnv); v != "" {
			return v, nil
		}
		return "", fmt.Errorf("smtp notifier %q: %s is empty", dest.Name, dest.TokenEnv)
	}
	if dest.TokenURL == "" || dest.ClientID == "" {
		return "", fmt.Errorf("smtp notifier %q: xoauth2 needs tokenEnv, or tokenUrl + clientId + clientSecretEnv", dest.Name)
	}
	t.mu.Lock()
	if c, ok := t.cache[dest.Name]; ok && time.Until(c.expires) > time.Minute {
		t.mu.Unlock()
		return c.value, nil
	}
	t.mu.Unlock()
	form := url.Values{"grant_type": {"client_credentials"}, "client_id": {dest.ClientID}}
	if dest.ClientSecretEnv != "" {
		form.Set("client_secret", os.Getenv(dest.ClientSecretEnv))
	}
	if dest.Scope != "" {
		form.Set("scope", dest.Scope)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dest.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := t.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("oauth token request: %w", err)
	}
	defer resp.Body.Close()
	var body struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("oauth token response: %w", err)
	}
	if resp.StatusCode >= 300 || body.AccessToken == "" {
		return "", fmt.Errorf("oauth token endpoint returned %d: %s %s", resp.StatusCode, body.Error, body.ErrorDesc)
	}
	ttl := time.Duration(body.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	t.mu.Lock()
	t.cache[dest.Name] = cachedToken{value: body.AccessToken, expires: time.Now().Add(ttl)}
	t.mu.Unlock()
	return body.AccessToken, nil
}

// smtpAuth picks the SASL mechanism for a destination.
func (n *Notifier) smtpAuth(ctx context.Context, dest config.Notifier) (smtp.Auth, error) {
	switch strings.ToLower(dest.AuthMethod) {
	case "", "plain":
		return smtp.PlainAuth("", dest.Username, os.Getenv(dest.PasswordEnv), dest.Host), nil
	case "login":
		return &loginAuth{username: dest.Username, password: os.Getenv(dest.PasswordEnv)}, nil
	case "xoauth2", "oauth2":
		tok, err := n.tokens.Token(ctx, dest)
		if err != nil {
			return nil, err
		}
		return &xoauth2Auth{username: dest.Username, token: tok}, nil
	}
	return nil, fmt.Errorf("smtp notifier %q: unknown authMethod %q", dest.Name, dest.AuthMethod)
}

// teamsPayload builds the Workflows (Power Automate) webhook body: an Adaptive
// Card wrapped in a message attachment (audit A09). The retired Office 365
// connector format is kept behind `legacy: true`.
func teamsPayload(dest config.Notifier, subject, body string) map[string]any {
	if dest.Legacy {
		return map[string]any{"@type": "MessageCard", "@context": "https://schema.org/extensions", "title": subject, "text": body, "summary": subject}
	}
	return map[string]any{
		"type": "message",
		"attachments": []map[string]any{{
			"contentType": "application/vnd.microsoft.card.adaptive",
			"contentUrl":  nil,
			"content": map[string]any{
				"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
				"type":    "AdaptiveCard",
				"version": "1.4",
				"msteams": map[string]any{"width": "Full"},
				"body": []map[string]any{
					{"type": "TextBlock", "text": subject, "weight": "Bolder", "size": "Medium", "wrap": true},
					{"type": "TextBlock", "text": body, "wrap": true},
				},
			},
		}},
	}
}
