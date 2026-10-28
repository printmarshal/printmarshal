import { useCallback, useEffect, useMemo, useState } from 'react'
import { Link, useLocation, useNavigate, useSearchParams } from 'react-router-dom'
import { api, ago, fmtTime } from '../api'
import type { Group, User } from '../types'
import type { CampaignView, FirmwarePackage, Inventory, InventoryEntry } from '../types-firmware'
import { fmtBytes, isActive, isFinished, shortSha, windowLabel } from '../types-firmware'
import { useEvents } from '../ws'
import { Ic } from '../components/icons'
import { Banner, Btn, Card, ChipSelect, EmptyState, Kpi, Pill, SearchBox, Seg, Tabs, Tag } from '../components/ui'
import { fwApi, uploadPackage } from '../components/firmware/api'
import { CampaignBar, CampaignPill, ConfirmDialog, Legend } from '../components/firmware/shared'
import CampaignWizard, { type WizardPreset } from '../components/firmware/CampaignWizard'

type Tab = 'inventory' | 'packages' | 'campaigns'

export default function Firmware({ user }: { user: User }) {
  const [params, setParams] = useSearchParams()
  const location = useLocation()
  const navigate = useNavigate()
  const tab = (params.get('tab') as Tab) || 'inventory'
  const setTab = (t: Tab) => { const p = new URLSearchParams(params); p.set('tab', t); setParams(p, { replace: true }) }
  const [packages, setPackages] = useState<FirmwarePackage[] | null>(null)
  const [campaigns, setCampaigns] = useState<CampaignView[] | null>(null)
  const [wizard, setWizard] = useState<WizardPreset | null>(location.pathname.endsWith('/campaigns/new') ? {} : null)
  const canOperate = user.role !== 'viewer'
  const isAdmin = user.role === 'admin'

  const loadPackages = useCallback(() => { fwApi.packages().then(setPackages).catch(() => setPackages([])) }, [])
  const loadCampaigns = useCallback(() => { fwApi.campaigns().then(setCampaigns).catch(() => setCampaigns([])) }, [])
  useEffect(() => { loadPackages(); loadCampaigns() }, [loadPackages, loadCampaigns])
  useEvents((e) => { if (e.type === 'firmware.campaign.updated') loadCampaigns() }, [loadCampaigns])

  const active = campaigns?.filter(isActive).length ?? 0
  const openWizard = (preset: WizardPreset = {}) => { if (canOperate) setWizard(preset) }
  const closeWizard = () => { setWizard(null); if (location.pathname.endsWith('/campaigns/new')) navigate('/firmware?tab=campaigns', { replace: true }) }

  return (
    <>
      <div className="ph">
        <div><h1 className="h1">Firmware</h1><div className="cap num">{packages ? `${packages.length} package${packages.length === 1 ? '' : 's'}` : '…'} · {campaigns ? `${campaigns.length} campaign${campaigns.length === 1 ? '' : 's'}, ${active} active` : '…'} · delivery always goes through a staged campaign with two gates</div></div>
        <div className="row">
          <Link className="btn sm" to="/jobs?type=firmware.campaign"><Ic.activity />Campaign jobs</Link>
          {canOperate && <Btn sm variant="pri" onClick={() => openWizard()}><Ic.plus />New campaign</Btn>}
        </div>
      </div>
      <Tabs label="Firmware sections" value={tab} onChange={setTab} tabs={[
        { v: 'inventory', label: 'Inventory' },
        { v: 'packages', label: <>Packages{packages && packages.length > 0 && <span className="cap num"> {packages.length}</span>}</> },
        { v: 'campaigns', label: <>Campaigns{active > 0 && <span className="pill info xs" style={{ marginLeft: 6 }}>{active} active</span>}</> },
      ]} />
      {tab === 'inventory' && <InventoryTab packages={packages || []} canOperate={canOperate} onUpdate={openWizard} />}
      {tab === 'packages' && <PackagesTab packages={packages} campaigns={campaigns || []} isAdmin={isAdmin} canOperate={canOperate} reload={loadPackages} onCampaign={openWizard} />}
      {tab === 'campaigns' && <CampaignsTab campaigns={campaigns} packages={packages || []} canOperate={canOperate} onNew={() => openWizard()} />}
      {wizard && <CampaignWizard preset={wizard} onClose={closeWizard} onCreated={(c, o) => { setWizard(null); loadCampaigns(); navigate(`/firmware/campaigns/${c.id}`, { state: { dryRun: o.dryRun, created: true } }) }} />}
    </>
  )
}

