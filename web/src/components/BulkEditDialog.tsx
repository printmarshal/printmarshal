import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api } from '../api'
import type { BulkPatchResponse, Device } from '../types'
import { Btn, Dialog } from './ui'

type Tri = 'leave' | 'on' | 'off'

/** Whether reverse DNS lookup is switched off in Settings (read as the device page does). */
export function useReverseDnsOff(): boolean {
  const [off, setOff] = useState(false)
  useEffect(() => { api.get<{ runtime?: { reverseDns?: { enabled?: boolean } } }>('/api/settings').then((st) => setOff(st.runtime?.reverseDns?.enabled === false)).catch(() => {}) }, [])
  return off
}

/** The hint that DNS sync only follows DNS while reverse DNS lookup runs. */
export function ReverseDnsOffHint({ canEdit }: { canEdit: boolean }) {
  return <span className="cap" style={{ fontSize: 11 }}>Reverse DNS is off, so DNS sync will not keep these names up to date on its own. {canEdit ? <Link to="/settings">Turn it on in Settings</Link> : 'An administrator can turn it on in Settings'}.</span>
}

/** Edit operator fields on many devices at once; every field can be left unchanged. */
export function BulkEditDialog({ deviceIds, devices, onClose, onApplied }: { deviceIds: string[]; devices: Device[]; onClose: () => void; onApplied: (res: BulkPatchResponse) => void }) {
  const [dnsSync, setDnsSync] = useState<Tri>('leave')
  const [rename, setRename] = useState(false)
  const [withLocation, setWithLocation] = useState(false)
  const [location, setLocation] = useState('')
  const [note, setNote] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const n = deviceIds.length
  const syncedNow = devices.filter((d) => d.dnsSync).length
  const rdnsOff = useReverseDnsOff()
  const nothing = dnsSync === 'leave' && !rename && !(withLocation && location.trim()) && !note.trim()

  const apply = async () => {
    setBusy(true)
    setErr('')
    try {
      const set: { dnsSync?: boolean; location?: string; appendNote?: string } = {}
      if (dnsSync !== 'leave') set.dnsSync = dnsSync === 'on'
      if (withLocation && location.trim()) set.location = location.trim()
      if (note.trim()) set.appendNote = note.trim()
      const res = await api.post<BulkPatchResponse>('/api/devices/bulk', { deviceIds, renameFromDns: rename, set })
      onApplied(res)
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Dialog
      title={`Edit ${n} device${n === 1 ? '' : 's'}`}
      sub="Operator fields only. Each field you leave alone stays as it is on every device; notes are added, never replaced."
      onClose={onClose}
      footer={<><Btn variant="ghost" onClick={onClose}>Cancel</Btn><Btn variant="pri" disabled={busy || nothing} onClick={apply}>{busy ? 'Applying…' : `Apply to ${n}`}</Btn></>}
    >
      <div className="stack" style={{ gap: 14 }}>
        <div className="row" style={{ gap: 10 }}>
          <label className="cap" htmlFor="be-sync">DNS sync</label>
          <select id="be-sync" className="fin" style={{ width: 240 }} value={dnsSync} onChange={(e) => setDnsSync(e.target.value as Tri)}>
            <option value="leave">Leave as is</option>
            <option value="on">On: keep display names synced with DNS</option>
            <option value="off">Off</option>
          </select>
          <span className="cap" style={{ color: 'var(--text-3)' }}>Currently on for {syncedNow} of {devices.length}</span>
        </div>
        {rdnsOff && <ReverseDnsOffHint canEdit />}
        <label className="cap" style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
          <input className="cbx" type="checkbox" checked={rename} onChange={(e) => setRename(e.target.checked)} />
          Rename from DNS now <span style={{ color: 'var(--text-3)' }}>(devices without a usable DNS name are skipped)</span>
        </label>
        <div className="row" style={{ gap: 10, alignItems: 'center' }}>
          <label className="cap" style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
            <input className="cbx" type="checkbox" checked={withLocation} onChange={(e) => setWithLocation(e.target.checked)} />
            Set location
          </label>
          <input className="fin" aria-label="Location" style={{ width: 280 }} value={location} disabled={!withLocation} maxLength={200} placeholder="e.g. Building B · Floor 3" onChange={(e) => setLocation(e.target.value)} />
        </div>
        <div>
          <label className="cap" htmlFor="be-note">Add a note to each device</label>
          <textarea id="be-note" className="fin" rows={3} maxLength={2000} value={note} placeholder="Appended on its own paragraph; existing notes are kept." onChange={(e) => setNote(e.target.value)} />
        </div>
        {err && <div className="banner crit">{err}</div>}
      </div>
    </Dialog>
  )
}
