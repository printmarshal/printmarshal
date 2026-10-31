import { useEffect, useMemo, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api, fmtTime } from '../api'
import type { Group } from '../types'
import type {
  AlertsReport, ComplianceReport, FirmwareReport, FirmwareStatus, HealthReport, ReportName, ReportRange, SuppliesReport, UtilisationReport,
} from '../types-reports'
import { Ic } from '../components/icons'
import { Banner, Btn, Card, ChipSelect, EmptyState, Kpi, Pill, Seg, SevPill, Tabs, Tag, fmtInt, supplyColor } from '../components/ui'
import { fmtDays } from '../components/consumables'

type Tab = 'utilisation' | 'health' | 'supplies' | 'alerts' | 'firmware' | 'compliance'
type Preset = '7' | '30' | '90' | 'custom'

const tabReport: Record<Tab, ReportName> = { utilisation: 'utilisation', health: 'fleet-health', supplies: 'supplies', alerts: 'alerts', firmware: 'firmware', compliance: 'compliance' }
const CAT = ['var(--cat1)', 'var(--cat2)', 'var(--cat3)', 'var(--cat4)', 'var(--cat5)']

function isoDay(d: Date): string { return d.toISOString().slice(0, 10) }

/** Reports: utilisation, fleet health, supplies, alerts, firmware (spec §8.2, §8.3, §8.5). */
export default function Reports() {
  const [params, setParams] = useSearchParams()
  const tab = (params.get('tab') as Tab) || 'utilisation'
  const preset = (params.get('range') as Preset) || '30'
  const group = params.get('group') || ''
  const today = isoDay(new Date())
  const from = params.get('from') || isoDay(new Date(Date.now() - 30 * 86400000))
  const to = params.get('to') || today
  const [groups, setGroups] = useState<Group[]>([])
  // A result remembers which report and query asked for it: `tab` changes before the new response
  // arrives, and every tab reads a different shape, so a result only counts while its key matches.
  const [loaded, setLoaded] = useState<{ key: string; data: object } | null>(null)
  const [failed, setFailed] = useState<{ key: string; message: string } | null>(null)
  const [attempt, setAttempt] = useState(0)

  const set = (kv: Record<string, string>) => { const p = new URLSearchParams(params); for (const [k, v] of Object.entries(kv)) { if (v) p.set(k, v); else p.delete(k) } setParams(p, { replace: true }) }
  const qs = useMemo(() => {
    const p = new URLSearchParams()
    if (preset === 'custom') { p.set('from', from); p.set('to', to) } else p.set('days', preset)
    if (group) p.set('group', group)
    return p.toString()
  }, [preset, from, to, group])

  useEffect(() => { api.get<Group[]>('/api/groups').then(setGroups).catch(() => {}) }, [])
  const key = `${tabReport[tab]}?${qs}`
  useEffect(() => {
    let current = true // cleared when the tab or query moves on, so a late response is ignored
    setFailed(null)
    api.get<object>(`/api/reports/${key}`)
      .then((r) => { if (current) setLoaded({ key, data: r }) })
      .catch((e) => { if (current) { setLoaded(null); setFailed({ key, message: e.message }) } })
    return () => { current = false }
  }, [key, attempt])
  const data = loaded?.key === key ? loaded.data : null
  const error = failed?.key === key ? failed.message : ''

  const csv = (table: string) => `/api/reports/${tabReport[tab]}?${qs}&format=csv&table=${table}`
  const range = (data as { range?: ReportRange } | null)?.range
  const groupName = groups.find((g) => g.id === group)?.name

  return (
    <>
      <div className="ph">
        <div><h1 className="h1">Reports</h1><div className="cap num">{range ? `${range.from.slice(0, 10)} → ${isoDay(new Date(new Date(range.to).getTime() - 1))} · ${range.days} days · ${range.devices} device${range.devices === 1 ? '' : 's'}${groupName ? ` in ${groupName}` : ''}` : 'compliance, utilisation and health over time'}</div></div>
        <div className="row">
          <Seg label="Range" value={preset} onChange={(v) => set({ range: v })} options={[{ v: '7', label: '7d' }, { v: '30', label: '30d' }, { v: '90', label: '90d' }, { v: 'custom', label: 'Custom' }]} />
          {preset === 'custom' && (
            <span className="row" style={{ gap: 4 }}>
              <input type="date" aria-label="From" className="chip" value={from} max={to} onChange={(e) => set({ from: e.target.value })} style={{ height: 30 }} />
              <span className="cap">→</span>
              <input type="date" aria-label="To" className="chip" value={to} min={from} max={today} onChange={(e) => set({ to: e.target.value })} style={{ height: 30 }} />
            </span>
          )}
          <ChipSelect label="Group" value={group} onChange={(v) => set({ group: v })} options={groups.map((g) => ({ v: g.id, label: g.name }))} any="All devices" />
        </div>
      </div>
      <Tabs label="Report" value={tab} onChange={(v) => set({ tab: v })} tabs={[
        { v: 'utilisation', label: 'Utilisation' }, { v: 'health', label: 'Fleet health' }, { v: 'supplies', label: 'Supplies' },
        { v: 'alerts', label: 'Alerts' }, { v: 'firmware', label: 'Firmware' }, { v: 'compliance', label: 'Compliance' },
      ]} />
      {error && <Banner tone="crit" role="alert" right={<Btn sm onClick={() => setAttempt((n) => n + 1)}><Ic.refresh />Retry</Btn>}>{error}</Banner>}
      {!data && !error && <div className="cap" aria-busy="true">Computing report…</div>}
      {data && tab === 'utilisation' && <Utilisation r={data as UtilisationReport} csv={csv} />}
      {data && tab === 'health' && <Health r={data as HealthReport} csv={csv} />}
      {data && tab === 'supplies' && <Supplies r={data as SuppliesReport} csv={csv} />}
      {data && tab === 'alerts' && <AlertsTab r={data as AlertsReport} csv={csv} />}
      {data && tab === 'firmware' && <Firmware r={data as FirmwareReport} csv={csv} />}
      {data && tab === 'compliance' && <Compliance r={data as ComplianceReport} />}
    </>
  )
}