// --- Inventory ----------------------------------------------------------------

function InventoryTab({ packages, canOperate, onUpdate }: { packages: FirmwarePackage[]; canOperate: boolean; onUpdate: (p: WizardPreset) => void }) {
  const [inv, setInv] = useState<Inventory | null>(null)
  const [error, setError] = useState('')
  const [groups, setGroups] = useState<Group[]>([])
  const [make, setMake] = useState('')
  const [model, setModel] = useState('')
  const [group, setGroup] = useState('')
  const [q, setQ] = useState('')
  const [only, setOnly] = useState<'all' | 'outdated' | 'unknown'>('all')
  const [all, setAll] = useState<Inventory | null>(null)
  useEffect(() => { api.get<Group[]>('/api/groups').then(setGroups).catch(() => {}) }, [])
  useEffect(() => { fwApi.inventory({}).then(setAll).catch(() => {}) }, [])
  const load = useCallback(() => { setError(''); fwApi.inventory({ make, model, group, q }).then(setInv).catch((e) => { setError((e as Error).message); setInv({ entries: [], devices: 0, outdated: 0, unknown: 0 }) }) }, [make, model, group, q])
  useEffect(() => { const t = setTimeout(load, q ? 250 : 0); return () => clearTimeout(t) }, [load, q])
  useEvents((e) => { if (e.type === 'device.updated' || e.type === 'device.polled' || e.type === 'firmware.device.updated' || e.type === 'firmware.campaign.updated') load() }, [load])

  const makes = useMemo(() => Array.from(new Set((all?.entries || []).map((e) => e.make).filter(Boolean))).sort(), [all])
  const models = useMemo(() => Array.from(new Set((all?.entries || []).filter((e) => !make || e.make === make).map((e) => e.model).filter(Boolean))).sort(), [all, make])
  const entries = useMemo(() => {
    const list = (inv?.entries || []).filter((e) => only === 'all' || (only === 'outdated' ? e.outdated : e.version === ''))
    return [...list].sort((a, b) => (a.make || '~').localeCompare(b.make || '~') || (a.model || '~').localeCompare(b.model || '~') || (b.version || '').localeCompare(a.version || ''))
  }, [inv, only])
  const byMake = useMemo(() => { const m = new Map<string, InventoryEntry[]>(); for (const e of entries) (m.get(e.make || '') || m.set(e.make || '', []).get(e.make || '')!).push(e); return m }, [entries])
  const filtered = !!(make || model || group || q)

  const devicesLink = (e: InventoryEntry) => { const p = new URLSearchParams(); if (e.make) p.set('make', e.make); if (e.model) p.set('model', e.model); if (group) p.set('group', group); return `/devices?${p.toString()}` }

  return (
    <>
      <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(160px, 1fr))', gap: 12 }}>
        <Kpi label="Devices" value={inv ? inv.devices : '—'} detail={filtered ? 'matching filters' : 'with a known make/model'} />
        <Kpi label="Outdated" value={inv ? inv.outdated : '—'} color={inv && inv.outdated > 0 ? 'var(--warn)' : undefined} detail="a newer registered package fits" onClick={() => setOnly(only === 'outdated' ? 'all' : 'outdated')} to="#" />
        <Kpi label="Version unknown" value={inv ? inv.unknown : '—'} detail="device reports no firmware version" onClick={() => setOnly(only === 'unknown' ? 'all' : 'unknown')} to="#" />
        <Kpi label="Packages" value={packages.length} detail="registered images" />
      </div>
      <div className="row">
        <SearchBox sm filter value={q} onChange={setQ} placeholder="Filter by make, model, version…" />
        <ChipSelect label="Make" value={make} onChange={(v) => { setMake(v); setModel('') }} options={makes.map((m) => ({ v: m, label: m }))} />
        <ChipSelect label="Model" value={model} onChange={setModel} options={models.map((m) => ({ v: m, label: m }))} />
        <ChipSelect label="Group" value={group} onChange={setGroup} options={groups.map((g) => ({ v: g.id, label: g.name }))} />
        <Seg sm label="Show" value={only} onChange={setOnly} options={[{ v: 'all', label: 'All' }, { v: 'outdated', label: 'Outdated' }, { v: 'unknown', label: 'Unknown' }]} />
        {(filtered || only !== 'all') && <Btn sm variant="ghost" onClick={() => { setMake(''); setModel(''); setGroup(''); setQ(''); setOnly('all') }}>Clear all</Btn>}
      </div>
      {error && <Banner tone="crit">{error}</Banner>}
      <Card aria-label="Firmware inventory" title="Installed versions" id="inv" sub="grouped by make · model · installed version · “outdated” means a newer compatible package is registered">
        {inv === null ? <div className="cb" aria-busy="true">{[0, 1, 2].map((i) => <div key={i} className="row" style={{ padding: '8px 0' }}><span className="skel" style={{ width: 60, height: 18, borderRadius: 999 }} /><span className="skel" style={{ flex: 1, height: 10 }} /></div>)}</div>
          : entries.length === 0 ? <div className="cb"><EmptyState icon={<Ic.chip size={20} />} title={filtered || only !== 'all' ? 'Nothing matches' : 'No firmware inventory yet'} text={filtered || only !== 'all' ? 'Try fewer filters.' : 'Versions appear once devices are polled; register a package to see which devices are outdated.'} /></div>
          : (
            <div className="tw"><table className="tbl" style={{ minWidth: 820 }}>
              <thead><tr><th>Model</th><th>Installed</th><th className="r">Devices</th><th>Latest package</th><th>Status</th><th /></tr></thead>
              <tbody>
                {Array.from(byMake.entries()).map(([mk, list]) => (
                  <GroupRows key={mk || '(unknown)'} make={mk} list={list} canOperate={canOperate} onUpdate={onUpdate} devicesLink={devicesLink} packages={packages} />
                ))}
              </tbody>
            </table></div>
          )}
      </Card>
    </>
  )
}

