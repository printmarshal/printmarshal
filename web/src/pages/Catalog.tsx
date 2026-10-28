import { useCallback, useEffect, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api } from '../api'
import type { User } from '../types'
import type { CatalogSummary, FamilyRow } from '../types-catalog'
import { operationLabels, operations, qualificationLabels, stateLabels, stateTone } from '../types-catalog'
import { Ic } from '../components/icons'
import { Banner, Card, ChipSelect, EmptyState, Kpi, Pill, SearchBox, Tag, fmtInt } from '../components/ui'
import FamilyDrawer from '../components/catalog/FamilyDrawer'

/**
 * Device model catalogue: the research inventory of enterprise printer
 * families, what primary sources document per operation, and which fleet
 * devices match each family. Documented, observed and unknown stay separate.
 */
export default function Catalog({ user }: { user: User }) {
  const [params, setParams] = useSearchParams()
  const [summary, setSummary] = useState<CatalogSummary | null>(null)
  const [rows, setRows] = useState<FamilyRow[] | null>(null)
  const [err, setErr] = useState('')
  const brand = params.get('brand') || ''
  const segment = params.get('segment') || ''
  const q = params.get('q') || ''
  const family = params.get('family') || ''
  const set = (k: string, v: string) => { const p = new URLSearchParams(params); if (v) p.set(k, v); else p.delete(k); setParams(p, { replace: true }) }
  const canOperate = user.role !== 'viewer'

  const loadSummary = useCallback(() => { api.get<CatalogSummary>('/api/catalog/summary').then(setSummary).catch((e) => setErr((e as Error).message)) }, [])
  const loadRows = useCallback(() => {
    const qs = new URLSearchParams(); if (brand) qs.set('brand', brand); if (segment) qs.set('segment', segment); if (q) qs.set('q', q)
    api.get<{ families: FamilyRow[] }>(`/api/catalog/families?${qs}`).then((r) => setRows(r.families)).catch((e) => setErr((e as Error).message))
  }, [brand, segment, q])
  useEffect(() => { loadSummary() }, [loadSummary])
  useEffect(() => { loadRows() }, [loadRows])
  const reload = () => { loadSummary(); loadRows() }

  const byBrand = useMemo(() => summary ? Object.entries(summary.catalogue.byBrand).sort((a, b) => b[1] - a[1]) : [], [summary])
  const fleetByBrand = useMemo(() => { const m: Record<string, number> = {}; for (const r of rows || []) m[r.brand] = (m[r.brand] || 0) + r.fleetCount; return m }, [rows])
  const c = summary?.catalogue
  const f = summary?.fleet

  return (
    <>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div><h1 className="h1">Device model catalogue</h1><div className="cap">{c ? `${c.title} · researched ${c.researchedOn} · schema ${c.schemaVersion}` : 'Research inventory of enterprise printer families'}</div></div>
        <Pill tone="unk" title="The catalogue ships every family as a research candidate; no device was physically validated by the research.">{c ? `${c.physicallyValidated} physically validated` : '…'}</Pill>
      </div>
      {err && <Banner tone="crit">{err}</Banner>}
      <div className="grid-auto">
        <Kpi label="Families" value={c ? fmtInt(c.families) : '—'} detail={c ? `${fmtInt(c.baseIdentifiers)} base model identifiers` : undefined} />
        <Kpi label="Brands" value={c ? fmtInt(c.brands) : '—'} detail={c ? `${c.profiles} vendor management profiles` : undefined} />
        <Kpi label="Evidence" value={c ? fmtInt(c.evidenceEntries) : '—'} detail={c ? `model-specific records · ${c.sources} sources · ${c.identityConflicts} identity conflict${c.identityConflicts === 1 ? '' : 's'}` : undefined} />
        <Kpi label="Fleet matched" value={f ? <>{fmtInt(f.matched)} <span style={{ fontSize: 15, color: 'var(--text-3)', fontWeight: 500 }}>/ {fmtInt(f.devices)}</span></> : '—'} detail={f ? `${f.byConfidence.exact} exact · ${f.byConfidence.family} family · ${f.byConfidence['brand-only']} brand only · ${f.byConfidence.none} no match` : undefined} to="/devices" />
      </div>
      <Banner tone="info" role="note">
        <b>Documented</b> means a primary vendor source describes the operation for a model (or the platform offers a route). <b>Observed</b> means a PrintMarshal driver answered on a real device in your fleet. <b>Unknown</b> means neither — it is never turned into a “no”, and nothing here is “supported” until your lab says so via the qualification control.
      </Banner>

      <div className="row" role="group" aria-label="Brand filter">
        <button type="button" className={`chip${brand === '' ? ' set' : ''}`} onClick={() => set('brand', '')}>All brands{summary && <b> {fmtInt(summary.catalogue.baseIdentifiers)}</b>}</button>
        {byBrand.map(([b, n]) => <button key={b} type="button" className={`chip${brand === b ? ' set' : ''}`} onClick={() => set('brand', brand === b ? '' : b)} title={`${n} base identifiers${fleetByBrand[b] ? ` · ${fleetByBrand[b]} fleet devices` : ''}`}>{b}<b> {n}</b>{fleetByBrand[b] ? <span className="ct num" style={{ marginLeft: 2 }}>{fleetByBrand[b]} in fleet</span> : null}</button>)}
      </div>
      <div className="row">
        <SearchBox sm value={q} onChange={(v) => set('q', v)} placeholder="Search family, model id, prefix, profile…" />
        <ChipSelect label="Segment" value={segment} onChange={(v) => set('segment', v)} options={(summary?.segments || []).map((s) => ({ v: s, label: s }))} />
        <span className="cap" style={{ marginLeft: 'auto' }}>{rows ? `${rows.length} famil${rows.length === 1 ? 'y' : 'ies'}` : ''}</span>
      </div>

      <Card title="Families" id="fam" sub="evidence pills show what is documented per operation for the family's strongest model record · nothing here is observed">
        {rows === null ? <div className="cb cap" aria-busy="true">Loading…</div> : rows.length === 0 ? <div className="cb"><EmptyState icon={<Ic.chart size={20} />} title="No families match" text="Clear the brand, segment or search filter." /></div> : (
          <div className="tw"><table className="tbl compact" style={{ minWidth: 980 }}>
            <thead><tr><th>Family</th><th>Brand · prefix</th><th>Segment</th><th>Management profile</th>{operations.map((op) => <th key={op}>{operationLabels[op]}</th>)}<th className="r">Fleet</th><th>Qualification</th></tr></thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r.id} className={family === r.id ? 'sel' : undefined} style={{ cursor: 'pointer' }} onClick={() => set('family', r.id)}>
                  <td><button type="button" className="link mono" style={{ fontWeight: 600 }} onClick={(e) => { e.stopPropagation(); set('family', r.id) }}>{r.id}</button><div className="cap">{r.model_ids.length} ids · {r.model_ids.slice(0, 3).join(', ')}{r.model_ids.length > 3 ? '…' : ''}</div></td>
                  <td><div style={{ fontWeight: 500 }}>{r.brand}</div><div className="cap">{r.model_prefix || '—'}</div></td>
                  <td className="cap" style={{ whiteSpace: 'normal', maxWidth: 160 }}>{r.segment}</td>
                  <td style={{ whiteSpace: 'normal', maxWidth: 220 }}><div style={{ fontSize: 12 }}>{r.profileLabel}</div><div className="cap">{r.management_profile}{r.evidenceCount > 0 ? ` · ${r.evidenceCount} model record${r.evidenceCount === 1 ? '' : 's'}` : ''}</div></td>
                  {operations.map((op) => { const o = r.evidence[op]; return <td key={op}><Pill tone={stateTone(o.documented)} xs noIcon={o.documented === 'unknown'} title={o.documentedDetail ? `${o.documentedBasis}: ${o.documentedDetail}` : 'nothing documented either way'}>{stateLabels[o.documented]}</Pill></td> })}
                  <td className="r num">{r.fleetCount > 0 ? <b>{r.fleetCount}</b> : <span className="cap">0</span>}</td>
                  <td><Tag title={r.override ? `${r.override.actor}: ${r.override.note || 'no note'}` : 'as shipped: research candidate, unvalidated'}>{r.override && <Ic.user size={11} />}{qualificationLabels[r.qualification]}</Tag></td>
                </tr>
              ))}
            </tbody>
          </table></div>
        )}
        <div className="cf cap" style={{ display: 'flex', flexWrap: 'wrap', gap: 12, alignItems: 'center' }}>
          <span>Legend:</span><Pill tone="info" xs>documented</Pill><span>primary source</span><Pill tone="ok" xs>observed</Pill><span>driver answered (device pages only)</span><Pill tone="off" xs>unsupported</Pill><span>source says the checked tool path lacks it</span><Pill tone="unk" xs noIcon>unknown</Pill><span>no evidence either way</span>
        </div>
      </Card>
      {family && <FamilyDrawer familyId={family} canOperate={canOperate} onClose={() => set('family', '')} onChanged={reload} />}
    </>
  )
}
