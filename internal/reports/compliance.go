package reports

import "context"

// ComplianceReport is the hook for "compliance reports per device, group and
// policy, over time" (spec §8.5). It stays unavailable until the security
// feature persists assessments (model.Assessment) in the store: once a
// SecurityStore method lists assessments by range exists, this report should
// aggregate them per policy and device (pass/fail counts, score trend, newest
// result per device) without touching any security logic itself.
type ComplianceReport struct {
	Range     Range  `json:"range"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// Compliance returns the (not yet available) compliance report.
func (s *Service) Compliance(ctx context.Context, p Params) (*ComplianceReport, error) {
	sc, err := s.resolve(ctx, p)
	if err != nil {
		return nil, err
	}
	return &ComplianceReport{Range: sc.rng, Available: false, Reason: "compliance reporting needs stored security assessments; the security feature has not landed yet"}, nil
}

// Tables implements CSV export (empty until the report is available).
func (r *ComplianceReport) Tables() []Table {
	return []Table{{Name: "assessments", Columns: []string{"deviceId", "name", "policyId", "policy", "score", "compliant", "at"}}}
}
