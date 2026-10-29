package ldap

import (
	"context"
	"errors"
	"strings"
	"testing"

	"printmarshal/internal/auth/ldap/ldaptest"
)

var guid = []byte{0x33, 0x22, 0x11, 0x00, 0x55, 0x44, 0x77, 0x66, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}

func directory(t *testing.T, s *ldaptest.Server) {
	t.Helper()
	s.Passwords["cn=svc,ou=service,dc=example,dc=com"] = "svcpass"
	s.Passwords["cn=ann,ou=people,dc=example,dc=com"] = "annpass"
	s.Passwords["cn=bob,ou=people,dc=example,dc=com"] = "bobpass"
	s.Entries = []*ldaptest.Entry{
		{DN: "CN=ann,OU=people,DC=example,DC=com", Attrs: map[string][]string{"sAMAccountName": {"ann"}, "userPrincipalName": {"ann@example.com"}, "mail": {"Ann@Example.com"},
			"memberOf": {"CN=Printer Admins,OU=Groups,DC=example,DC=com", "CN=Staff,OU=Groups,DC=example,DC=com"}}, Raw: map[string][][]byte{"objectGUID": {guid}}},
		{DN: "CN=bob,OU=people,DC=example,DC=com", Attrs: map[string][]string{"uid": {"bob"}, "entryUUID": {"8b2c-bob"}, "memberOf": {"cn=staff,ou=groups,dc=example,dc=com"}}},
		{DN: "CN=svc,OU=service,DC=example,DC=com", Attrs: map[string][]string{"cn": {"svc"}}},
	}
}

func TestServiceAccountFlow(t *testing.T) {
	s := ldaptest.New(t)
	directory(t, s)
	cfg := Config{URL: s.URL, BindDN: "cn=svc,ou=service,dc=example,dc=com", BindPassword: "svcpass", BaseDN: "dc=example,dc=com"}

	id, err := Authenticate(context.Background(), cfg, "ann@example.com", "annpass")
	if err != nil {
		t.Fatal(err)
	}
	// "whatever matched": the UPN was typed, so it is the username; objectGUID is the subject.
	if id.Username != "ann@example.com" || id.Subject != "00112233-4455-6677-8899-aabbccddeeff" || id.DN != "CN=ann,OU=people,DC=example,DC=com" {
		t.Fatalf("identity %+v", id)
	}
	if c := id.Candidates(); len(c) != 4 || c[1] != "Printer Admins" || c[3] != "Staff" {
		t.Fatalf("candidates %v", c)
	}
	if !id.InGroup([]string{"printer admins"}) || id.InGroup([]string{"nobody"}) {
		t.Fatal("group check")
	}
	if len(s.Binds) != 2 || s.Binds[0] != cfg.BindDN || s.Binds[1] != id.DN {
		t.Fatalf("binds %v", s.Binds)
	}
	if !strings.Contains(s.Searches[0], "(userPrincipalName=ann@example.com)") {
		t.Fatalf("search %v", s.Searches)
	}

	// Typing the sAMAccountName yields it as the username; the subject is unchanged.
	id, err = Authenticate(context.Background(), cfg, "ANN", "annpass")
	if err != nil || id.Username != "ann" || id.Subject != "00112233-4455-6677-8899-aabbccddeeff" {
		t.Fatalf("%v %+v", err, id)
	}
	// bob has no objectGUID: entryUUID is the subject, uid the username.
	id, err = Authenticate(context.Background(), cfg, "bob", "bobpass")
	if err != nil || id.Username != "bob" || id.Subject != "8b2c-bob" || id.Candidates()[1] != "staff" {
		t.Fatalf("%v %+v", err, id)
	}
	// Explicit attributes.
	cfg2 := cfg
	cfg2.UsernameAttr, cfg2.SubjectAttr = "mail", "userPrincipalName"
	id, err = Authenticate(context.Background(), cfg2, "ann", "annpass")
	if err != nil || id.Username != "Ann@Example.com" || id.Subject != "ann@example.com" {
		t.Fatalf("%v %+v", err, id)
	}

	// Wrong password, unknown user and an injection attempt all look the same.
	for _, c := range [][2]string{{"ann", "wrong"}, {"nobody", "annpass"}, {"ann", ""}, {"*", "annpass"}, {"ann)(|(uid=*", "annpass"}} {
		if _, err := Authenticate(context.Background(), cfg, c[0], c[1]); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("%q: %v", c[0], err)
		}
	}
	if !strings.Contains(s.Searches[len(s.Searches)-1], `ann\29\28|\28uid=\2a`) {
		t.Fatalf("filter not escaped: %v", s.Searches[len(s.Searches)-1])
	}
	// A wrong service-account password is a configuration error, not a user error.
	cfg3 := cfg
	cfg3.BindPassword = "nope"
	if _, err := Authenticate(context.Background(), cfg3, "ann", "annpass"); err == nil || errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("service bind: %v", err)
	}
}