// --- shared bits --------------------------------------------------------------

function ExportLinks({ csv, tables }: { csv: (t: string) => string; tables: { v: string; label: string }[] }) {
  return (
    <span className="row" style={{ gap: 6 }}>
      {tables.map((t) => <a key={t.v} className="btn sm" href={csv(t.v)} download><Ic.download />{t.label}</a>)}
    </span>
  )
}

function Legend({ items }: { items: { label: string; color: string; dashed?: boolean }[] }) {
  return (
    <div className="row cap" style={{ gap: 16 }}>
      {items.map((it) => <span key={it.label} style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}><span style={it.dashed ? { width: 16, height: 0, borderTop: `2px dashed ${it.color}` } : { width: 16, height: 3, background: it.color, borderRadius: 2 }} />{it.label}</span>)}
    </div>
  )
}

function shortDay(d: string): string { return d.slice(5) }

/** Line chart over days: hand-built SVG, categorical colours, dashed second series, hover titles. A null value is a gap (no data that day). */
function LineChart({ days, series, max: maxIn, unit = '', label }: { days: string[]; series: { name: string; color: string; dashed?: boolean; values: (number | null)[] }[]; max?: number; unit?: string; label: string }) {
  if (days.length < 2) return <div className="cap">A trend needs at least two days of history.</div>
  const w = 640, h = 160, x0 = 40, top = 10, bottom = 136
  const max = Math.max(1, maxIn ?? Math.max(...series.flatMap((s) => s.values.map((v) => v ?? 0))))
  const x = (i: number) => x0 + (i / (days.length - 1)) * (w - x0 - 6)
  const y = (v: number) => bottom - (Math.min(v, max) / max) * (bottom - top)
  const mid = Math.floor(days.length / 2)
  const fmt = (v: number) => (v >= 10000 ? `${Math.round(v / 1000)}k` : fmtInt(Math.round(v))) + unit
  // Split each series into contiguous runs so gaps are not bridged.
  const runs = (vals: (number | null)[]) => vals.reduce<{ i: number; v: number }[][]>((acc, v, i) => { if (v === null) { if (acc[acc.length - 1].length) acc.push([]) } else acc[acc.length - 1].push({ i, v }); return acc }, [[]]).filter((r) => r.length)
  const lastOf = (vals: (number | null)[]) => { for (let i = vals.length - 1; i >= 0; i--) if (vals[i] !== null) return { i, v: vals[i] as number }; return null }
  return (
    <svg viewBox={`0 0 ${w} ${h}`} width="100%" height={h} role="img" aria-label={label} style={{ display: 'block' }}>
      <g style={{ stroke: 'var(--border)' }}><line x1={x0} y1={top} x2={w} y2={top} /><line x1={x0} y1={(top + bottom) / 2} x2={w} y2={(top + bottom) / 2} /><line x1={x0} y1={bottom} x2={w} y2={bottom} /></g>
      <g style={{ fill: 'var(--text-3)' }} fontSize="10" fontFamily="IBM Plex Sans, sans-serif" className="num">
        <text x="0" y={top + 4}>{fmt(max)}</text>{max >= 2 && <text x="0" y={(top + bottom) / 2 + 4}>{fmt(max / 2)}</text>}<text x={x0 - 10} y={bottom + 4}>0</text>
        <text x={x0} y={h - 2}>{shortDay(days[0])}</text><text x={x(mid)} y={h - 2} textAnchor="middle">{shortDay(days[mid])}</text><text x={w - 2} y={h - 2} textAnchor="end">{shortDay(days[days.length - 1])}</text>
      </g>
      {series.map((s) => runs(s.values).map((run, k) => run.length === 1
        ? <circle key={`${s.name}-${k}`} cx={x(run[0].i)} cy={y(run[0].v)} r="2" style={{ fill: s.color }} />
        : <polyline key={`${s.name}-${k}`} fill="none" style={{ stroke: s.color }} strokeWidth="2" strokeDasharray={s.dashed ? '5 3' : undefined} strokeLinejoin="round" points={run.map((p) => `${x(p.i).toFixed(1)},${y(p.v).toFixed(1)}`).join(' ')} />))}
      {series.map((s) => { const l = lastOf(s.values); return l && <circle key={s.name + '-last'} cx={x(l.i)} cy={y(l.v)} r="2.5" style={{ fill: s.color }} /> })}
      {days.map((d, i) => {
        const half = (w - x0) / (days.length - 1) / 2
        return <rect key={d} x={x(i) - half} y={top} width={half * 2} height={bottom - top} fill="transparent"><title>{`${d}: ${series.map((s) => `${s.name} ${s.values[i] === null ? 'no data' : fmtInt(Math.round(s.values[i] as number)) + unit}`).join(', ')}`}</title></rect>
      })}
    </svg>
  )
}

