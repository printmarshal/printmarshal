import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { api, fmtTime } from '../api'
import type { Job, JobResult } from '../types'
import { useEvents } from '../ws'
import { Banner, Card, JobBar, JobPill, LongMessage, Pill, Tag } from '../components/ui'
import { jobTitle } from './Jobs'

export default function JobDetail() {
  const { id } = useParams()
  const [job, setJob] = useState<Job | null>(null)
  const [results, setResults] = useState<JobResult[]>([])
  const [error, setError] = useState('')
  const load = useCallback(() => { api.get<{ job: Job; results: JobResult[] }>(`/api/jobs/${id}`).then((r) => { setJob(r.job); setResults(r.results) }).catch((e) => setError(e.message)) }, [id])
  useEffect(() => { load() }, [load])
  useEvents((e) => { if (e.type === 'job.updated' && (e.data as Job).id === id) load() }, [id, load])
  if (error) return <Banner tone="crit">{error}</Banner>
  if (!job) return <div className="cap">Loading…</div>
  const ok = results.filter((r) => r.status === 'ok').length, failed = results.filter((r) => r.status === 'error').length, skipped = results.filter((r) => r.status === 'skipped').length
  return (
    <>
      <nav aria-label="Breadcrumb" className="cap"><Link to="/jobs">Jobs</Link> / <span style={{ color: 'var(--text-2)' }}>{jobTitle(job)}</span></nav>
      <div className="ph"><div style={{ display: 'flex', alignItems: 'center', gap: 10, flexWrap: 'wrap' }}><h1 className="h1">{jobTitle(job)}</h1><JobPill status={job.status} /></div><span className="cap mono">{job.id}</span></div>
      <Card>
        <div className="cb" style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between' }}><span>Per-device progress</span><span className="num" style={{ fontWeight: 600 }}>{job.done} <span className="cap" style={{ fontWeight: 400 }}>/ {job.total}</span></span></div>
          <JobBar done={job.done} failed={job.failed} total={job.total} running={job.status === 'running'} />
          <div className="row cap" style={{ gap: 14 }}><span style={{ display: 'inline-flex', alignItems: 'center', gap: 5 }}><span className="dot" style={{ background: 'var(--ok)' }} />{ok} succeeded</span><span style={{ display: 'inline-flex', alignItems: 'center', gap: 5 }}><span className="dot" style={{ background: 'var(--crit)' }} />{failed} failed</span><span style={{ display: 'inline-flex', alignItems: 'center', gap: 5 }}><span className="dot" style={{ background: 'var(--track)', boxShadow: 'inset 0 0 0 1px var(--border-strong)' }} />{skipped} skipped</span></div>
        </div>
        <div style={{ display: 'flex', flexWrap: 'wrap', borderTop: '1px solid var(--border)' }}>
          <div className="stat"><span className="cap">Created</span><span>{fmtTime(job.createdAt)}</span><span className="cap">by {job.createdBy}</span></div>
          <div className="stat"><span className="cap">Started</span><span>{fmtTime(job.startedAt)}</span></div>
          <div className="stat"><span className="cap">Finished</span><span>{fmtTime(job.finishedAt)}</span></div>
          <div className="stat"><span className="cap">Target</span><span className="mono" style={{ fontSize: 11, whiteSpace: 'normal' }}>{JSON.stringify(job.target)}</span></div>
        </div>
        {job.error && <div className="cf error">{job.error}</div>}
      </Card>
      <Card title="Per-device results" id="res">
        {results.length === 0 ? <div className="cb cap">No results yet.</div> : (
          <div className="tw"><table className="tbl" style={{ minWidth: 760 }}>
            <thead><tr><th>Device / address</th><th>Status</th><th>Message</th><th>Details</th><th>At</th></tr></thead>
            <tbody>{results.map((r) => (
              <tr key={r.id} className={r.status === 'skipped' ? 'dim' : ''}>
                <td>{r.deviceId ? <Link to={`/devices/${r.deviceId}`} style={{ fontWeight: 500 }}>{r.address || r.deviceId.slice(0, 8)}</Link> : <span className="mono">{r.address || '—'}</span>}</td>
                <td><Pill tone={r.status === 'ok' ? 'ok' : r.status === 'error' ? 'crit' : 'off'}>{r.status}</Pill></td>
                <td className="wrap"><LongMessage text={r.message || ''} /></td>
                <td className="cap" style={{ whiteSpace: 'normal', maxWidth: 360 }}>{r.data ? Object.entries(r.data).map(([k, v]) => <Tag key={k} title={typeof v === 'object' ? JSON.stringify(v) : String(v)}>{k}: {typeof v === 'object' ? JSON.stringify(v).slice(0, 40) : String(v)}</Tag>) : ''}</td>
                <td className="cap num">{fmtTime(r.at)}</td>
              </tr>
            ))}</tbody>
          </table></div>
        )}
      </Card>
    </>
  )
}
