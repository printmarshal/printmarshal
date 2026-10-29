package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"

	"printmarshal/internal/auth"
	"printmarshal/internal/auth/ldap"
	"printmarshal/internal/auth/oidc"
)

// LDAP / Active Directory behind the login form (docs/auth.md).

func (s *Server) ldapEnabled() bool { return strings.TrimSpace(s.Cfg.Auth.LDAP.URL) != "" }

func (s *Server) ldapIssuer() string { return s.ldapConfig().Issuer() }

func (s *Server) ldapName() string {
	if n := s.Cfg.Auth.LDAP.DisplayName; n != "" {
		return n
	}
	return hostOf(s.Cfg.Auth.LDAP.URL)
}

func (s *Server) ldapConfig() ldap.Config {
	c := s.Cfg.Auth.LDAP
	cfg := ldap.Config{URL: strings.TrimSpace(c.URL), StartTLS: c.StartTLS, CAFile: c.CAFile, InsecureSkipVerify: c.InsecureSkipVerify,
		BindDN: c.BindDN, UserDNTemplate: c.UserDNTemplate, BaseDN: c.BaseDN, UserFilter: c.UserFilter,
		UsernameAttr: c.UsernameAttr, SubjectAttr: c.SubjectAttr, GroupAttr: c.GroupAttr, Timeout: c.Timeout.D()}
	if c.BindPasswordEnv != "" {
		cfg.BindPassword = os.Getenv(c.BindPasswordEnv)
	}
	return cfg
}

// ldapAuthenticate binds as the user and maps groups to a role.
func (s *Server) ldapAuthenticate(ctx context.Context, username, password string) (*externalIdentity, error) {
	c := s.Cfg.Auth.LDAP
	cfg := s.ldapConfig()
	if c.BindDN != "" && c.BindPasswordEnv != "" && cfg.BindPassword == "" {
		return nil, errors.New("ldap: the service-account password variable " + c.BindPasswordEnv + " is empty")
	}
	id, err := ldap.Authenticate(ctx, cfg, username, password)
	if err != nil {
		if errors.Is(err, ldap.ErrInvalidCredentials) {
			return nil, errInvalidCredentials
		}
		return nil, err
	}
	if len(c.AllowedGroups) > 0 && !id.InGroup(c.AllowedGroups) {
		return nil, &loginError{status: http.StatusUnauthorized, msg: "your directory account is not in a group allowed to sign in", detail: "ldap: not in an allowed group", result: "denied"}
	}
	role := oidc.MapRole(lowerAll(id.Candidates()), lowerKeys(c.RoleMap), c.DefaultRole)
	return &externalIdentity{ExternalIdentity: auth.ExternalIdentity{Issuer: cfg.Issuer(), Subject: id.Subject, Username: id.Username}, Role: role, SyncRole: len(c.RoleMap) > 0}, nil
}

// Directory names (DNs, CNs) compare case-insensitively, so the role map is
// looked up that way.
func lowerAll(v []string) []string {
	out := make([]string, len(v))
	for i, s := range v {
		out[i] = strings.ToLower(strings.TrimSpace(s))
	}
	return out
}

func lowerKeys(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[strings.ToLower(strings.TrimSpace(k))] = v
	}
	return out
}

// splitHostPortLoose splits host:port, tolerating a bare host.
func splitHostPortLoose(addr string) (string, string, error) {
	if h, p, err := net.SplitHostPort(addr); err == nil {
		return h, p, nil
	}
	if addr == "" {
		return "", "", errors.New("empty address")
	}
	return addr, "", nil
}
