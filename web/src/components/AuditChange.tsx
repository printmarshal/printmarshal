import { useState } from 'react'
import type { AuditEntry } from '../types'

const isObj = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v)
const showVal = (v: unknown) => v === undefined ? '—' : JSON.stringify(v)

/** The keys whose value differs between before and after, or null when either side is not an object. */
function auditDiff(before: unknown, after: unknown): { key: string; from: unknown; to: unknown }[] | null {
  if (!isObj(before) || !isObj(after)) return null
  const keys = Array.from(new Set([...Object.keys(before), ...Object.keys(after)])).sort()
  return keys.filter((k) => JSON.stringify(before[k]) !== JSON.stringify(after[k])).map((k) => ({ key: k, from: before[k], to: after[k] }))
}

/** The table cell showing what an audit entry changed, with the full entry on request. */
export function AuditChangeCell({ e }: { e: Pick<AuditEntry, 'before' | 'after'> }) {
  const [open, setOpen] = useState(false)
  const diff = auditDiff(e.before, e.after)
  const hasPayload = e.before !== undefined || e.after !== undefined
  return (
    <td className="cap" style={{ whiteSpace: 'normal', minWidth: 240, maxWidth: 420, overflowWrap: 'anywhere' }}>
      {diff ? (diff.length === 0 ? <span>no field changed</span> : diff.map((d) => <div key={d.key}><span className="mono">{d.key}</span>: <span className="old mono">{showVal(d.from)}</span> → <span className="new-v mono">{showVal(d.to)}</span></div>))
        : <>{e.before && <>from <span className="old mono">{JSON.stringify(e.before)}</span> </>}{e.after && <>to <span className="new-v mono">{JSON.stringify(e.after)}</span></>}</>}
      {hasPayload && <div><button type="button" className="link" aria-expanded={open} onClick={() => setOpen(!open)}>{open ? 'Hide full entry' : 'Show full entry'}</button></div>}
      {open && <pre className="mono" style={{ whiteSpace: 'pre-wrap', margin: '4px 0 0' }}>{`before: ${JSON.stringify(e.before ?? null, null, 2)}\nafter: ${JSON.stringify(e.after ?? null, null, 2)}`}</pre>}
    </td>
  )
}
