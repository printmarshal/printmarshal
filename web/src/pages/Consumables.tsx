import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useSearchParams } from 'react-router-dom'
import { api, ago } from '../api'
import type { Consumable, ConsumableRow, InstalledSupply, StockEvent, User } from '../types'
import { consumableTypes } from '../types'
import { useEvents } from '../ws'
import { BatchBar, ColorSwatch, ConsumableLink, ForecastCell, LevelBar, ModelChips, QtyCell, StockPill, fmtDays, typeLabel, typeLabels } from '../components/consumables'
import ConsumableForm from '../components/ConsumableForm'

type Tab = 'inventory' | 'fleet' | 'unmatched'

export default function Consumables({ user }: { user: User }) {
  const [params, setParams] = useSearchParams()
  const tab = (params.get('tab') as Tab) || 'inventory'
  const setTab = (t: Tab) => { const p = new URLSearchParams(params); if (t === 'inventory') p.delete('tab'); else p.set('tab', t); setParams(p, { replace: true }) }
  const canEdit = user.role !== 'viewer'
  const [pendingCount, setPendingCount] = useState(0)
  const loadPending = useCallback(() => { api.get<StockEvent[]>('/api/consumables/replacements').then((r) => setPendingCount(r.length)).catch(() => {}) }, [])
  useEffect(() => { loadPending() }, [loadPending])
  useEvents((e) => { if (e.type.startsWith('consumable.')) loadPending() }, [loadPending])
  return (
    <>
      <h1>Consumables</h1>
      <div className="tabs">
        <button className={tab === 'inventory' ? 'active' : ''} onClick={() => setTab('inventory')}>Inventory</button>
        <button className={tab === 'fleet' ? 'active' : ''} onClick={() => setTab('fleet')}>Fleet supplies</button>
        <button className={tab === 'unmatched' ? 'active' : ''} onClick={() => setTab('unmatched')}>Unmatched &amp; replacements{pendingCount > 0 && <span className="count">{pendingCount}</span>}</button>
      </div>
      {tab === 'inventory' && <Inventory canEdit={canEdit} />}
      {tab === 'fleet' && <FleetSupplies />}
      {tab === 'unmatched' && <Unmatched canEdit={canEdit} onChanged={loadPending} />}
    </>
  )
}

// --- Inventory ----------------------------------------------------------------

