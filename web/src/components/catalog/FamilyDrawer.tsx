import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, fmtTime } from '../../api'
import type { Evidence, FamilyDetail, Operation, OperationEvidence, QualificationStatus, Source } from '../../types-catalog'
import { basisLabel, confidenceLabel, confidenceTone, operationLabels, operations, qualificationLabels, qualificationStatuses, stateLabels, stateTone } from '../../types-catalog'
import { Ic } from '../icons'
import { Btn, Pill, Tag } from '../ui'

const evidenceOps: { key: keyof Pick<OperationEvidence, 'monitoring' | 'configuration' | 'security' | 'firmware' | 'app_management'>; label: string; op?: Operation }[] = [
  { key: 'monitoring', label: 'Monitoring', op: 'monitor' }, { key: 'configuration', label: 'Configuration', op: 'configure' },
  { key: 'security', label: 'Security', op: 'secure' }, { key: 'firmware', label: 'Firmware', op: 'update' }, { key: 'app_management', label: 'App management' },
]

/** Side drawer with everything the catalogue holds about one family, plus the fleet devices matched to it. */
export default function FamilyDrawer({ familyId, canOperate, onClose, onChanged }: { familyId: string; canOperate: boolean; onClose: () => void; onChanged: () => void }) {
  const [d, setD] = useState<FamilyDetail | null>(null)
  const [err, setErr] = useState('')
  const [status, setStatus] = useState<QualificationStatus>('research_candidate')
  const [note, setNote] = useState('')
  const [msg, setMsg] = useState('')
  const [busy, setBusy] = useState(false)
  const load = () => api.get<FamilyDetail>(`/api/catalog/families/${familyId}`).then((r) => { setD(r); setStatus(r.qualification); setNote(r.override?.note || '') }).catch((e) => setErr((e as Error).message))
  useEffect(() => { setD(null); setErr(''); setMsg(''); load() }, [familyId]) // eslint-disable-line react-hooks/exhaustive-deps
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose() }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])
  const save = async (e: React.FormEvent) => {
    e.preventDefault(); setBusy(true); setMsg('')
    try { await api.put(`/api/catalog/families/${familyId}/qualification`, { status, note }); setMsg('Saved and audited.'); await load(); onChanged() } catch (er) { setMsg((er as Error).message) } finally { setBusy(false) }
  }
  const dirty = d ? status !== d.qualification || note !== (d.override?.note || '') : false
  const H = ({ children }: { children: React.ReactNode }) => <h3 className="cap" style={{ textTransform: 'uppercase', letterSpacing: '.04em', fontWeight: 600, marginTop: 6 }}>{children}</h3>
  return (
    <div className="drawer-backdrop" onClick={onClose}>
      <aside className="drawer" onClick={(e) => e.stopPropagation()} aria-label={`Family ${familyId}`} style={{ width: 'min(680px, 100%)' }}>
        <div className="row between">
          <div style={{ minWidth: 0 }}>
            <div className="cap"><span className="mono">{familyId}</span>{d && <> · {d.family.segment}</>}</div>
            <h2 className="h2" style={{ fontSize: 16 }}>{d ? `${d.family.brand} ${d.family.model_prefix}`.trim() : 'Loading…'}</h2>
          </div>
          <Btn sm variant="ghost" ic onClick={onClose} aria-label="Close"><Ic.x /></Btn>
        </div>
        {err && <div className="error small">{err}</div>}
        {d && (
          <>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, alignItems: 'center' }}>
              <Tag up>{d.family.model_evidence.replace(/_/g, ' ')}</Tag>
              <Tag title="as shipped in the catalogue">{d.catalogueQualification.replace(/_/g, ' ')}</Tag>
              {d.override && <Tag title={`${d.override.actor} · ${fmtTime(d.override.updatedAt)}`}><Ic.user size={11} />{qualificationLabels[d.qualification]}</Tag>}
              {d.family.release_year !== null ? <Tag>released {d.family.release_year}</Tag> : <Tag title="Null means unverified: a support-table date is not a launch year">release year unverified</Tag>}
            </div>
            <div className="cap">{d.family.lifecycle}{d.family.note ? <> · {d.family.note}</> : null}{d.family.management_support_evidence_date ? <> · vendor management listing: {d.family.management_support_evidence_date}</> : null}</div>

            <H>Base model identifiers · {d.family.model_ids.length}</H>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>{d.family.model_ids.map((m) => <span key={m} className="tag mono" style={{ fontSize: 11 }}>{m}</span>)}</div>
            <div className="cap">Identity sources: {d.family.model_source_ids.map((s) => <SourceRef key={s} id={s} sources={d.sources} />)}</div>

            <H>Evidence per operation</H>
            <div className="cap">Model-specific evidence (operation_evidence) wins; otherwise the vendor profile's documented route is shown, which is platform context and not a per-model guarantee. Nothing here is an observation.</div>
            <table className="tbl compact"><thead><tr><th>Operation</th><th>Documented</th><th>Basis</th></tr></thead><tbody>
              {operations.map((op) => { const o = d.summary[op]; return (
                <tr key={op}><td style={{ fontWeight: 500 }}>{operationLabels[op]}</td>
                  <td><Pill tone={stateTone(o.documented)} xs>{stateLabels[o.documented]}</Pill></td>
                  <td className="wrap" style={{ whiteSpace: 'normal' }}><span className="cap">{basisLabel(o.documentedBasis) || '—'}</span>{o.documentedDetail && <div style={{ fontSize: 12 }}>{o.documentedDetail}</div>}{o.sourceIds.length > 0 && <div className="cap">{o.sourceIds.map((s) => <SourceRef key={s} id={s} sources={d.sources} />)}</div>}</td></tr>
              ) })}
            </tbody></table>

            {d.evidence.length > 0 && (<>
              <H>Model-specific evidence records · {d.evidence.length}</H>
              {d.evidence.map((e) => <EvidenceBlock key={e.id} e={e} sources={d.sources} />)}
            </>)}
            {d.brandEvidence.length > 0 && (<>
              <H>Brand-level context (not model claims)</H>
              {d.brandEvidence.map((e) => <EvidenceBlock key={e.id} e={e} sources={d.sources} />)}
            </>)}

            <H>Management profile · {d.family.management_profile}</H>
            {d.profile ? (
              <div style={{ display: 'grid', gridTemplateColumns: 'minmax(96px, 130px) 1fr', gap: '6px 12px', fontSize: 13 }}>
                <span className="cap">Label</span><span style={{ fontWeight: 500 }}>{d.profile.label}</span>
                <span className="cap">Read</span><span>{d.profile.read_path}</span>
                <span className="cap">Configuration</span><span>{d.profile.configuration_path}</span>
                <span className="cap">Firmware</span><span>{d.profile.firmware_path}</span>
                <span className="cap">General admin API</span><span>{d.profile.general_admin_api}</span>
                <span className="cap">Scope</span><span className="cap">{d.profile.scope}</span>
                <span className="cap">Sources</span><span className="cap">{d.profile.source_ids.map((s) => <SourceRef key={s} id={s} sources={d.sources} />)}</span>
              </div>
            ) : <div className="cap">profile not found</div>}

            {d.variants.length > 0 && (<>
              <H>Documented product numbers · {d.variants.length}</H>
              <div className="cap">Example registry from vendor tables, not a complete regional SKU database. None physically validated.</div>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>{d.variants.map((v) => <span key={v.product_number} className="tag mono" style={{ fontSize: 11 }} title={`sources ${v.source_ids.join(', ')}`}>{v.base_model}{v.bundle} · {v.product_number}</span>)}</div>
            </>)}
            {d.conflicts.length > 0 && (<>
              <H>Documented identity conflicts</H>
              {d.conflicts.map((k) => (
                <div key={k.product_number} className="banner warn" role="note" style={{ alignItems: 'flex-start' }}><Ic.warn style={{ color: 'var(--warn)' }} /><div style={{ flex: 1 }}>
                  <b>{k.product_number}</b>: {k.observations.map((o, i) => <span key={i}>{i > 0 ? ' vs ' : ''}<span className="mono">{o.model}{o.bundle}</span> <SourceRef id={o.source_id} sources={d.sources} /></span>)}
                  <div className="cap">{k.resolution}</div>
                </div></div>
              ))}
            </>)}

            <H>Devices in your fleet · {d.devices.length}</H>
            {d.devices.length === 0 ? <div className="cap">No discovered device matches this family.</div> : (
              <div style={{ border: '1px solid var(--border)', borderRadius: 6 }}>
                {d.devices.map((v) => <div className="lrow" key={v.id} style={{ padding: '8px 12px' }}><Pill tone={confidenceTone(v.confidence)} xs>{confidenceLabel(v.confidence)}</Pill><div style={{ flex: 1, minWidth: 0 }}><Link to={`/devices/${v.id}`} style={{ fontWeight: 500 }}>{v.name}</Link><div className="cap mono">{v.model || v.make} · {v.address}{v.modelId ? ` · ${v.modelId}` : ''}</div></div><Tag>{v.state.replace('_', ' ')}</Tag></div>)}
              </div>
            )}

            <H>Qualification</H>
            <form onSubmit={save} style={{ display: 'flex', flexDirection: 'column', gap: 8 }} aria-label="Qualification override">
              <div className="cap">The catalogue ships every family as a research candidate. Record here what your lab or production has actually established; the change is attributed to you and written to the audit trail. It never changes the documented or observed rows above.</div>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
                <select className="fin" style={{ width: 220 }} value={status} disabled={!canOperate} aria-label="Qualification status" onChange={(e) => setStatus(e.target.value as QualificationStatus)}>
                  {qualificationStatuses.map((q) => <option key={q} value={q}>{qualificationLabels[q]}</option>)}
                </select>
                <input className="fin" style={{ flex: '1 1 200px' }} placeholder="note — which device, firmware, what was tested" value={note} disabled={!canOperate} onChange={(e) => setNote(e.target.value)} aria-label="Qualification note" />
                {canOperate && <Btn type="submit" sm variant={dirty ? 'pri' : undefined} disabled={!dirty || busy}>Save</Btn>}
              </div>
              {d.override && <div className="cap" style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}><Ic.user size={12} />Last set by {d.override.actor} · {fmtTime(d.override.updatedAt)}</div>}
              {!canOperate && <div className="cap">Changing the qualification needs the operator role.</div>}
              {msg && <div className="cap">{msg}</div>}
            </form>

            <H>Sources referenced · {Object.keys(d.sources).length}</H>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              {Object.entries(d.sources).sort(([a], [b]) => a.localeCompare(b)).map(([id, s]) => (
                <div key={id} style={{ display: 'grid', gridTemplateColumns: '58px 1fr', gap: 8, fontSize: 12 }}>
                  <span className="mono" style={{ color: 'var(--text-2)' }}>{id}</span>
                  <span style={{ minWidth: 0, overflowWrap: 'anywhere' }}><a href={s.url} target="_blank" rel="noreferrer">{s.title}</a><span className="cap"> · {s.access.replace(/_/g, ' ')} · checked {s.checked_on}</span>{s.first_pass_access_note && <div className="cap">{s.first_pass_access_note}</div>}</span>
                </div>
              ))}
            </div>
          </>
        )}
      </aside>
    </div>
  )
}

