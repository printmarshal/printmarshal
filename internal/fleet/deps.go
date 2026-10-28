package fleet

import (
	"log/slog"

	"printmarshal/internal/config"
	"printmarshal/internal/engine"
	"printmarshal/internal/jobs"
	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// Deps is the dependency bundle handed to feature services (configuration,
// security, firmware, …). Services register their job types on Jobs and
// publish live updates through Events.
type Deps struct {
	Store  store.Store
	Engine *engine.Engine
	Jobs   *jobs.Runner
	Fleet  *Service
	Cfg    *config.Config
	Log    *slog.Logger
	Events func(model.Event)
}

// DepsOf builds a Deps from a fleet service.
func (s *Service) DepsOf() Deps {
	return Deps{Store: s.Store, Engine: s.Engine, Jobs: s.Jobs, Fleet: s, Cfg: s.Cfg, Log: s.Log, Events: s.Events}
}
