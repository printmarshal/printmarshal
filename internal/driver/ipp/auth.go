package ipp

// HTTP authentication for the write path (review finding H-016).
//
// Printers protect Set-Printer-Attributes with HTTP authentication (RFC 7235):
// the first request comes back 401 with one or more WWW-Authenticate
// challenges and the client repeats it with an Authorization header. Basic
// (RFC 7617) and Digest (RFC 7616: MD5 and SHA-256, with the -sess variants and
// qop=auth / auth-int) are supported; Digest is preferred when both are
// offered because it never puts the password on the wire.

import (
	"crypto/md5" //nolint:gosec // RFC 7616 still allows MD5 digests; the server picks the algorithm
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"net/url"
	"strings"
)

// ErrAuthRequired is returned when the device demands authentication and no
// admin credential is bound to it.
var ErrAuthRequired = errors.New("the printer requires authentication but no admin credential is bound")

// ErrAuthRejected is returned when the device rejected the bound admin credential.
var ErrAuthRejected = errors.New("the printer rejected the bound admin credential (401)")

// ErrUnsupportedChallenge is returned when the device offers only
// authentication schemes this driver does not implement.
var ErrUnsupportedChallenge = errors.New("the printer requires an unsupported authentication scheme")

// challenge is one parsed WWW-Authenticate challenge.
type challenge struct {
	scheme string            // lower-case: "basic", "digest", ...
	params map[string]string // lower-case keys, unquoted values
}

// parseChallenges parses the WWW-Authenticate header values (RFC 7235 §4.1).
// A single header may list several challenges separated by commas, which is
// ambiguous with the comma-separated auth-params; a token followed by
// whitespace (not "=") starts a new challenge.
func parseChallenges(headers []string) []challenge {
	var out []challenge
	for _, h := range headers {
		for _, tok := range splitAuthTokens(h) {
			tok = strings.TrimSpace(tok)
			if tok == "" {
				continue
			}
			// "Scheme param=value" or a bare "Scheme".
			if i := strings.IndexAny(tok, " \t"); i > 0 && !strings.Contains(tok[:i], "=") {
				out = append(out, challenge{scheme: strings.ToLower(tok[:i]), params: map[string]string{}})
				tok = strings.TrimSpace(tok[i:])
			} else if !strings.Contains(tok, "=") {
				out = append(out, challenge{scheme: strings.ToLower(tok), params: map[string]string{}})
				continue
			}
			if len(out) == 0 {
				continue // params before any scheme: malformed
			}
			k, v, ok := strings.Cut(tok, "=")
			if !ok {
				continue
			}
			k = strings.ToLower(strings.TrimSpace(k))
			v = strings.TrimSpace(v)
			if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
				v = unquoteAuth(v[1 : len(v)-1])
			}
			out[len(out)-1].params[k] = v
		}
	}
	return out
}

// splitAuthTokens splits on commas outside quoted strings.
func splitAuthTokens(s string) []string {
	var out []string
	var b strings.Builder
	inQuote, escaped := false, false
	for _, r := range s {
		switch {
		case escaped:
			b.WriteRune(r)
			escaped = false
		case r == '\\' && inQuote:
			b.WriteRune(r)
			escaped = true
		case r == '"':
			b.WriteRune(r)
			inQuote = !inQuote
		case r == ',' && !inQuote:
			out = append(out, b.String())
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	out = append(out, b.String())
	return out
}

func unquoteAuth(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	escaped := false
	for _, r := range s {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func quoteAuth(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// authorizationFor picks the strongest supported challenge and builds the
// Authorization header value for a request of the given method, target URL and
// entity body. ErrUnsupportedChallenge is returned when none is usable.
func authorizationFor(challenges []challenge, method string, target *url.URL, body []byte, auth Auth) (string, error) {
	var basic *challenge
	for i := range challenges {
		c := &challenges[i]
		switch c.scheme {
		case "digest":
			if _, ok := digestHash(c.params["algorithm"]); ok {
				return digestAuthorization(c, method, target, body, auth)
			}
		case "basic":
			if basic == nil {
				basic = c
			}
		}
	}
	if basic != nil {
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(auth.User+":"+auth.Password)), nil
	}
	return "", ErrUnsupportedChallenge
}

// digestHash maps an RFC 7616 algorithm token to its hash and whether it is
// a -sess variant. An empty token means MD5 (RFC 7616 §3.3).
func digestHash(algorithm string) (func() hash.Hash, bool) {
	switch strings.ToUpper(strings.TrimSpace(algorithm)) {
	case "", "MD5", "MD5-SESS":
		return md5.New, true
	case "SHA-256", "SHA-256-SESS":
		return sha256.New, true
	}
	return nil, false
}

// digestAuthorization computes the Digest response (RFC 7616 §3.4).
func digestAuthorization(c *challenge, method string, target *url.URL, body []byte, auth Auth) (string, error) {
	newHash, _ := digestHash(c.params["algorithm"])
	h := func(parts ...string) string {
		hh := newHash()
		hh.Write([]byte(strings.Join(parts, ":")))
		return hex.EncodeToString(hh.Sum(nil))
	}
	realm, nonce, opaque := c.params["realm"], c.params["nonce"], c.params["opaque"]
	if nonce == "" {
		return "", fmt.Errorf("%w: digest challenge without a nonce", ErrUnsupportedChallenge)
	}
	algorithm := strings.TrimSpace(c.params["algorithm"])
	sess := strings.HasSuffix(strings.ToUpper(algorithm), "-SESS")

	// qop: prefer auth; accept auth-int; a challenge without qop is RFC 2069 style.
	qop := ""
	for _, q := range strings.Split(c.params["qop"], ",") {
		switch strings.TrimSpace(q) {
		case "auth":
			qop = "auth"
		case "auth-int":
			if qop == "" {
				qop = "auth-int"
			}
		}
	}
	if c.params["qop"] != "" && qop == "" {
		return "", fmt.Errorf("%w: digest qop %q", ErrUnsupportedChallenge, c.params["qop"])
	}

	cnonceRaw := make([]byte, 16)
	if _, err := rand.Read(cnonceRaw); err != nil {
		return "", err
	}
	cnonce := hex.EncodeToString(cnonceRaw)
	const nc = "00000001"

	uri := target.EscapedPath()
	if uri == "" {
		uri = "/"
	}
	if target.RawQuery != "" {
		uri += "?" + target.RawQuery
	}

	a1 := h(auth.User, realm, auth.Password)
	if sess {
		a1 = h(a1, nonce, cnonce)
	}
	var a2 string
	if qop == "auth-int" {
		a2 = h(method, uri, h(string(body)))
	} else {
		a2 = h(method, uri)
	}
	var response string
	if qop == "" {
		response = h(a1, nonce, a2)
	} else {
		response = h(a1, nonce, nc, cnonce, qop, a2)
	}

	var b strings.Builder
	b.WriteString("Digest username=" + quoteAuth(auth.User))
	b.WriteString(", realm=" + quoteAuth(realm))
	b.WriteString(", nonce=" + quoteAuth(nonce))
	b.WriteString(", uri=" + quoteAuth(uri))
	b.WriteString(", response=" + quoteAuth(response))
	if algorithm != "" {
		b.WriteString(", algorithm=" + algorithm)
	}
	if qop != "" {
		b.WriteString(", qop=" + qop + ", nc=" + nc + ", cnonce=" + quoteAuth(cnonce))
	}
	if opaque != "" {
		b.WriteString(", opaque=" + quoteAuth(opaque))
	}
	return b.String(), nil
}