/** Stacked bars per day. */
function BarChart({ days, series, label, unit = '' }: { days: string[]; series: { name: string; color: string; values: number[] }[]; label: string; unit?: string }) {
  if (days.length < 1) return <div className="cap">No days in range.</div>
  const w = 640, h = 160, x0 = 40, top = 10, bottom = 136
  const totals = days.map((_, i) => series.reduce((a, s) => a + s.values[i], 0))
  const max = Math.max(1, ...totals)
  const slot = (w - x0 - 6) / days.length
  const bw = Math.max(2, slot * 0.7)
  const y = (v: number) => (v / max) * (bottom - top)
  const mid = Math.floor(days.length / 2)
  return (
    <svg viewBox={`0 0 ${w} ${h}`} width="100%" height={h} role="img" aria-label={label} style={{ display: 'block' }}>
      <g style={{ stroke: 'var(--border)' }}><line x1={x0} y1={top} x2={w} y2={top} /><line x1={x0} y1={(top + bottom) / 2} x2={w} y2={(top + bottom) / 2} /><line x1={x0} y1={bottom} x2={w} y2={bottom} /></g>
      <g style={{ fill: 'var(--text-3)' }} fontSize="10" fontFamily="IBM Plex Sans, sans-serif">
        <text x="0" y={top + 4}>{fmtInt(max)}{unit}</text>{max >= 2 && <text x="0" y={(top + bottom) / 2 + 4}>{fmtInt(Math.round(max / 2))}{unit}</text>}<text x={x0 - 10} y={bottom + 4}>0</text>
        <text x={x0} y={h - 2}>{shortDay(days[0])}</text>{days.length > 2 && <text x={x0 + slot * mid + slot / 2} y={h - 2} textAnchor="middle">{shortDay(days[mid])}</text>}{days.length > 1 && <text x={w - 2} y={h - 2} textAnchor="end">{shortDay(days[days.length - 1])}</text>}
      </g>
      {days.map((d, i) => {
        let acc = 0
        const x = x0 + slot * i + (slot - bw) / 2
        return (
          <g key={d}>
            <title>{`${d}: ${series.map((s) => `${s.name} ${fmtInt(s.values[i])}${unit}`).join(', ')}`}</title>
            {series.map((s) => { const hh = y(s.values[i]); acc += hh; return hh > 0 ? <rect key={s.name} x={x} y={bottom - acc} width={bw} height={hh} style={{ fill: s.color }} /> : null })}
            <rect x={x0 + slot * i} y={top} width={slot} height={bottom - top} fill="transparent" />
          </g>
        )
      })}
    </svg>
  )
}

/** Horizontal bars with labels, optionally stacked (parts). */
function HBars({ rows, label, format = fmtInt }: { rows: { name: string; to?: string; value: number; parts?: { value: number; color: string; name: string }[]; color?: string; detail?: string }[]; label: string; format?: (n: number) => string }) {
  if (rows.length === 0) return <div className="cap">Nothing to show.</div>
  const max = Math.max(1, ...rows.map((r) => r.value))
  return (
    <div role="img" aria-label={label} style={{ display: 'grid', gridTemplateColumns: 'minmax(140px, 2fr) 3fr auto', gap: '6px 10px', alignItems: 'center' }}>
      {rows.map((r, i) => (
        <RowFrag key={r.name + i} r={r} max={max} format={format} />
      ))}
    </div>
  )
}

function RowFrag({ r, max, format }: { r: { name: string; to?: string; value: number; parts?: { value: number; color: string; name: string }[]; color?: string; detail?: string }; max: number; format: (n: number) => string }) {
  const title = r.parts ? r.parts.filter((p) => p.value > 0).map((p) => `${p.name} ${format(p.value)}`).join(', ') : `${r.name}: ${format(r.value)}`
  return (
    <>
      <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontSize: 13 }}>{r.to ? <Link to={r.to}>{r.name}</Link> : r.name}</span>
      <span className="bar seg" title={title} style={{ height: 10, borderRadius: 3, gap: 1 }}>
        {(r.parts || [{ value: r.value, color: r.color || 'var(--cat1)', name: r.name }]).filter((p) => p.value > 0).map((p, j) => <span key={j} style={{ width: `${(p.value / max) * 100}%`, background: p.color, borderRadius: 2 }} />)}
      </span>
      <span className="num cap" style={{ color: 'var(--text-2)', textAlign: 'right' }}>{format(r.value)}{r.detail ? ` · ${r.detail}` : ''}</span>
    </>
  )
}

