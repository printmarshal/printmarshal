// Response of GET /api/public/status[/{slug}] (spec R2). The switchable fields are present only
// when the administrator turned the matching switch on; nothing else is ever sent.

export type PublicLevel = 'ok' | 'low' | 'empty' | 'unknown'

export type PublicAlertCategory = 'offline' | 'paper' | 'toner' | 'door' | 'jam' | 'maintenance' | 'other'

export interface PublicTray {
  name: string
  status: PublicLevel
}

export interface PublicSupply {
  name: string
  colorant: string
  percent: number | null
  status: PublicLevel
}

export interface PublicAlert {
  severity: string
  category: PublicAlertCategory | string
}

export interface PublicDevice {
  label: string
  location?: string
  online?: boolean
  /** Sent only when `online` is hidden and the printer did not answer its last poll. */
  unconfirmed?: boolean
  state?: string
  paper?: PublicTray[]
  toner?: PublicSupply[]
  alerts?: PublicAlert[]
}

export interface PublicBoard {
  title: string
  updatedAt: string
  stale: boolean
  devices: PublicDevice[]
}
