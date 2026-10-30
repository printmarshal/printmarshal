package snmp

import (
	"context"

	"printmarshal/internal/engine"
	"printmarshal/internal/model"
)

// Preflight returns the engine's shared pre-identify read: one GetRequest for
// sysDescr and sysObjectID through dial (and so through its Gate). A dial or
// read failure is reported as "SNMP did not answer" rather than as nothing
// learned, so vendor plugins still run for their IPP matching while the
// enterprise-gated ones are skipped once the agent has spoken (issue #19).
func Preflight(dial Dialer) engine.PreflightFunc {
	return func(ctx context.Context, address string, creds model.Creds) (engine.Preflight, bool) {
		s, _, err := dial(ctx, address, creds)
		if err != nil {
			return engine.Preflight{}, true
		}
		defer s.Close()
		res, err := s.Get(ctx, []string{OIDSysDescr, OIDSysObjectID})
		if err != nil {
			return engine.Preflight{}, true
		}
		pf := engine.Preflight{SNMP: true}
		for _, p := range res {
			if absent(p) {
				continue
			}
			switch p.OID {
			case OIDSysDescr:
				pf.SysDescr = Str(p)
			case OIDSysObjectID:
				pf.Enterprise = EnterpriseID(Str(p))
			}
		}
		return pf, true
	}
}
