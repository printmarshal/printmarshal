package api

import (
	"context"
	"errors"
	"os"
	"strings"

	"printmarshal/internal/auth"
	"printmarshal/internal/auth/oidc"
	"printmarshal/internal/auth/radius"
)

// RADIUS (RFC 2865 PAP) behind the login form (docs/auth.md). Tried after
// LDAP when both are configured.

func (s *Server) radiusEnabled() bool {
	for _, srv := range s.Cfg.Auth.RADIUS.Servers {
		if strings.TrimSpace(srv) != "" {
			return true
		}
	}
	return false
}

func (s *Server) radiusIssuer() string { return s.radiusConfig().Issuer() }

func (s *Server) radiusName() string {
	if n := s.Cfg.Auth.RADIUS.DisplayName; n != "" {
		return n
	}
	for _, srv := range s.Cfg.Auth.RADIUS.Servers {
		if strings.TrimSpace(srv) != "" {
			return hostOf(strings.TrimSpace(srv))
		}
	}
	return "RADIUS"
}

func (s *Server) radiusConfig() radius.Config {
	c := s.Cfg.Auth.RADIUS
	var servers []string
	for _, srv := range c.Servers {
		if srv = strings.TrimSpace(srv); srv != "" {
			servers = append(servers, srv)
		}
	}
	cfg := radius.Config{Servers: servers, Timeout: c.Timeout.D(), Retries: c.Retries, NASIdentifier: c.NASIdentifier}
	if c.SecretEnv != "" {
		cfg.Secret = os.Getenv(c.SecretEnv)
	}
	return cfg
}

// radiusAuthenticate sends the Access-Request and maps the reply attribute
// (Filter-Id or Class) to a role.
func (s *Server) radiusAuthenticate(ctx context.Context, username, password string) (*externalIdentity, error) {
	c := s.Cfg.Auth.RADIUS
	cfg := s.radiusConfig()
	if cfg.Secret == "" {
		if c.SecretEnv == "" {
			return nil, errors.New("radius: secretEnv is not configured")
		}
		return nil, errors.New("radius: the shared-secret variable " + c.SecretEnv + " is empty")
	}
	res, err := radius.Authenticate(ctx, cfg, username, password)
	if err != nil {
		if errors.Is(err, radius.ErrRejected) {
			return nil, errInvalidCredentials
		}
		return nil, err
	}
	role := oidc.MapRole(res.Values(c.RoleAttribute), c.RoleMap, c.DefaultRole)
	return &externalIdentity{ExternalIdentity: auth.ExternalIdentity{Issuer: cfg.Issuer(), Subject: radius.Subject(username), Username: strings.TrimSpace(username)}, Role: role, SyncRole: len(c.RoleMap) > 0}, nil
}
