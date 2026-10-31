import { useEffect, useRef, useState } from 'react'
import { api } from '../api'
import type { Job, User } from '../types'
import type { ImportOutcome, PlanResult } from '../types-import'
import { importColumnLabel, importColumns, importKeyColumns } from '../types-import'
import { Ic } from './icons'
import { Banner, Btn, Dialog, Pill } from './ui'

const PAGE = 200

const outcomePill: Record<ImportOutcome, { tone: 'ok' | 'info' | 'off'; label: string }> = {
  create: { tone: 'ok', label: 'Create' },
  update: { tone: 'info', label: 'Update' },
  skip: { tone: 'off', label: 'Skip' },
}

/** A header-only CSV with the known columns, in the console's words. */
function downloadTemplate() {
  const blob = new Blob([importColumns.map((c) => c.label).join(',') + '\r\n'], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = 'printmarshal-import-template.csv'
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

function fmtValue(v: string | boolean): string {
  if (typeof v === 'boolean') return v ? 'yes' : 'no'
  return v
}

/** Choose a CSV file, preview what the import would do, and (administrators) apply it. */
export function ImportDialog({ user, onClose, onApplied }: { user: User; onClose: () => void; onApplied: (job: Job) => void }) {
  const [file, setFile] = useState<File | null>(null)
  const [replace, setReplace] = useState<string[]>([])
  const [plan, setPlan] = useState<PlanResult | null>(null)
  const [loading, setLoading] = useState(false)
  const [applying, setApplying] = useState(false)
  const [err, setErr] = useState('')
  const [page, setPage] = useState(0)
  const seq = useRef(0)
  const isAdmin = user.role === 'admin'

  const form = (mode: 'preview' | 'apply', f: File) => {
    const fd = new FormData()
    fd.append('file', f)
    fd.append('mode', mode)
    fd.append('replace', replace.join(','))
    return fd
  }

  // Preview whenever the file or the replace columns change; only the latest answer is shown.
  useEffect(() => {
    if (!file) return
    const n = ++seq.current
    setLoading(true)
    setErr('')
    api.upload<PlanResult>('/api/discovery/import', form('preview', file))
      .then((p) => { if (n === seq.current) { setPlan(p); setPage((pg) => Math.min(pg, Math.max(0, Math.ceil(p.rows.length / PAGE) - 1))) } })
      // A failed re-run keeps the last preview on screen, so the choice that caused it can be undone.
      .catch((e) => { if (n === seq.current) setErr((e as Error).message) })
      .finally(() => { if (n === seq.current) setLoading(false) })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [file, replace])

  const choose = (f: File | null) => {
    setPlan(null)
    setReplace([])
    setPage(0)
    setFile(f)
  }
  const toggle = (col: string, on: boolean) => setReplace((r) => (on ? [...r, col] : r.filter((c) => c !== col)))

  const apply = async () => {
    if (!file) return
    setApplying(true)
    setErr('')
    try {
      const job = await api.upload<Job>('/api/discovery/import', form('apply', file))
      onApplied(job)
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setApplying(false)
    }
  }

  const n = plan ? plan.totals.create + plan.totals.update : 0
  // Group is left out: a row only ever joins a group, so there is nothing to replace.
  const replaceable = plan ? plan.knownColumns.filter((c) => !importKeyColumns.includes(c) && c !== 'group') : []
  const pages = plan ? Math.max(1, Math.ceil(plan.rows.length / PAGE)) : 1
  const rows = plan ? plan.rows.slice(page * PAGE, (page + 1) * PAGE) : []

  return (
    <Dialog
      width={1040}
      title="Import printers from a CSV file"
      sub="Each row needs an address, a hostname the console can resolve, or the serial of a printer it already manages. The preview changes nothing and contacts no printer."
      onClose={onClose}
      footer={
        <>
          {plan && !isAdmin && <span className="cap" style={{ marginRight: 'auto', alignSelf: 'center' }}>An administrator applies the import.</span>}
          <Btn variant="ghost" onClick={onClose}>{isAdmin ? 'Cancel' : 'Close'}</Btn>
          {isAdmin && <Btn variant="pri" disabled={!plan || loading || applying || n === 0} onClick={apply}>{applying ? 'Importing…' : `Import ${n} printer${n === 1 ? '' : 's'}`}</Btn>}
        </>
      }
    >
      <div className="stack" style={{ gap: 12 }}>
        <div className="row" style={{ gap: 10 }}>
          <label className="btn sm">
            <Ic.upload />{file ? 'Choose another file' : 'Choose a CSV file'}
            <input type="file" accept=".csv,text/csv" style={{ display: 'none' }} onChange={(e) => { choose(e.target.files?.[0] ?? null); e.target.value = '' }} />
          </label>
          {file && <span className="mono cap">{file.name}</span>}
          <span style={{ flex: 1 }} />
          <Btn sm variant="ghost" onClick={downloadTemplate}><Ic.download />Download template</Btn>
        </div>
        {err && <Banner tone="crit" role="alert">{err}</Banner>}
        {!file && !err && <div className="cap">Up to 5,000 rows and 2 MB, comma or semicolon separated. Columns the console does not know are ignored.</div>}
        {file && loading && !plan && <div className="cap">Reading the file…</div>}
        {plan && (
          <>
            <div className="row" style={{ gap: 12 }}>
              <span style={{ fontWeight: 500 }}>{plan.totals.create} to create, {plan.totals.update} to update, {plan.totals.skip} skipped</span>
              {loading && <span className="cap">Updating the preview…</span>}
            </div>
            {plan.ignoredColumns.length > 0 && <div className="cap">Ignored columns: {plan.ignoredColumns.join(', ')}</div>}
            {replaceable.length > 0 && (
              <div className="row" style={{ gap: 14 }}>
                <span className="cap" style={{ color: 'var(--text-2)', fontWeight: 500 }}>Replace values an operator set</span>
                {replaceable.map((c) => (
                  <label key={c} className="cap" style={{ display: 'inline-flex', alignItems: 'center', gap: 6 }}>
                    <input className="cbx" type="checkbox" checked={replace.includes(c)} disabled={loading} onChange={(e) => toggle(c, e.target.checked)} />
                    {importColumnLabel(c)}
                  </label>
                ))}
              </div>
            )}
            {plan.rows.length === 0 ? <div className="cap">The file has no data rows.</div> : (
              <div className="tw" style={{ border: '1px solid var(--border)', borderRadius: 8 }}>
                <table className="tbl compact">
                  <thead><tr><th className="r">Row</th><th>Key</th><th>Outcome</th><th>Fields</th><th>Held back</th><th>Reason</th></tr></thead>
                  <tbody>{rows.map((r) => (
                    <tr key={r.row} className={r.outcome === 'skip' ? 'dim' : undefined}>
                      <td className="num r" style={{ verticalAlign: 'top' }}>{r.row}</td>
                      <td className="mono" style={{ verticalAlign: 'top' }}>{r.key || '—'}</td>
                      <td style={{ verticalAlign: 'top' }}><Pill tone={outcomePill[r.outcome].tone} xs>{outcomePill[r.outcome].label}</Pill></td>
                      <td className="wrap" style={{ minWidth: 200, verticalAlign: 'top' }}>{Object.keys(r.fields).length === 0 ? <span className="cap">—</span> : Object.entries(r.fields).map(([k, v]) => <div key={k}><span className="cap">{importColumnLabel(k)}:</span> {fmtValue(v)}</div>)}</td>
                      <td className="wrap" style={{ minWidth: 160, verticalAlign: 'top' }}>{r.heldBack.length === 0 ? <span className="cap">—</span> : r.heldBack.map((h) => <div key={h.field}><span className="cap">{importColumnLabel(h.field)}:</span> {h.reason}</div>)}</td>
                      <td className="wrap" style={{ minWidth: 160, verticalAlign: 'top' }}>{r.reason || <span className="cap">—</span>}</td>
                    </tr>
                  ))}</tbody>
                </table>
              </div>
            )}
            {pages > 1 && (
              <div className="row" style={{ gap: 10, justifyContent: 'flex-end' }}>
                <span className="cap num">Rows {page * PAGE + 1}–{Math.min((page + 1) * PAGE, plan.rows.length)} of {plan.rows.length}</span>
                <Btn sm disabled={page === 0} onClick={() => setPage(page - 1)}>Previous</Btn>
                <Btn sm disabled={page >= pages - 1} onClick={() => setPage(page + 1)}>Next</Btn>
              </div>
            )}
          </>
        )}
      </div>
    </Dialog>
  )
}
