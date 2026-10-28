package store

import (
	"context"
	"strings"
	"time"

	"printmarshal/internal/model"
)

// Aggregate queries for the reports area (ReportStore). Days are UTC: `at` is
// stored as RFC 3339 UTC, so substr(at,1,10) is the day. Queries that need the
// value belonging to the newest/oldest row use SQLite's documented bare-column
// rule: with exactly one MIN()/MAX() aggregate, bare columns come from that row.

// deviceClause appends an IN (...) filter for ids (nil = no filter).
func deviceClause(col string, ids []string, where *[]string, args *[]any) {
	if ids == nil {
		return
	}
	if len(ids) == 0 {
		*where = append(*where, "0")
		return
	}
	*where = append(*where, col+" IN ("+placeholders(len(ids))+")")
	*args = append(*args, toAny(ids)...)
}

func rangeWhere(col string, r ReportRange, metric string) ([]string, []any) {
	where := []string{col + ">=?", col + "<?"}
	args := []any{ts(r.From), ts(r.To)}
	if metric != "" {
		where = append([]string{"metric=?"}, where...)
		args = append([]any{metric}, args...)
	}
	deviceClause("device_id", r.DeviceIDs, &where, &args)
	return where, args
}

func (s *SQLite) DailyTelemetry(ctx context.Context, metric string, r ReportRange) ([]DailyAgg, error) {
	where, args := rangeWhere("at", r, metric)
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
	rows, err := s.db.QueryContext(ctx, `SELECT device_id,label,substr(at,1,10) AS day,MIN(value),MAX(value),AVG(value),COUNT(*) FROM telemetry WHERE `+w+` GROUP BY 1,2,3 ORDER BY 1,2,3`, args...)
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
	// Last and first sample of each day (bare-column rule, one MAX/MIN per query).
	for _, q := range []struct {
		agg  string
		last bool
	}{{"MAX", true}, {"MIN", false}} {
		rows, err := s.db.QueryContext(ctx, `SELECT device_id,label,substr(at,1,10) AS day,value,`+q.agg+`(at) FROM telemetry WHERE `+w+` GROUP BY 1,2,3`, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var dev, label, day, at string
			var v float64
			if err := rows.Scan(&dev, &label, &day, &v, &at); err != nil {
				rows.Close()
				return nil, err
			}
			a := get(dev, label, day)
			if q.last {
				a.Last, a.LastAt = v, parseTime(at)
			} else {
				a.First, a.FirstAt = v, parseTime(at)
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

func (s *SQLite) LastBefore(ctx context.Context, metric string, before time.Time, deviceIDs []string) ([]LabelValue, error) {
	where := []string{"metric=?", "at<?"}
	args := []any{metric, ts(before)}
	deviceClause("device_id", deviceIDs, &where, &args)
	rows, err := s.db.QueryContext(ctx, `SELECT device_id,label,value,MAX(at) FROM telemetry WHERE `+strings.Join(where, " AND ")+` GROUP BY device_id,label`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LabelValue{}
	for rows.Next() {
		var lv LabelValue
		var at string
		if err := rows.Scan(&lv.DeviceID, &lv.Label, &lv.Value, &at); err != nil {
			return nil, err
		}
		lv.At = parseTime(at)
		out = append(out, lv)
	}
	return out, rows.Err()
}

func (s *SQLite) OfflineStats(ctx context.Context, r ReportRange) ([]OfflineStat, error) {
	where, args := rangeWhere("at", r, "status.online")
	// The gap after an unreachable poll lasts until the next poll, or the range
	// end (clamped to now) when it is the last sample.
	end := r.To
	if now := time.Now().UTC(); end.After(now) {
		end = now
	}
	q := `SELECT device_id,
	  COUNT(*), SUM(value=0),
	  COALESCE(SUM(CASE WHEN value=0 THEN MAX(0,(julianday(COALESCE(next_at,?))-julianday(at))*1440) ELSE 0 END),0),
	  SUM(CASE WHEN value=0 AND (prev IS NULL OR prev<>0) THEN 1 ELSE 0 END)
	FROM (SELECT device_id,value,at,
	        LEAD(at) OVER (PARTITION BY device_id ORDER BY at) AS next_at,
	        LAG(value) OVER (PARTITION BY device_id ORDER BY at) AS prev
	      FROM telemetry WHERE ` + strings.Join(where, " AND ") + `)
	GROUP BY device_id ORDER BY device_id`
	rows, err := s.db.QueryContext(ctx, q, append([]any{ts(end)}, args...)...)
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

func (s *SQLite) AlertsByDay(ctx context.Context, r ReportRange) ([]AlertDayCount, error) {
	where, args := rangeWhere("raised_at", r, "")
	rows, err := s.db.QueryContext(ctx, `SELECT substr(raised_at,1,10) AS day,severity,source,COUNT(*) FROM alerts WHERE `+strings.Join(where, " AND ")+` GROUP BY 1,2,3 ORDER BY 1,2,3`, args...)
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

func (s *SQLite) AlertsByDevice(ctx context.Context, r ReportRange) ([]AlertDeviceStat, error) {
	where, args := rangeWhere("raised_at", r, "")
	q := `SELECT device_id, COUNT(*), SUM(severity='critical'), SUM(severity='warning'),
	  SUM(acked_at IS NOT NULL), SUM(cleared_at IS NOT NULL),
	  COALESCE(SUM(CASE WHEN acked_at IS NOT NULL THEN MAX(0,(julianday(acked_at)-julianday(raised_at))*1440) ELSE 0 END),0),
	  COALESCE(SUM(CASE WHEN cleared_at IS NOT NULL THEN MAX(0,(julianday(cleared_at)-julianday(raised_at))*1440) ELSE 0 END),0)
	FROM alerts WHERE ` + strings.Join(where, " AND ") + ` GROUP BY device_id ORDER BY 2 DESC`
	rows, err := s.db.QueryContext(ctx, q, args...)
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

func (s *SQLite) ReplacementEvents(ctx context.Context, r ReportRange) ([]*model.StockEvent, error) {
	where, args := rangeWhere("at", r, "")
	where = append([]string{"kind=?"}, where...)
	args = append([]any{model.StockEventReplacement}, args...)
	rows, err := s.db.QueryContext(ctx, `SELECT `+stockEventCols+` FROM consumable_stock_events WHERE `+strings.Join(where, " AND ")+` ORDER BY at DESC, id DESC LIMIT 5000`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.StockEvent{}
	for rows.Next() {
		e, err := scanStockEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *SQLite) StockConsumption(ctx context.Context, from, to time.Time) ([]StockConsumption, error) {
	q := `SELECT e.consumable_id, COALESCE(c.name,''), COALESCE(c.unit,''),
	  SUM(CASE WHEN e.delta<0 THEN -e.delta ELSE 0 END), SUM(CASE WHEN e.delta>0 THEN e.delta ELSE 0 END), COUNT(*)
	FROM consumable_stock_events e LEFT JOIN consumables c ON c.id=e.consumable_id
	WHERE e.status=? AND e.kind<>? AND e.at>=? AND e.at<? GROUP BY e.consumable_id ORDER BY 4 DESC, 2`
	rows, err := s.db.QueryContext(ctx, q, model.StockEventApplied, model.StockEventSet, ts(from), ts(to))
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

var _ ReportStore = (*SQLite)(nil)
