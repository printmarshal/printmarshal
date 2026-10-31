import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api, ago } from '../api'
import type { Consumable, ConsumableRow, InstalledSupply, StockEvent, User } from '../types'
import { consumableTypes } from '../types'
import { useEvents } from '../ws'
import { Ic } from '../components/icons'
import { Btn, Card, ChipSelect, EmptyState, Pill, SearchBox, Seg, Tabs } from '../components/ui'
import { BatchBar, ColorSwatch, ConsumableLink, ForecastCell, LevelBar, ModelChips, QtyCell, StockPill, fmtDays, typeLabel, typeLabels } from '../components/consumables'
import ConsumableForm from '../components/ConsumableForm'

type Tab = 'inventory' | 'fleet' | 'unmatched'

export default function Consumables({ user }: { user: User }) {
  const [params, setParams] = useSearchParams()
  const tab = (params.get('tab') as Tab) || 'inventory'
  const setTab = (t: Tab) => { const p = new URLSearchParams(params); if (t === 'inventory') p.delete('tab'); else p.set('tab', t); setParams(p, { replace: true }) }
  const canEdit = user.role !== 'viewer'
  const [pendingCount, setPendingCount] = useState(0)
  const [adding, setAdding] = useState(false)
  const [reload, setReload] = useState(0)
  const loadPending = useCallback(() => { api.get<StockEvent[]>('/api/consumables/replacements').then((r) => setPendingCount(r.length)).catch(() => {}) }, [])
  useEffect(() => { loadPending() }, [loadPending])
  useEvents((e) => { if (e.type.startsWith('consumable.')) loadPending() }, [loadPending])
  return (
    <>
      <div className="ph">
        <div><h1 className="h1">Consumables</h1><div className="cap">Stock on the shelf, matched to the supplies installed in your devices · stock counts are operator-entered</div></div>
        <div className="row">
          <a className="btn sm" href="/api/consumables.csv"><Ic.download />Export CSV</a>
          {canEdit && <Btn sm variant="pri" onClick={() => setAdding(true)}><Ic.plus />Add consumable</Btn>}
        </div>
      </div>
      <Tabs label="Consumables sections" value={tab} onChange={setTab} tabs={[
        { v: 'inventory', label: 'Inventory' },
        { v: 'fleet', label: 'Fleet supplies' },
        { v: 'unmatched', label: <>Unmatched & replacements{pendingCount > 0 && <span className="cnt warn">{pendingCount}</span>}</> },
      ]} />
      {tab === 'inventory' && <Inventory key={reload} canEdit={canEdit} />}
      {tab === 'fleet' && <FleetSupplies />}
      {tab === 'unmatched' && <Unmatched canEdit={canEdit} onChanged={loadPending} />}
      {adding && <ConsumableForm onClose={() => setAdding(false)} onSaved={() => { setAdding(false); setReload((n) => n + 1) }} />}
    </>
  )
}