function Inventory({ canEdit }: { canEdit: boolean }) {
  const [params, setParams] = useSearchParams()
  const [rows, setRows] = useState<ConsumableRow[]>([])
  const [error, setError] = useState('')
  const [edits, setEdits] = useState<Record<string, number>>({})
  const [saving, setSaving] = useState(false)
  const [adding, setAdding] = useState(false)
  const [importMsg, setImportMsg] = useState('')
  const navigate = useNavigate()
  const q = params.get('q') || ''
  const filterQS = useMemo(() => { const p = new URLSearchParams(); for (const k of ['q', 'type', 'color', 'status', 'model']) { const v = params.get(k); if (v) p.set(k, v) } return p.toString() }, [params])

  const load = useCallback(() => {
    api.get<{ items: ConsumableRow[] }>(`/api/consumables?${filterQS}`).then((r) => setRows(r.items)).catch((e) => setError(e.message))
  }, [filterQS])
  useEffect(() => { load() }, [load])
  useEvents((e) => { if (e.type.startsWith('consumable.') || e.type === 'device.updated') load() }, [load])

  const set = (k: string, v: string) => { const p = new URLSearchParams(params); if (v) p.set(k, v); else p.delete(k); setParams(p, { replace: true }) }
  const models = useMemo(() => Array.from(new Set(rows.flatMap((r) => r.compatibleModels))).sort(), [rows])
  const colors = useMemo(() => Array.from(new Set(rows.map((r) => r.color).filter(Boolean))).sort(), [rows])
  const dirty = Object.entries(edits).filter(([id, v]) => { const r = rows.find((x) => x.id === id); return r && r.onHand !== v })
  const saveAll = async (reason: string) => {
    setSaving(true)
    try {
      await api.post('/api/consumables/bulk-stock', { items: dirty.map(([id, onHand]) => ({ id, onHand })), reason })
      setEdits({}); load()
    } catch (e) { setError((e as Error).message) } finally { setSaving(false) }
  }
  const importCSV = async (file: File) => {
    setImportMsg('Importing…')
    try {
      const res = await fetch('/api/consumables/import', { method: 'POST', body: await file.text(), headers: { 'Content-Type': 'text/csv' }, credentials: 'same-origin' })
      const data = await res.json() as { created?: number; updated?: number; errors?: string[]; error?: string }
      if (!res.ok) throw new Error(data.error || res.statusText)
      setImportMsg(`Imported: ${data.created} created, ${data.updated} updated${data.errors && data.errors.length ? ` · ${data.errors.length} rows skipped: ${data.errors.join('; ')}` : ''}`)
      load()
    } catch (e) { setImportMsg((e as Error).message) }
  }

  return (
    <>
      <div className="panel row between">
        <div className="row">
          <input placeholder="Search name, part number, location, model…" value={q} onChange={(e) => set('q', e.target.value)} style={{ minWidth: 260 }} />
          <select value={params.get('type') || ''} onChange={(e) => set('type', e.target.value)}><option value="">Any type</option>{consumableTypes.map((t) => <option key={t} value={t}>{typeLabels[t]}</option>)}</select>
          <select value={params.get('color') || ''} onChange={(e) => set('color', e.target.value)}><option value="">Any colour</option>{colors.map((c) => <option key={c} value={c}>{c}</option>)}</select>
          <select value={params.get('status') || ''} onChange={(e) => set('status', e.target.value)}><option value="">Any stock status</option><option value="ok">In stock</option><option value="low">Low</option><option value="critical">Critical</option><option value="out">Out</option></select>
          <select value={params.get('model') || ''} onChange={(e) => set('model', e.target.value)}><option value="">Any compatible model</option>{models.map((m) => <option key={m} value={m}>{m}</option>)}</select>
        </div>
        <div className="row">
          <a href={`/api/consumables.csv?${filterQS}`}>Export CSV</a>
          {canEdit && <label className="link-like">Import CSV<input type="file" accept=".csv,text/csv" style={{ display: 'none' }} onChange={(e) => { const f = e.target.files?.[0]; if (f) importCSV(f); e.target.value = '' }} /></label>}
          {canEdit && <button className="primary" onClick={() => setAdding(true)}>Add consumable</button>}
        </div>
      </div>
      {importMsg && <div className="muted small" style={{ marginBottom: 8 }}>{importMsg}</div>}
      {error && <div className="error">{error}</div>}
      <div className="panel">
        <table className="consumables">
          <thead><tr><th>Consumable</th><th>Type</th><th>Part number</th><th>Compatible models</th><th style={{ width: 150 }}>On hand</th><th>Stock</th><th>Devices</th><th>Forecast</th></tr></thead>
          <tbody>
            {rows.map((r) => {
              const v = edits[r.id] ?? r.onHand
              return (
                <tr key={r.id} className={edits[r.id] !== undefined && edits[r.id] !== r.onHand ? 'dirty-row' : ''}>
                  <td><Link to={`/consumables/${r.id}`} className="row" style={{ gap: 8, display: 'inline-flex' }}><ColorSwatch color={r.color} /><strong>{r.name}</strong></Link>{r.location && <div className="muted small">{r.location}</div>}</td>
                  <td>{typeLabel(r.type)}{r.color && <div className="muted small">{r.color}</div>}</td>
                  <td className="small">{r.partNumber || <span className="muted">—</span>}</td>
                  <td><ModelChips models={r.compatibleModels} /></td>
                  <td onClick={(e) => e.stopPropagation()}><QtyCell value={v} original={r.onHand} disabled={!canEdit} onChange={(n) => setEdits({ ...edits, [r.id]: n })} />{r.unit !== 'each' && <span className="muted small"> {r.unit}</span>}</td>
                  <td><StockPill status={r.status} /></td>
                  <td>{r.linkedDevices > 0 ? <Link to={`/consumables/${r.id}`}>{r.linkedDevices}</Link> : <span className="muted">0</span>}</td>
                  <td><ForecastCell f={r.forecast} compact /></td>
                </tr>
              )
            })}
            {rows.length === 0 && (
              <tr><td colSpan={8} className="muted" style={{ padding: 24 }}>
                {filterQS ? 'No consumables match these filters.' : <>No consumables yet. {canEdit ? <>Add one, or <button className="link" onClick={() => navigate('/consumables?tab=unmatched')}>create items from the supplies discovered on your devices</button>.</> : 'An operator can add stock items.'}</>}
              </td></tr>
            )}
          </tbody>
        </table>
        <div className="muted small" style={{ marginTop: 8 }}>Quantities and item fields are operator-owned and never overwritten by scans. Forecasts come from how fast installed levels drop on linked devices.</div>
      </div>
      <BatchBar count={dirty.length} saving={saving} onSave={saveAll} onDiscard={() => setEdits({})} />
      {adding && <ConsumableForm onClose={() => setAdding(false)} onSaved={() => { setAdding(false); load() }} />}
    </>
  )
}

