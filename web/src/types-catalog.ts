// Types for the device model catalogue feature (internal/catalog). Field names
// with underscores mirror the research JSON verbatim; camelCase fields are
// computed by PrintMarshal.

export type Operation = 'identify' | 'monitor' | 'configure' | 'secure' | 'update'
export const operations: Operation[] = ['identify', 'monitor', 'configure', 'secure', 'update']
export const operationLabels: Record<Operation, string> = { identify: 'Identify', monitor: 'Monitor', configure: 'Configure', secure: 'Secure', update: 'Update' }

/** Documented side: what a primary source says. Never "supported". */
export type DocumentedState = 'documented' | 'unsupported' | 'unknown'
/** Observed side: what PrintMarshal's own drivers answered. */
export type ObservedState = 'observed' | 'unknown'
export type OpState = 'documented+observed' | 'documented' | 'observed' | 'unsupported' | 'unknown'
export type Confidence = 'exact' | 'family' | 'brand-only' | 'none'
export type QualificationStatus = 'research_candidate' | 'lab_validated' | 'production_validated' | 'unsupported'
export const qualificationStatuses: QualificationStatus[] = ['research_candidate', 'lab_validated', 'production_validated', 'unsupported']
export const qualificationLabels: Record<QualificationStatus, string> = {
  research_candidate: 'Research candidate', lab_validated: 'Lab validated', production_validated: 'Production validated', unsupported: 'Unsupported',
}
export type Tier = 'validated' | 'documented+observed' | 'observed only (standards)' | 'documented only' | 'unsupported' | 'unknown'

export interface Family {
  id: string; brand: string; model_prefix: string; model_ids: string[]; segment: string; management_profile: string
  model_source_ids: string[]; model_evidence: string; release_year: number | null; lifecycle: string
  qualification_status: string; note: string; release_year_scope: string; management_support_evidence_date?: string
}
export interface ManagementProfile {
  label: string; read_path: string; configuration_path: string; firmware_path: string; general_admin_api: string; source_ids: string[]; scope: string
}
export interface Evidence { status: string; source_ids: string[]; minimum_firmware?: string; licence?: string }
export interface OperationEvidence {
  id: string; model_ids: string[]; brand: string
  monitoring?: Evidence; configuration?: Evidence; security?: Evidence; firmware?: Evidence; app_management?: Evidence
  general_device_management?: string; third_party_admin_contract?: string; physical_validation: boolean
}
export interface Source { title: string; url: string; access: string; checked_on: string; first_pass_access_note?: string }
export interface IdentityConflict { brand: string; product_number: string; observations: { model: string; bundle: string; source_id: string }[]; resolution: string }
export interface VariantExample { base_model: string; bundle: string; product_number: string; source_ids: string[]; physical_validation: boolean }
export interface Qualification { familyId: string; status: QualificationStatus; note: string; actor: string; updatedAt: string }

export interface OpStatus {
  operation: Operation; documented: DocumentedState; documentedBasis?: string; documentedDetail?: string; sourceIds: string[]
  observed: ObservedState; observedBy: string[]; status: OpState
}
export interface SupportStatus {
  operations: OpStatus[]; catalogueQualification?: string; qualification: QualificationStatus; override?: Qualification; tier: Tier; physicalValidation: boolean
}
export interface Match {
  confidence: Confidence; brand?: string; brandBasis?: string; familyId?: string; family?: Family; profileId?: string; profile?: ManagementProfile
  modelId?: string; matchedToken?: string; evidence: OperationEvidence[]; candidates?: string[]; reason: string
  observations: { make: string; model: string; sysDescr: string; enterpriseId: number }
}
export interface CatalogSummary {
  catalogue: {
    title: string; researchedOn: string; schemaVersion: string; purpose: string; families: number; baseIdentifiers: number; brands: number
    byBrand: Record<string, number>; profiles: number; evidenceEntries: number; sources: number; identityConflicts: number; variantExamples: number
    physicallyValidated: number; qualificationByState: Record<string, number>
  }
  fleet: { devices: number; matched: number; unmatched: number; byConfidence: Record<Confidence, number> }
  overrides: Record<string, number>
  brands: string[]; segments: string[]
}
export interface FamilyRow extends Family {
  profileLabel: string; evidence: Record<Operation, OpStatus>; evidenceCount: number; fleetCount: number; qualification: QualificationStatus; override?: Qualification
}
export interface FleetDevice { id: string; name: string; make: string; model: string; address: string; state: string; confidence: Confidence; modelId?: string }
export interface FamilyDetail {
  family: Family; profile: ManagementProfile | null; evidence: OperationEvidence[]; brandEvidence: OperationEvidence[]
  summary: Record<Operation, OpStatus>; sources: Record<string, Source>; conflicts: IdentityConflict[]; variants: VariantExample[]
  devices: FleetDevice[]; qualification: QualificationStatus; override?: Qualification; catalogueQualification: string
}

/** Pill tone for a documented/observed/unknown/unsupported state (design system tones). */
export function stateTone(s: OpState | DocumentedState | ObservedState): 'info' | 'ok' | 'unk' | 'off' {
  switch (s) {
    case 'documented+observed': return 'ok'
    case 'observed': return 'ok'
    case 'documented': return 'info'
    case 'unsupported': return 'off'
    default: return 'unk'
  }
}
export const stateLabels: Record<OpState, string> = {
  'documented+observed': 'documented + observed', documented: 'documented', observed: 'observed', unsupported: 'unsupported', unknown: 'unknown',
}
export function confidenceLabel(c: Confidence): string {
  switch (c) { case 'exact': return 'Exact'; case 'family': return 'Family'; case 'brand-only': return 'Brand only'; default: return 'No match' }
}
export function confidenceTone(c: Confidence): 'ok' | 'info' | 'warn' | 'unk' {
  switch (c) { case 'exact': return 'ok'; case 'family': return 'info'; case 'brand-only': return 'warn'; default: return 'unk' }
}
/** "operation_evidence:HP_E877" -> human text. */
export function basisLabel(b?: string): string {
  if (!b) return ''
  const [kind, id] = b.split(':')
  if (kind === 'operation_evidence') return `model evidence ${id}`
  if (kind === 'management_profile') return `vendor profile ${id} (platform context, not a per-model guarantee)`
  if (kind === 'family') return `family record ${id}`
  return b
}
