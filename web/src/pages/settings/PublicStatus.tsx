import { useCallback, useEffect, useMemo, useState } from 'react'
import { api } from '../../api'
import type { Device, Group } from '../../types'
import { deviceName } from '../../types'
import type { PublicStatusConfig, PublicStatusShow } from '../../types-public-config'
import { Ic } from '../../components/icons'
import { Banner, Btn, Card, ChipSelect, EmptyState, SearchBox } from '../../components/ui'

// Settings → Public status (admin): what the unauthenticated board at /status
// shows. Spec: intent/public-status-dashboard/spec.md, R13–R15. The board
// previews the saved configuration only, so the address and the preview link
// follow what the server last returned, not unsaved edits.

const DEFAULT_TITLE = 'Printer status'
const TITLE_MAX = 60
const LABEL_MAX = 80
const MAX_DEVICES = 200
const CANDIDATES_SHOWN = 50

const empty: PublicStatusConfig = {
  enabled: false, unlisted: false, slug: '', title: DEFAULT_TITLE, frameOrigins: [], devices: [],
  show: { location: true, online: true, state: false, paper: true, toner: true, alerts: false },
}

const SIGNALS: { k: keyof PublicStatusShow; label: string; summary: string; hint: string }[] = [
  { k: 'location', label: 'Location', summary: 'location', hint: 'The location written on each printer’s record in the console.' },
  { k: 'online', label: 'Up or down', summary: 'up/down', hint: 'Whether the console could reach the printer at its last check. When off, a printer that did not answer reads No status.' },
  { k: 'state', label: 'Printer state', summary: 'state', hint: 'The printer’s own word for what it is doing, such as ready, sleeping or printing.' },
  { k: 'paper', label: 'Paper', summary: 'paper', hint: 'How full each paper tray is, where the printer reports it.' },
  { k: 'toner', label: 'Toner', summary: 'toner', hint: 'How much toner or ink is left in each supply.' },
  { k: 'alerts', label: 'Alerts', summary: 'alerts', hint: 'Open alerts as a short category such as jam or toner; never the alert text.' },
]

// scheme://host[:port], nothing after it: what the browser compares for
// frame-ancestors. IPv6 hosts are accepted in brackets.
const ORIGIN_RE = /^[a-z][a-z0-9+.-]*:\/\/(\[[0-9a-f:.]+\]|[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*)(:\d{1,5})?$/i

function normalise(c: PublicStatusConfig | null | undefined): PublicStatusConfig {
  if (!c) return { ...empty, show: { ...empty.show } }
  return {
    enabled: !!c.enabled, unlisted: !!c.unlisted, slug: c.slug || '', title: c.title || DEFAULT_TITLE,
    frameOrigins: c.frameOrigins || [], devices: (c.devices || []).map((d) => ({ id: d.id, label: d.label || '' })),
    show: { ...empty.show, ...(c.show || {}) },
  }
}

export function boardPath(c: Pick<PublicStatusConfig, 'unlisted' | 'slug'>): string {
  return c.unlisted && c.slug ? `/status/${c.slug}` : '/status'
}

export function summaryLine(c: PublicStatusConfig): string {
  const n = c.devices.length
  const printers = `${n} printer${n === 1 ? '' : 's'}`
  const shown = SIGNALS.filter((s) => c.show[s.k]).map((s) => s.summary)
  return `${printers}; ${shown.length ? shown.join(', ') : 'names only'}`
}

function SRow({ label, hint, htmlFor, children }: { label: React.ReactNode; hint?: React.ReactNode; htmlFor?: string; children: React.ReactNode }) {
  return <div className="srow2"><span>{htmlFor ? <label htmlFor={htmlFor}>{label}</label> : label}{hint && <div className="cap">{hint}</div>}</span><div>{children}</div></div>
}

function Check({ checked, onChange, children, disabled }: { checked: boolean; onChange: (v: boolean) => void; children: React.ReactNode; disabled?: boolean }) {
  return <label className="cap" style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}><input className="cbx" type="checkbox" checked={checked} disabled={disabled} onChange={(e) => onChange(e.target.checked)} />{children}</label>
}

