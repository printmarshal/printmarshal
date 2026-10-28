import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../../api'
import type { Match, SupportStatus } from '../../types-catalog'
import { basisLabel, confidenceLabel, confidenceTone, operationLabels, qualificationLabels, stateLabels, stateTone } from '../../types-catalog'
import { Ic } from '../icons'
import { Card, Pill, Tag } from '../ui'

/**
 * "Support & evidence" card for the device detail page: what the catalogue
 * documents for the matched family, what PrintMarshal's drivers have observed
 * on this device, and what is simply unknown. The three are never merged, and
 * the wording never says "supported".
 */
export default function SupportCard({ deviceId, lastPolled }: { deviceId: string; lastPolled?: string }) {
  const [data, setData] = useState<{ match: Match; support: SupportStatus } | null>(null)
  const [err, setErr] = useState('')
  useEffect(() => { api.get<{ match: Match; support: SupportStatus }>(`/api/devices/${deviceId}/support`).then(setData).catch((e) => setErr((e as Error).message)) }, [deviceId, lastPolled])
  const tierTone = (t: SupportStatus['tier']) => t === 'validated' || t === 'documented+observed' ? 'ok' : t === 'observed only (standards)' ? 'ok' : t === 'documented only' ? 'info' : t === 'unsupported' ? 'off' : 'unk'
  return (
    <Card title="Support & evidence" id="sup-ev" sub="catalogue documentation vs what drivers observed · unknown stays unknown" right={<Link to="/catalog" className="cap" style={{ color: 'var(--accent)' }}>Catalogue →</Link>}>
      {err && <div className="cb error small">{err}</div>}
      {!data && !err && <div className="cb cap" aria-busy="true">Loading…</div>}
      {data && (
        <>
          <div className="cb" style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 8 }}>
              <Pill tone={confidenceTone(data.match.confidence)}>{confidenceLabel(data.match.confidence)}</Pill>
              {data.match.family ? (
                <Link to={`/catalog?family=${data.match.familyId}`} style={{ fontWeight: 500 }}>{data.match.family.brand} {data.match.family.model_prefix} <span className="mono">{data.match.familyId}</span></Link>
              ) : data.match.brand ? <span>{data.match.brand} · no catalogue family contains this model</span> : <span className="cap">make and enterprise number are not in the catalogue</span>}
            </div>
            <div className="cap">{data.match.reason}{data.match.modelId ? <> · base identifier <span className="mono">{data.match.modelId}</span> (observed as <span className="mono">{data.match.matchedToken}</span>)</> : null}</div>
            {data.match.candidates && data.match.candidates.length > 0 && <div className="cap" style={{ color: 'var(--warn)' }}>Also matches {data.match.candidates.join(', ')} — confirm the physical device before relying on this family.</div>}
            <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 8 }}>
              <span className="cap">Tier</span><Pill tone={tierTone(data.support.tier)}>{data.support.tier}</Pill>
              <span className="cap">Qualification</span><Tag title={data.support.override ? `${data.support.override.actor} · ${data.support.override.note || 'no note'}` : 'as shipped in the catalogue'}>{qualificationLabels[data.support.qualification]}</Tag>
              {data.support.override && <span className="cap" style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}><Ic.user size={12} />set by {data.support.override.actor}</span>}
            </div>
          </div>
          <div className="tw"><table className="tbl compact" style={{ minWidth: 360 }}>
            <thead><tr><th>Operation</th><th>Documented</th><th>Observed</th></tr></thead>
            <tbody>
              {data.support.operations.map((o) => (
                <tr key={o.operation}>
                  <td style={{ fontWeight: 500 }}>{operationLabels[o.operation]}</td>
                  <td><Pill tone={stateTone(o.documented)} xs title={o.documentedDetail ? `${basisLabel(o.documentedBasis)} — ${o.documentedDetail}${o.sourceIds.length ? ` [${o.sourceIds.join(', ')}]` : ''}` : 'nothing documented either way'}>{stateLabels[o.documented]}</Pill>{o.documentedBasis?.startsWith('management_profile') && <span className="cap" title="Vendor platform route from the management profile: context, not a per-model guarantee"> · profile</span>}</td>
                  <td><Pill tone={stateTone(o.observed)} xs title={o.observedBy.length ? o.observedBy.join(', ') : 'no driver serves this on this device'}>{o.observed}</Pill>{o.observedBy.length > 0 && <span className="cap"> · {o.observedBy.map((b) => b.split('·')[1]).filter((v, i, a) => a.indexOf(v) === i).join(', ')}</span>}</td>
                </tr>
              ))}
            </tbody>
          </table></div>
          <div className="cf cap">“Documented” is a primary vendor source; “observed” is a driver answering on this device. Neither is a validation — the catalogue ships {data.support.physicalValidation ? 'with' : 'with no'} physically validated device for this family.</div>
        </>
      )}
    </Card>
  )
}
