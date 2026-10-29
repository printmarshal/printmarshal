// Package radius is a dependency-free RFC 2865 PAP client (docs/auth.md):
// it sends an Access-Request over UDP with the User-Password hidden under
// the shared secret and a Message-Authenticator (RFC 2869), verifies the
// reply's Response Authenticator and Message-Authenticator, and returns the
// reply attributes so roles can be mapped from Filter-Id or Class.
//
// RADIUS has no stable user identifier: callers bind accounts to the
// lowercased username under the issuer "radius:<first server>".
package radius

import (
	"context"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// Config is what one authentication needs.
type Config struct {
	Servers       []string // host:port, tried in order
	Secret        string
	Timeout       time.Duration // per attempt (default 3s)
	Retries       int           // retransmissions per server (default 2)
	NASIdentifier string        // default "printmarshal"
}

// Attribute types used here (RFC 2865 / 2869).
const (
	AttrUserName             = 1
	AttrUserPassword         = 2
	AttrNASIPAddress         = 4
	AttrServiceType          = 6
	AttrFilterID             = 11
	AttrReplyMessage         = 18
	AttrClass                = 25
	AttrNASIdentifier        = 32
	AttrMessageAuthenticator = 80
)

// Packet codes.
const (
	CodeAccessRequest   = 1
	CodeAccessAccept    = 2
	CodeAccessReject    = 3
	CodeAccessChallenge = 11
)

// Result is a verified Access-Accept.
type Result struct {
	Attributes map[byte][][]byte
}

// Values returns the string values of a reply attribute ("Filter-Id",
// "Class" or a numeric type).
func (r *Result) Values(name string) []string {
	var typ byte
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "filter-id", "filterid", "11":
		typ = AttrFilterID
	case "class", "25":
		typ = AttrClass
	case "reply-message", "18":
		typ = AttrReplyMessage
	default:
		var n int
		if _, err := fmt.Sscanf(name, "%d", &n); err != nil || n < 1 || n > 255 {
			return nil
		}
		typ = byte(n)
	}
	var out []string
	for _, v := range r.Attributes[typ] {
		out = append(out, string(v))
	}
	return out
}

// ErrRejected is an Access-Reject (or an unsupported Access-Challenge):
// callers must treat it exactly like a wrong password.
var ErrRejected = errors.New("invalid credentials")

// Issuer is the identity-provider identifier accounts are bound to.
func (c Config) Issuer() string {
	if len(c.Servers) == 0 {
		return "radius:"
	}
	return "radius:" + strings.TrimSpace(c.Servers[0])
}

// Subject is the stable identifier RADIUS can offer: the lowercased name.
func Subject(username string) string { return strings.ToLower(strings.TrimSpace(username)) }

// Authenticate sends an Access-Request to the servers in turn and returns the
// accept's attributes. Every server timing out is a transport error; a
// reject from any server is final.
func Authenticate(ctx context.Context, cfg Config, username, password string) (*Result, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil, ErrRejected
	}
	if len(cfg.Servers) == 0 || cfg.Secret == "" {
		return nil, errors.New("radius: servers and a shared secret are required")
	}
	if len(password) > 128 {
		return nil, errors.New("radius: password longer than 128 bytes cannot be sent with PAP")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	retries := cfg.Retries
	if retries < 0 {
		retries = 0
	}
	if cfg.Retries == 0 {
		retries = 2
	}
	nas := cfg.NASIdentifier
	if nas == "" {
		nas = "printmarshal"
	}
	var lastErr error
	for _, server := range cfg.Servers {
		server = strings.TrimSpace(server)
		if server == "" {
			continue
		}
		req, auth, err := buildRequest(cfg.Secret, username, password, nas)
		if err != nil {
			return nil, err
		}
		res, err := exchange(ctx, server, req, auth, cfg.Secret, timeout, retries)
		if err == nil || errors.Is(err, ErrRejected) {
			return res, err
		}
		lastErr = err
		if ctx.Err() != nil {
			break
		}
	}
	if lastErr == nil {
		lastErr = errors.New("radius: no server configured")
	}
	return nil, lastErr
}

// buildRequest assembles the Access-Request; auth is its Request Authenticator.
func buildRequest(secret, username, password, nas string) ([]byte, []byte, error) {
	auth := make([]byte, 16)
	if _, err := rand.Read(auth); err != nil {
		return nil, nil, err
	}
	idb := make([]byte, 1)
	if _, err := rand.Read(idb); err != nil {
		return nil, nil, err
	}
	var attrs []byte
	attrs = appendAttr(attrs, AttrUserName, []byte(username))
	attrs = appendAttr(attrs, AttrUserPassword, hidePassword(secret, auth, []byte(password)))
	attrs = appendAttr(attrs, AttrNASIdentifier, []byte(nas))
	attrs = appendAttr(attrs, AttrNASIPAddress, []byte{127, 0, 0, 1})
	attrs = appendAttr(attrs, AttrMessageAuthenticator, make([]byte, 16)) // filled below
	pkt := make([]byte, 20, 20+len(attrs))
	pkt[0], pkt[1] = CodeAccessRequest, idb[0]
	copy(pkt[4:20], auth)
	pkt = append(pkt, attrs...)
	pkt[2], pkt[3] = byte(len(pkt)>>8), byte(len(pkt))
	// Message-Authenticator = HMAC-MD5(secret, packet with the attribute zeroed).
	off := len(pkt) - 16
	mac := hmac.New(md5.New, []byte(secret))
	mac.Write(pkt)
	copy(pkt[off:], mac.Sum(nil))
	return pkt, auth, nil
}

