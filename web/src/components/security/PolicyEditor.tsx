// Policy editor drawer: items are built from the security catalogue
// (GET /api/security/catalog) so each value input is typed per key.
import { useMemo, useState } from 'react'
import { api } from '../../api'
import { Ic } from '../icons'
import { Banner, Btn, Tag } from '../ui'
import type { ItemSeverity, ManagementPath, Policy, PolicyItem, PolicyOperator, SecurityCatalog, SettingDef } from '../../types-security'
import { impliedManagementPath } from '../../types-security'
import { MgmtPathTag, SevMark, operatorLabel } from './common'

const areaOrder = ['protocols', 'security', 'network', 'identity', 'other']

function slug(key: string, existing: string[]): string {
  const base = key.replace(/[^a-z0-9]+/gi, '-').replace(/^-|-$/g, '').toLowerCase() || 'item'
  let s = base, n = 2
  while (existing.includes(s)) s = `${base}-${n++}`
  return s
}

/** Parse the text form of a value according to the catalogue type and the operator. */
function parseValue(text: string, def: SettingDef | undefined, op: PolicyOperator): unknown {
  if (op === 'set' || op === 'unset') return undefined
  const one = (t: string): unknown => {
    const v = t.trim()
    if (!def) return v
    if (def.Type === 'bool') return v === 'true'
    if (def.Type === 'int') { const n = Number(v); return isNaN(n) ? v : n }
    return v
  }
  if (op === 'in' || op === 'notIn' || def?.Type === 'list') return text.split(',').map((s) => s.trim()).filter(Boolean).map(one)
  return one(text)
}
function textOf(v: unknown): string { return v === undefined || v === null ? '' : Array.isArray(v) ? v.map(String).join(', ') : String(v) }

