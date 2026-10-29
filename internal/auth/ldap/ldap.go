// Package ldap authenticates a username/password against an LDAP or Active
// Directory server (docs/auth.md). It supports a read-only service account
// that finds the user's entry (search, then bind as the user) and a direct
// bind through a DN / UPN template when no service account is available. It
// returns a stable subject (objectGUID, entryUUID or the DN) so the local
// account can be bound to the directory identity, never to the login name.
package ldap

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	goldap "github.com/go-ldap/ldap/v3"
)

// Config is what one authentication needs; the API layer fills it from
// config.LDAP and the environment.
type Config struct {
	URL                string
	StartTLS           bool
	CAFile             string
	InsecureSkipVerify bool
	BindDN             string
	BindPassword       string
	UserDNTemplate     string
	BaseDN             string
	UserFilter         string
	UsernameAttr       string
	SubjectAttr        string
	GroupAttr          string
	Timeout            time.Duration
}

// DefaultUserFilter matches AD and POSIX directories out of the box.
const DefaultUserFilter = "(|(sAMAccountName=%s)(uid=%s)(userPrincipalName=%s)(mail=%s))"

// DefaultTimeout bounds the whole exchange when Config.Timeout is unset.
const DefaultTimeout = 10 * time.Second

// Identity is the authenticated directory user.
type Identity struct {
	DN       string
	Username string
	Subject  string
	// Groups are the raw group values (DNs with memberOf). Candidates adds
	// each group's first RDN value (its CN) so role maps can use either.
	Groups []string
}

// ErrInvalidCredentials covers an unknown user, a wrong password and an
// ambiguous search alike: callers must not tell them apart.
var ErrInvalidCredentials = errors.New("invalid credentials")

// Issuer is the identity-provider identifier accounts are bound to.
func (c Config) Issuer() string { return "ldap:" + strings.TrimSpace(c.URL) }

// Candidates returns every value a role map or allow-list may name for the
// user's groups: the full DN and, for each DN, the value of its first RDN
// (typically the CN).
func (id *Identity) Candidates() []string {
	out := make([]string, 0, 2*len(id.Groups))
	for _, g := range id.Groups {
		out = append(out, g)
		if cn := firstRDN(g); cn != "" && cn != g {
			out = append(out, cn)
		}
	}
	return out
}

// InGroup reports whether any allowed value names one of the user's groups
// (DN or CN, case-insensitive, as directories compare them).
func (id *Identity) InGroup(allowed []string) bool {
	for _, a := range allowed {
		for _, c := range id.Candidates() {
			if strings.EqualFold(strings.TrimSpace(a), c) {
				return true
			}
		}
	}
	return false
}

