// Package plugins registers vendor plugins with the driver registry. The core
// has no compile-time knowledge of any vendor beyond this list; each plugin
// lives in its own sub-package and implements engine.Driver plus the
// capability interfaces it can serve (docs/adding-a-driver.md).
package plugins

import (
	"log/slog"

	"printmarshal/internal/engine"
)

// RegisterAll registers every built-in vendor plugin.
func RegisterAll(reg *engine.Registry, log *slog.Logger) {
	_ = reg
	_ = log
}
