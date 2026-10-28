package firmware

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"printmarshal/internal/model"
)

var weekdays = map[string]time.Weekday{
	"sun": time.Sunday, "mon": time.Monday, "tue": time.Tuesday, "wed": time.Wednesday,
	"thu": time.Thursday, "fri": time.Friday, "sat": time.Saturday,
}

func parseDay(s string) (time.Weekday, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) < 3 {
		return 0, false
	}
	d, ok := weekdays[s[:3]]
	return d, ok
}

// parseClock parses "HH:MM" into minutes since midnight.
func parseClock(s string) (int, error) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("time %q must be HH:MM", s)
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, fmt.Errorf("time %q must be HH:MM", s)
	}
	return h*60 + m, nil
}

// ValidateWindow checks a maintenance window's syntax.
func ValidateWindow(w *model.MaintenanceWindow) error {
	if w == nil {
		return nil
	}
	for _, d := range w.Days {
		if _, ok := parseDay(d); !ok {
			return invalidf("window day %q is not mon..sun", d)
		}
	}
	if (w.Start == "") != (w.End == "") {
		return invalidf("window start and end must both be set")
	}
	if w.Start != "" {
		if _, err := parseClock(w.Start); err != nil {
			return invalidf("window start: %v", err)
		}
		if _, err := parseClock(w.End); err != nil {
			return invalidf("window end: %v", err)
		}
	}
	if w.TZ != "" {
		if _, err := time.LoadLocation(w.TZ); err != nil {
			return invalidf("window tz %q: %v", w.TZ, err)
		}
	}
	return nil
}

// InWindow reports whether now falls inside the maintenance window. A nil or
// empty window is always open. A window whose end is at or before its start
// spans midnight; the day check then applies to the day the window opened.
func InWindow(w *model.MaintenanceWindow, now time.Time) bool {
	if w == nil || (len(w.Days) == 0 && w.Start == "" && w.End == "") {
		return true
	}
	loc := time.Local
	if w.TZ != "" {
		if l, err := time.LoadLocation(w.TZ); err == nil {
			loc = l
		}
	}
	t := now.In(loc)
	dayOK := func(d time.Weekday) bool {
		if len(w.Days) == 0 {
			return true
		}
		for _, s := range w.Days {
			if wd, ok := parseDay(s); ok && wd == d {
				return true
			}
		}
		return false
	}
	if w.Start == "" || w.End == "" {
		return dayOK(t.Weekday())
	}
	start, err1 := parseClock(w.Start)
	end, err2 := parseClock(w.End)
	if err1 != nil || err2 != nil {
		return false
	}
	minutes := t.Hour()*60 + t.Minute()
	if end > start {
		return minutes >= start && minutes < end && dayOK(t.Weekday())
	}
	// Overnight window.
	switch {
	case minutes >= start:
		return dayOK(t.Weekday())
	case minutes < end:
		return dayOK((t.Weekday() + 6) % 7)
	}
	return false
}
