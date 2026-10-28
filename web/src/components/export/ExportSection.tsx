import { useCallback, useEffect, useState } from 'react'
import { api, fmtTime } from '../../api'
import { Ic } from '../icons'
import { Btn, Card, EmptyState, Pill, Tag } from '../ui'

// Exports & SIEM (docs/export.md): sink states of the security-event export,
// the printer syslog receiver, and the incremental sync contract. Everything
// here is read-only; sinks are configured in the server's JSON config / PM_*
// variables. Types are local because the module lives outside the core.

interface SinkStatus { name: string; type: string; target: string; format?: string; ok: boolean; sent: number; failed: number; lastError?: string; lastAt?: string; pending?: number }
interface IngestStatus { enabled: boolean; listen?: string; listening: boolean; received: number; stored: number; unknownSource: number; dropped: number; lastAt?: string; retention: string }
interface ExportStatus { enabled: boolean; sinks: SinkStatus[]; queued: number; queueSize: number; received: number; dropped: number; events: string[]; ingest: IngestStatus; syncCollections: string[] }

const collectionHelp: Record<string, string> = {
  devices: 'every device, ordered by last change; deleted devices appear once as { id, deleted: true, updatedAt }',
  alerts: 'raised, acknowledged and cleared alerts, ordered by last change',
  audit: 'the audit trail (operator role), ordered by time of entry',
  jobs: 'jobs; a job resurfaces when it starts or finishes',
  'config-writes': 'configuration write records with per-setting results',
}

function CopyLine({ text }: { text: string }) {
  const [done, setDone] = useState(false)
  const copy = async () => {
    try { await navigator.clipboard.writeText(text); setDone(true); window.setTimeout(() => setDone(false), 1500) } catch { /* clipboard unavailable: the text is still selectable */ }
  }
  return (
    <div className="row" style={{ alignItems: 'flex-start', gap: 8 }}>
      <pre className="mono" style={{ margin: 0, flex: '1 1 auto', whiteSpace: 'pre-wrap', wordBreak: 'break-all', fontSize: 11, padding: '6px 8px', background: 'var(--hover)', borderRadius: 6 }}>{text}</pre>
      <Btn sm variant="ghost" onClick={copy} aria-label="Copy command">{done ? <><Ic.check size={13} /> Copied</> : 'Copy'}</Btn>
    </div>
  )
}

function sinkTone(s: SinkStatus): 'ok' | 'warn' | 'crit' | 'off' {
  if (s.sent === 0 && s.failed === 0) return 'off'
  return s.ok ? 'ok' : s.failed > 0 && s.sent === 0 ? 'crit' : 'warn'
}