function Inventory({ canEdit }: { canEdit: boolean }) {
  const [params, setParams] = useSearchParams()
  const [rows, setRows] = useState<ConsumableRow[]>([])
  const [error, setError] = useState('')
  const [edits, setEdits] = useState<Record<string, number>>({})
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
  const [importMsg, setImportMsg] = useState('')
  const q = params.get('q') || ''
  const status = params.get('status') || ''
  const filterQS = useMemo(() => { const p = new URLSearchParams(); for (const k of ['q', 'type', 'color', 'model']) { const v = params.get(k); if (v) p.set(k, v) } return p.toString() }, [params])
  const load = useCallback(() => { api.get<{ items: ConsumableRow[] }>(`/api/consumables?${filterQS}`).then((r) => setRows(r.items)).catch((e) => setError(e.message)) }, [filterQS])
  useEffect(() => { load() }, [load])
  useEvents((e) => { if (e.type.startsWith('consumable.') || e.type === 'device.updated') load() }, [load])
  const set = (k: string, v: string) => { const p = new URLSearchParams(params); if (v) p.set(k, v); else p.delete(k); setParams(p, { replace: true }) }
  const models = useMemo(() => Array.from(new Set(rows.flatMap((r) => r.compatibleModels))).sort(), [rows])
  const colors = useMemo(() => Array.from(new Set(rows.map((r) => r.color).filter(Boolean))).sort(), [rows])
  const shown = rows.filter((r) => !status || (status === 'low' ? r.status === 'low' || r.status === 'critical' : r.status === status))
  const counts = { low: rows.filter((r) => r.status === 'low' || r.status === 'critical').length, out: rows.filter((r) => r.status === 'out').length }
  const dirty = Object.entries(edits).filter(([id, v]) => { const r = rows.find((x) => x.id === id); return r && r.onHand !== v })
  const saveAll = async (reason: string) => {
    setSaving(true)
    try { await api.post('/api/consumables/bulk-stock', { items: dirty.map(([id, onHand]) => ({ id, onHand })), reason: reason || 'Count correction' }); setEdits({}); setSaved(true); setTimeout(() => setSaved(false), 5000); load() } catch (e) { setError((e as Error).message) } finally { setSaving(false) }
  }
  const importCSV = async (file: File) => {
    setImportMsg('Importing…')
    try {
      const res = await fetch('/api/consumables/import', { method: 'POST', body: await file.text(), headers: { 'Content-Type': 'text/csv' }, credentials: 'same-origin' })
      const data = await res.json() as { created?: number; updated?: number; errors?: string[]; error?: string }
      if (!res.ok) throw new Error(data.error || res.statusText)
      setImportMsg(`Imported: ${data.created} created, ${data.updated} updated${data.errors?.length ? ` · ${data.errors.length} rows skipped: ${data.errors.join('; ')}` : ''}`)
      load()
    } catch (e) { setImportMsg((e as Error).message) }
  }
  return (
    <>
      <div className="row">
        <SearchBox sm filter value={q} onChange={(v) => set('q', v)} placeholder="Name or part number…" />
        <Seg label="Stock status" value={status} onChange={(v) => set('status', v)} options={[
          { v: '', label: <>All <span className="cap num">{rows.length}</span></> },
          { v: 'low', label: <><span className="dot" style={{ background: 'var(--warn)' }} />Low <span className="num">{counts.low}</span></> },
          { v: 'out', label: <><span className="dot" style={{ background: 'var(--crit)' }} />Out <span className="num">{counts.out}</span></> },
        ]} />
        <ChipSelect label="Type" value={params.get('type') || ''} onChange={(v) => set('type', v)} options={consumableTypes.map((t) => ({ v: t, label: typeLabels[t] }))} />
        <ChipSelect label="Colour" value={params.get('color') || ''} onChange={(v) => set('color', v)} options={colors.map((c) => ({ v: c, label: c }))} />
        <ChipSelect label="Compatible model" value={params.get('model') || ''} onChange={(v) => set('model', v)} options={models.map((m) => ({ v: m, label: m }))} />
        {canEdit && <label className="btn sm" style={{ marginLeft: 'auto' }}><Ic.upload />Import CSV<input type="file" accept=".csv,text/csv" style={{ display: 'none' }} onChange={(e) => { const f = e.target.files?.[0]; if (f) importCSV(f); e.target.value = '' }} /></label>}
      </div>
      {importMsg && <div className="banner info" role="status"><Ic.info style={{ color: 'var(--info)' }} /><div style={{ flex: 1 }}>{importMsg}</div><Btn sm variant="ghost" ic onClick={() => setImportMsg('')}><Ic.x /></Btn></div>}
      {error && <div className="banner crit">{error}</div>}
      <Card aria-label="Consumable items">
        <BatchBar count={dirty.length} saving={saving} onSave={saveAll} onDiscard={() => setEdits({})} />
        {saved && dirty.length === 0 && <div role="status" style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '10px 14px', background: 'var(--ok-bg)', color: 'var(--ok)', borderBottom: '1px solid var(--border)', fontWeight: 500 }}><Ic.check />Saved · stock history updated</div>}
        {shown.length === 0 ? <div className="cb"><EmptyState icon={<Ic.consumables size={20} />} title={rows.length === 0 ? 'No consumables yet' : 'No items match'} text={rows.length === 0 ? 'Add stock items, or create them from the supplies discovered on your devices.' : 'Try another filter.'} action={rows.length === 0 && canEdit ? <Link className="btn sm pri" to="/consumables?tab=unmatched">Create from discovered supplies</Link> : undefined} /></div> : (
          <div className="tw"><table className="tbl" style={{ minWidth: 1100 }}>
            <thead><tr><th>Consumable</th><th>Type</th><th>Part no.</th><th>Compatible models</th><th>On hand <Ic.user size={12} style={{ verticalAlign: -1, color: 'var(--accent)' }} /></th><th>Stock</th><th className="r">Linked devices</th><th>Forecast</th></tr></thead>
            <tbody>{shown.map((r) => {
              const v = edits[r.id] ?? r.onHand
              const ed = edits[r.id] !== undefined && edits[r.id] !== r.onHand
              return (
                <tr key={r.id} className={ed ? 'ed' : ''}>
                  <td><Link to={`/consumables/${r.id}`} style={{ display: 'inline-flex', alignItems: 'center', gap: 8, color: 'var(--text)', fontWeight: 500 }}><ColorSwatch color={r.color} />{r.name}</Link>{r.location && <div className="cap">{r.location}</div>}</td>
                  <td className="cap" style={{ color: 'var(--text-2)' }}>{typeLabel(r.type)}{r.color && <div className="cap">{r.color}</div>}</td>
                  <td className="mono">{r.partNumber || <span className="cap">—</span>}</td>
                  <td><ModelChips models={r.compatibleModels} max={2} /></td>
                  <td><QtyCell value={v} original={r.onHand} disabled={!canEdit} onChange={(n) => setEdits({ ...edits, [r.id]: n })} />{r.unit !== 'each' && <span className="cap"> {r.unit}</span>}</td>
                  <td><StockPill status={r.status} /></td>
                  <td className="num r">{r.linkedDevices > 0 ? <Link to={`/consumables/${r.id}`}>{r.linkedDevices}</Link> : <span className="cap">0</span>}</td>
                  <td><ForecastCell f={r.forecast} compact /></td>
                </tr>
              )
            })}</tbody>
          </table></div>
        )}
        <div className="cf" style={{ display: 'flex', flexWrap: 'wrap', justifyContent: 'space-between', gap: 8 }}><span className="cap">Forecast = on hand ÷ fleet consumption. “Reorder soon” when it falls inside the lead time.</span><span className="cap" style={{ display: 'inline-flex', alignItems: 'center', gap: 5 }}><Ic.user size={12} style={{ color: 'var(--accent)' }} />operator-set · never overwritten by scans</span></div>
      </Card>
    </>
  )
}