export default function PublicStatusSection() {
  const [saved, setSaved] = useState<PublicStatusConfig | null>(null)
  const [form, setForm] = useState<PublicStatusConfig>(empty)
  const [originsText, setOriginsText] = useState('')
  const [devices, setDevices] = useState<Device[]>([])
  const [groups, setGroups] = useState<Group[]>([])
  const [q, setQ] = useState('')
  const [group, setGroup] = useState('')
  const [msg, setMsg] = useState<{ tone: 'ok' | 'warn' | 'crit'; text: string } | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const adopt = (c: PublicStatusConfig) => { const n = normalise(c); setSaved(n); setForm(n); setOriginsText(n.frameOrigins.join('\n')) }

  const load = useCallback(() => {
    api.get<PublicStatusConfig>('/api/settings/public-status').then(adopt).catch((e) => setError((e as Error).message))
    api.get<{ devices: Device[] }>('/api/devices').then((r) => setDevices(r.devices || [])).catch(() => {})
    api.get<Group[]>('/api/groups').then((g) => setGroups(g || [])).catch(() => {})
  }, [])
  useEffect(() => { load() }, [load])

  const byId = useMemo(() => new Map(devices.map((d) => [d.id, d])), [devices])
  const chosen = useMemo(() => new Set(form.devices.map((d) => d.id)), [form.devices])
  const origins = originsText.split('\n').map((x) => x.trim()).filter(Boolean)
  const badOrigins = origins.filter((o) => !ORIGIN_RE.test(o))

  const candidates = useMemo(() => {
    const needle = q.trim().toLowerCase()
    const members = group ? new Set(groups.find((g) => g.id === group)?.memberIds || []) : null
    return devices
      .filter((d) => !chosen.has(d.id))
      .filter((d) => !members || members.has(d.id))
      .filter((d) => !needle || [deviceName(d), d.address, d.location, d.model, d.make, d.hostname].some((v) => v && v.toLowerCase().includes(needle)))
      .sort((a, b) => deviceName(a).localeCompare(deviceName(b)))
  }, [devices, groups, chosen, q, group])

  const set = <K extends keyof PublicStatusConfig>(k: K, v: PublicStatusConfig[K]) => setForm((f) => ({ ...f, [k]: v }))
  const setShow = (k: keyof PublicStatusShow, v: boolean) => setForm((f) => ({ ...f, show: { ...f.show, [k]: v } }))
  const add = (id: string) => setForm((f) => (f.devices.length >= MAX_DEVICES || f.devices.some((d) => d.id === id) ? f : { ...f, devices: [...f.devices, { id, label: '' }] }))
  const remove = (i: number) => setForm((f) => ({ ...f, devices: f.devices.filter((_, j) => j !== i) }))
  const move = (i: number, by: -1 | 1) => setForm((f) => {
    const j = i + by
    if (j < 0 || j >= f.devices.length) return f
    const next = [...f.devices];[next[i], next[j]] = [next[j], next[i]]
    return { ...f, devices: next }
  })
  const setLabel = (i: number, label: string) => setForm((f) => ({ ...f, devices: f.devices.map((d, j) => (j === i ? { ...d, label } : d)) }))

  // The slug is server-owned: the PUT body carries regenerateSlug instead of a new value.
  const body = (regenerateSlug: boolean): PublicStatusConfig & { regenerateSlug?: boolean } => ({
    ...form, title: form.title.trim() || DEFAULT_TITLE, frameOrigins: origins,
    devices: form.devices.map((d) => ({ id: d.id, label: d.label.trim() })),
    ...(regenerateSlug ? { regenerateSlug: true } : {}),
  })

  const put = async (regenerateSlug: boolean, okText: string) => {
    if (badOrigins.length) { setMsg({ tone: 'crit', text: 'Fix the framing origins before saving.' }); return }
    setMsg(null); setBusy(true)
    try {
      const c = await api.put<PublicStatusConfig>('/api/settings/public-status', body(regenerateSlug))
      adopt(c)
      const n = normalise(c)
      setMsg(n.devices.length === 0
        ? { tone: 'warn', text: `${okText} No printers are chosen, so the board will be empty.` }
        : { tone: 'ok', text: okText })
    } catch (err) { setMsg({ tone: 'crit', text: (err as Error).message }) } finally { setBusy(false) }
  }
  const save = (e: React.FormEvent) => { e.preventDefault(); put(false, 'Saved.') }
  const regenerate = () => {
    if (!confirm('Make a new address for the board? The current link stops working, and any other changes on this page are saved too.')) return
    put(true, 'New address saved. The old link no longer works.')
  }

  if (error) return <Banner tone="crit">{error}</Banner>
  if (!saved) return <Card title="Public status board"><div className="cb cap">Loading…</div></Card>

  const savedPath = boardPath(saved)
  const savedUrl = `${window.location.origin}${savedPath}`
  const unlistedPending = form.unlisted !== saved.unlisted

  return (
    <form className="card" onSubmit={save} autoComplete="off" aria-labelledby="pubh">
      <div className="ch">
        <div><h2 className="h2" id="pubh">Public status board</h2><div className="cap">a read-only page anyone on your network can open without signing in</div></div>
        <div className="row" style={{ gap: 8 }}>
          {saved.enabled && <a className="btn sm" href={savedPath} target="_blank" rel="noopener noreferrer"><Ic.external size={13} />Open the board</a>}
        </div>
      </div>
      <div className="cap" style={{ padding: '10px 16px', borderBottom: '1px solid var(--border)' }}>
        Choose which printers appear and what people see about them. The board never shows addresses, serial numbers, models or anything from security checks. It shows: <b style={{ color: 'var(--text)' }}>{summaryLine(form)}</b>.
      </div>

      <SRow label="Board" hint="off answers “not available” at the address">
        <Check checked={form.enabled} onChange={(v) => set('enabled', v)}>Publish the board</Check>
      </SRow>
      <SRow label="Address" hint={<>An unlisted address hides the board from people who do not have the link; it is not a sign-in.</>}>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <Check checked={form.unlisted} onChange={(v) => set('unlisted', v)}>Use an unlisted address</Check>
          <div className="row" style={{ gap: 8 }}>
            <span className="mono" style={{ wordBreak: 'break-all' }}>{savedUrl}</span>
            <Btn sm disabled={!form.unlisted || busy} onClick={regenerate}><Ic.refresh size={13} />Regenerate address</Btn>
          </div>
          {unlistedPending && <span className="cap" style={{ color: 'var(--warn)' }}>{form.unlisted ? 'Save to move the board to a new unlisted address.' : 'Save to move the board back to /status.'}</span>}
        </div>
      </SRow>
      <SRow label="Board title" htmlFor="pub-title" hint={`shown at the top of the board · up to ${TITLE_MAX} characters`}>
        <input className="fin" id="pub-title" style={{ width: '100%', maxWidth: 360 }} maxLength={TITLE_MAX} placeholder={DEFAULT_TITLE} value={form.title} onChange={(e) => set('title', e.target.value)} />
      </SRow>
      <SRow label="Allow framing from" htmlFor="pub-frame" hint="sites allowed to show the board inside their own pages, one per line; empty keeps framing blocked">
        <textarea className="fin mono" id="pub-frame" rows={3} style={{ width: '100%' }} placeholder="https://intranet.example.com" value={originsText} aria-invalid={badOrigins.length > 0} onChange={(e) => setOriginsText(e.target.value)} />
        {badOrigins.length > 0 && <div className="error small" style={{ marginTop: 6 }}>Not an origin: {badOrigins.join(', ')}. Use scheme://host or scheme://host:port, with no path.</div>}
      </SRow>

      <div className="cap" style={{ padding: '12px 16px 4px', fontWeight: 600 }}>Printers on the board</div>
      <div style={{ padding: '8px 16px 12px', borderBottom: '1px solid var(--border)' }}>
        {form.devices.length === 0 ? (
          <Banner tone="warn">No printers chosen. You can save, but the board will be empty.</Banner>
        ) : (
          <div className="tw"><table className="tbl" style={{ minWidth: 560 }}><thead><tr><th>#</th><th>Printer</th><th>Public label</th><th /></tr></thead><tbody>
            {form.devices.map((sel, i) => {
              const d = byId.get(sel.id)
              return (
                <tr key={sel.id}>
                  <td className="num cap">{i + 1}</td>
                  <td>{d ? <><div style={{ fontWeight: 500 }}>{deviceName(d)}</div><div className="cap">{[d.location, d.address].filter(Boolean).join(' · ')}</div></> : <span className="cap">Printer no longer in the console</span>}</td>
                  <td><input className="fin" style={{ width: '100%', minWidth: 200 }} maxLength={LABEL_MAX} placeholder="Public label, optional" aria-label={`Public label for ${d ? deviceName(d) : 'printer'}`} value={sel.label} onChange={(e) => setLabel(i, e.target.value)} /></td>
                  <td className="r" style={{ whiteSpace: 'nowrap' }}>
                    <Btn sm variant="ghost" disabled={i === 0} onClick={() => move(i, -1)}>Up</Btn>
                    <Btn sm variant="ghost" disabled={i === form.devices.length - 1} onClick={() => move(i, 1)}>Down</Btn>
                    <Btn sm variant="ghost" onClick={() => remove(i)}>Remove</Btn>
                  </td>
                </tr>
              )
            })}
          </tbody></table></div>
        )}
        <div className="cap" style={{ marginTop: 8 }}>The board lists printers in this order. A public label replaces the console’s name on the board only. Up to {MAX_DEVICES} printers.</div>
      </div>

      <div className="cap" style={{ padding: '12px 16px 4px', fontWeight: 600 }}>Add printers</div>
      <div style={{ padding: '8px 16px 12px', borderBottom: '1px solid var(--border)' }}>
        <div className="row" style={{ gap: 8, marginBottom: 8 }}>
          <SearchBox sm value={q} onChange={setQ} placeholder="Search by name, address or location" />
          <ChipSelect label="Group" value={group} onChange={setGroup} options={groups.map((g) => ({ v: g.id, label: g.name }))} />
        </div>
        {devices.length === 0 ? (
          <EmptyState icon={<Ic.printer size={20} />} title="No printers yet" text="Printers appear here once discovery has found them." tone="muted" />
        ) : candidates.length === 0 ? (
          <div className="cap">{q || group ? 'No printers match.' : 'Every printer is already on the board.'}</div>
        ) : (
          <>
            <div className="tw"><table className="tbl" style={{ minWidth: 480 }}><tbody>
              {candidates.slice(0, CANDIDATES_SHOWN).map((d) => (
                <tr key={d.id}>
                  <td><div style={{ fontWeight: 500 }}>{deviceName(d)}</div><div className="cap">{[d.location, d.address, d.model].filter(Boolean).join(' · ')}</div></td>
                  <td className="r"><Btn sm disabled={form.devices.length >= MAX_DEVICES} onClick={() => add(d.id)}><Ic.plus size={13} />Add</Btn></td>
                </tr>
              ))}
            </tbody></table></div>
            {candidates.length > CANDIDATES_SHOWN && <div className="cap" style={{ marginTop: 8 }}>Showing {CANDIDATES_SHOWN} of {candidates.length}. Narrow the search to find others.</div>}
          </>
        )}
      </div>

      <div className="cap" style={{ padding: '12px 16px 4px', fontWeight: 600 }}>What the board shows for each printer</div>
      {SIGNALS.map((s) => (
        <SRow key={s.k} label={s.label} hint={s.hint}>
          <Check checked={form.show[s.k]} onChange={(v) => setShow(s.k, v)}>Show {s.label.toLowerCase()}</Check>
        </SRow>
      ))}

      <div className="cf" style={{ display: 'flex', justifyContent: 'flex-end', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
        {msg && <span className={msg.tone === 'crit' ? 'error small' : 'cap'} style={{ marginRight: 'auto', color: msg.tone === 'warn' ? 'var(--warn)' : undefined }} role="status">{msg.text}</span>}
        <Btn type="submit" variant="pri" disabled={busy || badOrigins.length > 0}>Save changes</Btn>
      </div>
    </form>
  )
}