export default function ExportSection() {
  const [st, setSt] = useState<ExportStatus | null>(null)
  const [err, setErr] = useState('')
  const load = useCallback(async () => {
    try { setSt(await api.get<ExportStatus>('/api/export/status')); setErr('') } catch (e) { setErr((e as Error).message) }
  }, [])
  useEffect(() => {
    load()
    const t = window.setInterval(load, 10000)
    return () => window.clearInterval(t)
  }, [load])
  const origin = window.location.origin
  const token = '$PM_TOKEN'
  return (
    <>
      <Card title="Security event export" id="exp" sub="every security-flagged event, alert.* and notification.dead, forwarded from the event bus to the sinks below; configured in the server's JSON config (export) or PM_EXPORT_* variables" right={<Btn sm variant="ghost" onClick={load}><Ic.refresh size={13} /> Refresh</Btn>}>
        {err ? <div className="cb error small">{err}</div> : !st ? <div className="cb cap">Loading…</div> : st.sinks.length === 0 ? (
          <div className="cb"><EmptyState icon={<Ic.upload size={20} />} title="No export sinks configured" tone="muted" text={<>Set <span className="mono">PM_EXPORT_SYSLOG=udp://collector:514</span>, <span className="mono">PM_EXPORT_FILE=/var/log/printmarshal/events.jsonl</span> or <span className="mono">PM_EXPORT_HTTP=https://collector/ingest</span> and restart. See docs/export.md.</>} /></div>
        ) : (
          <>
            <div className="tw"><table className="tbl" style={{ minWidth: 640 }}><thead><tr><th>Sink</th><th>Target</th><th>Format</th><th>State</th><th className="r">Sent</th><th className="r">Failed</th><th>Last</th></tr></thead><tbody>
              {st.sinks.map((s) => <tr key={s.name}><td style={{ fontWeight: 500 }}>{s.name}</td><td className="mono cap">{s.target}</td><td>{s.format && <Tag>{s.format}</Tag>}</td><td><Pill tone={sinkTone(s)} title={s.lastError}>{s.sent === 0 && s.failed === 0 ? 'idle' : s.ok ? 'delivering' : 'failing'}</Pill>{s.pending ? <span className="cap"> · {s.pending} pending</span> : null}</td><td className="r num">{s.sent.toLocaleString()}</td><td className="r num">{s.failed > 0 ? <span style={{ color: 'var(--crit)' }}>{s.failed.toLocaleString()}</span> : '0'}</td><td className="cap">{s.lastAt ? fmtTime(s.lastAt) : 'never'}{s.lastError && <div className="small" style={{ color: 'var(--crit)', whiteSpace: 'normal', maxWidth: 320 }}>{s.lastError}</div>}</td></tr>)}
            </tbody></table></div>
            <div className="cf cap" style={{ display: 'flex', gap: 16, flexWrap: 'wrap' }}>
              <span>queue <span className="num">{st.queued}</span> / {st.queueSize}</span>
              <span>received <span className="num">{st.received.toLocaleString()}</span></span>
              <span>dropped {st.dropped > 0 ? <Pill tone="crit" xs>{st.dropped.toLocaleString()}</Pill> : <span className="num">0</span>}</span>
              <span title={st.events.join(', ')}>{st.events.length} event types exported</span>
            </div>
          </>
        )}
      </Card>
      <Card title="Printer syslog receiver" id="ing" sub="UDP syslog from printers, attributed by source address to a known device and kept per device; never acted on automatically">
        {!st ? <div className="cb cap">Loading…</div> : !st.ingest.enabled ? (
          <div className="cb"><EmptyState icon={<Ic.server size={20} />} title="Receiver is off" tone="muted" text={<>Set <span className="mono">PM_EXPORT_INGEST_LISTEN=0.0.0.0:1514</span> and point each printer's syslog server at this host. Lines are kept for {st.ingest.retention}.</>} /></div>
        ) : (
          <div className="cb" style={{ display: 'flex', gap: 16, flexWrap: 'wrap', alignItems: 'center' }}>
            {st.ingest.listening ? <Pill tone="ok">listening on {st.ingest.listen}</Pill> : <Pill tone="crit">not listening ({st.ingest.listen})</Pill>}
            <span className="cap">received <span className="num">{st.ingest.received.toLocaleString()}</span></span>
            <span className="cap">stored <span className="num">{st.ingest.stored.toLocaleString()}</span></span>
            <span className="cap">unknown source <span className="num">{st.ingest.unknownSource.toLocaleString()}</span></span>
            <span className="cap">rate-limited {st.ingest.dropped > 0 ? <Pill tone="warn" xs>{st.ingest.dropped.toLocaleString()}</Pill> : <span className="num">0</span>}</span>
            <span className="cap">last line {st.ingest.lastAt ? fmtTime(st.ingest.lastAt) : 'never'}</span>
            <span className="cap">retention {st.ingest.retention}</span>
          </div>
        )}
        <div className="cf cap">Per-device lines: <span className="mono">GET /api/devices/{'{id}'}/logs?limit=200</span></div>
      </Card>
      <Card title="Incremental sync" id="sync" sub="GET /api/sync/{collection}?since=<cursor>&limit= · rows ordered by (updatedAt, id); keep the last cursor and pass it back to get only what changed, never a missed or repeated row">
        <div className="cb stack" style={{ gap: 12 }}>
          <div className="cap">Sign in once and keep the token (or use the session cookie); <span className="mono">PM_TOKEN</span> below is the <span className="mono">token</span> field of the login response.</div>
          <CopyLine text={`PM_TOKEN=$(curl -s -X POST ${origin}/api/auth/login -H 'Content-Type: application/json' -d '{"username":"admin","password":"..."}' | jq -r .token)`} />
          {(st?.syncCollections ?? Object.keys(collectionHelp)).map((c) => (
            <div key={c}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8, marginBottom: 4 }}><Tag mono>{c}</Tag><span className="cap">{collectionHelp[c] ?? ''}</span></div>
              <CopyLine text={`curl -s -H "Authorization: Bearer ${token}" "${origin}/api/sync/${c}?limit=200"`} />
            </div>
          ))}
          <div className="cap">Each response is <span className="mono">{'{ "collection", "items": [...], "cursor": "...", "hasMore": true|false }'}</span>. Repeat with <span className="mono">?since=&lt;cursor&gt;</span> while <span className="mono">hasMore</span> is true, then poll the same way on a schedule; an <span className="mono">If-None-Match</span> with the returned <span className="mono">ETag</span> answers 304 when nothing changed.</div>
          <CopyLine text={`curl -s -H "Authorization: Bearer ${token}" "${origin}/api/sync/devices?since=$CURSOR&limit=200"`} />
        </div>
      </Card>
    </>
  )
}