func appendAttr(b []byte, typ byte, v []byte) []byte {
	if len(v) > 253 {
		v = v[:253]
	}
	return append(append(b, typ, byte(2+len(v))), v...)
}

// hidePassword applies the RFC 2865 §5.2 XOR chain.
func hidePassword(secret string, auth, pw []byte) []byte {
	n := (len(pw) + 15) / 16 * 16
	if n == 0 {
		n = 16
	}
	padded := make([]byte, n)
	copy(padded, pw)
	out := make([]byte, n)
	prev := auth
	for i := 0; i < n; i += 16 {
		h := md5.New()
		h.Write([]byte(secret))
		h.Write(prev)
		b := h.Sum(nil)
		for j := 0; j < 16; j++ {
			out[i+j] = padded[i+j] ^ b[j]
		}
		prev = out[i : i+16]
	}
	return out
}

// RevealPassword is the inverse of the XOR chain (used by servers and tests).
func RevealPassword(secret string, auth, hidden []byte) []byte {
	if len(hidden)%16 != 0 {
		return nil
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
	return []byte(strings.TrimRight(string(out), "\x00"))
}

func exchange(ctx context.Context, server string, req, auth []byte, secret string, timeout time.Duration, retries int) (*Result, error) {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "udp", server)
	if err != nil {
		return nil, fmt.Errorf("radius: %s: %w", server, err)
	}
	defer conn.Close()
	buf := make([]byte, 4096)
	for attempt := 0; attempt <= retries; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if _, err := conn.Write(req); err != nil {
			return nil, fmt.Errorf("radius: %s: %w", server, err)
		}
		deadline := time.Now().Add(timeout)
		if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
			deadline = dl
		}
		_ = conn.SetReadDeadline(deadline)
		for {
			n, err := conn.Read(buf)
			if err != nil {
				var ne net.Error
				if errors.As(err, &ne) && ne.Timeout() {
					err = fmt.Errorf("radius: %s: no reply within %s", server, timeout)
				}
				if attempt == retries {
					return nil, fmt.Errorf("radius: %s: %w", server, err)
				}
				break // retransmit
			}
			res, err := parseReply(buf[:n], req, auth, secret)
			if err != nil && !errors.Is(err, ErrRejected) {
				// A malformed or forged reply is ignored, like a lost one;
				// keep waiting for the real one until the deadline.
				continue
			}
			return res, err
		}
	}
	return nil, fmt.Errorf("radius: %s: no reply", server)
}

// parseReply verifies a reply's shape, identifier, Response Authenticator
// and (when present) Message-Authenticator, then classifies it.
func parseReply(reply, req, reqAuth []byte, secret string) (*Result, error) {
	if len(reply) < 20 || len(reply) > 4096 {
		return nil, errors.New("short reply")
	}
	length := int(reply[2])<<8 | int(reply[3])
	if length < 20 || length > len(reply) {
		return nil, errors.New("bad length")
	}
	reply = reply[:length]
	if reply[1] != req[1] {
		return nil, errors.New("identifier mismatch")
	}
	// ResponseAuth = MD5(Code+ID+Length+RequestAuth+Attributes+Secret)
	h := md5.New()
	h.Write(reply[:4])
	h.Write(reqAuth)
	h.Write(reply[20:])
	h.Write([]byte(secret))
	if !hmac.Equal(h.Sum(nil), reply[4:20]) {
		return nil, errors.New("response authenticator mismatch")
	}
	attrs, err := parseAttrs(reply[20:])
	if err != nil {
		return nil, err
	}
	if ma := attrs[AttrMessageAuthenticator]; len(ma) > 0 {
		if len(ma[0]) != 16 {
			return nil, errors.New("bad message authenticator")
		}
		// HMAC over the reply with the Request Authenticator in the
		// authenticator field and the attribute zeroed (RFC 2869 §5.14).
		tmp := append([]byte(nil), reply...)
		copy(tmp[4:20], reqAuth)
		off := attrOffset(tmp[20:], AttrMessageAuthenticator)
		if off < 0 {
			return nil, errors.New("bad message authenticator")
		}
		for i := 0; i < 16; i++ {
			tmp[20+off+2+i] = 0
		}
		mac := hmac.New(md5.New, []byte(secret))
		mac.Write(tmp)
		if !hmac.Equal(mac.Sum(nil), ma[0]) {
			return nil, errors.New("message authenticator mismatch")
		}
	}
	switch reply[0] {
	case CodeAccessAccept:
		return &Result{Attributes: attrs}, nil
	case CodeAccessReject, CodeAccessChallenge:
		return nil, ErrRejected
	}
	return nil, fmt.Errorf("unexpected code %d", reply[0])
}

func parseAttrs(b []byte) (map[byte][][]byte, error) {
	out := map[byte][][]byte{}
	for len(b) > 0 {
		if len(b) < 2 {
			return nil, errors.New("truncated attribute")
		}
		l := int(b[1])
		if l < 2 || l > len(b) {
			return nil, errors.New("bad attribute length")
		}
		out[b[0]] = append(out[b[0]], append([]byte(nil), b[2:l]...))
		b = b[l:]
	}
	return out, nil
}

// attrOffset returns the offset of the first attribute of the given type.
func attrOffset(b []byte, typ byte) int {
	off := 0
	for off+2 <= len(b) {
		l := int(b[off+1])
		if l < 2 || off+l > len(b) {
			return -1
		}
		if b[off] == typ {
			return off
		}
		off += l
	}
	return -1
}