function minutes(m: number | null | undefined): string {
  if (m === null || m === undefined) return '—'
  if (m < 60) return `${Math.round(m)} min`
  if (m < 1440) return `${(m / 60).toFixed(1)} h`
  return `${(m / 1440).toFixed(1)} d`
}

function pct(n: number): string { return `${n.toFixed(1)}%` }

// --- utilisation --------------------------------------------------------------

function Utilisation({ r, csv }: { r: UtilisationReport; csv: (t: string) => string }) {
  const s = r.summary
  const days = r.days.map((d) => d.day)
  const series = s.hasSplit
    ? [{ name: 'Mono', color: CAT[0], values: r.days.map((d) => d.mono) }, { name: 'Colour', color: CAT[1], dashed: true, values: r.days.map((d) => d.color) }, { name: 'Total pages', color: CAT[2], values: r.days.map((d) => d.pages) }]
    : [{ name: 'Pages', color: CAT[0], values: r.days.map((d) => d.pages) }]
  const noSplit = Math.max(0, s.devicesWithData - (s.pagesKnown ?? s.devicesWithData))
  const splitLine = `${fmtInt(s.mono)} mono, ${fmtInt(s.color)} colour${noSplit > 0 ? `, ${noSplit} device${noSplit === 1 ? '' : 's'} without a split` : ''}`
  const opt = (n: number | null) => (n == null ? '—' : fmtInt(n))
  const idle = r.devices.filter((d) => d.idle)
  return (
    <>
      <div className="grid-auto">
        <Kpi label="Pages printed" value={fmtInt(s.pages)} detail={s.hasSplit ? splitLine : `${s.devicesWithData} devices with counters`} />
        <Kpi label="Pages / day" value={fmtInt(Math.round(s.perDay))} detail={s.peakDay ? `peak ${fmtInt(s.peakPages)} on ${s.peakDay}` : 'no pages in range'} />
        <Kpi label="Per device / day" value={s.perDevicePerDay.toFixed(1)} detail="over devices with counter data" />
        <Kpi label="Idle devices" value={fmtInt(s.idleDevices)} color={s.idleDevices ? 'var(--warn)' : undefined} detail="0 pages over the range" />
        <Kpi label="Busiest device" value={s.busiestDevice ? <span style={{ fontSize: 18 }}>{s.busiestDevice}</span> : '—'} detail={s.busiestDevice ? `${s.busiestDevicePct}% of all pages` : undefined} />
        {s.devicesNoData > 0 && <Kpi label="No counter data" value={fmtInt(s.devicesNoData)} detail="devices never polled in range" />}
      </div>
      <div className="grid-2">
        <Card title="Pages per day" id="u-days" sub="delta of the last counter sample each day · resets count 0" right={<Legend items={series.map((x) => ({ label: x.name, color: x.color, dashed: x.dashed }))} />}>
          <div className="cb"><LineChart days={days} series={series} label={`Pages per day over ${days.length} days: ${fmtInt(s.pages)} total, average ${fmtInt(Math.round(s.perDay))} per day, peak ${fmtInt(s.peakPages)}${s.peakDay ? ` on ${s.peakDay}` : ''}`} /></div>
        </Card>
        <Card title="Top devices" id="u-top" sub={`${r.top.length} of ${r.devices.length} · pages over the range`}>
          <div className="cb"><HBars label={`Top devices by pages: ${r.top.map((d) => `${d.name} ${fmtInt(d.pages)}`).join(', ')}`} rows={r.top.map((d) => ({ name: d.name, to: `/devices/${d.deviceId}`, value: d.pages, color: CAT[0], detail: `${d.perDay.toFixed(0)}/day` }))} /></div>
        </Card>
      </div>
      {r.groups.length > 0 && (
        <Card title="By group" id="u-groups" right={<ExportLinks csv={csv} tables={[{ v: 'groups', label: 'Export CSV' }]} />}>
          <div className="cb"><HBars label={`Pages by group: ${r.groups.map((g) => `${g.name} ${fmtInt(g.pages)}`).join(', ')}`} rows={r.groups.map((g) => ({ name: g.name, to: `/groups`, value: g.pages, color: CAT[2], detail: `${g.devices} devices` }))} /></div>
        </Card>
      )}
      <Card title="Devices" id="u-devs" sub={`${r.devices.length} with counter data · ${idle.length} idle · ${s.resets} counter reset${s.resets === 1 ? '' : 's'}`} right={<ExportLinks csv={csv} tables={[{ v: 'devices', label: 'Export CSV' }, { v: 'days', label: 'Days CSV' }]} />}>
        {r.devices.length === 0 ? <div className="cb"><EmptyState tone="muted" icon={<Ic.chart size={20} />} title="No counter history in range" text="Page counters appear after devices have been polled on at least two days." /></div> : (
          <div className="tw"><table className="tbl">
            <thead><tr><th>Device</th><th>Model</th><th>Location</th><th className="r">Pages</th>{s.hasSplit && <><th className="r">Mono</th><th className="r">Colour</th></>}<th className="r">Per day</th><th className="r">Active days</th><th className="r">Resets</th><th>State</th></tr></thead>
            <tbody>{r.devices.map((d) => (
              <tr key={d.deviceId}><td><Link to={`/devices/${d.deviceId}`}>{d.name}</Link></td><td className="mono">{d.model}</td><td className="cap">{d.location || '—'}</td><td className="num r">{fmtInt(d.pages)}</td>
                {s.hasSplit && <><td className="num r">{opt(d.mono)}</td><td className="num r">{opt(d.color)}</td></>}
                <td className="num r">{d.perDay.toFixed(1)}</td><td className="num r">{d.activeDays} / {d.dataDays}</td><td className="num r">{d.resets || '—'}</td>
                <td>{d.idle ? <Pill tone="warn" xs noIcon>Idle</Pill> : <Pill tone="ok" xs noIcon>In use</Pill>}</td></tr>
            ))}</tbody>
          </table></div>
        )}
      </Card>
    </>
  )
}

