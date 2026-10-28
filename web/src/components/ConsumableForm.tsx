import { useEffect, useState } from 'react'
import { api } from '../api'
import type { Consumable, ConsumableSettings, Device } from '../types'
import { consumableTypes } from '../types'
import { typeLabels } from './consumables'
import { Ic } from './icons'
import { Btn } from './ui'

type Draft = Omit<Consumable, 'id' | 'createdAt' | 'updatedAt'>

const empty = (s?: ConsumableSettings): Draft => ({
  name: '', type: 'toner', color: '', partNumber: '', compatibleModels: [], unit: 'each', onHand: 0,
  warningAt: s?.defaultWarningAt ?? 2, criticalAt: s?.defaultCriticalAt ?? 1, location: '', notes: '', reorderUrl: '',
})

/** Add / edit consumable, rendered as a side drawer. All values are operator-owned. */
export default function ConsumableForm({ initial, existing, onClose, onSaved }: {
  initial?: Partial<Consumable>; existing?: Consumable; onClose: () => void; onSaved: (c: Consumable) => void
}) {
  const [settings, setSettings] = useState<ConsumableSettings | null>(null)
  const [form, setForm] = useState<Draft>({ ...empty(), ...(existing || initial || {}) } as Draft)
  const [models, setModels] = useState<string[]>([])
  const [modelInput, setModelInput] = useState('')
  const [cost, setCost] = useState(existing?.unitCost !== undefined ? String(existing.unitCost) : '')
  const [msg, setMsg] = useState('')
  const [busy, setBusy] = useState(false)
  useEffect(() => {
    api.get<ConsumableSettings>('/api/consumables/settings').then((s) => { setSettings(s); if (!existing && !initial?.warningAt) setForm((f) => ({ ...f, warningAt: s.defaultWarningAt, criticalAt: s.defaultCriticalAt })) }).catch(() => {})
    api.get<{ devices: Device[] }>('/api/devices').then((r) => setModels(Array.from(new Set(r.devices.map((d) => d.model || '').filter((m) => m !== ''))).sort())).catch(() => {})
  }, [existing, initial])
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  const addModel = (m: string) => { const v = m.trim(); if (v && !form.compatibleModels.includes(v)) setForm({ ...form, compatibleModels: [...form.compatibleModels, v] }); setModelInput('') }
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true); setMsg('')
    try {
      const body: Record<string, unknown> = { ...form }
      if (cost.trim() !== '') body.unitCost = Number(cost); else if (existing) body.clearUnitCost = true
      let saved: Consumable
      if (existing) {
        delete body.onHand
        saved = await api.patch<Consumable>(`/api/consumables/${existing.id}`, body)
      } else {
        saved = await api.post<Consumable>('/api/consumables', body)
      }
      onSaved(saved)
    } catch (err) { setMsg((err as Error).message) } finally { setBusy(false) }
  }
  return (
    <div className="drawer-backdrop" onClick={onClose}>
      <form className="drawer" onClick={(e) => e.stopPropagation()} onSubmit={save} aria-label={existing ? 'Edit consumable' : 'Add consumable'}>
        <div className="row between"><h2 className="h2" style={{ fontSize: 16 }}>{existing ? 'Edit consumable' : 'Add consumable'}</h2><Btn sm variant="ghost" ic onClick={onClose} aria-label="Close"><Ic.x /></Btn></div>
        <div className="banner accent" role="note"><span className="mk op"><Ic.user /></span><span>Every field here is <b>operator-owned</b>. Scans and polls only read device levels — they never change these values or the on-hand count.</span></div>
        <label className="cap">Name<input className="fin" value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required autoFocus style={{ marginTop: 4 }} /><div className="hint" style={{ fontSize: 12, color: 'var(--text-3)', marginTop: 4 }}>How it appears in lists, e.g. “Cyan toner · C-750”.</div></label>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
          <label className="cap">Type<select className="fin" value={form.type} onChange={(e) => setForm({ ...form, type: e.target.value as Consumable['type'] })} style={{ marginTop: 4 }}>{consumableTypes.map((t) => <option key={t} value={t}>{typeLabels[t]}</option>)}</select></label>
          <label className="cap">Colour<input className="fin" list="pm-colors" value={form.color} onChange={(e) => setForm({ ...form, color: e.target.value })} placeholder="black, cyan, magenta, yellow…" style={{ marginTop: 4 }} />
            <datalist id="pm-colors">{['black', 'cyan', 'magenta', 'yellow', 'light cyan', 'light magenta', 'gray'].map((c) => <option key={c} value={c} />)}</datalist></label>
          <label className="cap">Part number / SKU<input className="fin mono" value={form.partNumber} onChange={(e) => setForm({ ...form, partNumber: e.target.value })} placeholder="matched against device-reported supplies" style={{ marginTop: 4 }} /></label>
          <label className="cap">Unit<input className="fin" value={form.unit} onChange={(e) => setForm({ ...form, unit: e.target.value })} style={{ marginTop: 4 }} /></label>
        </div>
        <div className="cap">Compatible models
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, alignItems: 'center', marginTop: 4 }}>
            {form.compatibleModels.map((m) => <span key={m} className="tag mono" style={{ fontSize: 11 }}>{m}<button type="button" className="link" aria-label={`remove ${m}`} onClick={() => setForm({ ...form, compatibleModels: form.compatibleModels.filter((x) => x !== m) })} style={{ display: 'inline-flex', color: 'var(--text-3)' }}><Ic.x size={12} /></button></span>)}
            <input className="fin" style={{ flex: '1 1 160px' }} list="pm-models" value={modelInput} placeholder="type a model and press Enter" onChange={(e) => setModelInput(e.target.value)}
              onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ',') { e.preventDefault(); addModel(modelInput) } }} onBlur={() => modelInput && addModel(modelInput)} />
            <datalist id="pm-models">{models.map((m) => <option key={m} value={m} />)}</datalist>
          </div>
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3, 1fr)', gap: 10 }}>
          {!existing && <label className="cap">Starting on hand<input className="fin num" type="number" min={0} value={form.onHand} onChange={(e) => setForm({ ...form, onHand: Math.max(0, Number(e.target.value) || 0) })} style={{ marginTop: 4 }} /></label>}
          <label className="cap" style={{ color: 'var(--warn)', fontWeight: 500 }}>Warning at or below<input className={`fin num${form.criticalAt >= form.warningAt ? '' : ''}`} type="number" min={0} value={form.warningAt} onChange={(e) => setForm({ ...form, warningAt: Math.max(0, Number(e.target.value) || 0) })} style={{ marginTop: 4 }} /></label>
          <label className="cap" style={{ color: 'var(--crit)', fontWeight: 500 }}>Critical at or below<input className={`fin num${form.criticalAt >= form.warningAt && form.warningAt > 0 ? ' err' : ''}`} type="number" min={0} value={form.criticalAt} aria-invalid={form.criticalAt >= form.warningAt && form.warningAt > 0} onChange={(e) => setForm({ ...form, criticalAt: Math.max(0, Number(e.target.value) || 0) })} style={{ marginTop: 4 }} /></label>
        </div>
        {form.criticalAt >= form.warningAt && form.warningAt > 0 && <div className="error small" style={{ display: 'flex', alignItems: 'center', gap: 5 }}><Ic.info size={13} />Critical should be lower than warning.</div>}
        {settings && !existing && <div className="cap">Defaults from Settings: warning {settings.defaultWarningAt}, critical {settings.defaultCriticalAt}. Alerts appear in the stream with source “consumable”.</div>}
        <label className="cap">Storeroom / location<input className="fin" value={form.location} onChange={(e) => setForm({ ...form, location: e.target.value })} style={{ marginTop: 4 }} /></label>
        <label className="cap">Notes<textarea className="fin" rows={2} value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} style={{ marginTop: 4 }} /></label>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
          <label className="cap">Reorder link · optional<input className="fin mono" type="url" value={form.reorderUrl} onChange={(e) => setForm({ ...form, reorderUrl: e.target.value })} placeholder="https://…" style={{ marginTop: 4 }} /></label>
          <label className="cap">Unit cost · reference only<input className="fin num" type="number" step="0.01" min={0} value={cost} onChange={(e) => setCost(e.target.value)} style={{ marginTop: 4 }} /></label>
        </div>
        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, marginTop: 'auto', paddingTop: 12, borderTop: '1px solid var(--border)' }}><span className="error small" style={{ marginRight: 'auto' }}>{msg}</span><Btn variant="ghost" onClick={onClose}>Cancel</Btn><Btn type="submit" variant="pri" disabled={busy}>{busy ? 'Saving…' : existing ? 'Save changes' : 'Add consumable'}</Btn></div>
        {existing && <div className="cap">On-hand quantity is changed with the Adjust stock control so the history stays complete.</div>}
      </form>
    </div>
  )
}