// Authenticate checks the password and returns the user's identity.
func Authenticate(ctx context.Context, cfg Config, username, password string) (*Identity, error) {
	username = strings.TrimSpace(username)
	// An empty password would be an unauthenticated bind, which many servers
	// accept: never send one.
	if username == "" || password == "" {
		return nil, ErrInvalidCredentials
	}
	if cfg.BindDN == "" && cfg.UserDNTemplate == "" {
		return nil, errors.New("ldap: either bindDN or userDNTemplate must be configured")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	if dl, ok := ctx.Deadline(); ok {
		if rem := time.Until(dl); rem < timeout {
			timeout = rem
		}
	}
	tlsCfg, err := tlsConfig(cfg)
	if err != nil {
		return nil, err
	}
	conn, err := goldap.DialURL(cfg.URL, goldap.DialWithDialer(&net.Dialer{Timeout: timeout}), goldap.DialWithTLSConfig(tlsCfg))
	if err != nil {
		return nil, fmt.Errorf("ldap: connect: %w", err)
	}
	defer conn.Close()
	conn.SetTimeout(timeout)
	if cfg.StartTLS && !strings.HasPrefix(strings.ToLower(cfg.URL), "ldaps://") {
		if err := conn.StartTLS(tlsCfg); err != nil {
			return nil, fmt.Errorf("ldap: starttls: %w", err)
		}
	}

	filter := expandFilter(cfg.UserFilter, username)
	attrs := requestAttrs(cfg)
	var entry *goldap.Entry
	if cfg.BindDN != "" {
		if err := conn.Bind(cfg.BindDN, cfg.BindPassword); err != nil {
			return nil, fmt.Errorf("ldap: service account bind: %w", err)
		}
		entry, err = findOne(conn, cfg.BaseDN, goldap.ScopeWholeSubtree, filter, attrs)
		if err != nil {
			return nil, err
		}
		if err := conn.Bind(entry.DN, password); err != nil {
			if goldap.IsErrorWithCode(err, goldap.LDAPResultInvalidCredentials) {
				return nil, ErrInvalidCredentials
			}
			return nil, fmt.Errorf("ldap: user bind: %w", err)
		}
	} else {
		bindName := strings.ReplaceAll(cfg.UserDNTemplate, "%s", goldap.EscapeDN(username))
		if err := conn.Bind(bindName, password); err != nil {
			if goldap.IsErrorWithCode(err, goldap.LDAPResultInvalidCredentials) {
				return nil, ErrInvalidCredentials
			}
			return nil, fmt.Errorf("ldap: user bind: %w", err)
		}
		// Read the entry as the user. With a base DN the configured filter
		// finds it; otherwise the bind name must itself be the entry DN.
		if cfg.BaseDN != "" {
			entry, err = findOne(conn, cfg.BaseDN, goldap.ScopeWholeSubtree, filter, attrs)
		} else {
			entry, err = findOne(conn, bindName, goldap.ScopeBaseObject, "(objectClass=*)", attrs)
		}
		if err != nil {
			return nil, err
		}
	}

	id := &Identity{DN: entry.DN}
	id.Username = usernameOf(cfg, entry, username, cfg.BindDN == "")
	id.Subject = subjectOf(cfg, entry)
	id.Groups = entry.GetEqualFoldAttributeValues(groupAttr(cfg))
	if id.Username == "" || id.Subject == "" {
		return nil, errors.New("ldap: the entry has no usable username or subject attribute")
	}
	return id, nil
}

func tlsConfig(cfg Config) (*tls.Config, error) {
	u, err := url.Parse(cfg.URL)
	if err != nil || u.Host == "" || (u.Scheme != "ldap" && u.Scheme != "ldaps") {
		return nil, fmt.Errorf("ldap: %q is not an ldap:// or ldaps:// URL", cfg.URL)
	}
	tc := &tls.Config{ServerName: u.Hostname(), MinVersion: tls.VersionTLS12, InsecureSkipVerify: cfg.InsecureSkipVerify}
	if cfg.CAFile != "" {
		pem, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("ldap: caFile: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("ldap: caFile %s holds no certificates", cfg.CAFile)
		}
		tc.RootCAs = pool
	}
	return tc, nil
}

// expandFilter substitutes the escaped login name for every %s.
func expandFilter(filter, username string) string {
	if strings.TrimSpace(filter) == "" {
		filter = DefaultUserFilter
	}
	return strings.ReplaceAll(filter, "%s", goldap.EscapeFilter(username))
}

// filterAttrs lists the attributes an equality filter compares, in order,
// so the "whatever matched" username rule can be applied.
func filterAttrs(filter string) []string {
	if strings.TrimSpace(filter) == "" {
		filter = DefaultUserFilter
	}
	var out []string
	for _, part := range strings.Split(filter, "(") {
		name, _, ok := strings.Cut(part, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" || strings.ContainsAny(name, "&|!)") {
			continue
		}
		name = strings.TrimRight(name, "~><:")
		out = append(out, name)
	}
	return out
}

func groupAttr(cfg Config) string {
	if cfg.GroupAttr != "" {
		return cfg.GroupAttr
	}
	return "memberOf"
}