// --- fleet health -----------------------------------------------------------

function Health({ r, csv }: { r: HealthReport; csv: (t: string) => string }) {
  const s = r.summary
  const days = r.days.map((d) => d.day)
  return (
    <>
      <div className="grid-auto">
        <Kpi label="Availability" value={pct(s.availabilityPct)} color={s.availabilityPct < 95 ? 'var(--warn)' : undefined} detail="successful polls / polls" />
        <Kpi label="Offline time" value={minutes(s.offlineMinutes)} detail={`${s.incidents} outage${s.incidents === 1 ? '' : 's'} across ${s.devicesOffline} device${s.devicesOffline === 1 ? '' : 's'}`} />
        <Kpi label="Devices polled" value={`${fmtInt(s.polled)} / ${fmtInt(s.devices)}`} detail="in scope · polled at least once" />
        <Kpi label="Days with critical" value={fmtInt(s.daysCritical)} color={s.daysCritical ? 'var(--crit)' : undefined} detail={`of ${r.range.days} days`} />
        <Kpi label="Worst day" value={s.worstDay ? <span style={{ fontSize: 18 }}>{s.worstDay}</span> : '—'} detail={s.worstDay ? `${pct(s.worstDayPct)} of polls reachable` : 'no polls in range'} />
      </div>
      <div className="grid-2">
        <Card title="Reachability" id="h-reach" sub="share of polls that reached the device, per day" right={<Legend items={[{ label: '% reachable', color: CAT[2] }]} />}>
          <div className="cb"><LineChart days={days} max={100} unit="%" series={[{ name: 'Reachable', color: CAT[2], values: r.days.map((d) => (d.polled ? d.reachablePct : null)) }]} label={`Reachability per day over ${days.length} days: average ${pct(s.availabilityPct)}, worst ${s.worstDay || 'none'} at ${pct(s.worstDayPct)}`} /></div>
        </Card>
        <Card title="Devices by worst status" id="h-sev" sub="each device's worst polled status that day" right={<Legend items={[{ label: 'OK', color: 'var(--ok)' }, { label: 'Warning', color: 'var(--warn)' }, { label: 'Critical', color: 'var(--crit)' }, { label: 'Unreachable', color: 'var(--off)' }]} />}>
          <div className="cb"><BarChart days={days} series={[{ name: 'OK', color: 'var(--ok)', values: r.days.map((d) => d.ok) }, { name: 'Warning', color: 'var(--warn)', values: r.days.map((d) => d.warning) }, { name: 'Critical', color: 'var(--crit)', values: r.days.map((d) => d.critical) }, { name: 'Unreachable', color: 'var(--off)', values: r.days.map((d) => d.unreachable) }]} label={`Devices by worst status per day: ${r.days.filter((d) => d.polled).map((d) => `${d.day} ${d.ok} ok, ${d.warning} warning, ${d.critical} critical, ${d.unreachable} unreachable`).join('; ') || 'no polls'}`} /></div>
        </Card>
      </div>
      <Card title="Devices" id="h-devs" sub="sorted by offline time" right={<ExportLinks csv={csv} tables={[{ v: 'devices', label: 'Export CSV' }, { v: 'days', label: 'Days CSV' }]} />}>
        {r.devices.length === 0 ? <div className="cb"><EmptyState tone="muted" icon={<Ic.activity size={20} />} title="No polls in range" text="Fleet health appears once devices have been polled." /></div> : (
          <div className="tw"><table className="tbl">
            <thead><tr><th>Device</th><th>Model</th><th>Location</th><th className="r">Availability</th><th className="r">Polls</th><th className="r">Failed</th><th className="r">Offline</th><th className="r">Outages</th><th className="r">Mean outage</th><th>Worst</th><th className="r">Days crit / warn</th></tr></thead>
            <tbody>{r.devices.map((d) => (
              <tr key={d.deviceId}><td><Link to={`/devices/${d.deviceId}`}>{d.name}</Link></td><td className="mono">{d.model}</td><td className="cap">{d.location || '—'}</td>
                <td className="num r" style={{ color: d.availabilityPct < 95 ? 'var(--warn)' : undefined, fontWeight: d.availabilityPct < 95 ? 600 : 400 }}>{pct(d.availabilityPct)}</td>
                <td className="num r">{fmtInt(d.polls)}</td><td className="num r">{d.failed || '—'}</td><td className="num r">{d.offlineMinutes ? minutes(d.offlineMinutes) : '—'}</td><td className="num r">{d.incidents || '—'}</td><td className="num r">{d.incidents ? minutes(d.meanOfflineMinutes) : '—'}</td>
                <td>{d.worstSeverity ? <SevPill sev={d.worstSeverity} xs /> : <span className="cap">—</span>}</td><td className="num r">{d.daysCritical} / {d.daysWarning}</td></tr>
            ))}</tbody>
          </table></div>
        )}
      </Card>
    </>
  )
}

