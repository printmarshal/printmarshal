package model

import (
	"strings"
	"testing"
)

// The picker shows these to people who may not know the terms, so every setting
// has one, and each reads as a sentence.
func TestEverySettingHasHelp(t *testing.T) {
	for _, d := range Catalog {
		h := SettingHelp(d.Key)
		if h == "" {
			t.Errorf("%s has no explanation", d.Key)
			continue
		}
		if !strings.HasSuffix(h, ".") || len(h) < 20 || len(h) > 160 {
			t.Errorf("%s: explanation should be one short sentence: %q", d.Key, h)
		}
	}
	if SettingHelp("no.such.setting") != "" {
		t.Error("an unknown key has no explanation")
	}
}
