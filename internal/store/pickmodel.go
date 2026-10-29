package store

import (
	"strings"

	"printmarshal/internal/model"
)

// pickModel chooses the model to store when a device is identified again.
// A new non-empty value normally wins, but a generic print-server name
// (model.IsGenericModel) never replaces a model that is already specific: a
// read that missed the real model must not make the device worse than it was.
func pickModel(newv, old string) string {
	if strings.TrimSpace(newv) == "" {
		return old
	}
	if model.IsGenericModel(newv) && strings.TrimSpace(old) != "" && !model.IsGenericModel(old) {
		return old
	}
	return newv
}
