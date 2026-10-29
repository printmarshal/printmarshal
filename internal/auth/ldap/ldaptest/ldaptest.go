// Package ldaptest is a tiny LDAP v3 server for tests: it answers simple
// BindRequests from a DN→password table and SearchRequests over a fixed list
// of entries (and/or/not/equality/present filters, base or subtree scope),
// optionally over TLS or after a StartTLS upgrade. It never talks to the
// network beyond its own loopback listener.
package ldaptest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	ber "github.com/go-asn1-ber/asn1-ber"
)

// Entry is one directory object. Attrs values are strings; Raw holds binary
// attribute values (an objectGUID) that are served byte for byte.
type Entry struct {
	DN    string
	Attrs map[string][]string
	Raw   map[string][][]byte
}

// Server is the running fake.
type Server struct {
	URL    string // ldap://127.0.0.1:port or ldaps://…
	CAFile string // PEM of the self-signed certificate when TLS is on
	ln     net.Listener
	tlsCfg *tls.Config

	mu sync.Mutex
	// Passwords maps a bind DN (case-insensitive) to its password.
	Passwords map[string]string
	// Entries are the searchable objects.
	Entries []*Entry
	// AnonymousSearch allows searches before a successful bind.
	AnonymousSearch bool
	// Binds and Searches record what the client sent, for assertions.
	Binds    []string
	Searches []string
}

// New starts a plaintext ldap:// server.
func New(t *testing.T) *Server { return start(t, false) }

// NewTLS starts an ldaps:// server with a fresh self-signed certificate for
// 127.0.0.1; CAFile points at its PEM.
func NewTLS(t *testing.T) *Server { return start(t, true) }

func start(t *testing.T, useTLS bool) *Server {
	t.Helper()
	s := &Server{Passwords: map[string]string{}}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s.tlsCfg, s.CAFile = selfSigned(t)
	if useTLS {
		ln = tls.NewListener(ln, s.tlsCfg)
		s.URL = "ldaps://" + ln.Addr().String()
	} else {
		s.URL = "ldap://" + ln.Addr().String()
	}
	s.ln = ln
	go s.serve()
	t.Cleanup(func() { ln.Close() })
	return s
}

func selfSigned(t *testing.T) (*tls.Config, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "ldaptest"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	caFile := filepath.Join(t.TempDir(), "ldaptest-ca.pem")
	if err := os.WriteFile(caFile, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	return &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}, caFile
}

func (s *Server) serve() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(c)
	}
}

func (s *Server) handle(c net.Conn) {
	defer c.Close()
	bound := false
	for {
		pkt, err := ber.ReadPacket(c)
		if err != nil || len(pkt.Children) < 2 {
			return
		}
		id, _ := pkt.Children[0].Value.(int64)
		op := pkt.Children[1]
		switch op.Tag {
		case 0: // BindRequest
			if len(op.Children) < 3 {
				return
			}
			name := string(op.Children[1].Data.Bytes())
			pw := string(op.Children[2].Data.Bytes())
			s.mu.Lock()
			s.Binds = append(s.Binds, name)
			want, ok := s.Passwords[strings.ToLower(name)]
			s.mu.Unlock()
			code := int64(49) // invalidCredentials
			if name == "" && pw == "" {
				code, bound = 0, false // anonymous
			} else if ok && pw == want {
				code, bound = 0, true
			} else {
				bound = false
			}
			write(c, id, result(1, code, ""))
		case 2: // UnbindRequest
			return
		case 3: // SearchRequest
			if len(op.Children) < 8 {
				return
			}
			s.mu.Lock()
			s.Searches = append(s.Searches, describeFilter(op.Children[6]))
			anon := s.AnonymousSearch
			entries := append([]*Entry(nil), s.Entries...)
			s.mu.Unlock()
			if !bound && !anon {
				write(c, id, result(5, 50, "bind first")) // insufficientAccessRights
				continue
			}
			base := strings.ToLower(string(op.Children[0].Data.Bytes()))
			scope, _ := op.Children[1].Value.(int64)
			var want []string
			for _, a := range op.Children[7].Children {
				want = append(want, string(a.Data.Bytes()))
			}
			for _, e := range entries {
				dn := strings.ToLower(e.DN)
				switch scope {
				case 0:
					if dn != base {
						continue
					}
				default:
					if dn != base && !strings.HasSuffix(dn, ","+base) {
						continue
					}
				}
				if match(op.Children[6], e) {
					write(c, id, entryPacket(e, want))
				}
			}
			write(c, id, result(5, 0, ""))
		case 23: // ExtendedRequest: only StartTLS
			oid := ""
			if len(op.Children) > 0 {
				oid = string(op.Children[0].Data.Bytes())
			}
			if oid != "1.3.6.1.4.1.1466.20037" {
				write(c, id, result(24, 53, "unsupported")) // unwillingToPerform
				continue
			}
			write(c, id, result(24, 0, ""))
			tc := tls.Server(c, s.tlsCfg)
			if err := tc.Handshake(); err != nil {
				return
			}
			c = tc
		default:
			write(c, id, result(int(op.Tag)+1, 53, "unsupported"))
		}
	}
}

