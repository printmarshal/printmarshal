// Security feature types (spec §8.5, §11) — mirrors internal/model/security.go
// and the handlers in internal/api/security.go. Kept apart from types.ts so the
// shared file stays untouched.

export type PolicyOperator = 'eq' | 'ne' | 'gte' | 'lte' | 'in' | 'notIn' | 'set' | 'unset'
export type ItemSeverity = 'critical' | 'high' | 'medium' | 'low'
export type ManagementPath = 'snmp' | 'ipp' | 'http' | ''

export interface SettingChange { key: string; value?: unknown }

export interface PolicyItem {
  id: string; key: string; name: string; description?: string
  operator: PolicyOperator; value?: unknown
  severity: ItemSeverity; remediate: boolean
  remediation?: SettingChange; managementPath?: ManagementPath
}
export interface Policy {
  id: string; name: string; description: string; items: PolicyItem[]
  createdBy: string; createdAt: string; updatedAt: string
}

export type ItemStatus = 'pass' | 'fail' | 'unknown'
export interface ItemResult { itemId: string; key: string; name: string; status: ItemStatus; current?: unknown; desired?: unknown; severity: string; message?: string }
export interface Assessment {
  id: string; deviceId: string; policyId: string; items: ItemResult[]
  passed: number; failed: number; unknown: number; score: number; compliant: boolean; jobId?: string; at: string
}
export interface ComplianceSummary { devices: number; compliant: number; nonCompliant: number; avgScore: number; failingItems: Record<string, number> }
export interface ComplianceResponse { results: Assessment[]; summary: ComplianceSummary }
export interface CompliancePoint { date: string; assessed: number; compliant: number; avgScore: number }

export type RemediationStatus = 'written' | 'would_write' | 'unchanged' | 'skipped' | 'failed'
export interface RemediationItem { itemId: string; key: string; name: string; status: RemediationStatus; before?: unknown; after?: unknown; driver?: string; message?: string }
/** Per-device `data` of a security.remediate job result. */
export interface RemediationData { dryRun: boolean; items: RemediationItem[]; written: number; skipped: number; failed: number }

export interface Certificate {
  id: string; use: string; subject: string; issuer: string; serial?: string
  notBefore: string; notAfter: string; fingerprint?: string; selfSigned: boolean; keyType?: string
}
export interface DeviceCertificate extends Certificate { deviceId: string; source: string; seenAt: string }
export interface IssuedCertificate {
  id: string; deviceId: string; caProfileId: string; protocol: string; use: string; subject: string; issuer: string; serial: string
  notBefore: string; notAfter: string; fingerprint: string; keyType: string; certPem: string; chainPem?: string
  installed: boolean; installedBy?: string; installedAt?: string; jobId?: string; createdAt: string
}
export interface TLSReport { port?: number; minVersion?: string; error?: string; [k: string]: unknown }
export interface DeviceCertificates {
  deviceId: string; installed: DeviceCertificate[]; observed: DeviceCertificate[]; issued: IssuedCertificate[]
  canInstall: boolean; drivers?: string[]; tls?: TLSReport[]
}

export type EnrollProtocol = 'scep' | 'est' | 'acme'
export interface CAProfile {
  id: string; name: string; protocol: EnrollProtocol; url: string; credentialId?: string; caFingerprint?: string
  validityDays?: number; keyType?: string; options?: Record<string, string>; createdAt: string; updatedAt: string
}
export interface EnrollRequest { caProfileId: string; use: 'identity' | 'ca'; commonName?: string; sans?: string[]; dryRun: boolean }
/** Per-device `data` of a security.enroll job result. */
export interface EnrollData {
  dryRun: boolean; caProfileId: string; protocol: string; use: string; commonName: string; sans?: string[]; keyType: string
  installDriver?: string; canInstall: boolean; issuedId?: string; fingerprint?: string; notAfter?: string; subject?: string
  installed?: boolean; installError?: string; dnsRecord?: { name: string; type: string; value: string }
}

export type RotateKind = 'snmpCommunity' | 'adminPassword'
/** Per-device `data` of a security.rotate job result — never carries the secret. */
export interface RotationData { deviceId: string; kind: RotateKind; dryRun: boolean; status: 'rotated' | 'would_rotate' | 'skipped' | 'failed'; driver?: string; credentialId?: string }

export interface SecurityTarget { deviceIds?: string[]; groupId?: string }

export interface SettingDef { Key: string; Area: string; Name: string; Type: 'string' | 'int' | 'bool' | 'enum' | 'list' | 'secret'; Options?: unknown[]; Security: boolean }
export interface SecurityCatalog { settings: SettingDef[]; operators: PolicyOperator[]; severities: ItemSeverity[]; managementPaths: string[] }

export interface DeviceSecurity { assessments: Assessment[]; certificates: DeviceCertificates; alerts: { id: string; severity: string; key: string; description: string; raisedAt: string }[] }

export const severityWeight: Record<string, number> = { critical: 3, high: 2, medium: 1, low: 0.5 }
export const severityOrder: ItemSeverity[] = ['critical', 'high', 'medium', 'low']

/** Days until a certificate expires (negative when expired). */
export function daysLeft(notAfter: string, now = Date.now()): number {
  return Math.floor((new Date(notAfter).getTime() - now) / 864e5)
}

/** Management path a setting key could cut, by convention of the catalogue keys. */
export function impliedManagementPath(key: string): ManagementPath {
  if (key.startsWith('protocols.snmp.')) return 'snmp'
  if (key.startsWith('protocols.ipp')) return 'ipp'
  if (key.startsWith('protocols.http') || key.startsWith('security.tls.') || key === 'security.firewall.enabled' || key === 'security.firewall.allowList') return 'http'
  return ''
}