export default function PolicyEditor({ policy, catalog, canEdit, onClose, onSaved, onDeleted }: { policy: Policy | null; catalog: SecurityCatalog; canEdit: boolean; onClose: () => void; onSaved: (p: Policy) => void; onDeleted?: () => void }) {
  const [name, setName] = useState(policy?.name || '')
  const [description, setDescription] = useState(policy?.description || '')
  const [items, setItems] = useState<PolicyItem[]>(policy?.items.map((i) => ({ ...i })) || [])
  const [values, setValues] = useState<Record<string, string>>(Object.fromEntries((policy?.items || []).map((i) => [i.id, textOf(i.value)])))
  const [remValues, setRemValues] = useState<Record<string, string>>(Object.fromEntries((policy?.items || []).map((i) => [i.id, textOf(i.remediation?.value)])))
  const [msg, setMsg] = useState('')
  const [busy, setBusy] = useState(false)
  const defs = useMemo(() => Object.fromEntries(catalog.settings.map((d) => [d.Key, d])) as Record<string, SettingDef>, [catalog])
  const grouped = useMemo(() => {
    const m = new Map<string, SettingDef[]>()
    for (const d of catalog.settings) (m.get(d.Area) || m.set(d.Area, []).get(d.Area)!).push(d)
    return Array.from(m.entries()).sort((a, b) => (areaOrder.indexOf(a[0]) + 99) % 99 - (areaOrder.indexOf(b[0]) + 99) % 99)
  }, [catalog])

  const update = (id: string, patch: Partial<PolicyItem>) => setItems((list) => list.map((i) => (i.id === id ? { ...i, ...patch } : i)))
  const addItem = (key: string) => {
    const def = defs[key]
    const id = slug(key, items.map((i) => i.id))
    const op: PolicyOperator = def?.Type === 'secret' ? 'set' : 'eq'
    const item: PolicyItem = { id, key, name: def?.Name || key, operator: op, value: def?.Type === 'bool' ? false : def?.Type === 'enum' ? def.Options?.[0] : undefined, severity: 'medium', remediate: def?.Type !== 'secret', managementPath: impliedManagementPath(key) }
    setItems((l) => [...l, item]); setValues((v) => ({ ...v, [id]: textOf(item.value) }))
  }
  const remove = (id: string) => setItems((l) => l.filter((i) => i.id !== id))
  const setValue = (it: PolicyItem, text: string) => { setValues((v) => ({ ...v, [it.id]: text })); update(it.id, { value: parseValue(text, defs[it.key], it.operator) }) }
  const setOp = (it: PolicyItem, op: PolicyOperator) => update(it.id, { operator: op, value: parseValue(values[it.id] || '', defs[it.key], op), remediation: op === 'eq' ? undefined : it.remediation })
  const setRem = (it: PolicyItem, text: string) => { setRemValues((v) => ({ ...v, [it.id]: text })); update(it.id, { remediation: text.trim() === '' ? undefined : { key: it.key, value: parseValue(text, defs[it.key], 'eq') } }) }

  const save = async () => {
    setBusy(true); setMsg('')
    const body = { name: name.trim(), description, items: items.map((i) => ({ ...i, managementPath: i.managementPath || undefined, remediation: i.remediation && i.remediation.value !== undefined ? i.remediation : undefined })) }
    try {
      const p = policy ? await api.patch<Policy>(`/api/policies/${policy.id}`, body) : await api.post<Policy>('/api/policies', body)
      onSaved(p)
    } catch (e) { setMsg((e as Error).message) } finally { setBusy(false) }
  }
  const del = async () => {
    if (!policy || !confirm(`Delete policy “${policy.name}”? Stored assessments stay in history.`)) return
    try { await api.del(`/api/policies/${policy.id}`); onDeleted?.() } catch (e) { setMsg((e as Error).message) }
  }
  const problems = items.filter((i) => !['set', 'unset'].includes(i.operator) && (i.value === undefined || i.value === '' || (Array.isArray(i.value) && i.value.length === 0)))
  const valid = name.trim() !== '' && items.length > 0 && problems.length === 0

  return (
    <div className="drawer-backdrop" onClick={onClose}>
      <div className="drawer" role="dialog" aria-modal="true" aria-label={policy ? 'Edit policy' : 'New policy'} style={{ width: 'min(760px, 100%)' }} onClick={(e) => e.stopPropagation()}>
        <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
          <div><div className="lbl">{policy ? 'Edit policy' : 'New policy'}</div><h2 style={{ fontSize: 16, fontWeight: 600, marginTop: 2 }}>{name || 'Untitled policy'}</h2><div className="cap">{items.length} item{items.length === 1 ? '' : 's'} · severity weights critical 3 · high 2 · medium 1 · low ½</div></div>
          <Btn ic variant="ghost" aria-label="Close" onClick={onClose}><Ic.x /></Btn>
        </div>
        {!canEdit && <Banner tone="info"><Tag>Read-only · Viewer</Tag> Editing policies needs the operator role.</Banner>}
        {msg && <Banner tone="crit">{msg}</Banner>}
        <div style={{ display: 'grid', gridTemplateColumns: 'minmax(180px, 1fr) minmax(200px, 2fr)', gap: 10 }}>
          <label className="cap">Name<input className="fin" value={name} disabled={!canEdit} onChange={(e) => setName(e.target.value)} style={{ marginTop: 4 }} placeholder="e.g. Baseline 2026" /></label>
          <label className="cap">Description<input className="fin" value={description} disabled={!canEdit} onChange={(e) => setDescription(e.target.value)} style={{ marginTop: 4 }} /></label>
        </div>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          {items.length === 0 && <div className="state" style={{ minHeight: 100 }}><b>No items yet</b><span className="cap">Add a setting from the catalogue below; each item says what value is desired and how severe a failure is.</span></div>}
          {items.map((it) => {
            const def = defs[it.key]
            const noValue = it.operator === 'set' || it.operator === 'unset'
            const listy = it.operator === 'in' || it.operator === 'notIn' || def?.Type === 'list'
            const ops: PolicyOperator[] = def?.Type === 'secret' ? ['set', 'unset'] : catalog.operators
            const implied = impliedManagementPath(it.key)
            return (
              <div key={it.id} className="cond" style={{ flexDirection: 'column', alignItems: 'stretch', gap: 8 }}>
                <div className="row" style={{ justifyContent: 'space-between' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8, minWidth: 0, flexWrap: 'wrap' }}>
                    <input className="fin" aria-label="Item name" value={it.name} disabled={!canEdit} onChange={(e) => update(it.id, { name: e.target.value })} style={{ width: 240, fontWeight: 500 }} />
                    <span className="cap mono">{it.key}</span>
                    {def && <Tag>{def.Type}</Tag>}
                    {it.managementPath ? <MgmtPathTag path={it.managementPath} xs /> : null}
                  </div>
                  {canEdit && <Btn sm variant="ghost" ic aria-label={`Remove ${it.name}`} onClick={() => remove(it.id)}><Ic.x /></Btn>}
                </div>
                <div className="row">
                  <select className="fin" aria-label="Operator" value={it.operator} disabled={!canEdit} onChange={(e) => setOp(it, e.target.value as PolicyOperator)} style={{ width: 130 }}>{ops.map((o) => <option key={o} value={o}>{operatorLabel[o]}</option>)}</select>
                  {noValue ? <span className="cap" style={{ flex: '1 1 160px' }}>{def?.Type === 'secret' ? 'secrets are reported as <set> / <unset> only' : 'passes when the setting is present and non-empty'}</span>
                    : def?.Type === 'bool' && !listy ? <select className="fin mono" aria-label="Desired value" value={values[it.id] ?? 'false'} disabled={!canEdit} onChange={(e) => setValue(it, e.target.value)} style={{ flex: '1 1 160px' }}><option value="true">true (enabled)</option><option value="false">false (disabled)</option></select>
                    : def?.Type === 'enum' && !listy ? <select className="fin mono" aria-label="Desired value" value={values[it.id] ?? ''} disabled={!canEdit} onChange={(e) => setValue(it, e.target.value)} style={{ flex: '1 1 160px' }}><option value="">choose…</option>{(def.Options || []).map((o) => <option key={String(o)} value={String(o)}>{String(o)}</option>)}</select>
                    : <input className={`fin mono${problems.includes(it) ? ' err' : ''}`} aria-label="Desired value" type={def?.Type === 'int' && !listy ? 'number' : 'text'} value={values[it.id] ?? ''} disabled={!canEdit} onChange={(e) => setValue(it, e.target.value)} placeholder={listy ? 'value, value, …' : def?.Type === 'int' ? '0' : 'value'} style={{ flex: '1 1 160px' }} />}
                  <label className="chip" style={{ gap: 4 }}><SevMark sev={it.severity} noLabel />Severity<select aria-label="Severity" value={it.severity} disabled={!canEdit} onChange={(e) => update(it.id, { severity: e.target.value as ItemSeverity })}>{catalog.severities.map((s) => <option key={s} value={s}>{s}</option>)}</select></label>
                  <label className="chip" style={{ gap: 6 }} title="When off the item is assess-only and never written"><input className="cbx" type="checkbox" checked={it.remediate} disabled={!canEdit || def?.Type === 'secret'} onChange={(e) => update(it.id, { remediate: e.target.checked })} />Remediate</label>
                  <label className="chip" title="Marks the management channel this change could remove; the engine keeps at least one open (spec §11)"><Ic.shield size={13} />Path<select aria-label="Management path" value={it.managementPath || ''} disabled={!canEdit} onChange={(e) => update(it.id, { managementPath: e.target.value as ManagementPath })}><option value="">none</option>{catalog.managementPaths.map((p) => <option key={p} value={p}>{p.toUpperCase()}{implied === p ? ' (implied)' : ''}</option>)}</select></label>
                </div>
                {it.remediate && it.operator !== 'eq' && !noValue && (
                  <div className="row"><span className="cap" style={{ width: 130 }}>Write on failure</span><input className="fin mono" aria-label="Remediation value" value={remValues[it.id] ?? ''} disabled={!canEdit} onChange={(e) => setRem(it, e.target.value)} placeholder={`value to write to ${it.key} (required for ${operatorLabel[it.operator]})`} style={{ flex: '1 1 200px' }} /></div>
                )}
                {it.description && <div className="cap">{it.description}</div>}
              </div>
            )
          })}
          {canEdit && (
            <div className="row">
              <label className="chip"><Ic.plus />Add item<select aria-label="Add item from catalogue" value="" onChange={(e) => { if (e.target.value) addItem(e.target.value) }}>
                <option value="">setting…</option>
                {grouped.map(([area, list]) => <optgroup key={area} label={area}>{list.map((d) => <option key={d.Key} value={d.Key}>{d.Name} · {d.Key}</option>)}</optgroup>)}
              </select></label>
              <span className="cap">{catalog.settings.length} security-relevant settings in the catalogue</span>
            </div>
          )}
        </div>
        {problems.length > 0 && <div className="cap" style={{ color: 'var(--crit)' }}>{problems.length} item{problems.length === 1 ? ' needs' : 's need'} a desired value.</div>}
        <div style={{ display: 'flex', justifyContent: 'space-between', gap: 8, marginTop: 'auto', paddingTop: 8, borderTop: '1px solid var(--border)' }}>
          <span>{policy && canEdit && <Btn variant="dng" onClick={del}>Delete policy</Btn>}</span>
          <span style={{ display: 'flex', gap: 8 }}><Btn onClick={onClose}>{canEdit ? 'Cancel' : 'Close'}</Btn>{canEdit && <Btn variant="pri" disabled={!valid || busy} onClick={save}>{busy ? 'Saving…' : policy ? 'Save policy' : 'Create policy'}</Btn>}</span>
        </div>
      </div>
    </div>
  )
}
