package store

import (
	"context"
	"strings"
	"time"

	"printmarshal/internal/model"
)

// Aggregate queries for the reports area (ReportStore) on PostgreSQL. Days are
// UTC: `at` is TIMESTAMPTZ, so the day is to_char(at AT TIME ZONE 'UTC',
// 'YYYY-MM-DD'). Where the SQLite code uses the bare-column MIN/MAX rule the
// PostgreSQL code uses DISTINCT ON, and julianday arithmetic becomes
// EXTRACT(EPOCH FROM …).

const pgDay = `to_char(at AT TIME ZONE 'UTC','YYYY-MM-DD')`

func rangeWherePG(col string, r ReportRange, metric string) ([]string, []any) {
	where := []string{col + ">=?", col + "<?"}
	args := []any{pgt(r.From), pgt(r.To)}
	if metric != "" {
		where = append([]string{"metric=?"}, where...)
		args = append([]any{metric}, args...)
	}
	deviceClause("device_id", r.DeviceIDs, &where, &args)
	return where, args
}

func (s *Postgres) DailyTelemetry(ctx context.Context, metric string, r ReportRange) ([]DailyAgg, error) {
	where, args := rangeWherePG("at", r, metric)
	w := strings.Join(where, " AND ")
	byKey := map[string]*DailyAgg{}
	var order []string
	key := func(dev, label, day string) string { return dev + "\x00" + label + "\x00" + day }
	get := func(dev, label, day string) *DailyAgg {
		k := key(dev, label, day)
		a, ok := byKey[k]
		if !ok {
			a = &DailyAgg{DeviceID: dev, Label: label, Day: day}
			byKey[k] = a
			order = append(order, k)
		}
		return a
	}
	// Totals.
	rows, err := pgQuery(ctx, s.db, `SELECT device_id,label,`+pgDay+` AS day,MIN(value),MAX(value),AVG(value),COUNT(*) FROM telemetry WHERE `+w+` GROUP BY 1,2,3 ORDER BY 1,2,3`, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var dev, label, day string
		var mn, mx, av float64
		var n int
		if err := rows.Scan(&dev, &label, &day, &mn, &mx, &av, &n); err != nil {
			rows.Close()
			return nil, err
		}
		a := get(dev, label, day)
		a.Min, a.Max, a.Avg, a.Count = mn, mx, av, n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Last and first sample of each day (DISTINCT ON picks one row per group).
	for _, q := range []struct {
		dir  string
		last bool
	}{{"DESC", true}, {"ASC", false}} {
		rows, err := pgQuery(ctx, s.db, `SELECT DISTINCT ON (device_id,label,`+pgDay+`) device_id,label,`+pgDay+` AS day,value,at FROM telemetry WHERE `+w+
			` ORDER BY device_id,label,`+pgDay+`,at `+q.dir, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var dev, label, day string
			var v float64
			var at time.Time
			if err := rows.Scan(&dev, &label, &day, &v, &at); err != nil {
				rows.Close()
				return nil, err
			}
			a := get(dev, label, day)
			if q.last {
				a.Last, a.LastAt = v, at.UTC()
			} else {
				a.First, a.FirstAt = v, at.UTC()
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	out := make([]DailyAgg, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	return out, nil
}

func (s *Postgres) LastBefore(ctx context.Context, metric string, before time.Time, deviceIDs []string) ([]LabelValue, error) {
	where := []string{"metric=?", "at<?"}
	args := []any{metric, pgt(before)}
	deviceClause("device_id", deviceIDs, &where, &args)
	rows, err := pgQuery(ctx, s.db, `SELECT DISTINCT ON (device_id,label) device_id,label,value,at FROM telemetry WHERE `+strings.Join(where, " AND ")+` ORDER BY device_id,label,at DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LabelValue{}
	for rows.Next() {
		var lv LabelValue
		var at time.Time
		if err := rows.Scan(&lv.DeviceID, &lv.Label, &lv.Value, &at); err != nil {
			return nil, err
		}
		lv.At = at.UTC()
		out = append(out, lv)
	}
	return out, rows.Err()
}

func (s *Postgres) OfflineStats(ctx context.Context, r ReportRange) ([]OfflineStat, error) {
	where, args := rangeWherePG("at", r, "status.online")
	// The gap after an unreachable poll lasts until the next poll, or the range
	// end (clamped to now) when it is the last sample.
	end := r.To
	if now := time.Now().UTC(); end.After(now) {
		end = now
	}
	q := `SELECT device_id,
	  COUNT(*), COUNT(*) FILTER (WHERE value=0),
	  COALESCE(SUM(CASE WHEN value=0 THEN GREATEST(0, EXTRACT(EPOCH FROM (COALESCE(next_at,?::timestamptz)-at))/60) ELSE 0 END),0)::float8,
	  COUNT(*) FILTER (WHERE value=0 AND (prev IS NULL OR prev<>0))
	FROM (SELECT device_id,value,at,
	        LEAD(at) OVER (PARTITION BY device_id ORDER BY at) AS next_at,
	        LAG(value) OVER (PARTITION BY device_id ORDER BY at) AS prev
	      FROM telemetry WHERE ` + strings.Join(where, " AND ") + `) t
	GROUP BY device_id ORDER BY device_id`
	rows, err := pgQuery(ctx, s.db, q, append([]any{pgt(end)}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []OfflineStat{}
	for rows.Next() {
		var st OfflineStat
		if err := rows.Scan(&st.DeviceID, &st.Polls, &st.Failed, &st.OfflineMinutes, &st.Incidents); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (s *Postgres) AlertsByDay(ctx context.Context, r ReportRange) ([]AlertDayCount, error) {
	where, args := rangeWherePG("raised_at", r, "")
	rows, err := pgQuery(ctx, s.db, `SELECT to_char(raised_at AT TIME ZONE 'UTC','YYYY-MM-DD') AS day,severity,source,COUNT(*) FROM alerts WHERE `+strings.Join(where, " AND ")+` GROUP BY 1,2,3 ORDER BY 1,2,3`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AlertDayCount{}
	for rows.Next() {
		var c AlertDayCount
		if err := rows.Scan(&c.Day, &c.Severity, &c.Source, &c.Count); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Postgres) AlertsByDevice(ctx context.Context, r ReportRange) ([]AlertDeviceStat, error) {
	where, args := rangeWherePG("raised_at", r, "")
	q := `SELECT device_id, COUNT(*), COUNT(*) FILTER (WHERE severity='critical'), COUNT(*) FILTER (WHERE severity='warning'),
	  COUNT(*) FILTER (WHERE acked_at IS NOT NULL), COUNT(*) FILTER (WHERE cleared_at IS NOT NULL),
	  COALESCE(SUM(CASE WHEN acked_at IS NOT NULL THEN GREATEST(0, EXTRACT(EPOCH FROM (acked_at-raised_at))/60) ELSE 0 END),0)::float8,
	  COALESCE(SUM(CASE WHEN cleared_at IS NOT NULL THEN GREATEST(0, EXTRACT(EPOCH FROM (cleared_at-raised_at))/60) ELSE 0 END),0)::float8
	FROM alerts WHERE ` + strings.Join(where, " AND ") + ` GROUP BY device_id ORDER BY 2 DESC`
	rows, err := pgQuery(ctx, s.db, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AlertDeviceStat{}
	for rows.Next() {
		var st AlertDeviceStat
		if err := rows.Scan(&st.DeviceID, &st.Count, &st.Critical, &st.Warning, &st.Acked, &st.Cleared, &st.AckMinutesSum, &st.ClearMinutesSum); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (s *Postgres) ReplacementEvents(ctx context.Context, r ReportRange) ([]*model.StockEvent, error) {
	where, args := rangeWherePG("at", r, "")
	where = append([]string{"kind=?"}, where...)
	args = append([]any{model.StockEventReplacement}, args...)
	rows, err := pgQuery(ctx, s.db, `SELECT `+stockEventCols+` FROM consumable_stock_events WHERE `+strings.Join(where, " AND ")+` ORDER BY at DESC, id DESC LIMIT 5000`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.StockEvent{}
	for rows.Next() {
		e, err := scanStockEventPG(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Postgres) StockConsumption(ctx context.Context, from, to time.Time) ([]StockConsumption, error) {
	q := `SELECT e.consumable_id, COALESCE(c.name,''), COALESCE(c.unit,''),
	  COALESCE(SUM(CASE WHEN e.delta<0 THEN -e.delta ELSE 0 END),0), COALESCE(SUM(CASE WHEN e.delta>0 THEN e.delta ELSE 0 END),0), COUNT(*)
	FROM consumable_stock_events e LEFT JOIN consumables c ON c.id=e.consumable_id
	WHERE e.status=? AND e.kind<>? AND e.at>=? AND e.at<? GROUP BY e.consumable_id, c.name, c.unit ORDER BY 4 DESC, 2`
	rows, err := pgQuery(ctx, s.db, q, model.StockEventApplied, model.StockEventSet, pgt(from), pgt(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StockConsumption{}
	for rows.Next() {
		var c StockConsumption
		if err := rows.Scan(&c.ConsumableID, &c.Name, &c.Unit, &c.UnitsOut, &c.UnitsIn, &c.Events); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

var _ ReportStore = (*Postgres)(nil)