// --- supplies -----------------------------------------------------------------

function Supplies({ r, csv }: { r: SuppliesReport; csv: (t: string) => string }) {
  const s = r.summary
  const soon = r.supplies.filter((x) => x.daysToEmpty !== null).slice(0, 10)
  const col = (x: { description: string }) => supplyColor({ colorant: '', description: x.description, class: 'consumed', type: '' })
  return (
    <>
      <div className="grid-auto">
        <Kpi label="Supplies tracked" value={fmtInt(s.supplies)} detail={`${s.declining} with a measurable rate`} />
        <Kpi label="Due within 30 days" value={fmtInt(s.dueWithin30)} color={s.dueWithin7 ? 'var(--crit)' : s.dueWithin30 ? 'var(--warn)' : undefined} detail={`${s.dueWithin7} within 7 days · ${s.empty} empty now`} />
        <Kpi label="Mean usage" value={s.meanRate !== null ? `${s.meanRate.toFixed(1)}%/day` : '—'} detail={s.fastestSupply ? `fastest: ${s.fastestSupply}` : 'over declining supplies'} />
        <Kpi label="Replacements seen" value={fmtInt(s.replacements)} detail="level jumps of 20+ points in telemetry" />
        <Kpi label="Units out of stock" value={fmtInt(s.unitsOut)} to="/consumables" detail={`${s.stockEvents} replacement event${s.stockEvents === 1 ? '' : 's'} recorded`} />
      </div>
      <Card title="Soonest to run out" id="s-soon" sub="projected days to empty at the measured rate" right={<Legend items={[{ label: 'days left', color: 'var(--text-3)' }]} />}>
        <div className="cb"><HBars format={(n) => fmtDays(n)} label={`Days to empty: ${soon.map((x) => `${x.deviceName} ${x.description} ${fmtDays(x.daysToEmpty)}`).join(', ') || 'no supply has a measurable rate yet'}`} rows={soon.map((x) => ({ name: `${x.deviceName} · ${x.description}`, to: `/devices/${x.deviceId}`, value: x.daysToEmpty ?? 0, color: col(x), detail: `${x.nowPct.toFixed(0)}% now` }))} /></div>
      </Card>
      <Card title="Supply levels" id="s-levels" sub="start vs now, consumption rate and projection" right={<ExportLinks csv={csv} tables={[{ v: 'supplies', label: 'Export CSV' }]} />}>
        {r.supplies.length === 0 ? <div className="cb"><EmptyState tone="muted" icon={<Ic.consumables size={20} />} title="No supply history in range" /></div> : (
          <div className="tw"><table className="tbl">
            <thead><tr><th>Device</th><th>Supply</th><th className="r">Start</th><th className="r">Now</th><th className="r">Used</th><th className="r">Rate</th><th className="r">Days left</th><th>Empty by</th><th className="r">Replaced</th></tr></thead>
            <tbody>{r.supplies.map((x) => (
              <tr key={`${x.deviceId}:${x.index}`}><td><Link to={`/devices/${x.deviceId}`}>{x.deviceName}</Link></td><td><span className="row" style={{ gap: 6 }}><span className="sw" style={{ background: col(x) }} />{x.description}</span></td>
                <td className="num r">{x.startPct.toFixed(0)}%</td><td className="num r" style={{ fontWeight: x.nowPct <= 10 ? 600 : 400, color: x.nowPct <= 0 ? 'var(--crit)' : x.nowPct <= 10 ? 'var(--warn)' : undefined }}>{x.nowPct.toFixed(0)}%</td>
                <td className="num r">{x.consumed.toFixed(0)} pt</td><td className="num r">{x.ratePerDay !== null ? `${x.ratePerDay.toFixed(1)}%/d` : '—'}</td>
                <td className="num r">{fmtDays(x.daysToEmpty)}</td><td className="cap">{x.emptyBy ? x.emptyBy.slice(0, 10) : '—'}</td><td className="num r">{x.replacements || '—'}</td></tr>
            ))}</tbody>
          </table></div>
        )}
      </Card>
      <div className="grid-2">
        <Card title="Replacement events" id="s-repl" sub="stock events attributed to devices" right={<ExportLinks csv={csv} tables={[{ v: 'replacements', label: 'Export CSV' }]} />}>
          {r.replacements.length === 0 ? <div className="cb cap">No replacements recorded in range.</div> : (
            <div className="tw"><table className="tbl compact">
              <thead><tr><th>When</th><th>Device</th><th>Item</th><th>Status</th><th>By</th></tr></thead>
              <tbody>{r.replacements.slice(0, 50).map((e, i) => (
                <tr key={i}><td className="cap num">{fmtTime(e.at)}</td><td>{e.deviceId ? <Link to={`/devices/${e.deviceId}`}>{e.deviceName}</Link> : '—'}</td><td>{e.consumableId ? <Link to={`/consumables/${e.consumableId}`}>{e.consumableName || e.consumableId.slice(0, 8)}</Link> : '—'}</td><td><Tag>{e.status}</Tag></td><td className="cap">{e.actor}</td></tr>
              ))}</tbody>
            </table></div>
          )}
        </Card>
        <Card title="Stock consumption" id="s-cons" sub={r.consumptionScoped ? 'units taken out of stock per item' : 'fleet-wide: stock movements are not scoped to a group'} right={<ExportLinks csv={csv} tables={[{ v: 'consumption', label: 'Export CSV' }]} />}>
          {r.consumption.length === 0 ? <div className="cb cap">No stock movements in range.</div> : (
            <div className="tw"><table className="tbl compact">
              <thead><tr><th>Item</th><th className="r">Out</th><th className="r">In</th><th className="r">Events</th></tr></thead>
              <tbody>{r.consumption.map((c) => (
                <tr key={c.consumableId}><td><Link to={`/consumables/${c.consumableId}`}>{c.name || c.consumableId.slice(0, 8)}</Link></td><td className="num r">{c.unitsOut} {c.unit}</td><td className="num r">{c.unitsIn}</td><td className="num r">{c.events}</td></tr>
              ))}</tbody>
            </table></div>
          )}
        </Card>
      </div>
    </>
  )
}

