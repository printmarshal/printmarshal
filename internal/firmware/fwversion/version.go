// Package fwversion compares printer firmware version strings and checks a
// firmware package's compatibility with a device. It is a leaf package so the
// firmware service, the standard delivery driver and vendor plugins can share
// one tolerant comparator (spec §8.6 "version/compatibility checks").
package fwversion

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"printmarshal/internal/model"
)

var (
	tokenRe    = regexp.MustCompile(`\d+|[a-z]+`)
	dateRe     = regexp.MustCompile(`^(\d{4})[.\-/]?(\d{2})[.\-/]?(\d{2})$`)
	prefixRe   = regexp.MustCompile(`^(?:version|firmware|ver|rev|fw|v|r)[\s.:_-]*`)
	prerelease = map[string]bool{"alpha": true, "beta": true, "rc": true, "pre": true, "dev": true, "snapshot": true}
)

// Normalize trims, lower-cases and strips common prefixes ("v", "version", "fw").
func Normalize(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	v = prefixRe.ReplaceAllString(v, "")
	return strings.TrimSpace(v)
}

// Compare orders two version strings: -1 when a < b, 0 when equal, 1 when a > b.
//
// It understands numeric-dotted versions ("2.14.7", "3.0", "1.2.3-build4"),
// date-like versions ("20240115", "2024.01.15") and mixed tokens ("2.14.7a").
// Numeric tokens compare numerically, alphabetic tokens lexically; a trailing
// pre-release token (alpha/beta/rc/...) sorts before the bare version and a
// trailing numeric token sorts after it. Missing trailing zeros are equal
// ("3.0" == "3.0.0"). Empty strings sort first.
func Compare(a, b string) int {
	a, b = Normalize(a), Normalize(b)
	if a == b {
		return 0
	}
	if a == "" {
		return -1
	}
	if b == "" {
		return 1
	}
	if da, ok := dateValue(a); ok {
		if db, ok := dateValue(b); ok {
			return cmpInt(da, db)
		}
	}
	ta, tb := tokenRe.FindAllString(a, -1), tokenRe.FindAllString(b, -1)
	n := len(ta)
	if len(tb) > n {
		n = len(tb)
	}
	for i := 0; i < n; i++ {
		switch {
		case i >= len(ta):
			return -tailRank(tb[i:])
		case i >= len(tb):
			return tailRank(ta[i:])
		}
		if c := cmpToken(ta[i], tb[i]); c != 0 {
			return c
		}
	}
	return 0
}

// Equal reports whether two versions denote the same release.
func Equal(a, b string) bool { return Compare(a, b) == 0 }

// Less reports whether a is older than b.
func Less(a, b string) bool { return Compare(a, b) < 0 }

func dateValue(v string) (int64, bool) {
	m := dateRe.FindStringSubmatch(v)
	if m == nil {
		return 0, false
	}
	y, _ := strconv.Atoi(m[1])
	mo, _ := strconv.Atoi(m[2])
	d, _ := strconv.Atoi(m[3])
	if y < 1990 || mo < 1 || mo > 12 || d < 1 || d > 31 {
		return 0, false
	}
	return int64(y)*10000 + int64(mo)*100 + int64(d), true
}

// tailRank says how the remaining tokens of the longer version rank against
// the end of the shorter one: zeros are equal, a pre-release marker is lower,
// anything else is higher.
func tailRank(rest []string) int {
	for _, t := range rest {
		if isNumeric(t) {
			if strings.Trim(t, "0") != "" {
				return 1
			}
			continue
		}
		if prerelease[t] {
			return -1
		}
		return 1
	}
	return 0
}

func cmpToken(a, b string) int {
	na, nb := isNumeric(a), isNumeric(b)
	switch {
	case na && nb:
		ia, _ := strconv.ParseInt(strings.TrimLeft(a, "0"), 10, 64)
		ib, _ := strconv.ParseInt(strings.TrimLeft(b, "0"), 10, 64)
		return cmpInt(ia, ib)
	case na: // number beats letters ("1.2" > "1.b")
		return 1
	case nb:
		return -1
	}
	return strings.Compare(a, b)
}

