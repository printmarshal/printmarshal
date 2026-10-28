import { useCallback, useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { api, fmtTime } from '../api'
import type { Job, JobResult } from '../types'
import { useEvents } from '../ws'
import { JobStatus, Progress } from '../components/ui'

export default function JobDetail() {
  const { id } = useParams()
  const [job, setJob] = useState<Job | null>(null)
  const [results, setResults] = useState<JobResult[]>([])
  const [error, setError] = useState('')
  const load = useCallback(() => {
    api.get<{ job: Job; results: JobResult[] }>(`/api/jobs/${id}`).then((r) => { setJob(r.job); setResults(r.results) }).catch((e) => setError(e.message))
  }, [id])
  useEffect(() => { load() }, [load])
  useEvents((e) => { if (e.type === 'job.updated' && (e.data as Job).id === id) load() }, [id, load])
  if (error) return <div className="error">{error}</div>
  if (!job) return <div className="muted">Loading…</div>
  return (
    <>
      <h1>{job.type} job <JobStatus status={job.status} /></h1>
      <div className="panel">
        <dl className="kv">
          <dt>Created</dt><dd>{fmtTime(job.createdAt)} by {job.createdBy}</dd>
          <dt>Started</dt><dd>{fmtTime(job.startedAt)}</dd>
          <dt>Finished</dt><dd>{fmtTime(job.finishedAt)}</dd>
          <dt>Progress</dt><dd><Progress done={job.done} total={job.total} /> {job.done}/{job.total}{job.failed ? ` (${job.failed} failed)` : ''}</dd>
          {job.error && <><dt>Error</dt><dd className="error">{job.error}</dd></>}
          <dt>Target</dt><dd><pre>{JSON.stringify(job.target)}</pre></dd>
        </dl>
      </div>
      <div className="panel">
        <h2>Per-device results</h2>
        <table>
          <thead><tr><th>Device / address</th><th>Status</th><th>Message</th><th>Details</th><th>At</th></tr></thead>
          <tbody>
            {results.map((r) => (
              <tr key={r.id}>
                <td>{r.deviceId ? <Link to={`/devices/${r.deviceId}`}>{r.address || r.deviceId.slice(0, 8)}</Link> : r.address || '—'}</td>
                <td><span className={`badge ${r.status === 'ok' ? 'ok' : r.status === 'error' ? 'critical' : 'outline'}`}>{r.status}</span></td>
                <td>{r.message}</td>
                <td className="small muted">{r.data ? Object.entries(r.data).map(([k, v]) => `${k}=${typeof v === 'object' ? JSON.stringify(v) : String(v)}`).join(' ') : ''}</td>
                <td className="small">{fmtTime(r.at)}</td>
              </tr>
            ))}
            {results.length === 0 && <tr><td colSpan={5} className="muted">No results yet.</td></tr>}
          </tbody>
        </table>
      </div>
    </>
  )
}
