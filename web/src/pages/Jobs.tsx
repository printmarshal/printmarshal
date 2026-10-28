import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, ago } from '../api'
import type { Job } from '../types'
import { useEvents } from '../ws'
import { JobStatus, Progress } from '../components/ui'

export default function Jobs() {
  const [jobs, setJobs] = useState<Job[]>([])
  const load = useCallback(() => { api.get<Job[]>('/api/jobs?limit=200').then(setJobs).catch(() => {}) }, [])
  useEffect(() => { load() }, [load])
  useEvents((e) => { if (e.type === 'job.updated') load() }, [load])
  return (
    <>
      <h1>Jobs</h1>
      <div className="panel">
        <table>
          <thead><tr><th>Type</th><th>Status</th><th>Progress</th><th>Created</th><th>Finished</th><th>By</th></tr></thead>
          <tbody>
            {jobs.map((j) => (
              <tr key={j.id}>
                <td><Link to={`/jobs/${j.id}`}>{j.type}</Link><div className="muted small">{j.id.slice(0, 8)}</div></td>
                <td><JobStatus status={j.status} />{j.error && <div className="error small">{j.error}</div>}</td>
                <td style={{ width: 200 }}><Progress done={j.done} total={j.total} /><div className="muted small">{j.done}/{j.total}{j.failed ? ` · ${j.failed} failed` : ''}</div></td>
                <td className="small">{ago(j.createdAt)}</td><td className="small">{j.finishedAt ? ago(j.finishedAt) : '—'}</td><td>{j.createdBy}</td>
              </tr>
            ))}
            {jobs.length === 0 && <tr><td colSpan={6} className="muted">No jobs yet.</td></tr>}
          </tbody>
        </table>
      </div>
    </>
  )
}
