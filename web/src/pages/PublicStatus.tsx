// The public, session-less status board (spec R11, R12, R16-R18; design.md). It never touches the
// authenticated client: one plain fetch with credentials omitted, no links, no buttons.
import { useEffect, useRef, useState } from 'react'
import type { PublicAlert, PublicBoard, PublicDevice, PublicLevel, PublicSupply, PublicTray } from '../types-public'

const REFRESH_MS = 30_000
const PAGE_MS = 10_000
const PER_PAGE = 12
const MAX_FAILURES = 3

type Tone = 'ok' | 'warn' | 'crit' | 'off' | 'unk' | 'info'

/** The endpoint for the current path: /status → plain, /status/<slug> → slug. */
function endpoint(pathname: string): string {
  const rest = pathname.replace(/^\/status\/?/, '').replace(/\/+$/, '')
  return rest ? `/api/public/status/${encodeURIComponent(rest)}` : '/api/public/status'
}

function since(ms: number): string {
  const sec = Math.max(0, Math.floor(ms / 1000))
  if (sec < 60) return `${sec} s`
  if (sec < 3600) return `${Math.floor(sec / 60)} min`
  return `${Math.floor(sec / 3600)} h`
}

function levelTone(s: PublicLevel | string): Tone {
  switch (s) { case 'ok': return 'ok'; case 'low': return 'warn'; case 'empty': return 'crit'; default: return 'unk' }
}

function sevTone(s: string): Tone {
  switch (s) { case 'critical': return 'crit'; case 'warning': return 'warn'; case 'ok': return 'ok'; default: return 'info' }
}

const categoryWords: Record<string, string> = {
  jam: 'paper jam',
  door: 'cover open',
  toner: 'toner low',
  paper: 'out of paper',
  maintenance: 'needs service',
  offline: 'not reachable',
  other: 'needs attention',
}

const colorants: Record<string, { letter: string; color: string }> = {
  cyan: { letter: 'C', color: 'var(--sup-c)' },
  magenta: { letter: 'M', color: 'var(--sup-m)' },
  yellow: { letter: 'Y', color: 'var(--sup-y)' },
  black: { letter: 'K', color: 'var(--sup-k)' },
}

function ShieldGlyph({ size }: { size: number }) {
  return (
    <svg className="pub-glyph" width={size} height={size} viewBox="0 0 64 64" aria-hidden="true">
      <path d="M12 1.5 L22 5 V13 C22 19.5 17.5 24.5 12 26.5 C6.5 24.5 2 19.5 2 13 V5 Z" transform="translate(8.0,4.0) scale(2.0000)" fill="#1E3AA8" />
      <path d="M571 484Q571 421 542.5 373.0Q514 325 462.5 298.5Q411 272 344 272H202Q197 272 197 267V12Q197 7 193.5 3.5Q190 0 185 0H68Q63 0 59.5 3.5Q56 7 56 12V689Q56 694 59.5 697.5Q63 701 68 701H350Q416 701 466.0 674.0Q516 647 543.5 597.5Q571 548 571 484ZM431 481Q431 525 403.0 552.0Q375 579 329 579H202Q197 579 197 574V390Q197 385 202 385H329Q375 385 403.0 411.5Q431 438 431 481Z" transform="translate(22.59,45.00) scale(0.03000,-0.03000)" fill="#FFFFFF" />
    </svg>
  )
}

function Pill({ tone, children }: { tone: Tone; children: React.ReactNode }) {
  return <span className={`pub-pill pub-${tone}`}>{children}</span>
}

function Gauge({ supplies }: { supplies: PublicSupply[] }) {
  return (
    <span className="pub-toner">
      {supplies.map((s, i) => {
        const c = colorants[s.colorant?.toLowerCase()] || { letter: s.name, color: 'var(--off)' }
        const known = typeof s.percent === 'number'
        const pct = known ? Math.max(0, Math.min(100, Math.round(s.percent as number))) : 0
        const last = i === supplies.length - 1
        return (
          <span className="pub-supply" key={`${s.name}-${i}`}>
            <span className={known ? 'pub-bar' : 'pub-bar-unknown'} aria-hidden="true">
              {known && <span className="pub-fill" style={{ width: `${pct}%`, background: c.color }} />}
            </span>
            <span className="pub-letter" aria-hidden="true">{c.letter}</span>
            <span className="pub-sr">{`${s.name} ${known ? `${pct}%` : 'unknown'}${s.status === 'low' || s.status === 'empty' ? `, ${s.status}` : ''}`}</span>
            {last && known && <span className="pub-pct" aria-hidden="true">{pct}%</span>}
          </span>
        )
      })}
    </span>
  )
}

function Row({ d }: { d: PublicDevice }) {
  const down = d.online === false
  const edge = d.online === undefined ? 'var(--off)' : down ? 'var(--crit)' : 'var(--accent)'
  const alerts: PublicAlert[] = (d.alerts || []).filter((a) => !(down && a.category === 'offline'))
  const trays: PublicTray[] = d.paper || []
  const toner: PublicSupply[] = d.toner || []
  const hasSignals = down || trays.length > 0 || toner.length > 0 || alerts.length > 0
  return (
    <section className="pub-row">
      <span className="pub-edge" style={{ background: edge }} aria-hidden="true" />
      <div className="pub-body">
        <h2 className="pub-label">
          {d.label}
          {d.state && <span className="pub-state-word">{d.state}</span>}
        </h2>
        {d.location && <p className="pub-location">{d.location}</p>}
        {hasSignals && (
          <div className="pub-signals">
            {down && <Pill tone="crit">not reachable</Pill>}
            {trays.length > 0 && (
              <span className="pub-trays">
                {trays.map((t, i) => <Pill key={`${t.name}-${i}`} tone={levelTone(t.status)}>{`${t.name} ${t.status}`}</Pill>)}
              </span>
            )}
            {toner.length > 0 && <Gauge supplies={toner} />}
            {alerts.length > 0 && (
              <span className="pub-alerts">
                {alerts.map((a, i) => <Pill key={`${a.category}-${i}`} tone={sevTone(a.severity)}>{categoryWords[a.category] || 'needs attention'}</Pill>)}
              </span>
            )}
          </div>
        )}
      </div>
    </section>
  )
}

