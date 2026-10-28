package snmp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"

	"printmarshal/internal/model"
)

// PDU is one variable binding.
type PDU struct {
	OID   string
	Type  gosnmp.Asn1BER
	Value any
}

// Session is the minimal SNMP surface the driver needs. The production
// implementation wraps gosnmp; tests and the simulator use a Fixture.
//
// Set is the only write primitive (SNMP SetRequest, RFC 3416 §4.2.5). Only the
// write_config path calls it, never the read capabilities.
type Session interface {
	Get(ctx context.Context, oids []string) ([]PDU, error)
	Walk(ctx context.Context, root string) ([]PDU, error)
	Set(ctx context.Context, pdus []PDU) error
	Close() error
}

// Options tune device I/O.
type Options struct {
	Port    uint16
	Timeout time.Duration
	Retries int
}

// DefaultOptions are suitable for LAN printers.
func DefaultOptions() Options {
	return Options{Port: 161, Timeout: 2 * time.Second, Retries: 1}
}

// Dialer opens a Session to an address. It is a function so tests can inject a
// fixture-backed session.
type Dialer func(ctx context.Context, address string, creds model.Creds) (Session, string, error)

// NewDialer returns the production dialer. It prefers SNMPv3 when a USM user is
// configured (spec §7) and otherwise tries each v2c community in order, verifying
// each candidate with a sysDescr read. The returned string names the variant
// that answered ("v3", "v2c").
func NewDialer(opts Options) Dialer {
	return func(ctx context.Context, address string, creds model.Creds) (Session, string, error) {
		var errs []error
		if creds.V3 != nil {
			s, err := open(ctx, address, opts, gosnmp.Version3, "", creds.V3)
			if err == nil {
				return s, "v3", nil
			}
			errs = append(errs, fmt.Errorf("v3: %w", err))
		}
		communities := creds.Communities
		if len(communities) == 0 {
			communities = []string{"public"}
		}
		for _, c := range communities {
			s, err := open(ctx, address, opts, gosnmp.Version2c, c, nil)
			if err == nil {
				return s, "v2c", nil
			}
			errs = append(errs, err)
		}
		return nil, "", errors.Join(errs...)
	}
}

func open(ctx context.Context, address string, opts Options, version gosnmp.SnmpVersion, community string, v3 *model.V3Creds) (Session, error) {
	g := &gosnmp.GoSNMP{
		Target:    address,
		Port:      opts.Port,
		Version:   version,
		Timeout:   opts.Timeout,
		Retries:   opts.Retries,
		MaxOids:   gosnmp.MaxOids,
		Community: community,
	}
	if v3 != nil {
		g.SecurityModel = gosnmp.UserSecurityModel
		g.MsgFlags = gosnmp.AuthPriv
		if v3.PrivPassword == "" {
			g.MsgFlags = gosnmp.AuthNoPriv
		}
		if v3.AuthPassword == "" {
			g.MsgFlags = gosnmp.NoAuthNoPriv
		}
		g.SecurityParameters = &gosnmp.UsmSecurityParameters{
			UserName:                 v3.Username,
			AuthenticationProtocol:   authProto(v3.AuthProtocol),
			AuthenticationPassphrase: v3.AuthPassword,
			PrivacyProtocol:          privProto(v3.PrivProtocol),
			PrivacyPassphrase:        v3.PrivPassword,
		}
	}
	if err := g.Connect(); err != nil {
		return nil, err
	}
	s := &gosnmpSession{g: g}
	// Verify the session actually answers before handing it out.
	if _, err := s.Get(ctx, []string{OIDSysDescr}); err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}

func authProto(name string) gosnmp.SnmpV3AuthProtocol {
	switch strings.ToUpper(name) {
	case "MD5":
		return gosnmp.MD5
	case "SHA", "SHA1":
		return gosnmp.SHA
	case "SHA224":
		return gosnmp.SHA224
	case "SHA256":
		return gosnmp.SHA256
	case "SHA384":
		return gosnmp.SHA384
	case "SHA512":
		return gosnmp.SHA512
	}
	return gosnmp.NoAuth
}

func privProto(name string) gosnmp.SnmpV3PrivProtocol {
	switch strings.ToUpper(name) {
	case "DES":
		return gosnmp.DES
	case "AES", "AES128":
		return gosnmp.AES
	case "AES192":
		return gosnmp.AES192
	case "AES256":
		return gosnmp.AES256
	}
	return gosnmp.NoPriv
}

type gosnmpSession struct{ g *gosnmp.GoSNMP }

func (s *gosnmpSession) Get(ctx context.Context, oids []string) ([]PDU, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	pkt, err := s.g.Get(oids)
	if err != nil {
		return nil, err
	}
	if pkt.Error != gosnmp.NoError {
		return nil, fmt.Errorf("snmp error %s", pkt.Error)
	}
	return convert(pkt.Variables), nil
}

func (s *gosnmpSession) Walk(ctx context.Context, root string) ([]PDU, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var (
		vars []gosnmp.SnmpPDU
		err  error
	)
	if s.g.Version == gosnmp.Version1 {
		vars, err = s.g.WalkAll(root)
	} else {
		vars, err = s.g.BulkWalkAll(root)
	}
	if err != nil {
		return nil, err
	}
	return convert(vars), nil
}

// Set issues one SetRequest with all bindings (atomic per RFC 3416 §4.2.5: the
// agent applies all or none). A non-zero error-status is returned as an error
// naming the failing binding.
func (s *gosnmpSession) Set(ctx context.Context, pdus []PDU) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	vars := make([]gosnmp.SnmpPDU, 0, len(pdus))
	for _, p := range pdus {
		vars = append(vars, gosnmp.SnmpPDU{Name: p.OID, Type: p.Type, Value: p.Value})
	}
	pkt, err := s.g.Set(vars)
	if err != nil {
		return err
	}
	if pkt.Error != gosnmp.NoError {
		which := ""
		if i := int(pkt.ErrorIndex) - 1; i >= 0 && i < len(pdus) {
			which = " at " + pdus[i].OID
		}
		return fmt.Errorf("snmp set error %s%s", pkt.Error, which)
	}
	return nil
}

func (s *gosnmpSession) Close() error { return s.g.Close() }

func convert(vars []gosnmp.SnmpPDU) []PDU {
	out := make([]PDU, 0, len(vars))
	for _, v := range vars {
		out = append(out, PDU{OID: v.Name, Type: v.Type, Value: v.Value})
	}
	return out
}
