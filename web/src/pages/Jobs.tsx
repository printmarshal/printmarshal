import { useCallback, useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api, ago } from '../api'
import type { Job } from '../types'
import { useEvents } from '../ws'
import { Ic } from '../components/icons'
import { Card, ChipSelect, EmptyState, JobBar, JobPill } from '../components/ui'

export function jobTitle(j: Job): string {
  switch (j.type) {
    case 'discovery': return 'Discovery run'
    case 'poll': return j.target.all ? 'Scheduled poll · all devices' : `Poll · ${((j.target.deviceIds as string[]) || []).length} device(s)`
    default: return j.type
  }
}

export default function Jobs() {
  const [params, setParams] = useSearchParams()
  const [jobs, setJobs] = useState<Job[]>([])
  const type = params.get('type') || ''
  const status = params.get('status') || ''
  const load = useCallback(() => { api.get<Job[]>(`/api/jobs?limit=200${type ? `&type=${type}` : ''}${status ? `&status=${status}` : ''}`).then(setJobs).catch(() => {}) }, [type, status])
  useEffect(() => { load() }, [load])
  useEvents((e) => { if (e.type === 'job.updated') load() }, [load])
  const set = (k: string, v: string) => { const p = new URLSearchParams(params); if (v) p.set(k, v); else p.delete(k); setParams(p, { replace: true }) }
  const types = Array.from(new Set(jobs.map((j) => j.type)))
  return (
    <>
      <div className="ph"><div><h1 className="h1">Jobs</h1><div className="cap num">{jobs.filter((j) => j.status === 'running').length} running · {jobs.length} shown · newest first</div></div></div>
      <div className="row">
        <ChipSelect label="Type" value={type} onChange={(v) => set('type', v)} options={[...new Set([...types, 'discovery', 'poll'])].map((t) => ({ v: t, label: t }))} />
        <ChipSelect label="Status" value={status} onChange={(v) => set('status', v)} options={['queued', 'running', 'succeeded', 'partial', 'failed', 'cancelled'].map((s) => ({ v: s, label: s }))} />
      </div>
      <Card>
        {jobs.length === 0 ? <div className="cb"><EmptyState icon={<Ic.activity size={20} />} title="No jobs yet" text="Discovery runs and polls appear here with per-device results." /></div> : (
          <div className="tw"><table className="tbl" style={{ minWidth: 800 }}>
            <thead><tr><th>Job</th><th>Status</th><th style={{ width: 220 }}>Progress</th><th className="r">Done</th><th>Started</th><th>Finished</th><th>By</th></tr></thead>
            <tbody>{jobs.map((j) => (
              <tr key={j.id}>
                <td><Link to={`/jobs/${j.id}`} style={{ fontWeight: 500 }}>{jobTitle(j)}</Link><div className="cap mono" style={{ fontSize: 11 }}>{j.id.slice(0, 8)}</div></td>
                <td><JobPill status={j.status} />{j.error && <div className="error small">{j.error}</div>}</td>
                <td><JobBar done={j.done} failed={j.failed} total={j.total} running={j.status === 'running'} /></td>
                <td className="num r">{j.done} / {j.total}{j.failed ? <span style={{ color: 'var(--crit)' }}> · {j.failed} failed</span> : ''}</td>
                <td className="cap num">{ago(j.startedAt || j.createdAt)}</td><td className="cap num">{j.finishedAt ? ago(j.finishedAt) : '—'}</td><td>{j.createdBy}</td>
              </tr>
            ))}</tbody>
          </table></div>
        )}
      </Card>
    </>
  )
}