function GroupRows({ make, list, canOperate, onUpdate, devicesLink, packages }: { make: string; list: InventoryEntry[]; canOperate: boolean; onUpdate: (p: WizardPreset) => void; devicesLink: (e: InventoryEntry) => string; packages: FirmwarePackage[] }) {
  const total = list.reduce((n, e) => n + e.count, 0)
  const outdated = list.filter((e) => e.outdated).reduce((n, e) => n + e.count, 0)
  return (
    <>
      <tr className="grp"><td colSpan={6}><span>{make || 'Unknown make'}</span><span className="cap num" style={{ marginLeft: 10, fontWeight: 400 }}>{total} device{total === 1 ? '' : 's'}{outdated > 0 ? ` · ${outdated} outdated` : ''}</span></td></tr>
      {list.map((e) => {
        const key = `${e.make}|${e.model}|${e.version}`
        const pkg = e.latestPackageId ? packages.find((p) => p.id === e.latestPackageId) : undefined
        return (
          <tr key={key}>
            <td className="mono">{e.model || <span className="cap">unknown model</span>}</td>
            <td>{e.version ? <span className="mono">{e.version}</span> : <span className="cap">unknown</span>}</td>
            <td className="num r"><Link to={devicesLink(e)} title="Open these devices">{e.count}</Link></td>
            <td>{e.latest ? <span className="mono" title={pkg ? `${pkg.fileName} · ${fmtBytes(pkg.size)}` : undefined}>{e.latest}</span> : <span className="cap">none registered</span>}</td>
            <td>{e.outdated ? <Pill tone="warn" xs>Outdated</Pill> : e.version === '' ? <Pill tone="unk" xs>Unknown</Pill> : e.latest ? <Pill tone="ok" xs>Current</Pill> : <span className="pill off xs" title="No registered package for this make/model">No package</span>}</td>
            <td className="r">{canOperate && e.outdated && e.latestPackageId && <Btn sm variant="wr" onClick={() => onUpdate({ packageId: e.latestPackageId, deviceIds: e.deviceIds, name: `${e.model} → ${e.latest}` })}><Ic.upload />Update {e.count}…</Btn>}</td>
          </tr>
        )
      })}
    </>
  )
}

// --- Packages -----------------------------------------------------------------

