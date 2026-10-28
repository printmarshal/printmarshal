// Package firmware implements firmware inventory and staged update campaigns
// (spec §8.6) with safeguards.
package firmware

import "printmarshal/internal/fleet"

// Service is the firmware façade.
type Service struct {
	fleet.Deps
}

// New builds the service and registers its job types.
func New(d fleet.Deps) *Service { return &Service{Deps: d} }