func requestAttrs(cfg Config) []string {
	set := map[string]bool{}
	var out []string
	add := func(a string) {
		if a == "" || set[strings.ToLower(a)] {
			return
		}
		set[strings.ToLower(a)] = true
		out = append(out, a)
	}
	add(cfg.UsernameAttr)
	for _, a := range filterAttrs(cfg.UserFilter) {
		add(a)
	}
	add("sAMAccountName")
	add("uid")
	add(cfg.SubjectAttr)
	add("objectGUID")
	add("entryUUID")
	add(groupAttr(cfg))
	return out
}

// findOne runs the search and insists on exactly one entry; none or several
// are reported as invalid credentials so an attacker learns nothing.
func findOne(conn *goldap.Conn, base string, scope int, filter string, attrs []string) (*goldap.Entry, error) {
	res, err := conn.Search(&goldap.SearchRequest{BaseDN: base, Scope: scope, DerefAliases: goldap.NeverDerefAliases, SizeLimit: 2, Filter: filter, Attributes: attrs})
	if err != nil && !goldap.IsErrorWithCode(err, goldap.LDAPResultSizeLimitExceeded) {
		if goldap.IsErrorWithCode(err, goldap.LDAPResultNoSuchObject) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("ldap: search: %w", err)
	}
	if res == nil || len(res.Entries) != 1 {
		return nil, ErrInvalidCredentials
	}
	return res.Entries[0], nil
}

func usernameOf(cfg Config, e *goldap.Entry, typed string, directBind bool) string {
	if cfg.UsernameAttr != "" {
		return strings.TrimSpace(e.GetEqualFoldAttributeValue(cfg.UsernameAttr))
	}
	for _, a := range filterAttrs(cfg.UserFilter) {
		for _, v := range e.GetEqualFoldAttributeValues(a) {
			if strings.EqualFold(strings.TrimSpace(v), typed) {
				return strings.TrimSpace(v)
			}
		}
	}
	for _, a := range []string{"sAMAccountName", "uid"} {
		if v := strings.TrimSpace(e.GetEqualFoldAttributeValue(a)); v != "" {
			return v
		}
	}
	if directBind {
		return typed
	}
	return e.DN
}

func subjectOf(cfg Config, e *goldap.Entry) string {
	if cfg.SubjectAttr != "" {
		return attrString(cfg.SubjectAttr, e.GetEqualFoldRawAttributeValue(cfg.SubjectAttr))
	}
	if v := attrString("objectGUID", e.GetEqualFoldRawAttributeValue("objectGUID")); v != "" {
		return v
	}
	if v := attrString("entryUUID", e.GetEqualFoldRawAttributeValue("entryUUID")); v != "" {
		return v
	}
	return e.DN
}

// attrString renders an attribute value: a binary objectGUID becomes the
// canonical GUID string; other binary values are hex-encoded.
func attrString(name string, raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	if strings.EqualFold(name, "objectGUID") && len(raw) == 16 {
		return GUIDString(raw)
	}
	if !utf8.Valid(raw) {
		return fmt.Sprintf("%x", raw)
	}
	return strings.TrimSpace(string(raw))
}

// GUIDString renders a 16-byte Active Directory objectGUID the way AD tools
// print it (the first three fields are little-endian).
func GUIDString(b []byte) string {
	if len(b) != 16 {
		return ""
	}
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		b[3], b[2], b[1], b[0], b[5], b[4], b[7], b[6], b[8], b[9], b[10], b[11], b[12], b[13], b[14], b[15])
}

// firstRDN returns the value of the first RDN of a DN ("Admins" for
// "CN=Admins,OU=Groups,DC=example,DC=com"), or "" when dn is not a DN.
func firstRDN(dn string) string {
	parsed, err := goldap.ParseDN(dn)
	if err != nil || len(parsed.RDNs) == 0 || len(parsed.RDNs[0].Attributes) == 0 {
		return ""
	}
	return parsed.RDNs[0].Attributes[0].Value
}