func isNumeric(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func cmpInt(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// Verdict is the outcome of a compatibility check.
type Verdict struct {
	Compatible bool   // the package may be delivered to the device
	Current    bool   // the device already runs the package version (skip)
	Downgrade  bool   // the device runs a newer version than the package
	Reason     string // human explanation when not deliverable
}

// Check decides whether pkg may be delivered to dev. Vendor and model matching
// is a case-insensitive substring match in either direction so "HP" matches
// "Hewlett-Packard" and "4200" matches "Acme LaserPro 4200". allowDowngrade
// lets a package older than the installed version through (for rollback).
func Check(pkg *model.FirmwarePackage, dev *model.Device, allowDowngrade bool) Verdict {
	if pkg == nil || dev == nil {
		return Verdict{Reason: "no package or device"}
	}
	if pkg.Vendor != "" && !fuzzyMatch(pkg.Vendor, dev.Make) {
		return Verdict{Reason: fmt.Sprintf("package is for vendor %q, device make is %q", pkg.Vendor, orUnknown(dev.Make))}
	}
	if len(pkg.Models) > 0 && !ModelMatches(pkg.Models, dev.Model) {
		return Verdict{Reason: fmt.Sprintf("package models %s do not match device model %q", strings.Join(pkg.Models, ", "), orUnknown(dev.Model))}
	}
	if pkg.MinVersion != "" {
		if dev.Firmware == "" {
			return Verdict{Reason: fmt.Sprintf("package requires installed version %s or newer but the device's version is unknown", pkg.MinVersion)}
		}
		if Less(dev.Firmware, pkg.MinVersion) {
			return Verdict{Reason: fmt.Sprintf("installed version %s is older than the required minimum %s", dev.Firmware, pkg.MinVersion)}
		}
	}
	if dev.Firmware != "" {
		switch c := Compare(dev.Firmware, pkg.Version); {
		case c == 0:
			return Verdict{Compatible: true, Current: true, Reason: fmt.Sprintf("already at version %s", pkg.Version)}
		case c > 0 && !allowDowngrade:
			return Verdict{Downgrade: true, Reason: fmt.Sprintf("installed version %s is newer than package %s (downgrade not allowed)", dev.Firmware, pkg.Version)}
		case c > 0:
			return Verdict{Compatible: true, Downgrade: true}
		}
	}
	return Verdict{Compatible: true}
}

// Applies reports whether pkg targets dev's make and model at all, ignoring
// versions (used to find the latest known package for a device).
func Applies(pkg *model.FirmwarePackage, dev *model.Device) bool {
	if pkg == nil || dev == nil {
		return false
	}
	if pkg.Vendor != "" && !fuzzyMatch(pkg.Vendor, dev.Make) {
		return false
	}
	return len(pkg.Models) == 0 || ModelMatches(pkg.Models, dev.Model)
}

// ModelMatches reports whether any of the package's model substrings occurs in
// the device model (case-insensitive).
func ModelMatches(models []string, deviceModel string) bool {
	dm := strings.ToLower(strings.TrimSpace(deviceModel))
	for _, m := range models {
		m = strings.ToLower(strings.TrimSpace(m))
		if m == "" {
			continue
		}
		if m == "*" || strings.Contains(dm, m) {
			return true
		}
	}
	return false
}

func fuzzyMatch(a, b string) bool {
	a, b = strings.ToLower(strings.TrimSpace(a)), strings.ToLower(strings.TrimSpace(b))
	if a == "" || b == "" {
		return false
	}
	return strings.Contains(a, b) || strings.Contains(b, a)
}

func orUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "unknown"
	}
	return s
}
