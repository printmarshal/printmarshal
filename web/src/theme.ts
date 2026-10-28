import { useEffect, useState } from 'react'

export type Theme = 'light' | 'dark' | 'system'
export type Density = 'comfortable' | 'compact'

const THEME_KEY = 'pm.theme'
const DENSITY_KEY = 'pm.density'

function read(key: string, fallback: string): string {
  try { return localStorage.getItem(key) || fallback } catch { return fallback }
}
function write(key: string, v: string) {
  try { localStorage.setItem(key, v) } catch { /* ignore */ }
}

export function resolveTheme(t: Theme): 'light' | 'dark' {
  if (t !== 'system') return t
  return window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

export function applyTheme(t: Theme) {
  document.documentElement.setAttribute('data-theme', resolveTheme(t))
}

export function useTheme(): [Theme, (t: Theme) => void] {
  const [theme, setThemeState] = useState<Theme>(() => (read(THEME_KEY, 'system') as Theme))
  useEffect(() => {
    applyTheme(theme)
    if (theme !== 'system' || !window.matchMedia) return
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const h = () => applyTheme('system')
    mq.addEventListener('change', h)
    return () => mq.removeEventListener('change', h)
  }, [theme])
  return [theme, (t) => { write(THEME_KEY, t); setThemeState(t) }]
}

export function useDensity(): [Density, (d: Density) => void] {
  const [d, set] = useState<Density>(() => (read(DENSITY_KEY, 'comfortable') as Density))
  return [d, (v) => { write(DENSITY_KEY, v); set(v) }]
}

// Apply once at load so the first paint has the right theme.
applyTheme(read(THEME_KEY, 'system') as Theme)