type Level = '' | 'low' | 'empty' | 'ok'
function levelOf(s: InstalledSupply['supply']): Level { if (s.percent === null) return ''; const p = s.class === 'filled' ? 100 - s.percent : s.percent; return p <= 5 ? 'empty' : p <= 15 ? 'low' : 'ok' }
function statusFor(c: Consumable) { return c.onHand <= 0 ? 'out' : c.onHand <= c.criticalAt ? 'critical' : c.onHand <= c.warningAt ? 'low' : 'ok' }

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
  const filtered = rows.filter((r) => (!type || typeOf(r) === type) && (!color || colorOf(r) === color) && (!level || levelOf(r.supply) === level || (level === 'low' && levelOf(r.supply) === 'empty')))
  const colors = Array.from(new Set(rows.map(colorOf).filter(Boolean))).sort()
  const groups = useMemo(() => {
    const m = new Map<string, InstalledSupply[]>()
    for (const r of filtered) { const k = group === 'type' ? `${typeLabel(typeOf(r))}${colorOf(r) ? ` · ${colorOf(r)}` : ''}` : r.deviceName; m.set(k, [...(m.get(k) || []), r]) }
    return Array.from(m.entries()).sort(([a], [b]) => a.localeCompare(b))
  }, [filtered, group])
  const lowRows = rows.filter((r) => levelOf(r.supply) === 'low' || levelOf(r.supply) === 'empty')
  // The headline counts devices; the supplies behind it are counted beside it (#48).
  const lowCount = new Set(lowRows.map((r) => r.deviceId)).size
  const emptyCount = rows.filter((r) => levelOf(r.supply) === 'empty').length
  const short = rows.filter((r) => r.consumable && (levelOf(r.supply) === 'low' || levelOf(r.supply) === 'empty')).reduce<Record<string, number>>((m, r) => { m[r.consumable!.id] = (m[r.consumable!.id] || 0) + 1; return m }, {})
  const shortItems = Object.entries(short).filter(([id, n]) => { const c = rows.find((r) => r.consumable?.id === id)?.consumable; return c && n > c.onHand }).length
  return (
    <>
      <section className="card" style={{ display: 'flex', flexWrap: 'wrap' }}>
        <div className="stat"><span className="cap">Devices with a supply ≤ 15%</span><span className="v" style={{ color: lowCount ? 'var(--warn)' : undefined }}>{lowCount}</span><span className="cap">{lowRows.length} {lowRows.length === 1 ? 'supply' : 'supplies'} · {emptyCount} empty</span></div>
        <div className="stat"><span className="cap">Stock items short of demand</span><span className="v" style={{ color: shortItems ? 'var(--crit)' : undefined }}>{shortItems}</span><span className="cap">more low devices than spares</span></div>
        <div className="stat"><span className="cap">Installed supplies tracked</span><span className="v">{rows.length}</span><span className="cap">across polled devices</span></div>
      </section>
      <div className="row">
        <Seg label="Group" value={group} onChange={setGroup} options={[{ v: 'type', label: 'By consumable' }, { v: 'device', label: 'By device' }]} />
        <ChipSelect label="Type" value={type} onChange={setType} options={consumableTypes.map((t) => ({ v: t, label: typeLabels[t] }))} />
        <ChipSelect label="Colour" value={color} onChange={setColor} options={colors.map((c) => ({ v: c, label: c }))} />
        <ChipSelect label="Level" value={level} onChange={(v) => setLevel(v as Level)} options={[{ v: 'empty', label: 'Empty (≤ 5%)' }, { v: 'low', label: 'Low (≤ 15%)' }, { v: 'ok', label: 'OK' }]} />
        <a className="btn sm" style={{ marginLeft: 'auto' }} href="/api/supplies.csv"><Ic.download />Export</a>
      </div>
      {error && <div className="banner crit">{error}</div>}
      {groups.length === 0 && <Card><div className="cb"><EmptyState icon={<Ic.printer size={20} />} title="No installed supplies yet" text="Poll your devices to read their supply levels." /></div></Card>}
      {groups.map(([name, list]) => (
        <Card key={name} title={<span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>{group === 'type' && <ColorSwatch color={colorOf(list[0])} />}{name}</span>} right={<span className="row">{list.some((r) => levelOf(r.supply) !== 'ok' && levelOf(r.supply) !== '') && <Pill tone="warn" xs>{list.filter((r) => levelOf(r.supply) !== 'ok' && levelOf(r.supply) !== '').length} low / empty</Pill>}<span className="cap num">{list.length}</span></span>}>
          <div className="tw"><table className="tbl">
            <thead><tr><th>{group === 'type' ? 'Device' : 'Supply'}</th><th>Installed level</th><th>Stock item</th><th>On hand</th><th>Days to empty</th></tr></thead>
            <tbody>{list.slice().sort((a, b) => (a.supply.percent ?? 101) - (b.supply.percent ?? 101)).map((r) => (
              <tr key={`${r.deviceId}:${r.supply.index}`}>
                <td>{group === 'type' ? <><Link to={`/devices/${r.deviceId}`} style={{ fontWeight: 500 }}>{r.deviceName}</Link><div className="cap">{r.deviceModel} · {r.address}</div></> : <><span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}><ColorSwatch color={colorOf(r)} />{r.supply.description}</span><div className="cap">{typeLabel(typeOf(r))}</div></>}</td>
                <td><LevelBar percent={r.supply.percent} filled={r.supply.class === 'filled'} /></td>
                <td>{r.consumable ? <><ConsumableLink c={r.consumable} />{r.linkSource === 'manual' && <span className="cap"> · manual link</span>}</> : <Link to="/consumables?tab=unmatched" className="cap">unmatched →</Link>}</td>
                <td>{r.consumable ? <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}><StockPill status={statusFor(r.consumable)} xs /><span className="num">{r.consumable.onHand} in stock</span></span> : <span className="cap">—</span>}</td>
                <td className="num">{r.forecast?.daysToEmpty != null ? fmtDays(r.forecast.daysToEmpty) : <span className="cap" title={r.forecast?.rate.reason}>not enough history</span>}</td>
              </tr>
            ))}</tbody>
          </table></div>
        </Card>
      ))}
    </>
  )
}

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
  const link = async (s: InstalledSupply, consumableId: string) => { if (!consumableId) return; try { await api.put(`/api/devices/${s.deviceId}/supplies/${s.supply.index}/link`, { consumableId }); load(); onChanged() } catch (e) { setMsg((e as Error).message) } }
  const act = async (ev: StockEvent, what: 'confirm' | 'dismiss') => { try { await api.post(`/api/consumables/replacements/${ev.id}/${what}`); load(); onChanged() } catch (e) { setMsg((e as Error).message) } }
  const itemName = (id: string) => items.find((i) => i.id === id)?.name || id.slice(0, 8)
  return (
    <div className="grid-2">
      <Card title="Suggested from discovered supplies" id="um" sub="reported by devices, not yet stocked">
        {msg && <div className="cf error small">{msg}</div>}
        {unmatched.length === 0 ? <div className="cb"><EmptyState tone="ok" icon={<Ic.ok size={20} />} title="Every installed supply is linked" text="New devices' supplies will show up here when they have no matching stock item." /></div> : unmatched.map((s) => (
          <div className="lrow" key={`${s.deviceId}:${s.supply.index}`}>
            <ColorSwatch color={s.suggestion?.color || s.supply.colorant} />
            <div style={{ flex: 1, minWidth: 0 }}><div style={{ fontWeight: 500 }}>{s.supply.description}</div><div className="cap"><Link to={`/devices/${s.deviceId}`}>{s.deviceName}</Link> · {s.deviceModel} · {typeLabel(s.suggestion?.type || s.supply.type)}</div></div>
            <LevelBar percent={s.supply.percent} filled={s.supply.class === 'filled'} />
            {canEdit && (<><label className="chip"><select aria-label="Link to stock item" value="" onChange={(e) => link(s, e.target.value)}><option value="">Link to…</option>{items.map((i) => <option key={i.id} value={i.id}>{i.name}{i.color ? ` (${i.color})` : ''}</option>)}</select></label><Btn sm onClick={() => setCreating({ sup: s })}>Use</Btn></>)}
          </div>
        ))}
      </Card>
      <Card title="Replacements awaiting confirmation" id="rp" sub="a level jumped back up, so a cartridge was probably replaced">
        {pending.length === 0 ? <div className="cb cap">Nothing pending. Change the behaviour under Settings → System & consumables.</div> : pending.map((ev) => (
          <div className="lrow" key={ev.id}>
            <Ic.consumables style={{ color: 'var(--accent)' }} />
            <div style={{ flex: 1, minWidth: 0 }}><Link to={`/consumables/${ev.consumableId}`} style={{ fontWeight: 500 }}>{itemName(ev.consumableId)}</Link><div className="cap">{ev.reason} · {ago(ev.at)}{ev.deviceId && <> · <Link to={`/devices/${ev.deviceId}`}>device</Link></>}</div></div>
            {canEdit && <><Btn sm variant="pri" onClick={() => act(ev, 'confirm')}>Confirm −1</Btn><Btn sm variant="ghost" onClick={() => act(ev, 'dismiss')}>Dismiss</Btn></>}
          </div>
        ))}
      </Card>
      {creating && <ConsumableForm initial={creating.sup.suggestion as Partial<Consumable>} onClose={() => setCreating(null)} onSaved={async (c) => { const s = creating.sup; setCreating(null); try { await api.put(`/api/devices/${s.deviceId}/supplies/${s.supply.index}/link`, { consumableId: c.id }) } catch { /* auto-link may already cover it */ } load(); onChanged() }} />}
    </div>
  )
}
