import { useEffect, useState } from 'react'
import { api } from '../api'
import type { Consumable, ConsumableSettings, Device } from '../types'
import { consumableTypes } from '../types'
import { typeLabels } from './consumables'

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
      <form className="drawer" onClick={(e) => e.stopPropagation()} onSubmit={save}>
        <div className="row between"><h2 style={{ margin: 0 }}>{existing ? 'Edit consumable' : 'Add consumable'}</h2><button type="button" className="link" onClick={onClose}>Close</button></div>
        <div className="muted small">Operator-owned values · never overwritten by scans.</div>
        <label className="field">Name<input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} required autoFocus /></label>
        <div className="grid cols-2">
          <label className="field">Type<select value={form.type} onChange={(e) => setForm({ ...form, type: e.target.value as Consumable['type'] })}>{consumableTypes.map((t) => <option key={t} value={t}>{typeLabels[t]}</option>)}</select></label>
          <label className="field">Colour<input list="pm-colors" value={form.color} onChange={(e) => setForm({ ...form, color: e.target.value })} placeholder="black, cyan, C/M/Y/K…" />
            <datalist id="pm-colors">{['black', 'cyan', 'magenta', 'yellow', 'light cyan', 'light magenta', 'gray'].map((c) => <option key={c} value={c} />)}</datalist></label>
          <label className="field">Part number / SKU<input value={form.partNumber} onChange={(e) => setForm({ ...form, partNumber: e.target.value })} placeholder="matched against supply descriptions" /></label>
          <label className="field">Unit<input value={form.unit} onChange={(e) => setForm({ ...form, unit: e.target.value })} /></label>
        </div>
        <label className="field">Compatible models
          <div className="chips editable">
            {form.compatibleModels.map((m) => <span key={m} className="chip">{m} <button type="button" className="link" aria-label={`remove ${m}`} onClick={() => setForm({ ...form, compatibleModels: form.compatibleModels.filter((x) => x !== m) })}>×</button></span>)}
            <input list="pm-models" value={modelInput} placeholder="type a model and press Enter" onChange={(e) => setModelInput(e.target.value)}
              onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ',') { e.preventDefault(); addModel(modelInput) } }} onBlur={() => modelInput && addModel(modelInput)} />
            <datalist id="pm-models">{models.map((m) => <option key={m} value={m} />)}</datalist>
          </div>
        </label>
        <div className="grid cols-3">
          {!existing && <label className="field">Starting on-hand<input type="number" min={0} value={form.onHand} onChange={(e) => setForm({ ...form, onHand: Math.max(0, Number(e.target.value) || 0) })} /></label>}
          <label className="field">Warning at ≤<input type="number" min={0} value={form.warningAt} onChange={(e) => setForm({ ...form, warningAt: Math.max(0, Number(e.target.value) || 0) })} /></label>
          <label className="field">Critical at ≤<input type="number" min={0} value={form.criticalAt} onChange={(e) => setForm({ ...form, criticalAt: Math.max(0, Number(e.target.value) || 0) })} /></label>
        </div>
        {settings && !existing && <div className="muted small">Defaults from Settings: warning {settings.defaultWarningAt}, critical {settings.defaultCriticalAt}.</div>}
        <label className="field">Storeroom / location<input value={form.location} onChange={(e) => setForm({ ...form, location: e.target.value })} /></label>
        <label className="field">Notes<textarea rows={2} value={form.notes} onChange={(e) => setForm({ ...form, notes: e.target.value })} /></label>
        <div className="grid cols-2">
          <label className="field">Reorder link (optional)<input type="url" value={form.reorderUrl} onChange={(e) => setForm({ ...form, reorderUrl: e.target.value })} placeholder="https://…" /></label>
          <label className="field">Unit cost (reference only)<input type="number" step="0.01" min={0} value={cost} onChange={(e) => setCost(e.target.value)} /></label>
        </div>
        <div className="row" style={{ marginTop: 8 }}><button className="primary" disabled={busy}>{busy ? 'Saving…' : existing ? 'Save changes' : 'Add consumable'}</button><button type="button" onClick={onClose}>Cancel</button><span className="error small">{msg}</span></div>
        {existing && <div className="muted small">On-hand quantity is changed with the Adjust stock control so the history stays complete.</div>}
      </form>
    </div>
  )
}
