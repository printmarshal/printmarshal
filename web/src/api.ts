import type { User } from './types'

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) { super(message); this.status = status }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body !== undefined ? { 'Content-Type': 'application/json' } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: 'same-origin',
  })
  if (res.status === 401) {
    window.dispatchEvent(new CustomEvent('pm:unauthorized'))
  }
  const text = await res.text()
  let data: unknown = null
  try { data = text ? JSON.parse(text) : null } catch { data = text }
  if (!res.ok) {
    const msg = (data && typeof data === 'object' && 'error' in data) ? String((data as { error: string }).error) : res.statusText
    throw new ApiError(res.status, msg)
  }
  return data as T
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body),
  patch: <T>(path: string, body?: unknown) => request<T>('PATCH', path, body),
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, body),
  del: <T>(path: string) => request<T>('DELETE', path),
  login: (username: string, password: string) => request<{ user: User }>('POST', '/api/auth/login', { username, password }),
  logout: () => request<unknown>('POST', '/api/auth/logout'),
  me: () => request<User>('GET', '/api/auth/me'),
}

export function fmtTime(s?: string | null): string {
  if (!s) return '—'
  const d = new Date(s)
  if (isNaN(d.getTime()) || d.getFullYear() < 2000) return '—'
  return d.toLocaleString()
}

export function ago(s?: string | null): string {
  if (!s) return 'never'
  const d = new Date(s).getTime()
  if (isNaN(d) || d < 946684800000) return 'never'
  const sec = Math.max(0, Math.floor((Date.now() - d) / 1000))
  if (sec < 60) return `${sec}s ago`
  if (sec < 3600) return `${Math.floor(sec / 60)}m ago`
  if (sec < 86400) return `${Math.floor(sec / 3600)}h ago`
  return `${Math.floor(sec / 86400)}d ago`
}
