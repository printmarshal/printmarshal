package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"printmarshal/internal/auth"
	"printmarshal/internal/auth/ldap"
	"printmarshal/internal/auth/oidc"
)

// LDAP / Active Directory behind the login form (docs/auth.md). The
// effective configuration is resolved on every call from the file, the
// environment and the settings an administrator saved (directory.go), so a
// change in the UI is in force for the next sign-in without a restart.

func (s *Server) ldapEnabled(ctx context.Context) bool { return s.directoryState(ctx).Settings.Enabled }

func (s *Server) ldapIssuer(ctx context.Context) string {
	return s.directoryState(ctx).ldapConfig().Issuer()
}

func (s *Server) ldapName(ctx context.Context) string { return s.directoryState(ctx).name() }

// ldapAuthenticate binds as the user and maps groups to a role.
func (s *Server) ldapAuthenticate(ctx context.Context, username, password string) (*externalIdentity, error) {
	st := s.directoryState(ctx)
	cfg := st.ldapConfig()
	if cfg.BindDN != "" && cfg.BindPassword == "" {
		if env := s.Cfg.Auth.LDAP.BindPasswordEnv; env != "" {
			return nil, errors.New("ldap: the service-account password variable " + env + " is empty")
		}
		return nil, errors.New("ldap: a service account is configured but no password has been set")
	}
	id, err := ldap.Authenticate(ctx, cfg, username, password)
	if err != nil {
		if errors.Is(err, ldap.ErrInvalidCredentials) {
			return nil, errInvalidCredentials
		}
		return nil, err
	}
	e := st.Settings
	if len(e.AllowedGroups) > 0 && !id.InGroup(e.AllowedGroups) {
		return nil, &loginError{status: http.StatusUnauthorized, msg: "your directory account is not in a group allowed to sign in", detail: "ldap: not in an allowed group", result: "denied"}
	}
	role := oidc.MapRole(lowerAll(id.Candidates()), lowerKeys(e.RoleMap), e.DefaultRole)
	return &externalIdentity{ExternalIdentity: auth.ExternalIdentity{Issuer: cfg.Issuer(), Subject: id.Subject, Username: id.Username}, Role: role, SyncRole: len(e.RoleMap) > 0}, nil
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
