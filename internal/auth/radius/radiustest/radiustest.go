// Package radiustest is an in-process RFC 2865 RADIUS server for tests. It
// unhides PAP passwords with the shared secret, checks the request's
// Message-Authenticator, and answers Access-Accept (with the reply
// attributes configured for the user) or Access-Reject, both carrying a
// correct Response Authenticator and Message-Authenticator. It only ever
// listens on loopback.
package radiustest

import (
	"crypto/hmac"
	"crypto/md5"
	"net"
	"strings"
	"sync"
	"testing"
)

// RFC 2865 / 2869 codes and attribute types (kept local so the client
// package's own tests can import this fake without an import cycle).
const (
	codeAccessRequest = 1
	CodeAccessAccept  = 2
	CodeAccessReject  = 3

	AttrUserName             = 1
	AttrUserPassword         = 2
	AttrFilterID             = 11
	AttrClass                = 25
	AttrMessageAuthenticator = 80
)

// Attr is one reply attribute.
type Attr struct {
	Type  byte
	Value string
}

// Server is the running fake.
type Server struct {
	Addr   string // 127.0.0.1:port
	Secret string
	conn   net.PacketConn

	mu sync.Mutex
	// Users maps a username to its password.
	Users map[string]string
	// Reply holds the attributes sent in the Access-Accept for a user.
	Reply map[string][]Attr
	// Drop makes the server ignore this many requests (retry tests).
	Drop int
	// ReplySecret, when set, signs replies with a different secret (forgery tests).
	ReplySecret string
	// Requests counts packets received; BadMAC counts requests whose
	// Message-Authenticator did not verify.
	Requests, BadMAC int
	// LastUser / LastPassword are what the last request carried.
	LastUser, LastPassword string
}

// New starts the server with a shared secret.
func New(t *testing.T, secret string) *Server {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Addr: pc.LocalAddr().String(), Secret: secret, conn: pc, Users: map[string]string{}, Reply: map[string][]Attr{}}
	go s.serve()
	t.Cleanup(func() { pc.Close() })
	return s
}

func (s *Server) serve() {
	buf := make([]byte, 4096)
	for {
		n, from, err := s.conn.ReadFrom(buf)
		if err != nil {
			return
		}
		if reply := s.handle(append([]byte(nil), buf[:n]...)); reply != nil {
			_, _ = s.conn.WriteTo(reply, from)
		}
	}
}

func (s *Server) handle(req []byte) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Requests++
	if len(req) < 20 || req[0] != codeAccessRequest {
		return nil
	}
	if s.Drop > 0 {
		s.Drop--
		return nil
	}
	length := int(req[2])<<8 | int(req[3])
	if length > len(req) {
		return nil
	}
	req = req[:length]
	reqAuth := req[4:20]
	attrs := map[byte][]byte{}
	maOff := -1
	for off := 20; off+2 <= len(req); {
		l := int(req[off+1])
		if l < 2 || off+l > len(req) {
			return nil
		}
		if req[off] == AttrMessageAuthenticator {
			maOff = off
		}
		attrs[req[off]] = req[off+2 : off+l]
		off += l
	}
	if maOff >= 0 {
		tmp := append([]byte(nil), req...)
		for i := 0; i < 16; i++ {
			tmp[maOff+2+i] = 0
		}
		mac := hmac.New(md5.New, []byte(s.Secret))
		mac.Write(tmp)
		if !hmac.Equal(mac.Sum(nil), attrs[AttrMessageAuthenticator]) {
			s.BadMAC++
			return nil
		}
	}
	s.LastUser = string(attrs[AttrUserName])
	s.LastPassword = reveal(s.Secret, reqAuth, attrs[AttrUserPassword])
	code := byte(CodeAccessReject)
	var replyAttrs []byte
	if pw, ok := s.Users[s.LastUser]; ok && pw == s.LastPassword {
		code = CodeAccessAccept
		for _, a := range s.Reply[s.LastUser] {
			replyAttrs = append(append(replyAttrs, a.Type, byte(2+len(a.Value))), a.Value...)
		}
	}
	secret := s.Secret
	if s.ReplySecret != "" {
		secret = s.ReplySecret
	}
	return Build(code, req[1], reqAuth, replyAttrs, secret)
}

// Build assembles a reply with a Message-Authenticator and the Response
// Authenticator computed under secret.
func Build(code, id byte, reqAuth, attrs []byte, secret string) []byte {
	attrs = append(attrs, AttrMessageAuthenticator, 18)
	attrs = append(attrs, make([]byte, 16)...)
	pkt := make([]byte, 20, 20+len(attrs))
	pkt[0], pkt[1] = code, id
	pkt = append(pkt, attrs...)
	pkt[2], pkt[3] = byte(len(pkt)>>8), byte(len(pkt))
	// Message-Authenticator over the packet with the Request Authenticator in place.
	copy(pkt[4:20], reqAuth)
	mac := hmac.New(md5.New, []byte(secret))
	mac.Write(pkt)
	copy(pkt[len(pkt)-16:], mac.Sum(nil))
	// Response Authenticator.
	h := md5.New()
	h.Write(pkt[:4])
	h.Write(reqAuth)
	h.Write(pkt[20:])
	h.Write([]byte(secret))
	copy(pkt[4:20], h.Sum(nil))
	return pkt
}

// Resign recomputes only the Response Authenticator of pkt (tests that
// want a valid envelope around a bad Message-Authenticator).
func Resign(pkt, reqAuth []byte, secret string) []byte {
	out := append([]byte(nil), pkt...)
	h := md5.New()
	h.Write(out[:4])
	h.Write(reqAuth)
	h.Write(out[20:])
	h.Write([]byte(secret))
	copy(out[4:20], h.Sum(nil))
	return out
}

// reveal undoes the RFC 2865 §5.2 User-Password XOR chain.
func reveal(secret string, auth, hidden []byte) string {
	if len(hidden)%16 != 0 {
		return ""
	}
	out := make([]byte, len(hidden))
	prev := auth
	for i := 0; i < len(hidden); i += 16 {
		h := md5.New()
		h.Write([]byte(secret))
		h.Write(prev)
		b := h.Sum(nil)
		for j := 0; j < 16; j++ {
			out[i+j] = hidden[i+j] ^ b[j]
		}
		prev = hidden[i : i+16]
	}
	return strings.TrimRight(string(out), "\x00")
}
