import { useCallback, useEffect, useState } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { fmtTime } from '../../api'
import type { Device } from '../../types'
import { decommissionedReason, isDecommissioned } from '../../types'
import type { CampaignView, FirmwareHistoryEntry, FirmwarePackage } from '../../types-firmware'
import { checkCompatibility, isActive, latestPackageFor } from '../../types-firmware'
import { useEvents } from '../../ws'
import { Ic } from '../icons'
import { Btn, Card, EmptyState, Tag } from '../ui'
import { fwApi } from './api'
import { CampaignPill, DeviceStatusPill } from './shared'
import CampaignWizard from './CampaignWizard'

/** Firmware tab on the device page: installed version, latest compatible package, campaign membership and update history. */
export default function DeviceFirmwareTab({ device, canOperate }: { device: Device; canOperate: boolean }) {
  const [history, setHistory] = useState<FirmwareHistoryEntry[] | null>(null)
  const [packages, setPackages] = useState<FirmwarePackage[]>([])
  const [campaigns, setCampaigns] = useState<CampaignView[]>([])
  const [wizard, setWizard] = useState(false)
  const navigate = useNavigate()
  const load = useCallback(() => {
    fwApi.history(device.id).then(setHistory).catch(() => setHistory([]))
    fwApi.packages().then(setPackages).catch(() => {})
    fwApi.campaigns({ active: true }).then(setCampaigns).catch(() => {})
  }, [device.id])
  useEffect(() => { load() }, [load, device.firmware])
  useEvents((e) => { if (e.type === 'firmware.device.updated' && (e.data as { deviceId?: string })?.deviceId === device.id) load(); else if (e.type === 'firmware.campaign.updated') load() }, [load, device.id])

  const latest = latestPackageFor(packages, device)
  const verdict = latest ? checkCompatibility(latest, device, false) : null
  const updateAvailable = !!latest && !!verdict && verdict.compatible && !verdict.current
  const activeIn = campaigns.filter((c) => isActive(c) && history?.some((h) => h.campaignId === c.id && h.status !== 'skipped'))
  const latestInstalled = history?.find((h) => h.status === 'succeeded' && !h.dryRun)

  return (
    <>
      <Card title="Firmware" id="fw" sub="versions come from polling · delivery only through a campaign with two gates"
        right={canOperate && updateAvailable ? <Btn sm variant="wr" onClick={() => setWizard(true)} disabled={isDecommissioned(device)} title={isDecommissioned(device) ? decommissionedReason : undefined}><Ic.upload />Schedule update…</Btn> : canOperate ? <Link className="btn sm" to="/firmware?tab=packages"><Ic.chip />Packages</Link> : <Tag>Read-only · Viewer</Tag>}>
        <div className="cb" style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: 12 }}>
          <div><div className="cap">Installed</div><div className="mono" style={{ fontSize: 16 }}>{device.firmware || <span className="cap">unknown</span>}</div><div className="cap">{latestInstalled ? `since ${fmtTime(latestInstalled.finishedAt)} · campaign “${latestInstalled.campaignName}”` : `as reported ${device.lastPolled ? fmtTime(device.lastPolled) : 'at last poll'}`}</div></div>
          <div><div className="cap">Available</div>
            {latest ? <>
              <div className="mono" style={{ fontSize: 16, color: updateAvailable ? 'var(--info)' : undefined }}>{latest.version}</div>
              <div className="cap">{updateAvailable ? <span className="tag up">{verdict?.downgrade ? 'older than installed' : 'update available'}</span> : verdict?.current ? 'already installed' : verdict?.reason}{latest.notes ? ` · ${latest.notes}` : ''}</div>
            </> : <><div style={{ fontSize: 14, color: 'var(--text-2)' }}>No registered package</div><div className="cap">for {device.make || 'this make'} {device.model || ''}</div></>}
          </div>
          <div><div className="cap">Campaign</div>
            {activeIn.length ? activeIn.map((c) => <div key={c.id} style={{ display: 'flex', alignItems: 'center', gap: 8, flexWrap: 'wrap' }}><Link to={`/firmware/campaigns/${c.id}`} style={{ fontWeight: 500 }}>{c.name}</Link><CampaignPill c={c} xs /></div>) : <div style={{ color: 'var(--text-2)' }}>Not in an active campaign</div>}
          </div>
        </div>
        <div className="tw" style={{ borderTop: '1px solid var(--border)' }}>
          {history === null ? <div className="cb cap" aria-busy="true">Loading history…</div>
            : history.length === 0 ? <div className="cb"><EmptyState tone="muted" icon={<Ic.chip size={20} />} title="No firmware updates recorded" text="Campaigns this device takes part in appear here with before/after versions and the outcome." /></div>
            : (
              <table className="tbl" style={{ minWidth: 640 }}>
                <thead><tr><th>Date</th><th>From → to</th><th>Campaign</th><th>Wave</th><th>Outcome</th><th>Message</th></tr></thead>
                <tbody>
                  {history.map((h) => (
                    <tr key={`${h.campaignId}-${h.startedAt || ''}`} className={h.dryRun ? 'dim' : ''}>
                      <td className="num cap" title={fmtTime(h.startedAt)}>{fmtTime(h.finishedAt || h.startedAt)}</td>
                      <td className="mono">{h.fromVersion || '?'} <Ic.arrowRight size={12} /> {h.toVersion || '?'}</td>
                      <td><Link to={`/firmware/campaigns/${h.campaignId}`}>{h.campaignName}</Link>{h.dryRun && <Tag up>dry run</Tag>}</td>
                      <td className="num">{h.wave || '—'}</td>
                      <td><DeviceStatusPill status={h.status} xs /></td>
                      <td className="wrap cap" style={{ minWidth: 200 }}>{h.message || '—'}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
        </div>
      </Card>
      {wizard && latest && <CampaignWizard preset={{ packageId: latest.id, deviceIds: [device.id], name: `${device.displayName || device.model || device.address} → ${latest.version}` }} onClose={() => setWizard(false)} onCreated={(c, o) => { setWizard(false); navigate(`/firmware/campaigns/${c.id}`, { state: { dryRun: o.dryRun, created: true } }) }} />}
    </>
  )
}