function PackagesTab({ packages, campaigns, isAdmin, canOperate, reload, onCampaign }: { packages: FirmwarePackage[] | null; campaigns: CampaignView[]; isAdmin: boolean; canOperate: boolean; reload: () => void; onCampaign: (p: WizardPreset) => void }) {
  const [del, setDel] = useState<FirmwarePackage | null>(null)
  const [delErr, setDelErr] = useState('')
  const [busy, setBusy] = useState(false)
  const [note, setNote] = useState<{ tone: 'ok' | 'crit'; text: string } | null>(null)
  const inUse = (p: FirmwarePackage) => campaigns.filter((c) => c.packageId === p.id && !isFinished(c)).length
  const remove = async () => {
    if (!del) return
    setBusy(true); setDelErr('')
    try { await fwApi.deletePackage(del.id); setNote({ tone: 'ok', text: `Deleted ${del.vendor} ${del.version} (${del.fileName}).` }); setDel(null); reload() } catch (e) { setDelErr((e as Error).message) } finally { setBusy(false) }
  }
  return (
    <>
      {note && <Banner tone={note.tone} right={<Btn sm variant="ghost" ic onClick={() => setNote(null)} aria-label="Dismiss"><Ic.x /></Btn>}>{note.text}</Banner>}
      <div className="cols">
        <Card title="Registered packages" id="pk" sub="SHA-256 recorded at upload and re-verified before every delivery · vendor and model match as case-insensitive substrings" style={{ flex: '3 1 620px' }}>
          {packages === null ? <div className="cb cap" aria-busy="true">Loading…</div>
            : packages.length === 0 ? <div className="cb"><EmptyState icon={<Ic.upload size={20} />} title="No packages registered" text={isAdmin ? 'Upload a firmware image to register it.' : 'An admin uploads firmware images here; campaigns then deliver them.'} /></div>
            : (
              <div className="tw"><table className="tbl" style={{ minWidth: 900 }}>
                <thead><tr><th>Vendor</th><th>Models</th><th>Version</th><th>Min version</th><th className="r">Size</th><th>SHA-256</th><th>Notes</th><th>Registered</th><th /></tr></thead>
                <tbody>
                  {packages.map((p) => { const n = inUse(p); return (
                    <tr key={p.id}>
                      <td style={{ fontWeight: 500 }}>{p.vendor || <span className="cap">any</span>}</td>
                      <td><span style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>{p.models.length ? p.models.map((m) => <Tag key={m} mono>{m}</Tag>) : <Tag>any</Tag>}</span></td>
                      <td className="mono">{p.version}</td>
                      <td className="mono">{p.minVersion || <span className="cap">—</span>}</td>
                      <td className="num r" title={p.fileName}>{fmtBytes(p.size)}</td>
                      <td><span className="mono" title={p.sha256} style={{ fontSize: 11 }}>{shortSha(p.sha256)}</span></td>
                      <td className="wrap cap" style={{ minWidth: 160 }}>{p.notes || '—'}</td>
                      <td className="cap">{p.createdBy} · <span title={fmtTime(p.createdAt)}>{ago(p.createdAt)}</span>{n > 0 && <div><span className="pill info xs" title="Referenced by an unfinished campaign; cannot be deleted">{n} active campaign{n === 1 ? '' : 's'}</span></div>}</td>
                      <td className="r"><span className="row" style={{ justifyContent: 'flex-end', flexWrap: 'nowrap' }}>
                        {canOperate && <Btn sm onClick={() => onCampaign({ packageId: p.id })}><Ic.plus />Campaign</Btn>}
                        {isAdmin && <Btn sm variant="dng" onClick={() => { setDel(p); setDelErr('') }} title={n > 0 ? 'Refused while an unfinished campaign references it' : 'Delete package'}>Delete</Btn>}
                      </span></td>
                    </tr>
                  ) })}
                </tbody>
              </table></div>
            )}
        </Card>
        {isAdmin ? <UploadCard onUploaded={(p) => { setNote({ tone: 'ok', text: `Registered ${p.vendor} ${p.version} (${p.fileName}, ${fmtBytes(p.size)}).` }); reload() }} /> : (
          <Card style={{ flex: '1 1 300px', maxWidth: 420 }}><div className="cb"><EmptyState tone="muted" icon={<Ic.lock size={20} />} title="Upload needs Admin" text={`Your role is ${canOperate ? 'Operator' : 'Viewer'}. Registering firmware images is an admin action; campaigns are not.`} action={<Tag>Read-only</Tag>} /></div></Card>
        )}
      </div>
      {del && (
        <ConfirmDialog title={`Delete package ${del.vendor} ${del.version}?`} sub={`${del.fileName} · ${fmtBytes(del.size)} · ${shortSha(del.sha256)}`} tone="crit" confirmLabel="Delete package" confirmVariant="dngf" busy={busy} error={delErr} onConfirm={remove} onClose={() => setDel(null)}
          rows={[{ label: 'Models', value: del.models.join(', ') || 'any' }, { label: 'Campaigns', value: inUse(del) > 0 ? <span style={{ color: 'var(--crit)' }}>{inUse(del)} unfinished — the server will refuse</span> : `${campaigns.filter((c) => c.packageId === del.id).length} finished keep their history` }]}>
          <div className="cap" style={{ marginTop: 8 }}>The file is removed from the data directory. Devices are not touched.</div>
        </ConfirmDialog>
      )}
    </>
  )
}

