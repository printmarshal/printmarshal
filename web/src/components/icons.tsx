// Line icon set from the design system: 24px grid, 1.75 stroke, round caps.
import type { SVGProps } from 'react'
import appIcon from '../assets/app-icon.svg'

type P = SVGProps<SVGSVGElement> & { size?: number }

function I({ size, children, className, ...rest }: P) {
  const s = size ? { width: size, height: size } : undefined
  return (
    <svg className={`i ${className || ''}`} viewBox="0 0 24 24" aria-hidden="true" style={s} {...rest}>{children}</svg>
  )
}

export const Ic = {
  dashboard: (p: P) => <I {...p}><rect x="3" y="3" width="7" height="7" rx="1" /><rect x="14" y="3" width="7" height="7" rx="1" /><rect x="3" y="14" width="7" height="7" rx="1" /><rect x="14" y="14" width="7" height="7" rx="1" /></I>,
  printer: (p: P) => <I {...p}><path d="M6 9V3h12v6M6 18H4a2 2 0 0 1-2-2v-5a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2v5a2 2 0 0 1-2 2h-2" /><rect x="6" y="14" width="12" height="7" /></I>,
  consumables: (p: P) => <I {...p}><rect x="3" y="8" width="18" height="10" rx="2" /><path d="M7 8V5h5v3M7 13h6M17 13h.01" /></I>,
  bell: (p: P) => <I {...p}><path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9M10.3 21a1.94 1.94 0 0 0 3.4 0" /></I>,
  layers: (p: P) => <I {...p}><path d="m12 2 10 5-10 5L2 7l10-5zM2 17l10 5 10-5M2 12l10 5 10-5" /></I>,
  sliders: (p: P) => <I {...p}><path d="M4 21v-7M4 10V3M12 21v-9M12 8V3M20 21v-5M20 12V3M1 14h6M9 8h6M17 16h6" /></I>,
  shield: (p: P) => <I {...p}><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" /></I>,
  shieldCheck: (p: P) => <I {...p}><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" /><path d="m9 12 2 2 4-4" /></I>,
  shieldAlert: (p: P) => <I {...p}><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" /><path d="M12 8v4M12 16h.01" /></I>,
  chip: (p: P) => <I {...p}><rect x="5" y="5" width="14" height="14" rx="2" /><path d="M9 9h6v6H9zM9 1v4M15 1v4M9 19v4M15 19v4M1 9h4M1 15h4M19 9h4M19 15h4" /></I>,
  activity: (p: P) => <I {...p}><path d="M22 12h-4l-3 9L9 3l-3 9H2" /></I>,
  radar: (p: P) => <I {...p}><circle cx="12" cy="12" r="2" /><path d="M16.2 7.8a6 6 0 0 1 0 8.4M7.8 16.2a6 6 0 0 1 0-8.4M19.1 4.9a10 10 0 0 1 0 14.2M4.9 19.1a10 10 0 0 1 0-14.2" /></I>,
  chart: (p: P) => <I {...p}><path d="M3 3v18h18M8 17v-6M13 17V6M18 17v-4" /></I>,
  settings: (p: P) => <I {...p}><circle cx="12" cy="12" r="3" /><path d="M12 2v3M12 19v3M4.2 4.2l2.1 2.1M17.7 17.7l2.1 2.1M2 12h3M19 12h3M4.2 19.8l2.1-2.1M17.7 6.3l2.1-2.1" /></I>,
  search: (p: P) => <I {...p}><circle cx="11" cy="11" r="7" /><path d="m21 21-4.3-4.3" /></I>,
  filter: (p: P) => <I {...p}><path d="M3 5h18l-7 8v6l-4 2v-8z" /></I>,
  refresh: (p: P) => <I {...p}><path d="M21 12a9 9 0 1 1-3-6.7L21 8M21 3v5h-5" /></I>,
  download: (p: P) => <I {...p}><path d="M12 3v12M7 10l5 5 5-5M4 21h16" /></I>,
  upload: (p: P) => <I {...p}><path d="M12 15V3M7 8l5-5 5 5M4 15v4a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-4" /></I>,
  plus: (p: P) => <I {...p}><path d="M12 5v14M5 12h14" /></I>,
  x: (p: P) => <I {...p}><path d="M18 6 6 18M6 6l12 12" /></I>,
  check: (p: P) => <I {...p}><path d="M20 6 9 17l-5-5" /></I>,
  chevronDown: (p: P) => <I {...p}><path d="m6 9 6 6 6-6" /></I>,
  chevronLeft: (p: P) => <I {...p}><path d="m15 18-6-6 6-6" /></I>,
  chevronRight: (p: P) => <I {...p}><path d="m9 18 6-6-6-6" /></I>,
  chevronsLeft: (p: P) => <I {...p}><path d="m11 17-5-5 5-5M18 17l-5-5 5-5" /></I>,
  more: (p: P) => <I {...p}><circle cx="5" cy="12" r="1" /><circle cx="12" cy="12" r="1" /><circle cx="19" cy="12" r="1" /></I>,
  pencil: (p: P) => <I {...p}><path d="M17 3a2.8 2.8 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5z" /></I>,
  clock: (p: P) => <I {...p}><circle cx="12" cy="12" r="9" /><path d="M12 7v5l3 2" /></I>,
  lock: (p: P) => <I {...p}><rect x="4" y="11" width="16" height="10" rx="2" /><path d="M8 11V7a4 4 0 0 1 8 0v4" /></I>,
  user: (p: P) => <I {...p}><circle cx="12" cy="8" r="4" /><path d="M4 21a8 8 0 0 1 16 0" /></I>,
  users: (p: P) => <I {...p}><circle cx="9" cy="8" r="4" /><path d="M1 21a8 8 0 0 1 16 0M17 4a4 4 0 0 1 0 8M23 21a8 8 0 0 0-5-7.4" /></I>,
  discovered: (p: P) => <I {...p}><circle cx="12" cy="12" r="2" /><path d="M16.2 7.8a6 6 0 0 1 0 8.4M7.8 16.2a6 6 0 0 1 0-8.4" /></I>,
  list: (p: P) => <I {...p}><path d="M3 6h18M3 12h18M3 18h18" /></I>,
  columns: (p: P) => <I {...p}><rect x="3" y="3" width="18" height="18" rx="2" /><path d="M9 3v18M15 3v18" /></I>,
  grid: (p: P) => <I {...p}><rect x="3" y="3" width="18" height="18" rx="2" /><path d="M3 9h18M3 15h18M9 3v18" /></I>,
  building: (p: P) => <I {...p}><path d="M3 21h18M5 21V7l7-4 7 4v14M9 21v-6h6v6" /></I>,
  pause: (p: P) => <I {...p}><path d="M8 5v14M16 5v14" /></I>,
  play: (p: P) => <I {...p}><path d="m6 4 14 8-14 8z" /></I>,
  server: (p: P) => <I {...p}><rect x="3" y="4" width="18" height="6" rx="1.5" /><rect x="3" y="14" width="18" height="6" rx="1.5" /><path d="M7 7h.01M7 17h.01" /></I>,
  external: (p: P) => <I {...p}><path d="M7 17 17 7M8 7h9v9" /></I>,
  arrowRight: (p: P) => <I {...p}><path d="M5 12h14M13 6l6 6-6 6" /></I>,
  arrowUp: (p: P) => <I {...p}><path d="m6 15 6-6 6 6" /></I>,
  sidebar: (p: P) => <I {...p}><rect x="4" y="4" width="16" height="16" rx="2" /><path d="M9 4v16" /></I>,
  info: (p: P) => <I {...p}><circle cx="12" cy="12" r="9" /><path d="M12 16v-4M12 8h.01" /></I>,
  key: (p: P) => <I {...p}><circle cx="8" cy="15" r="4" /><path d="m11 12 9-9M15 8l2 2M18 5l2 2" /></I>,
  // Status shapes: circle-check, triangle, octagon, dash, question — distinguishable without colour.
  ok: (p: P) => <I {...p}><circle cx="12" cy="12" r="9" /><path d="m8 12 3 3 5-6" /></I>,
  warn: (p: P) => <I {...p}><path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0zM12 9v4M12 17h.01" /></I>,
  crit: (p: P) => <I {...p}><path d="M7.9 2h8.2L22 7.9v8.2L16.1 22H7.9L2 16.1V7.9z" /><path d="m15 9-6 6M9 9l6 6" /></I>,
  off: (p: P) => <I {...p}><circle cx="12" cy="12" r="9" /><path d="M8 12h8" /></I>,
  unknown: (p: P) => <I {...p}><circle cx="12" cy="12" r="9" /><path d="M9.1 9a3 3 0 0 1 5.8 1c0 2-3 3-3 3M12 17h.01" /></I>,
}

/** Brand mark: the app icon (layered MFP with the shield-P), see brand/README.md. */
export function Mark({ size = 26, title }: { size?: number; dark?: boolean; title?: string }) {
  return <img src={appIcon} width={size} height={size} alt={title ?? ''} aria-hidden={title ? undefined : true} style={{ display: 'block', borderRadius: size * 0.22 }} />
}

/** Wordmark: lowercase mono "print" in the surface ink beside "Marshal" in Barlow and Cobalt. */
export function Wordmark({ size = 17 }: { size?: number }) {
  return (
    <span className="wm" style={{ fontSize: size }}>
      <span className="wm-print">print</span>
      <span className="wm-marshal">Marshal</span>
    </span>
  )
}