function SourceRef({ id, sources }: { id: string; sources: Record<string, Source> }) {
  const s = sources[id]
  return s ? <a className="mono" href={s.url} target="_blank" rel="noreferrer" title={s.title} style={{ marginRight: 6, fontSize: 11 }}>{id}</a> : <span className="mono" style={{ marginRight: 6, fontSize: 11 }}>{id}</span>
}

function EvidenceBlock({ e, sources }: { e: OperationEvidence; sources: Record<string, Source> }) {
  const row = (label: string, ev?: Evidence) => ev ? (
    <div key={label} style={{ display: 'grid', gridTemplateColumns: 'minmax(96px, 120px) 1fr', gap: '2px 12px', fontSize: 12, padding: '4px 0', borderTop: '1px solid var(--border)' }}>
      <span className="cap">{label}</span>
      <span><Pill tone={ev.status.toLowerCase().includes('not_supported') ? 'off' : 'info'} xs>{ev.status.toLowerCase().includes('not_supported') ? 'unsupported' : 'documented'}</Pill> <span className="mono" style={{ fontSize: 11 }}>{ev.status}</span>{ev.minimum_firmware && <div className="cap">minimum firmware {ev.minimum_firmware}</div>}{ev.licence && <div className="cap">licence: {ev.licence}</div>}<div className="cap">{ev.source_ids.map((s) => <SourceRef key={s} id={s} sources={sources} />)}</div></span>
    </div>
  ) : null
  return (
    <div style={{ border: '1px solid var(--border)', borderRadius: 6, padding: '8px 12px', background: 'var(--surface-2)' }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, alignItems: 'center' }}><b className="mono" style={{ fontSize: 12 }}>{e.id}</b>{e.model_ids.map((m) => <span key={m} className="tag mono" style={{ fontSize: 11 }}>{m}</span>)}<Pill tone={e.physical_validation ? 'ok' : 'unk'} xs>{e.physical_validation ? 'physically validated' : 'not physically validated'}</Pill></div>
      {evidenceOps.map((o) => row(o.label, e[o.key]))}
      {e.general_device_management && <div className="cap" style={{ paddingTop: 4 }}>General device management: {e.general_device_management.replace(/_/g, ' ')}</div>}
      {e.third_party_admin_contract && <div className="cap" style={{ paddingTop: 4 }}>Third-party admin contract: {e.third_party_admin_contract.replace(/_/g, ' ')}</div>}
    </div>
  )
}
