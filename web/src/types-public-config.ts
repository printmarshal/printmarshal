// Configuration of the public status board, stored under the settings key
// `public.status` and served by GET|PUT /api/settings/public-status (admin).
// See intent/public-status-dashboard/spec.md, Design.

export interface PublicStatusDevice { id: string; label: string }

export interface PublicStatusShow {
  location: boolean
  online: boolean
  state: boolean
  paper: boolean
  toner: boolean
  alerts: boolean
}

export interface PublicStatusConfig {
  enabled: boolean
  unlisted: boolean
  slug: string
  title: string
  frameOrigins: string[]
  devices: PublicStatusDevice[]
  show: PublicStatusShow
}
