package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"printmarshal/internal/model"
)

// --- consumables -----------------------------------------------------------

const consumableCols = `id,name,type,color,part_number,compatible_models,unit,on_hand,warning_at,critical_at,location,notes,reorder_url,unit_cost,created_at,updated_at`

func scanConsumable(r rowScanner) (*model.Consumable, error) {
	var c model.Consumable
	var compat, created, upd string
	var cost sql.NullFloat64
	if err := r.Scan(&c.ID, &c.Name, &c.Type, &c.Color, &c.PartNumber, &compat, &c.Unit, &c.OnHand, &c.WarningAt, &c.CriticalAt,
		&c.Location, &c.Notes, &c.ReorderURL, &cost, &created, &upd); err != nil {
		return nil, err
	}
	c.CompatibleModels = []string{}
	fromJSON(sql.NullString{String: compat, Valid: true}, &c.CompatibleModels)
	if cost.Valid {
		v := cost.Float64
		c.UnitCost = &v
	}
	c.CreatedAt, c.UpdatedAt = parseTime(created), parseTime(upd)
	return &c, nil
}

func normalizeConsumable(c *model.Consumable) {
	c.Name = strings.TrimSpace(c.Name)
	c.PartNumber = strings.TrimSpace(c.PartNumber)
	c.Color = strings.ToLower(strings.TrimSpace(c.Color))
	if c.Type == "" {
		c.Type = model.ConsumableOther
	}
	if c.Unit == "" {
		c.Unit = "each"
	}
	if c.OnHand < 0 {
		c.OnHand = 0
	}
	if c.CriticalAt > c.WarningAt {
		c.WarningAt = c.CriticalAt
	}
	var models []string
	for _, m := range c.CompatibleModels {
		if m = strings.TrimSpace(m); m != "" {
			models = append(models, m)
		}
	}
	if models == nil {
		models = []string{}
	}
	c.CompatibleModels = models
}

