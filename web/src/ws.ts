import { useEffect, useRef } from 'react'
import type { Event } from './types'

type Listener = (e: Event) => void
const listeners = new Set<Listener>()
let socket: WebSocket | null = null
let retry = 1000

function connect() {
  if (socket && (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING)) return
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  socket = new WebSocket(`${proto}://${location.host}/api/stream`)
  socket.onmessage = (m) => {
    try {
      const e = JSON.parse(m.data) as Event
      listeners.forEach((l) => l(e))
    } catch { /* ignore */ }
  }
  socket.onopen = () => {
    retry = 1000
    const e: Event = { type: 'stream.connected', at: new Date().toISOString(), data: null }
    listeners.forEach((l) => l(e))
  }
  socket.onclose = () => {
    socket = null
    const e: Event = { type: 'stream.disconnected', at: new Date().toISOString(), data: null }
    listeners.forEach((l) => l(e))
    setTimeout(connect, retry)
    retry = Math.min(retry * 2, 30000)
  }
}

/** Subscribe to live events; the socket is shared and reconnects on its own. */
export function useEvents(handler: Listener, deps: unknown[] = []) {
  const ref = useRef(handler)
  ref.current = handler
  useEffect(() => {
    connect()
    const l: Listener = (e) => ref.current(e)
    listeners.add(l)
    if (socket && socket.readyState === WebSocket.OPEN) l({ type: 'stream.connected', at: new Date().toISOString(), data: null })
    return () => { listeners.delete(l) }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps)
}

export function disconnectEvents() {
  if (socket) { socket.onclose = null; socket.close(); socket = null }
}
