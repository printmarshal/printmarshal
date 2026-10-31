// The public, session-less status board (spec R11, R12, R16-R18; design.md version 2). It never
// touches the authenticated client: one plain fetch with credentials omitted, no links, no buttons.
// Each printer answers "can I print there now?" with one state word derived from its signals.
import { useEffect, useRef, useState } from 'react'
import type { PublicAlert, PublicBoard, PublicDevice, PublicSupply, PublicTray } from '../types-public'

const REFRESH_MS = 30_000
const PAGE_MS = 10_000
const PER_PAGE = 12
const MAX_FAILURES = 3

type Tone = 'ok' | 'warn' | 'crit' | 'off'

/** The derived view of one printer: the state word, its tone, and the detail that does not repeat it. */
interface DeviceView {
  word: string
  tone: Tone
  paper: string | null
  toner: PublicSupply[] | null
  extra: string[]
}

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

const alertWords: Record<string, string> = {
  jam: 'paper jam',
  door: 'cover open',
  maintenance: 'needs service',
  other: 'needs attention',
}

/** Paper as one sentence: the trays that need naming, at most two, then "+N more". */
function paperSentence(trays: PublicTray[]): string {
  const known = trays.filter((t) => t.status !== 'unknown')
  if (known.length === 0) return 'Paper unknown'
  const bad = known.filter((t) => t.status === 'low' || t.status === 'empty')
  if (bad.length === 0) return 'Paper ok'
  const named = bad.slice(0, 2)
  const rest = bad.length - named.length
  const sameStatus = named.every((t) => t.status === named[0].status)
  let text: string
  if (named.length === 1) text = `${named[0].name} ${named[0].status}`
  else if (sameStatus && rest === 0) text = `${named[0].name} and ${named[1].name} ${named[0].status}`
  else if (sameStatus) text = `${named[0].name}, ${named[1].name} ${named[0].status}`
  else text = `${named[0].name} ${named[0].status}, ${named[1].name} ${named[1].status}`
  return rest > 0 ? `${text}, +${rest} more` : text
}

/** The state word and tone from the signals, in the fixed precedence of design.md. */
function derive(d: PublicDevice): DeviceView {
  const trays = d.paper ?? null
  const supplies = d.toner ?? null
  const alerts: PublicAlert[] = d.alerts || []
  const has = (cat: string, sev?: string) => alerts.some((a) => a.category === cat && (!sev || a.severity === sev))
  const known = trays ? trays.filter((t) => t.status !== 'unknown') : []
  const anyOkTray = known.some((t) => t.status === 'ok')
  const anyEmptyTray = known.some((t) => t.status === 'empty')
  const anyBadTray = known.some((t) => t.status === 'low' || t.status === 'empty')
  const emptySupply = supplies ? supplies.some((s) => s.status === 'empty') : false
  const lowSupply = supplies ? supplies.some((s) => s.status === 'low') : false
  const signals = d.online !== undefined || trays !== null || supplies !== null || d.alerts !== undefined

  let word: string
  let tone: Tone
  if (d.online === false) { word = 'Not reachable'; tone = 'off' }
  else if (d.unconfirmed) { word = 'No status'; tone = 'off' }
  else if (has('maintenance')) { word = 'Needs service'; tone = has('maintenance', 'critical') ? 'crit' : 'warn' }
  else if (has('jam')) { word = 'Paper jam'; tone = 'crit' }
  else if (has('door')) { word = 'Cover open'; tone = 'crit' }
  else if (trays ? known.length > 0 && !anyOkTray && anyEmptyTray : has('paper', 'critical')) { word = 'Out of paper'; tone = 'crit' }
  else if (supplies ? emptySupply : has('toner', 'critical')) { word = 'Toner empty'; tone = 'crit' }
  else if (trays ? anyBadTray : has('paper')) { word = 'Low paper'; tone = 'warn' }
  else if (supplies ? lowSupply : has('toner')) { word = 'Toner low'; tone = 'warn' }
  else if (has('other')) { word = 'Needs attention'; tone = 'warn' }
  else if (signals) { word = 'Ready'; tone = 'ok' }
  else { word = 'No status'; tone = 'off' }

  // Alerts that add something: never paper or toner (the sentence and the bars say it), never the
  // one that became the state word, never offline (the word says it), each category once.
  const extra: string[] = []
  for (const a of alerts) {
    const w = alertWords[a.category]
    if (!w || w.toLowerCase() === word.toLowerCase() || extra.includes(w)) continue
    extra.push(w)
  }
  return { word, tone, paper: trays ? paperSentence(trays) : null, toner: supplies, extra }
}

const colorants: Record<string, string> = {
  cyan: 'var(--sup-c)',
  magenta: 'var(--sup-m)',
  yellow: 'var(--sup-y)',
  black: 'var(--sup-k)',
}