function UploadCard({ onUploaded }: { onUploaded: (p: FirmwarePackage) => void }) {
  const [file, setFile] = useState<File | null>(null)
  const [vendor, setVendor] = useState('')
  const [models, setModels] = useState('')
  const [version, setVersion] = useState('')
  const [minVersion, setMinVersion] = useState('')
  const [notes, setNotes] = useState('')
  const [busy, setBusy] = useState(false)
  const [msg, setMsg] = useState('')
  const submit = async (e: React.FormEvent) => {
    e.preventDefault(); setMsg('')
    if (!file) { setMsg('Choose a firmware file.'); return }
    if (!version.trim()) { setMsg('Version is required.'); return }
    setBusy(true)
    try {
      const p = await uploadPackage({ file, vendor: vendor.trim(), models, version: version.trim(), minVersion: minVersion.trim(), notes: notes.trim() })
      onUploaded(p); setFile(null); setVendor(''); setModels(''); setVersion(''); setMinVersion(''); setNotes('')
      ;(e.target as HTMLFormElement).reset()
    } catch (err) { setMsg((err as Error).message) } finally { setBusy(false) }
  }
  return (
    <Card title="Register a package" id="up" sub="admin · the file is stored under the data directory and hashed" style={{ flex: '1 1 320px', maxWidth: 460 }}>
      <form className="cb" onSubmit={submit} style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
        <label className="cap">Firmware file<input className="fin" type="file" required onChange={(e) => setFile(e.target.files?.[0] || null)} style={{ marginTop: 4, padding: '5px 10px' }} />{file && <div className="cap num" style={{ marginTop: 4 }}>{file.name} · {fmtBytes(file.size)}</div>}</label>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
          <label className="cap">Vendor<input className="fin" value={vendor} onChange={(e) => setVendor(e.target.value)} placeholder="e.g. HP" style={{ marginTop: 4 }} /></label>
          <label className="cap">Version<input className="fin mono" value={version} onChange={(e) => setVersion(e.target.value)} placeholder="3.0.0" required style={{ marginTop: 4 }} /></label>
        </div>
        <label className="cap">Models <span style={{ fontWeight: 400 }}>· comma-separated substrings</span><input className="fin mono" value={models} onChange={(e) => setModels(e.target.value)} placeholder="LaserPrint, 4200" style={{ marginTop: 4 }} /></label>
        <label className="cap">Minimum installed version <span style={{ fontWeight: 400 }}>· optional</span><input className="fin mono" value={minVersion} onChange={(e) => setMinVersion(e.target.value)} placeholder="2.0.0" style={{ marginTop: 4 }} /></label>
        <label className="cap">Notes<textarea className="fin" rows={2} value={notes} onChange={(e) => setNotes(e.target.value)} placeholder="release notes, security fixes…" style={{ marginTop: 4 }} /></label>
        {msg && <div className="error" style={{ fontSize: 12 }}>{msg}</div>}
        <Btn type="submit" variant="pri" disabled={busy}><Ic.upload />{busy ? 'Uploading…' : 'Register package'}</Btn>
        <div className="cap">Registering never touches a device. Vendor “HP” matches “Hewlett-Packard”; model “4200” matches “Acme LaserPro 4200”.</div>
      </form>
    </Card>
  )
}

// --- Campaigns ----------------------------------------------------------------