func write(c net.Conn, id int64, op *ber.Packet) {
	env := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSequence, nil, "LDAPMessage")
	env.AppendChild(ber.NewInteger(ber.ClassUniversal, ber.TypePrimitive, ber.TagInteger, id, "messageID"))
	env.AppendChild(op)
	_, _ = c.Write(env.Bytes())
}

func result(app int, code int64, msg string) *ber.Packet {
	p := ber.Encode(ber.ClassApplication, ber.TypeConstructed, ber.Tag(app), nil, "result")
	p.AppendChild(ber.NewInteger(ber.ClassUniversal, ber.TypePrimitive, ber.TagEnumerated, code, "resultCode"))
	p.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, "", "matchedDN"))
	p.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, msg, "diagnosticMessage"))
	return p
}

func entryPacket(e *Entry, want []string) *ber.Packet {
	p := ber.Encode(ber.ClassApplication, ber.TypeConstructed, 4, nil, "SearchResultEntry")
	p.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, e.DN, "objectName"))
	attrs := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSequence, nil, "attributes")
	wanted := func(name string) bool {
		if len(want) == 0 {
			return true
		}
		for _, w := range want {
			if w == "*" || strings.EqualFold(w, name) {
				return true
			}
		}
		return false
	}
	add := func(name string, vals [][]byte) {
		if !wanted(name) {
			return
		}
		a := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSequence, nil, "attribute")
		a.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, name, "type"))
		set := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSet, nil, "vals")
		for _, v := range vals {
			set.AppendChild(ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, string(v), "value"))
		}
		a.AppendChild(set)
		attrs.AppendChild(a)
	}
	for name, vals := range e.Attrs {
		b := make([][]byte, len(vals))
		for i, v := range vals {
			b[i] = []byte(v)
		}
		add(name, b)
	}
	for name, vals := range e.Raw {
		add(name, vals)
	}
	p.AppendChild(attrs)
	return p
}

func (e *Entry) values(attr string) []string {
	for name, vals := range e.Attrs {
		if strings.EqualFold(name, attr) {
			return vals
		}
	}
	for name, vals := range e.Raw {
		if strings.EqualFold(name, attr) {
			out := make([]string, len(vals))
			for i, v := range vals {
				out[i] = string(v)
			}
			return out
		}
	}
	return nil
}

// match evaluates an RFC 4511 filter packet against an entry.
func match(f *ber.Packet, e *Entry) bool {
	switch f.Tag {
	case 0: // and
		for _, c := range f.Children {
			if !match(c, e) {
				return false
			}
		}
		return true
	case 1: // or
		for _, c := range f.Children {
			if match(c, e) {
				return true
			}
		}
		return false
	case 2: // not
		return len(f.Children) == 1 && !match(f.Children[0], e)
	case 3: // equalityMatch
		if len(f.Children) != 2 {
			return false
		}
		attr, val := string(f.Children[0].Data.Bytes()), string(f.Children[1].Data.Bytes())
		for _, v := range e.values(attr) {
			if strings.EqualFold(v, val) {
				return true
			}
		}
		return false
	case 7: // present
		return len(e.values(string(f.Data.Bytes()))) > 0 || strings.EqualFold(string(f.Data.Bytes()), "objectClass")
	}
	return false
}

// escapeValue re-applies RFC 4515 escaping so a recorded filter shows how
// the value was delimited on the wire (an injected ")" stays a literal).
func escapeValue(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		switch c {
		case '*', '(', ')', '\\', 0:
			sb.WriteString(fmt.Sprintf("\\%02x", c))
		default:
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

// describeFilter renders the filter packet back to RFC 4515 text.
func describeFilter(f *ber.Packet) string {
	switch f.Tag {
	case 0, 1, 2:
		op := map[ber.Tag]string{0: "&", 1: "|", 2: "!"}[f.Tag]
		parts := make([]string, len(f.Children))
		for i, c := range f.Children {
			parts[i] = describeFilter(c)
		}
		return "(" + op + strings.Join(parts, "") + ")"
	case 3:
		if len(f.Children) == 2 {
			return "(" + string(f.Children[0].Data.Bytes()) + "=" + escapeValue(f.Children[1].Data.Bytes()) + ")"
		}
	case 7:
		return "(" + string(f.Data.Bytes()) + "=*)"
	}
	return "(?)"
}
