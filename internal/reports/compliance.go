package reports

import "context"

// ComplianceReport is the hook for "compliance reports per device, group and
// policy, over time" (spec §8.5). Assessments are already stored and shown under
// Security (model.Assessment, SecurityStore.ListAssessments); this report does not
// read them yet. When it does, it should aggregate them per policy and device
// (pass/fail counts, score trend, newest result per device) without touching any
// security logic itself.
type ComplianceReport struct {
	Range     Range  `json:"range"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

// Compliance returns the compliance report, which is not available yet.
func (s *Service) Compliance(ctx context.Context, p Params) (*ComplianceReport, error) {
	sc, err := s.resolve(ctx, p)
	if err != nil {
		return nil, err
	}
	return &ComplianceReport{Range: sc.rng, Available: false, Reason: "This report does not include stored assessment history yet. Assessments and their results are under Security."}, nil
}

// Tables implements CSV export (empty until the report is available).
func (r *ComplianceReport) Tables() []Table {
	return []Table{{Name: "assessments", Columns: []string{"deviceId", "name", "policyId", "policy", "score", "compliant", "at"}}}
}
