import { useEffect, useState } from 'react'
import { api } from '../../api'
import type { Job, JobResult } from '../../types'
import { Ic } from '../icons'
import { Banner, Btn, Card, Pill } from '../ui'
import type { ApplyResultData, CatalogDef, SettingResult } from '../../types-config'
import { Diff, ResultPill, settingName, waitForJob } from './shared'

export interface RevertTarget { writeId: string; deviceId: string; label: string }
interface Row { t: RevertTarget; job?: Job; results: SettingResult[]; error?: string; message?: string }

function rowsFrom(t: RevertTarget, r: { job: Job; results: JobResult[] }): Row {
  const res = r.results.find((x) => x.deviceId === t.deviceId) || r.results[0]
  const data = (res?.data || {}) as ApplyResultData
  return { t, job: r.job, results: data.results || [], message: res?.message, error: res?.status === 'error' && !(data.results || []).length ? res.message : undefined }
}

/**
 * Reverts one or more write records: a dry run of every revert first, then a
 * confirmed real revert. Each record is one device, so each revert is its own job.
 */
export default function RevertPanel({ targets, byKey, onClose, onDone }: { targets: RevertTarget[]; byKey: Map<string, CatalogDef>; onClose: () => void; onDone?: () => void }) {
  const [phase, setPhase] = useState<'dry' | 'preview' | 'writing' | 'done'>('dry')
  const [rows, setRows] = useState<Row[]>([])
  const [error, setError] = useState('')

  const run = async (dryRun: boolean) => {
    setPhase(dryRun ? 'dry' : 'writing'); setError('')
    const out = await Promise.all(targets.map(async (t): Promise<Row> => {
      try {
        const r = await api.post<{ job: Job }>(`/api/config/writes/${t.writeId}/revert`, { dryRun })
        return rowsFrom(t, await waitForJob(r.job.id))
      } catch (e) { return { t, results: [], error: (e as Error).message } }
    }))
    setRows(out)
    setPhase(dryRun ? 'preview' : 'done')
    if (!dryRun) onDone?.()
  }
  useEffect(() => { run(true) }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const wouldWrite = rows.reduce((n, r) => n + r.results.filter((x) => x.status === 'would_write').length, 0)
  const revertible = rows.filter((r) => !r.error && r.results.some((x) => x.status === 'would_write'))
  const written = rows.reduce((n, r) => n + r.results.filter((x) => x.status === 'written').length, 0)
  const failed = rows.filter((r) => r.error || r.results.some((x) => x.status === 'failed')).length

  return (
    <Card title="Revert previous values" id="revert" sub={`${targets.length} write record${targets.length === 1 ? '' : 's'} · the values saved before the write are applied back`} right={<Btn sm variant="ghost" ic aria-label="Close revert" onClick={onClose}><Ic.x /></Btn>}>
      <div className="cb" style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
        {phase === 'dry' && <div className="cap" aria-busy="true" style={{ display: 'flex', gap: 8, alignItems: 'center' }}><Ic.refresh />Dry run of the revert — reading each device…</div>}
        {phase === 'preview' && <Banner tone="info"><b style={{ color: 'var(--info)' }}>Dry run</b> — nothing has been written yet. {wouldWrite} setting{wouldWrite === 1 ? '' : 's'} on {revertible.length} device{revertible.length === 1 ? '' : 's'} would go back to the previous value.</Banner>}
        {phase === 'writing' && <div className="cap" aria-busy="true" style={{ display: 'flex', gap: 8, alignItems: 'center' }}><Ic.refresh />Reverting…</div>}
        {phase === 'done' && <Banner tone={failed ? 'warn' : 'ok'}><b style={{ color: failed ? 'var(--warn)' : 'var(--ok)' }}>Reverted</b> — {written} setting{written === 1 ? '' : 's'} restored{failed ? ` · ${failed} device${failed === 1 ? '' : 's'} failed` : ''}.</Banner>}
        {error && <Banner tone="crit">{error}</Banner>}
        {rows.map((r) => (
          <div key={r.t.writeId} style={{ border: '1px solid var(--border)', borderRadius: 8 }}>
            <div className="lrow" style={{ borderBottom: r.results.length ? '1px solid var(--border)' : 0 }}>
              <b style={{ fontWeight: 500, flex: 1 }}>{r.t.label}</b>
              {r.error ? <Pill tone="crit" xs>{r.error}</Pill> : r.results.length === 0 ? <Pill tone="off" xs>Nothing to revert</Pill> : <span className="cap">{r.message}</span>}
            </div>
            {r.results.map((x) => (
              <div key={x.key} style={{ display: 'grid', gridTemplateColumns: 'minmax(160px, 220px) minmax(0, 1fr) auto', gap: 12, alignItems: 'center', padding: '6px 16px', borderBottom: '1px dashed var(--border)' }}>
                <span className="cap" style={{ color: 'var(--text-2)' }}>{settingName(x.key, byKey)}</span>
                <span>{x.status === 'would_write' || x.status === 'written' ? <Diff before={x.before} after={x.after} def={byKey.get(x.key)} /> : <span className="cap">{x.error || x.status}</span>}</span>
                <ResultPill status={x.status} xs />
              </div>
            ))}
          </div>
        ))}
      </div>
      <div className="cf" style={{ display: 'flex', justifyContent: 'flex-end', gap: 8 }}>
        {phase === 'preview' && <><Btn onClick={onClose}>Cancel</Btn><Btn variant="dngf" disabled={revertible.length === 0} onClick={() => run(false)}>Revert {revertible.length} device{revertible.length === 1 ? '' : 's'}</Btn></>}
        {phase === 'done' && <Btn onClick={onClose}>Close</Btn>}
      </div>
    </Card>
  )
}
