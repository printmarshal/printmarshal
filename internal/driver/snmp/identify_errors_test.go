package snmp

import (
	"context"
	"errors"
	"testing"

	"github.com/gosnmp/gosnmp"

	"printmarshal/internal/model"
)

// sysOnlySession answers the system group and either fails or empties every walk.
type sysOnlySession struct {
	walkErr error
	walks   int
}

func (s *sysOnlySession) Get(_ context.Context, oids []string) ([]PDU, error) {
	var out []PDU
	for _, o := range oids {
		switch o {
		case OIDSysDescr:
			out = append(out, PDU{OID: o, Type: gosnmp.OctetString, Value: []byte("HP Color LaserJet MFP M283fdw")})
		case OIDSysObjectID:
			out = append(out, PDU{OID: o, Type: gosnmp.ObjectIdentifier, Value: ".1.3.6.1.4.1.11.2.3.9.1"})
		default:
			out = append(out, PDU{OID: o, Type: gosnmp.NoSuchObject})
		}
	}
	return out, nil
}
func (s *sysOnlySession) Walk(context.Context, string) ([]PDU, error) {
	s.walks++
	return nil, s.walkErr
}
func (s *sysOnlySession) Set(context.Context, []PDU) error { return errors.New("read-only") }
func (s *sysOnlySession) Close() error                     { return nil }

func sessionDialer(s Session) Dialer {
	return func(context.Context, string, model.Creds) (Session, string, error) { return s, "v2c", nil }
}

// Issue #19: a walk that times out under load must not turn a printer into
// "not a printer"; the error has to say the read failed.
func TestIdentifyReportsFailedWalksAsReadFailures(t *testing.T) {
	sess := &sysOnlySession{walkErr: errors.New("request timeout (after 1 retries)")}
	_, err := New(sessionDialer(sess)).Identify(context.Background(), "10.0.0.9", model.Creds{})
	if !errors.Is(err, ErrReadFailed) {
		t.Fatalf("want ErrReadFailed, got %v", err)
	}
	if errors.Is(err, ErrNotPrinter) {
		t.Fatalf("a failed read must not be reported as not-a-printer: %v", err)
	}
	if sess.walks == 0 {
		t.Fatal("walks were never attempted")
	}
}

func TestIdentifyEmptyTablesMeanNotAPrinter(t *testing.T) {
	_, err := New(sessionDialer(&sysOnlySession{})).Identify(context.Background(), "10.0.0.9", model.Creds{})
	if !errors.Is(err, ErrNotPrinter) {
		t.Fatalf("want ErrNotPrinter, got %v", err)
	}
}

func TestPreflightReadsEnterprise(t *testing.T) {
	pf, ok := Preflight(sessionDialer(&sysOnlySession{}))(context.Background(), "10.0.0.9", model.Creds{})
	if !ok || !pf.SNMP || pf.Enterprise != 11 || pf.SysDescr == "" {
		t.Fatalf("preflight: ok=%v %+v", ok, pf)
	}
	pf, ok = Preflight(func(context.Context, string, model.Creds) (Session, string, error) {
		return nil, "", errors.New("timeout")
	})(context.Background(), "10.0.0.9", model.Creds{})
	if !ok || pf.SNMP {
		t.Fatalf("silent agent: ok=%v %+v", ok, pf)
	}
}
