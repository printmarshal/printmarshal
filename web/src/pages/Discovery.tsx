import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, ago } from '../api'
import type { Job, User } from '../types'
import { useEvents } from '../ws'
import { JobStatus, Progress } from '../components/ui'

export default function Discovery({ user }: { user: User }) {
  const [runs, setRuns] = useState<Job[]>([])
  const [targets, setTargets] = useState('')
  const [zeroconf, setZeroconf] = useState(true)
  const [allowLarge, setAllowLarge] = useState(false)
  const [msg, setMsg] = useState('')
  const load = useCallback(() => { api.get<Job[]>('/api/discovery/runs').then(setRuns).catch(() => {}) }, [])
  useEffect(() => { load() }, [load])
  useEvents((e) => { if (e.type === 'job.updated') load() }, [load])

  const start = async (e: React.FormEvent) => {
    e.preventDefault(); setMsg('')
    const list = targets.split(/[\s,]+/).map((t) => t.trim()).filter(Boolean)
    try {
      const j = await api.post<Job>('/api/discovery/runs', { targets: list, zeroconf, allowLarge })
      setMsg(`Discovery run ${j.id.slice(0, 8)} queued.`)
      load()
    } catch (err) { setMsg((err as Error).message) }
  }
  return (
    <>
      <h1>Discovery</h1>
      {user.role !== 'viewer' && (
        <form className="panel" onSubmit={start}>
          <h2>Start a run <span className="muted small">(read-only; safe to repeat)</span></h2>
          <label className="field">Active scan targets — IPs, ranges (10.0.0.1-50) or CIDR (10.0.0.0/24), separated by spaces or commas
            <textarea rows={2} value={targets} onChange={(e) => setTargets(e.target.value)} placeholder="10.0.0.0/24 192.168.1.10-40" />
          </label>
          <div className="row" style={{ marginTop: 10 }}>
            <label><input type="checkbox" checked={zeroconf} onChange={(e) => setZeroconf(e.target.checked)} /> Zero-config (mDNS / DNS-SD + WS-Discovery)</label>
            <label><input type="checkbox" checked={allowLarge} onChange={(e) => setAllowLarge(e.target.checked)} /> Allow large scans (lift the address cap)</label>
            <button className="primary">Start discovery</button>
            <span className="muted small">{msg}</span>
          </div>
          <div className="muted small" style={{ marginTop: 8 }}>Zero-config discovery needs the server on the same L2 network as the printers (host networking in Docker). Only scan networks you are authorised to manage.</div>
        </form>
      )}
      <div className="panel">
        <h2>Runs</h2>
        <table>
          <thead><tr><th>Started</th><th>By</th><th>Targets</th><th>Status</th><th>Progress</th><th /></tr></thead>
          <tbody>
            {runs.map((j) => (
              <tr key={j.id}>
                <td>{ago(j.createdAt)}</td><td>{j.createdBy}</td>
                <td className="small">{(j.target.targets as string[] | undefined)?.join(', ') || ''}{j.target.zeroconf ? (j.target.targets && (j.target.targets as string[]).length ? ' + zeroconf' : 'zeroconf') : ''}</td>
                <td><JobStatus status={j.status} /></td>
                <td style={{ width: 180 }}><Progress done={j.done} total={j.total} /><div className="muted small">{j.done}/{j.total}{j.failed ? ` · ${j.failed} failed` : ''}</div></td>
                <td><Link to={`/jobs/${j.id}`}>details</Link></td>
              </tr>
            ))}
            {runs.length === 0 && <tr><td colSpan={6} className="muted">No discovery runs yet.</td></tr>}
          </tbody>
        </table>
      </div>
    </>
  )
}