// --- alerts -------------------------------------------------------------------

function AlertsTab({ r, csv }: { r: AlertsReport; csv: (t: string) => string }) {
  const s = r.summary
  const days = r.days.map((d) => d.day)
  return (
    <>
      <div className="grid-auto">
        <Kpi label="Alerts raised" value={fmtInt(s.total)} detail={`${s.perDay.toFixed(1)} per day · ${s.devicesAffected} device${s.devicesAffected === 1 ? '' : 's'}`} />
        <Kpi label="Critical · warning" value={<><span style={{ color: s.critical ? 'var(--crit)' : undefined }}>{s.critical}</span> <span style={{ fontSize: 15, color: 'var(--text-3)', fontWeight: 500 }}>·</span> <span style={{ color: s.warning ? 'var(--warn)' : undefined }}>{s.warning}</span></>} detail={s.peakDay ? `peak ${s.peakCount} on ${s.peakDay}` : 'none in range'} />
        <Kpi label="Mean time to acknowledge" value={minutes(s.meanAckMinutes)} detail={`${s.acked} acknowledged`} />
        <Kpi label="Mean time to clear" value={minutes(s.meanClearMinutes)} detail={`${s.cleared} cleared`} />
        <Kpi label="Top source" value={s.topSource ? <span style={{ fontSize: 18 }}>{s.topSource}</span> : '—'} detail={r.sources.length ? `${r.sources.length} source${r.sources.length === 1 ? '' : 's'}` : undefined} />
      </div>
      <div className="grid-2">
        <Card title="Alerts per day" id="a-days" sub="by severity, on the day raised" right={<Legend items={[{ label: 'Critical', color: 'var(--crit)' }, { label: 'Warning', color: 'var(--warn)' }]} />}>
          <div className="cb"><BarChart days={days} series={[{ name: 'Critical', color: 'var(--crit)', values: r.days.map((d) => d.critical) }, { name: 'Warning', color: 'var(--warn)', values: r.days.map((d) => d.warning) }]} label={`Alerts per day over ${days.length} days: ${s.total} total, ${s.critical} critical, ${s.warning} warning${s.peakDay ? `, peak ${s.peakCount} on ${s.peakDay}` : ''}`} /></div>
        </Card>
        <Card title="By source" id="a-src" right={<ExportLinks csv={csv} tables={[{ v: 'sources', label: 'Export CSV' }]} />}>
          <div className="cb"><HBars label={`Alerts by source: ${r.sources.map((x) => `${x.name} ${x.count}`).join(', ') || 'none'}`} rows={r.sources.map((x, i) => ({ name: x.name, value: x.count, color: CAT[i % CAT.length] }))} /></div>
        </Card>
      </div>
      <Card title="Devices" id="a-devs" sub="sorted by alert count" right={<ExportLinks csv={csv} tables={[{ v: 'devices', label: 'Export CSV' }, { v: 'days', label: 'Days CSV' }]} />}>
        {r.devices.length === 0 ? <div className="cb"><EmptyState tone="ok" icon={<Ic.ok size={20} />} title="No alerts raised in range" /></div> : (
          <div className="tw"><table className="tbl">
            <thead><tr><th>Device</th><th>Model</th><th>Location</th><th className="r">Alerts</th><th className="r">Critical</th><th className="r">Warning</th><th className="r">Acked</th><th className="r">Cleared</th><th className="r">Time to ack</th><th className="r">Time to clear</th></tr></thead>
            <tbody>{r.devices.map((d) => (
              <tr key={d.deviceId || 'stock'}><td>{d.deviceId ? <Link to={`/devices/${d.deviceId}`}>{d.name}</Link> : <Link to="/consumables">Stock</Link>}</td><td className="mono">{d.model}</td><td className="cap">{d.location || '—'}</td>
                <td className="num r">{d.alerts}</td><td className="num r" style={{ color: d.critical ? 'var(--crit)' : undefined }}>{d.critical || '—'}</td><td className="num r">{d.warning || '—'}</td><td className="num r">{d.acked || '—'}</td><td className="num r">{d.cleared || '—'}</td>
                <td className="num r">{minutes(d.meanAckMinutes)}</td><td className="num r">{minutes(d.meanClearMinutes)}</td></tr>
            ))}</tbody>
          </table></div>
        )}
      </Card>
    </>
  )
}

