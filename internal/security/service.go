// Package security implements security policies (spec §8.5): assess,
// remediate, report; certificate and credential management.
package security

import "printmarshal/internal/fleet"

// Service is the security façade.
type Service struct {
	fleet.Deps
}

// New builds the service and registers its job types.
func New(d fleet.Deps) *Service { return &Service{Deps: d} }