// --- Fleet supplies -----------------------------------------------------------

type Level = '' | 'low' | 'empty' | 'ok'
function levelOf(s: InstalledSupply['supply']): Level {
  if (s.percent === null) return ''
  const p = s.class === 'filled' ? 100 - s.percent : s.percent
  return p <= 5 ? 'empty' : p <= 15 ? 'low' : 'ok'
}

function FleetSupplies() {
  const [rows, setRows] = useState<InstalledSupply[]>([])
  const [error, setError] = useState('')
  const [type, setType] = useState('')
  const [color, setColor] = useState('')
  const [level, setLevel] = useState<Level>('')
  const [group, setGroup] = useState<'type' | 'device'>('type')
  const load = useCallback(() => { api.get<InstalledSupply[]>('/api/consumables/installed').then(setRows).catch((e) => setError(e.message)) }, [])
  useEffect(() => { load() }, [load])
  useEvents((e) => { if (e.type === 'device.updated' || e.type.startsWith('consumable.')) load() }, [load])
  const typeOf = (s: InstalledSupply) => s.consumable?.type || s.suggestion?.type || s.supply.type
  const colorOf = (s: InstalledSupply) => s.consumable?.color || s.suggestion?.color || s.supply.colorant || ''
  const filtered = rows.filter((r) => (!type || typeOf(r) === type) && (!color || colorOf(r) === color) && (!level || levelOf(r.supply) === level))
  const colors = Array.from(new Set(rows.map(colorOf).filter(Boolean))).sort()
  const groups = useMemo(() => {
    const m = new Map<string, InstalledSupply[]>()
    for (const r of filtered) { const k = group === 'type' ? `${typeLabel(typeOf(r))}${colorOf(r) ? ` · ${colorOf(r)}` : ''}` : r.deviceName; m.set(k, [...(m.get(k) || []), r]) }
    return Array.from(m.entries()).sort(([a], [b]) => a.localeCompare(b))
  }, [filtered, group])
  const lowCount = rows.filter((r) => levelOf(r.supply) !== 'ok' && levelOf(r.supply) !== '').length
  return (
    <>
      <div className="panel row between">
        <div className="row">
          <select value={type} onChange={(e) => setType(e.target.value)}><option value="">Any type</option>{consumableTypes.map((t) => <option key={t} value={t}>{typeLabels[t]}</option>)}</select>
          <select value={color} onChange={(e) => setColor(e.target.value)}><option value="">Any colour</option>{colors.map((c) => <option key={c} value={c}>{c}</option>)}</select>
          <select value={level} onChange={(e) => setLevel(e.target.value as Level)}><option value="">Any level</option><option value="empty">Empty / critical (≤5%)</option><option value="low">Low (≤15%)</option><option value="ok">OK</option></select>
          <select value={group} onChange={(e) => setGroup(e.target.value as 'type' | 'device')}><option value="type">Group by supply</option><option value="device">Group by device</option></select>
        </div>
        <div className="muted small">{rows.length} installed supplies · <span className={lowCount ? 'error' : ''}>{lowCount} low or empty</span> · <a href="/api/supplies.csv">CSV</a></div>
      </div>
      {error && <div className="error">{error}</div>}
      {groups.length === 0 && <div className="panel muted">No installed supplies yet. Poll your devices to read their supply levels.</div>}
      {groups.map(([name, list]) => (
        <div className="panel" key={name}>
          <h2 className="row" style={{ gap: 8 }}>{group === 'type' && <ColorSwatch color={colorOf(list[0])} />}{name} <span className="muted small">({list.length})</span>
            {list.some((r) => levelOf(r.supply) !== 'ok' && levelOf(r.supply) !== '') && <span className="badge warning">{list.filter((r) => levelOf(r.supply) !== 'ok' && levelOf(r.supply) !== '').length} low / empty</span>}</h2>
          <table>
            <thead><tr><th>{group === 'type' ? 'Device' : 'Supply'}</th><th>Installed level</th><th>Stock item</th><th>On hand</th><th>Days to empty</th></tr></thead>
            <tbody>
              {list.sort((a, b) => (a.supply.percent ?? 101) - (b.supply.percent ?? 101)).map((r) => (
                <tr key={`${r.deviceId}:${r.supply.index}`} className={levelOf(r.supply) === 'empty' ? 'row-critical' : levelOf(r.supply) === 'low' ? 'row-warning' : ''}>
                  <td>{group === 'type' ? <><Link to={`/devices/${r.deviceId}`}>{r.deviceName}</Link><div className="muted small">{r.deviceModel} · {r.address} · {r.supply.description}</div></> : <><ColorSwatch color={colorOf(r)} />{r.supply.description}<div className="muted small">{typeLabel(typeOf(r))}</div></>}</td>
                  <td><LevelBar percent={r.supply.percent} filled={r.supply.class === 'filled'} /></td>
                  <td>{r.consumable ? <><ConsumableLink c={r.consumable} />{r.linkSource === 'manual' && <span className="muted small"> · manual link</span>}</> : <Link to="/consumables?tab=unmatched" className="muted small">unmatched →</Link>}</td>
                  <td>{r.consumable ? <span className="row" style={{ gap: 6, display: 'inline-flex' }}><StockPill status={statusFor(r.consumable)} /><span className="small">{r.consumable.onHand} in stock</span></span> : <span className="muted">—</span>}</td>
                  <td className="small">{r.forecast?.daysToEmpty != null ? fmtDays(r.forecast.daysToEmpty) : <span className="muted" title={r.forecast?.rate.reason}>unknown</span>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ))}
    </>
  )
}

function statusFor(c: Consumable) { return c.onHand <= 0 ? 'out' : c.onHand <= c.criticalAt ? 'critical' : c.onHand <= c.warningAt ? 'low' : 'ok' }

// --- Unmatched supplies & pending replacements ---------------------------------

function Unmatched({ canEdit, onChanged }: { canEdit: boolean; onChanged: () => void }) {
  const [unmatched, setUnmatched] = useState<InstalledSupply[]>([])
  const [pending, setPending] = useState<StockEvent[]>([])
  const [items, setItems] = useState<Consumable[]>([])
  const [creating, setCreating] = useState<{ sup: InstalledSupply } | null>(null)
  const [msg, setMsg] = useState('')
  const load = useCallback(() => {
    api.get<InstalledSupply[]>('/api/consumables/installed?unmatched=true&forecast=false').then(setUnmatched).catch(() => {})
    api.get<StockEvent[]>('/api/consumables/replacements').then(setPending).catch(() => {})
    api.get<{ items: Consumable[] }>('/api/consumables').then((r) => setItems(r.items)).catch(() => {})
  }, [])
  useEffect(() => { load() }, [load])
  useEvents((e) => { if (e.type.startsWith('consumable.') || e.type === 'device.updated') load() }, [load])
  const link = async (s: InstalledSupply, consumableId: string) => {
    if (!consumableId) return
    try { await api.put(`/api/devices/${s.deviceId}/supplies/${s.supply.index}/link`, { consumableId }); load(); onChanged() } catch (e) { setMsg((e as Error).message) }
  }
  const act = async (ev: StockEvent, what: 'confirm' | 'dismiss') => {
    try { await api.post(`/api/consumables/replacements/${ev.id}/${what}`); load(); onChanged() } catch (e) { setMsg((e as Error).message) }
  }
  const itemName = (id: string) => items.find((i) => i.id === id)?.name || id.slice(0, 8)
  return (
    <div className="grid cols-2">
      <div className="panel">
        <h2>Unmatched installed supplies <span className="muted small">({unmatched.length})</span></h2>
        <div className="muted small" style={{ marginBottom: 8 }}>Supplies read from devices that no stock item matches yet. Link them to an existing item or create one pre-filled from the device.</div>
        {msg && <div className="error small">{msg}</div>}
        {unmatched.length === 0 && <div className="muted">Every installed supply is linked to a stock item.</div>}
        <table><tbody>
          {unmatched.map((s) => (
            <tr key={`${s.deviceId}:${s.supply.index}`}>
              <td><ColorSwatch color={s.suggestion?.color || s.supply.colorant} />{s.supply.description}<div className="muted small"><Link to={`/devices/${s.deviceId}`}>{s.deviceName}</Link> · {s.deviceModel} · {typeLabel(s.suggestion?.type || s.supply.type)}</div></td>
              <td><LevelBar percent={s.supply.percent} filled={s.supply.class === 'filled'} /></td>
              <td style={{ textAlign: 'right', whiteSpace: 'nowrap' }}>
                {canEdit && (<>
                  <select value="" onChange={(e) => link(s, e.target.value)}><option value="">Link to…</option>{items.map((i) => <option key={i.id} value={i.id}>{i.name}{i.color ? ` (${i.color})` : ''}</option>)}</select>{' '}
                  <button onClick={() => setCreating({ sup: s })}>Create item</button>
                </>)}
              </td>
            </tr>
          ))}
        </tbody></table>
      </div>
      <div className="panel">
        <h2>Replacements awaiting confirmation <span className="muted small">({pending.length})</span></h2>
        <div className="muted small" style={{ marginBottom: 8 }}>A supply level jumped back up, so a cartridge was probably replaced. Confirm to take one unit out of stock, or dismiss. Change the behaviour under Settings → consumables.</div>
        {pending.length === 0 && <div className="muted">Nothing pending.</div>}
        <table><tbody>
          {pending.map((ev) => (
            <tr key={ev.id}>
              <td><Link to={`/consumables/${ev.consumableId}`}>{itemName(ev.consumableId)}</Link><div className="muted small">{ev.reason} · {ago(ev.at)}{ev.deviceId && <> · <Link to={`/devices/${ev.deviceId}`}>device</Link></>}</div></td>
              <td style={{ textAlign: 'right', whiteSpace: 'nowrap' }}>{canEdit && (<><button className="primary" onClick={() => act(ev, 'confirm')}>Confirm −1</button> <button onClick={() => act(ev, 'dismiss')}>Dismiss</button></>)}</td>
            </tr>
          ))}
        </tbody></table>
      </div>
      {creating && <ConsumableForm initial={creating.sup.suggestion as Partial<Consumable>} onClose={() => setCreating(null)}
        onSaved={async (c) => { const s = creating.sup; setCreating(null); try { await api.put(`/api/devices/${s.deviceId}/supplies/${s.supply.index}/link`, { consumableId: c.id }) } catch { /* auto-link may already cover it */ } load(); onChanged() }} />}
    </div>
  )
}
