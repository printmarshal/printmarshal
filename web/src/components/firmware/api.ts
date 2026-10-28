import { api, ApiError } from '../../api'
import type { Campaign, CampaignDeviceRow, CampaignRequest, CampaignView, FirmwareHistoryEntry, FirmwarePackage, Inventory, OutdatedDevice } from '../../types-firmware'

/** Firmware endpoints (docs/firmware.md § API). Reads are open to viewers; packages are admin; campaign gates are operator. */
export const fwApi = {
  inventory: (q: { make?: string; model?: string; group?: string; q?: string }) => {
    const p = new URLSearchParams()
    for (const [k, v] of Object.entries(q)) if (v) p.set(k, v)
    const s = p.toString()
    return api.get<Inventory>(`/api/firmware/inventory${s ? `?${s}` : ''}`)
  },
  outdated: () => api.get<OutdatedDevice[]>('/api/firmware/outdated'),
  packages: () => api.get<FirmwarePackage[]>('/api/firmware/packages'),
  package: (id: string) => api.get<FirmwarePackage>(`/api/firmware/packages/${id}`),
  deletePackage: (id: string) => api.del<void>(`/api/firmware/packages/${id}`),
  campaigns: (q?: { status?: string; package?: string; active?: boolean }) => {
    const p = new URLSearchParams()
    if (q?.status) p.set('status', q.status)
    if (q?.package) p.set('package', q.package)
    if (q?.active) p.set('active', 'true')
    const s = p.toString()
    return api.get<CampaignView[]>(`/api/firmware/campaigns${s ? `?${s}` : ''}`)
  },
  campaign: (id: string) => api.get<{ campaign: CampaignView; devices: CampaignDeviceRow[] }>(`/api/firmware/campaigns/${id}`),
  campaignDevices: (id: string) => api.get<CampaignDeviceRow[]>(`/api/firmware/campaigns/${id}/devices`),
  createCampaign: (body: CampaignRequest) => api.post<CampaignView>('/api/firmware/campaigns', body),
  patchCampaign: (id: string, body: CampaignRequest) => api.patch<CampaignView>(`/api/firmware/campaigns/${id}`, body),
  deleteCampaign: (id: string) => api.del<void>(`/api/firmware/campaigns/${id}`),
  /** Gate for wave 1. A dry run walks the whole campaign without delivering anything. */
  start: (id: string, dryRun: boolean) => api.post<CampaignView>(`/api/firmware/campaigns/${id}/start`, { dryRun }),
  /** Gate for wave 2, after the canary wave paused the campaign. */
  confirm: (id: string) => api.post<CampaignView>(`/api/firmware/campaigns/${id}/confirm`),
  pause: (id: string) => api.post<CampaignView>(`/api/firmware/campaigns/${id}/pause`),
  resume: (id: string) => api.post<CampaignView>(`/api/firmware/campaigns/${id}/resume`),
  cancel: (id: string) => api.post<CampaignView>(`/api/firmware/campaigns/${id}/cancel`),
  /** GET /api/devices/{id}/firmware answers {deviceId, firmware, history}; only the history rows matter here. */
  history: (deviceId: string) => api.get<{ deviceId: string; firmware: string; history: FirmwareHistoryEntry[] | null }>(`/api/devices/${deviceId}/firmware`).then((r) => r.history || []),
}

export interface PackageUpload { file: File; vendor: string; models: string; version: string; minVersion: string; notes: string }

/** POST /api/firmware/packages is multipart, which the shared api client does not do. Admin only. */
export async function uploadPackage(u: PackageUpload): Promise<FirmwarePackage> {
  const fd = new FormData()
  fd.append('file', u.file, u.file.name)
  fd.append('vendor', u.vendor)
  fd.append('models', u.models)
  fd.append('version', u.version)
  fd.append('minVersion', u.minVersion)
  fd.append('notes', u.notes)
  const res = await fetch('/api/firmware/packages', { method: 'POST', body: fd, credentials: 'same-origin' })
  if (res.status === 401) window.dispatchEvent(new CustomEvent('pm:unauthorized'))
  const text = await res.text()
  let data: unknown = null
  try { data = text ? JSON.parse(text) : null } catch { data = text }
  if (!res.ok) {
    const msg = (data && typeof data === 'object' && 'error' in data) ? String((data as { error: string }).error) : res.statusText
    throw new ApiError(res.status, msg)
  }
  return data as FirmwarePackage
}

export type { Campaign }