function ShieldGlyph({ size }: { size: number }) {
  return (
    <svg className="pub-glyph" width={size} height={size} viewBox="0 0 64 64" aria-hidden="true">
      <path d="M12 1.5 L22 5 V13 C22 19.5 17.5 24.5 12 26.5 C6.5 24.5 2 19.5 2 13 V5 Z" transform="translate(8.0,4.0) scale(2.0000)" fill="#1E3AA8" />
      <path d="M571 484Q571 421 542.5 373.0Q514 325 462.5 298.5Q411 272 344 272H202Q197 272 197 267V12Q197 7 193.5 3.5Q190 0 185 0H68Q63 0 59.5 3.5Q56 7 56 12V689Q56 694 59.5 697.5Q63 701 68 701H350Q416 701 466.0 674.0Q516 647 543.5 597.5Q571 548 571 484ZM431 481Q431 525 403.0 552.0Q375 579 329 579H202Q197 579 197 574V390Q197 385 202 385H329Q375 385 403.0 411.5Q431 438 431 481Z" transform="translate(22.59,45.00) scale(0.03000,-0.03000)" fill="#FFFFFF" />
    </svg>
  )
}

/** Four bars in the supply colours and one number: the lowest known level, or the empty colour named. */
function Toner({ supplies }: { supplies: PublicSupply[] }) {
  const levels = supplies.map((s) => (typeof s.percent === 'number' ? Math.max(0, Math.min(100, Math.round(s.percent))) : null))
  const empty = supplies.find((s) => s.status === 'empty')
  const knownLevels = levels.filter((n): n is number => n !== null)
  const lowest = knownLevels.length > 0 ? Math.min(...knownLevels) : null
  const number = empty ? `${empty.name} empty` : lowest !== null ? `${lowest}%` : 'Toner unknown'
  const sr = supplies.map((s, i) => `${s.name} ${levels[i] === null ? 'unknown' : `${levels[i]}%`}${s.status === 'low' || s.status === 'empty' ? ` ${s.status}` : ''}`).join(', ')
  return (
    <span className="pub-toner">
      <span className="pub-bars" aria-hidden="true">
        {supplies.map((s, i) => {
          const color = colorants[s.colorant?.toLowerCase()] || 'var(--off)'
          const level = levels[i]
          return (
            <span className={level === null ? 'pub-bar-unknown' : 'pub-bar'} key={`${s.name}-${i}`}>
              {level !== null && <span className="pub-fill" style={{ height: `${level}%`, background: color }} />}
            </span>
          )
        })}
      </span>
      <span className="pub-toner-num" aria-hidden="true">{number}</span>
      <span className="pub-sr">{`Toner: ${sr}`}</span>
    </span>
  )
}

function Row({ d, tile }: { d: PublicDevice; tile: boolean }) {
  const v = derive(d)
  const detail = v.paper !== null || v.toner !== null || v.extra.length > 0
  return (
    <section className={tile ? 'pub-row pub-tile' : 'pub-row'}>
      <div className={`pub-state pub-${v.tone}${tile ? ' pub-band' : ''}`}>
        <span className="pub-word">{v.word}</span>
      </div>
      <div className="pub-body">
        <h2 className="pub-label">
          <span className="pub-sr">{`${v.word}: `}</span>
          {d.label}
          {typeof d.online === 'boolean' && <>{' '}<span className="pub-verb">{d.online ? 'Online' : 'Offline'}</span></>}
          {d.state && <>{' '}<span className="pub-verb">{d.state}</span></>}
        </h2>
        {d.location && <p className="pub-location">{d.location}</p>}
        {detail && (
          <p className="pub-detail">
            {v.paper !== null && <span className="pub-paper">{v.paper}</span>}
            {v.toner !== null && v.toner.length > 0 && <Toner supplies={v.toner} />}
            {v.extra.map((w) => <span className="pub-extra" key={w}>{w}</span>)}
          </p>
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
        <ShieldGlyph size={wall ? 36 : 28} />
        <h1 className="pub-title">{board.title || 'Printer status'}</h1>
        <p className="pub-updated" aria-live="polite">{lastOk !== null ? `Updated ${since(now - (isNaN(updatedAt) ? lastOk : updatedAt))} ago` : ''}</p>
      </header>
      {board.stale && (
        <p className="pub-stale">{`Last update ${isNaN(updatedAt) ? 'a while' : since(now - updatedAt)} ago. This may be out of date.`}</p>
      )}
      <main className="pub-main">
        <div key={wall ? page : 0} className={wall ? (reduced ? 'pub-grid' : 'pub-grid-fade') : 'pub-list'}>
          {shown.map((d, i) => <Row key={`${page}-${i}-${d.label}`} d={d} tile={wall} />)}
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