// --- firmware -----------------------------------------------------------------

const fwTone: Record<FirmwareStatus, 'ok' | 'warn' | 'unk' | 'off'> = { current: 'ok', outdated: 'warn', unregistered: 'unk', unknown: 'off' }
const fwLabel: Record<FirmwareStatus, string> = { current: 'Current', outdated: 'Outdated', unregistered: 'No package', unknown: 'Unknown' }

function Firmware({ r, csv }: { r: FirmwareReport; csv: (t: string) => string }) {
  const s = r.summary
  const versions = useMemo(() => Array.from(new Set(r.models.flatMap((m) => m.versions.map((v) => v.version)))), [r.models])
  const vcolor = (v: string) => (v === '' ? 'var(--off)' : CAT[versions.indexOf(v) % CAT.length])
  return (
    <>
      <div className="grid-auto">
        <Kpi label="Devices" value={fmtInt(s.devices)} detail={`${s.versions} distinct version${s.versions === 1 ? '' : 's'} · ${s.models} model${s.models === 1 ? '' : 's'}`} />
        <Kpi label="Outdated" value={fmtInt(s.outdated)} color={s.outdated ? 'var(--warn)' : undefined} detail="older than a registered package" />
        <Kpi label="Current" value={fmtInt(s.current)} detail="at the latest registered version" />
        <Kpi label="No package" value={fmtInt(s.unregistered)} detail={`${s.packages} package${s.packages === 1 ? '' : 's'} registered`} />
        <Kpi label="Unknown version" value={fmtInt(s.unknown)} detail="device reports no firmware" />
      </div>
      <Card title="Versions by model" id="f-models" sub="installed versions per make and model · point in time" right={<ExportLinks csv={csv} tables={[{ v: 'models', label: 'Export CSV' }]} />}>
        <div className="cb" style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          <Legend items={versions.map((v) => ({ label: v || 'unknown', color: vcolor(v) }))} />
          <HBars label={`Firmware versions by model: ${r.models.map((m) => `${m.make} ${m.model}: ${m.versions.map((v) => `${v.version || 'unknown'} ×${v.count}`).join(', ')}`).join('; ') || 'no devices'}`}
            rows={r.models.map((m) => ({ name: `${m.make} ${m.model}`.trim() || 'Unknown model', value: m.devices, parts: m.versions.map((v) => ({ value: v.count, color: vcolor(v.version), name: v.version || 'unknown' })), detail: m.outdated ? `${m.outdated} outdated` : m.latest ? `latest ${m.latest}` : 'no package' }))} />
        </div>
      </Card>
      <Card title="Devices" id="f-devs" sub="outdated first" right={<ExportLinks csv={csv} tables={[{ v: 'devices', label: 'Export CSV' }]} />}>
        {r.devices.length === 0 ? <div className="cb"><EmptyState tone="muted" icon={<Ic.chip size={20} />} title="No devices in scope" /></div> : (
          <div className="tw"><table className="tbl">
            <thead><tr><th>Device</th><th>Make</th><th>Model</th><th>Location</th><th>Installed</th><th>Latest package</th><th>Status</th></tr></thead>
            <tbody>{r.devices.map((d) => (
              <tr key={d.deviceId}><td><Link to={`/devices/${d.deviceId}`}>{d.name}</Link></td><td>{d.make || '—'}</td><td className="mono">{d.model || '—'}</td><td className="cap">{d.location || '—'}</td>
                <td className="mono">{d.installed || '—'}</td><td className="mono">{d.latest || '—'}</td><td><Pill tone={fwTone[d.status]} xs>{fwLabel[d.status]}</Pill></td></tr>
            ))}</tbody>
          </table></div>
        )}
      </Card>
    </>
  )
}

// --- compliance ---------------------------------------------------------------

function Compliance({ r }: { r: ComplianceReport }) {
  if (r.available) return <Banner tone="ok">Compliance data available.</Banner>
  return (
    <Card title="Compliance" id="c-hook" sub="per device, group and policy, over time (spec §8.5)">
      <div className="cb"><EmptyState tone="muted" icon={<Ic.shield size={20} />} title="Not available yet" text={r.reason || 'This report does not include stored assessment history yet.'} action={<Link className="btn sm" to="/security"><Ic.shield />Open security</Link>} /></div>
    </Card>
  )
}