function CampaignsTab({ campaigns, packages, canOperate, onNew }: { campaigns: CampaignView[] | null; packages: FirmwarePackage[]; canOperate: boolean; onNew: () => void }) {
  const [show, setShow] = useState<'active' | 'all' | 'finished' | 'draft'>('all')
  const [groups, setGroups] = useState<Group[]>([])
  useEffect(() => { api.get<Group[]>('/api/groups').then(setGroups).catch(() => {}) }, [])
  const list = (campaigns || []).filter((c) => show === 'all' || (show === 'active' ? isActive(c) : show === 'finished' ? isFinished(c) : c.status === 'draft'))
  const target = (c: CampaignView) => c.groupId ? `Group “${groups.find((g) => g.id === c.groupId)?.name || c.groupId}”` : `${c.deviceIds?.length || 0} device${c.deviceIds?.length === 1 ? '' : 's'}`
  const counts = { active: (campaigns || []).filter(isActive).length, draft: (campaigns || []).filter((c) => c.status === 'draft').length, finished: (campaigns || []).filter(isFinished).length }
  return (
    <Card aria-label="Campaigns" title="Campaigns" id="cp" sub="draft → scheduled → running → paused (canary · threshold · operator · interrupted) → completed · failed · cancelled"
      right={<Seg sm label="Show campaigns" value={show} onChange={setShow} options={[{ v: 'all', label: `All ${campaigns?.length ?? ''}` }, { v: 'active', label: `Active ${counts.active}` }, { v: 'draft', label: `Drafts ${counts.draft}` }, { v: 'finished', label: `Finished ${counts.finished}` }]} />}>
      {campaigns === null ? <div className="cb cap" aria-busy="true">Loading…</div>
        : list.length === 0 ? <div className="cb"><EmptyState icon={<Ic.chip size={20} />} title={show === 'all' ? 'No campaigns yet' : `No ${show} campaigns`} text="A campaign delivers one package to a group or device list in two waves: a canary, then the rest after you confirm." action={canOperate && show === 'all' ? <Btn sm variant="pri" onClick={onNew}><Ic.plus />New campaign</Btn> : undefined} /></div>
        : (
          <div className="tw"><table className="tbl" style={{ minWidth: 960 }}>
            <thead><tr><th>Campaign</th><th>Status</th><th>Package</th><th>Target</th><th>Wave</th><th style={{ width: 240 }}>Progress</th><th>Window · schedule</th><th className="r">Updated</th></tr></thead>
            <tbody>
              {list.map((c) => { const p = packages.find((x) => x.id === c.packageId); const s = c.summary; return (
                <tr key={c.id}>
                  <td><Link to={`/firmware/campaigns/${c.id}`} style={{ fontWeight: 500 }}>{c.name}</Link><div className="cap" style={{ fontSize: 11 }}>{c.dryRun && c.status !== 'draft' ? <span className="tag up" style={{ marginRight: 6 }}>dry run</span> : null}by {c.createdBy}{c.startedBy && c.startedBy !== c.createdBy ? ` · started by ${c.startedBy}` : ''}</div></td>
                  <td><CampaignPill c={c} /></td>
                  <td><span className="mono">{p ? `${p.vendor} ${p.version}` : c.packageVersion || c.packageId}</span>{p && p.models.length > 0 && <div className="cap" style={{ fontSize: 11 }}>{p.models.join(', ')}</div>}</td>
                  <td className="cap" style={{ color: 'var(--text-2)' }}>{target(c)}</td>
                  <td className="num">{c.status === 'draft' ? <span className="cap">—</span> : c.wave === 1 ? '1 · canary' : c.wave === 2 ? '2 · remainder' : <span className="cap">—</span>}{c.status !== 'draft' && c.canaryCount === 0 && <div className="cap" style={{ fontSize: 11 }}>no canary</div>}</td>
                  <td>{s.total > 0 ? <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}><CampaignBar s={s} height={6} /><Legend s={s} compact /></div> : <span className="cap">not planned yet</span>}</td>
                  <td className="cap" style={{ color: 'var(--text-2)' }}>{windowLabel(c.window)}{c.scheduledAt && <div className="num" style={{ fontSize: 11 }}><Ic.clock size={11} /> {fmtTime(c.scheduledAt)}</div>}</td>
                  <td className="cap num r" title={fmtTime(c.updatedAt)}>{ago(c.updatedAt)}</td>
                </tr>
              ) })}
            </tbody>
          </table></div>
        )}
    </Card>
  )
}