func TestDirectBindTemplate(t *testing.T) {
	s := ldaptest.New(t)
	directory(t, s)
	// Without a base DN the template must name the entry itself.
	cfg := Config{URL: s.URL, UserDNTemplate: "cn=%s,ou=people,dc=example,dc=com"}
	id, err := Authenticate(context.Background(), cfg, "ann", "annpass")
	if err != nil || id.Username != "ann" || id.Subject != "00112233-4455-6677-8899-aabbccddeeff" || len(id.Groups) != 2 {
		t.Fatalf("%v %+v", err, id)
	}
	if s.Binds[0] != "cn=ann,ou=people,dc=example,dc=com" {
		t.Fatalf("bind %v", s.Binds)
	}
	// A DN injection in the username is escaped, so the bind name is not what the attacker wrote.
	if _, err := Authenticate(context.Background(), cfg, "svc,ou=service,dc=example,dc=com", "svcpass"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("dn injection: %v", err)
	}
	if last := s.Binds[len(s.Binds)-1]; !strings.HasPrefix(last, `cn=svc\,ou=service`) {
		t.Fatalf("dn not escaped: %q", last)
	}
	// With a base DN the filter finds the entry after the user's own bind.
	cfg.BaseDN = "dc=example,dc=com"
	if id, err := Authenticate(context.Background(), cfg, "bob", "bobpass"); err != nil || id.Subject != "8b2c-bob" {
		t.Fatalf("%v %+v", err, id)
	}
	if _, err := Authenticate(context.Background(), cfg, "ann", "bad"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password: %v", err)
	}
	// Neither a service account nor a template is a configuration error.
	if _, err := Authenticate(context.Background(), Config{URL: s.URL}, "ann", "annpass"); err == nil || errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("misconfig: %v", err)
	}
}

func TestTLS(t *testing.T) {
	s := ldaptest.NewTLS(t)
	directory(t, s)
	cfg := Config{URL: s.URL, UserDNTemplate: "cn=%s,ou=people,dc=example,dc=com", CAFile: s.CAFile}
	if _, err := Authenticate(context.Background(), cfg, "ann", "annpass"); err != nil {
		t.Fatal(err)
	}
	cfg.CAFile = ""
	if _, err := Authenticate(context.Background(), cfg, "ann", "annpass"); err == nil {
		t.Fatal("self-signed server accepted without its CA")
	}
	cfg.InsecureSkipVerify = true
	if _, err := Authenticate(context.Background(), cfg, "ann", "annpass"); err != nil {
		t.Fatal(err)
	}

	// StartTLS on a plaintext listener.
	p := ldaptest.New(t)
	directory(t, p)
	cfg = Config{URL: p.URL, StartTLS: true, CAFile: p.CAFile, UserDNTemplate: "cn=%s,ou=people,dc=example,dc=com"}
	if _, err := Authenticate(context.Background(), cfg, "ann", "annpass"); err != nil {
		t.Fatal(err)
	}
	if _, err := Authenticate(context.Background(), Config{URL: "http://x", UserDNTemplate: "%s"}, "ann", "annpass"); err == nil {
		t.Fatal("bad scheme accepted")
	}
}

func TestHelpers(t *testing.T) {
	if got := filterAttrs(""); strings.Join(got, ",") != "sAMAccountName,uid,userPrincipalName,mail" {
		t.Fatalf("filterAttrs %v", got)
	}
	if got := filterAttrs("(&(objectClass=person)(cn=%s))"); strings.Join(got, ",") != "objectClass,cn" {
		t.Fatalf("filterAttrs %v", got)
	}
	if firstRDN("not a dn") != "" || firstRDN("CN=Ops Team,OU=x") != "Ops Team" {
		t.Fatal("firstRDN")
	}
	if GUIDString(guid) != "00112233-4455-6677-8899-aabbccddeeff" || GUIDString([]byte{1}) != "" {
		t.Fatal("guid")
	}
	if (Config{URL: " ldap://h "}).Issuer() != "ldap:ldap://h" {
		t.Fatal("issuer")
	}
}