func (s *SQLite) ListConsumables(ctx context.Context, f ConsumableFilter) ([]*model.Consumable, error) {
	var where []string
	var args []any
	if f.Type != "" {
		where = append(where, "type=?")
		args = append(args, f.Type)
	}
	if f.Color != "" {
		where = append(where, "color=?")
		args = append(args, strings.ToLower(f.Color))
	}
	if len(f.IDs) > 0 {
		where = append(where, "id IN ("+placeholders(len(f.IDs))+")")
		args = append(args, toAny(f.IDs)...)
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		like := "%" + strings.ToLower(q) + "%"
		where = append(where, "(lower(name) LIKE ? OR lower(part_number) LIKE ? OR lower(location) LIKE ? OR lower(notes) LIKE ? OR lower(compatible_models) LIKE ? OR lower(color) LIKE ?)")
		args = append(args, like, like, like, like, like, like)
	}
	if m := strings.TrimSpace(f.Model); m != "" {
		where = append(where, "lower(compatible_models) LIKE ?")
		args = append(args, "%"+strings.ToLower(m)+"%")
	}
	q := `SELECT ` + consumableCols + ` FROM consumables`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY lower(name), color"
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.Consumable{}
	for rows.Next() {
		c, err := scanConsumable(rows)
		if err != nil {
			return nil, err
		}
		if f.Status != "" && string(c.Status()) != f.Status {
			continue
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *SQLite) GetConsumable(ctx context.Context, id string) (*model.Consumable, error) {
	c, err := scanConsumable(s.db.QueryRowContext(ctx, `SELECT `+consumableCols+` FROM consumables WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return c, err
}

func (s *SQLite) CreateConsumable(ctx context.Context, c *model.Consumable) error {
	normalizeConsumable(c)
	if c.Name == "" {
		return fmt.Errorf("name is required")
	}
	if c.ID == "" {
		c.ID = newID()
	}
	n := now()
	c.CreatedAt, c.UpdatedAt = parseTime(n), parseTime(n)
	var cost any
	if c.UnitCost != nil {
		cost = *c.UnitCost
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO consumables(`+consumableCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		c.ID, c.Name, string(c.Type), c.Color, c.PartNumber, toJSON(c.CompatibleModels), c.Unit, c.OnHand, c.WarningAt, c.CriticalAt,
		c.Location, c.Notes, c.ReorderURL, cost, n, n); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO consumable_stock_events(consumable_id,actor,kind,delta,resulting,reason,status,at) VALUES(?,?,?,?,?,?,?,?)`,
		c.ID, "", model.StockEventSet, c.OnHand, c.OnHand, "initial stock", model.StockEventApplied, n); err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateConsumable writes every operator field except the on-hand quantity,
// which only changes through AdjustStock so that history stays complete.
func (s *SQLite) UpdateConsumable(ctx context.Context, c *model.Consumable) error {
	normalizeConsumable(c)
	if c.Name == "" {
		return fmt.Errorf("name is required")
	}
	n := now()
	var cost any
	if c.UnitCost != nil {
		cost = *c.UnitCost
	}
	res, err := s.db.ExecContext(ctx, `UPDATE consumables SET name=?, type=?, color=?, part_number=?, compatible_models=?, unit=?, warning_at=?, critical_at=?,
		location=?, notes=?, reorder_url=?, unit_cost=?, updated_at=? WHERE id=?`,
		c.Name, string(c.Type), c.Color, c.PartNumber, toJSON(c.CompatibleModels), c.Unit, c.WarningAt, c.CriticalAt,
		c.Location, c.Notes, c.ReorderURL, cost, n, c.ID)
	if err != nil {
		return err
	}
	if k, _ := res.RowsAffected(); k == 0 {
		return ErrNotFound
	}
	c.UpdatedAt = parseTime(n)
	return nil
}

func (s *SQLite) DeleteConsumable(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM consumables WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE alerts SET cleared_at=?, updated_at=? WHERE key=? AND cleared_at IS NULL`, now(), now(), "consumable:"+id)
	return nil
}

// AdjustStock implements Store.
func (s *SQLite) AdjustStock(ctx context.Context, id string, ev *model.StockEvent) (*model.Consumable, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var onHand int
	if err := tx.QueryRowContext(ctx, `SELECT on_hand FROM consumables WHERE id=?`, id).Scan(&onHand); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if ev.Status == "" {
		ev.Status = model.StockEventApplied
	}
	if ev.At.IsZero() {
		ev.At = time.Now().UTC()
	}
	ev.ConsumableID = id
	newQty := onHand
	switch {
	case ev.Status != model.StockEventApplied:
		// Recorded only (pending / logged / dismissed replacement): quantity unchanged.
		ev.Delta = 0
	case ev.Kind == model.StockEventSet || ev.Kind == model.StockEventImport:
		newQty = ev.Resulting
		if newQty < 0 {
			newQty = 0
		}
		ev.Delta = newQty - onHand
	default:
		newQty = onHand + ev.Delta
		if newQty < 0 {
			newQty = 0
			ev.Delta = -onHand
		}
	}
	ev.Resulting = newQty
	n := ts(ev.At)
	if newQty != onHand {
		if _, err := tx.ExecContext(ctx, `UPDATE consumables SET on_hand=?, updated_at=? WHERE id=?`, newQty, n, id); err != nil {
			return nil, err
		}
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO consumable_stock_events(consumable_id,actor,kind,delta,resulting,reason,device_id,supply_index,status,at) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		id, ev.Actor, ev.Kind, ev.Delta, ev.Resulting, ev.Reason, ev.DeviceID, ev.SupplyIndex, ev.Status, n)
	if err != nil {
		return nil, err
	}
	ev.ID, _ = res.LastInsertId()
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetConsumable(ctx, id)
}

const stockEventCols = `id,consumable_id,actor,kind,delta,resulting,reason,device_id,supply_index,status,at`

func scanStockEvent(r rowScanner) (*model.StockEvent, error) {
	var e model.StockEvent
	var at string
	if err := r.Scan(&e.ID, &e.ConsumableID, &e.Actor, &e.Kind, &e.Delta, &e.Resulting, &e.Reason, &e.DeviceID, &e.SupplyIndex, &e.Status, &at); err != nil {
		return nil, err
	}
	e.At = parseTime(at)
	return &e, nil
}

func (s *SQLite) ListStockEvents(ctx context.Context, f StockEventFilter) ([]*model.StockEvent, error) {
	var where []string
	var args []any
	if f.ConsumableID != "" {
		where = append(where, "consumable_id=?")
		args = append(args, f.ConsumableID)
	}
	if f.DeviceID != "" {
		where = append(where, "device_id=?")
		args = append(args, f.DeviceID)
	}
	if f.Kind != "" {
		where = append(where, "kind=?")
		args = append(args, f.Kind)
	}
	if f.Status != "" {
		where = append(where, "status=?")
		args = append(args, f.Status)
	}
	q := `SELECT ` + stockEventCols + ` FROM consumable_stock_events`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	limit := 500
	if f.Limit > 0 {
		limit = f.Limit
	}
	q += fmt.Sprintf(" ORDER BY at DESC, id DESC LIMIT %d", limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
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

func (s *SQLite) GetStockEvent(ctx context.Context, id int64) (*model.StockEvent, error) {
	e, err := scanStockEvent(s.db.QueryRowContext(ctx, `SELECT `+stockEventCols+` FROM consumable_stock_events WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return e, err
}

func (s *SQLite) SetStockEventStatus(ctx context.Context, id int64, from, to string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE consumable_stock_events SET status=? WHERE id=? AND status=?`, to, id, from)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- supply links ----------------------------------------------------------

func (s *SQLite) ListSupplyLinks(ctx context.Context, f SupplyLinkFilter) ([]*model.SupplyLink, error) {
	var where []string
	var args []any
	if f.DeviceID != "" {
		where = append(where, "device_id=?")
		args = append(args, f.DeviceID)
	}
	if f.ConsumableID != "" {
		where = append(where, "consumable_id=?")
		args = append(args, f.ConsumableID)
	}
	if f.Source != "" {
		where = append(where, "source=?")
		args = append(args, f.Source)
	}
	q := `SELECT device_id,supply_index,consumable_id,source,updated_at FROM supply_links`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY device_id, supply_index"
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.SupplyLink{}
	for rows.Next() {
		var l model.SupplyLink
		var upd string
		if err := rows.Scan(&l.DeviceID, &l.SupplyIndex, &l.ConsumableID, &l.Source, &upd); err != nil {
			return nil, err
		}
		l.UpdatedAt = parseTime(upd)
		out = append(out, &l)
	}
	return out, rows.Err()
}

func (s *SQLite) SetSupplyLink(ctx context.Context, l *model.SupplyLink) error {
	if l.Source == "" {
		l.Source = "manual"
	}
	n := now()
	l.UpdatedAt = parseTime(n)
	_, err := s.db.ExecContext(ctx, `INSERT INTO supply_links(device_id,supply_index,consumable_id,source,updated_at) VALUES(?,?,?,?,?)
		ON CONFLICT(device_id,supply_index) DO UPDATE SET consumable_id=excluded.consumable_id, source=excluded.source, updated_at=excluded.updated_at`,
		l.DeviceID, l.SupplyIndex, l.ConsumableID, l.Source, n)
	return err
}

func (s *SQLite) DeleteSupplyLink(ctx context.Context, deviceID string, supplyIndex int) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM supply_links WHERE device_id=? AND supply_index=?`, deviceID, supplyIndex)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ReplaceAutoLinks implements Store: manual links are operator-owned and kept.
func (s *SQLite) ReplaceAutoLinks(ctx context.Context, deviceID string, links []*model.SupplyLink) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM supply_links WHERE device_id=? AND source='auto'`, deviceID); err != nil {
		return err
	}
	n := now()
	for _, l := range links {
		// Never displace a manual link for the same slot.
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO supply_links(device_id,supply_index,consumable_id,source,updated_at) VALUES(?,?,?,'auto',?)`,
			deviceID, l.SupplyIndex, l.ConsumableID, n); err != nil {
			return err
		}
	}
	return tx.Commit()
}