function StateScreen({ text }: { text: string }) {
  return (
    <main className="pub-page-state">
      <ShieldGlyph size={56} />
      <h1 className="pub-state-text">{text}</h1>
    </main>
  )
}

function useReducedMotion(): boolean {
  const q = '(prefers-reduced-motion: reduce)'
  const [reduced, setReduced] = useState(() => !!window.matchMedia && window.matchMedia(q).matches)
  useEffect(() => {
    if (!window.matchMedia) return
    const mq = window.matchMedia(q)
    const h = () => setReduced(mq.matches)
    mq.addEventListener('change', h)
    return () => mq.removeEventListener('change', h)
  }, [])
  return reduced
}

export default function PublicStatus() {
  const [board, setBoard] = useState<PublicBoard | null>(null)
  const [missing, setMissing] = useState(false)
  const [failures, setFailures] = useState(0)
  const [lastOk, setLastOk] = useState<number | null>(null)
  const [now, setNow] = useState(() => Date.now())
  const [page, setPage] = useState(0)
  const wall = new URLSearchParams(window.location.search).get('wall') === '1'
  const reduced = useReducedMotion()
  const url = useRef(endpoint(window.location.pathname))

  // The board always uses the light theme (design.md: wall screens and bright rooms).
  useEffect(() => {
    const el = document.documentElement
    const prev = el.getAttribute('data-theme')
    el.setAttribute('data-theme', 'light')
    return () => { if (prev) el.setAttribute('data-theme', prev) }
  }, [])

  useEffect(() => {
    let alive = true
    const load = async () => {
      try {
        const res = await fetch(url.current, { credentials: 'omit', cache: 'no-store', headers: { Accept: 'application/json' } })
        if (!alive) return
        if (res.status === 404) { setMissing(true); setBoard(null); setFailures(0); return }
        if (!res.ok) throw new Error(String(res.status))
        const data = (await res.json()) as PublicBoard
        if (!alive) return
        setBoard({ ...data, devices: Array.isArray(data.devices) ? data.devices : [] })
        setMissing(false)
        setFailures(0)
        setLastOk(Date.now())
      } catch {
        if (alive) setFailures((n) => n + 1)
      }
    }
    load()
    const t = window.setInterval(load, REFRESH_MS)
    return () => { alive = false; window.clearInterval(t) }
  }, [])

  useEffect(() => {
    const t = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(t)
  }, [])

  useEffect(() => {
    if (board?.title) document.title = board.title
  }, [board?.title])

  const devices = board?.devices || []
  const pages = wall ? Math.max(1, Math.ceil(devices.length / PER_PAGE)) : 1
  useEffect(() => {
    if (pages <= 1) { setPage(0); return }
    setPage((p) => (p >= pages ? 0 : p))
    const t = window.setInterval(() => setPage((p) => (p + 1) % pages), PAGE_MS)
    return () => window.clearInterval(t)
  }, [pages])

  if (failures >= MAX_FAILURES) return <StateScreen text="Status unavailable. Trying again." />
  if (missing) return <StateScreen text="This board is not available." />
  if (!board) return <main className="pub-page-state" aria-busy="true" />
  if (devices.length === 0) return <StateScreen text="No printers are listed yet." />

  const shown = wall ? devices.slice(page * PER_PAGE, page * PER_PAGE + PER_PAGE) : devices
  const updatedAt = new Date(board.updatedAt).getTime()

  return (
    <div className={wall ? 'pub-wall' : 'pub-page'}>
      <header className="pub-head">
        <div className="pub-head-in">
          <ShieldGlyph size={28} />
          <h1 className="pub-title">{board.title || 'Printer status'}</h1>
          <p className="pub-updated" aria-live="polite">{lastOk !== null ? `Updated ${since(now - (isNaN(updatedAt) ? lastOk : updatedAt))} ago` : ''}</p>
        </div>
      </header>
      {board.stale && (
        <div className="pub-stale">
          <p className="pub-stale-in">{`Last update ${isNaN(updatedAt) ? 'a while' : since(now - updatedAt)} ago. This may be out of date.`}</p>
        </div>
      )}
      <main className="pub-main">
        <div key={wall ? page : 0} className={wall ? (reduced ? 'pub-grid' : 'pub-grid-fade') : 'pub-list'}>
          {shown.map((d, i) => <Row key={`${page}-${i}-${d.label}`} d={d} />)}
        </div>
        {wall && pages > 1 && (
          <div className="pub-dots">
            <span className="pub-sr">{`Page ${page + 1} of ${pages}`}</span>
            {Array.from({ length: pages }).map((_, i) => <span key={i} className={i === page ? 'pub-dot-on' : 'pub-dot'} aria-hidden="true" />)}
          </div>
        )}
      </main>
      <footer className="pub-foot">
        <span className="wm pub-wm">
          <span className="wm-print">print</span>
          <span className="wm-marshal">Marshal</span>
        </span>
      </footer>
    </div>
  )
}
