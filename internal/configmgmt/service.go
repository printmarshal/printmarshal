// Package configmgmt implements configuration management (spec §8.4):
// normalised config reads, profiles, drift reports and gated writes.
package configmgmt

import "printmarshal/internal/fleet"

// Service is the configuration-management façade.
type Service struct {
	fleet.Deps
}

// New builds the service and registers its job types.
func New(d fleet.Deps) *Service { return &Service{Deps: d} }
