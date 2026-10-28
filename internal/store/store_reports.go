package store

import (
	"context"
	"time"

	"printmarshal/internal/model"
)

// ReportRange selects telemetry, alerts and stock events for a report.
// From is inclusive, To exclusive. DeviceIDs nil means every device; an
// empty non-nil slice matches nothing.
type ReportRange struct {
	From      time.Time
	To        time.Time
	DeviceIDs []string
}

// DailyAgg is one metric aggregated per device, label and UTC day.
type DailyAgg struct {
	DeviceID string
	Label    string
	Day      string // YYYY-MM-DD (UTC)
	First    float64
	Last     float64
	FirstAt  time.Time
	LastAt   time.Time
	Min      float64
	Max      float64
	Avg      float64
	Count    int
}

// LabelValue is the last sample of a metric per device and label.
type LabelValue struct {
	DeviceID string
	Label    string
	Value    float64
	At       time.Time
}

// OfflineStat summarises reachability samples (status.online) per device.
type OfflineStat struct {
	DeviceID       string
	Polls          int
	Failed         int     // polls that found the device unreachable
	OfflineMinutes float64 // time spent between an unreachable poll and the next poll (or the range end)
	Incidents      int     // reachable → unreachable transitions (a leading unreachable poll counts)
}

// AlertDayCount is the number of alerts raised per day, severity and source.
type AlertDayCount struct {
	Day      string
	Severity string
	Source   string
	Count    int
}

// AlertDeviceStat aggregates alerts raised in a range per device. Minutes are
// sums so callers can compute weighted means across devices.
type AlertDeviceStat struct {
	DeviceID        string
	Count           int
	Critical        int
	Warning         int
	Acked           int
	Cleared         int
	AckMinutesSum   float64
	ClearMinutesSum float64
}

// StockConsumption is the stock movement of one consumable in a range
// (applied events only).
type StockConsumption struct {
	ConsumableID string
	Name         string
	Unit         string
	UnitsOut     int
	UnitsIn      int
	Events       int
}

// ReportStore is the aggregate read surface for the reports area. It is
// defined here (own file) and implemented on *SQLite in sqlite_reports.go.
// It is deliberately not part of Store yet: callers reach it through a type
// assertion until the embed line lands in store.go. Keep it additive.
type ReportStore interface {
	// DailyTelemetry aggregates metric per device, label and UTC day within r.
	DailyTelemetry(ctx context.Context, metric string, r ReportRange) ([]DailyAgg, error)
	// LastBefore returns the last sample of metric per device and label strictly before t.
	LastBefore(ctx context.Context, metric string, before time.Time, deviceIDs []string) ([]LabelValue, error)
	// OfflineStats derives reachability statistics from status.online samples in r.
	OfflineStats(ctx context.Context, r ReportRange) ([]OfflineStat, error)
	// AlertsByDay counts alerts raised in r per day, severity and source.
	AlertsByDay(ctx context.Context, r ReportRange) ([]AlertDayCount, error)
	// AlertsByDevice aggregates alerts raised in r per device.
	AlertsByDevice(ctx context.Context, r ReportRange) ([]AlertDeviceStat, error)
	// ReplacementEvents lists replacement stock events recorded in r, newest first.
	ReplacementEvents(ctx context.Context, r ReportRange) ([]*model.StockEvent, error)
	// StockConsumption sums applied stock movements per consumable between from and to.
	StockConsumption(ctx context.Context, from, to time.Time) ([]StockConsumption, error)
}
