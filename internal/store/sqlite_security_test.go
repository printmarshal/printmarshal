package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"printmarshal/internal/model"
)

func TestSecurityPoliciesCRUD(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	p := &model.Policy{Name: "P1", Description: "d", CreatedBy: "u", Items: []model.PolicyItem{
		{ID: "a", Key: "protocols.telnet.enabled", Operator: "eq", Value: false, Severity: "critical", Remediate: true, ManagementPath: ""},
		{ID: "b", Key: "security.tls.minVersion", Operator: "gte", Value: "1.2", Severity: "high", Remediation: &model.SettingChange{Key: "security.tls.minVersion", Value: "1.2"}},
	}}
	if err := s.CreatePolicy(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := s.CreatePolicy(ctx, &model.Policy{Name: "P1"}); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Errorf("duplicate name must fail: %v", err)
	}
	got, err := s.GetPolicy(ctx, p.ID)
	if err != nil || got.Name != "P1" || len(got.Items) != 2 || got.Items[1].Remediation == nil || got.Items[0].Value != false {
		t.Fatalf("get: %+v %v", got, err)
	}
	got.Name, got.Items = "P2", got.Items[:1]
	if err := s.UpdatePolicy(ctx, got); err != nil {
		t.Fatal(err)
	}
	ps, _ := s.ListPolicies(ctx)
	if len(ps) != 1 || ps[0].Name != "P2" || len(ps[0].Items) != 1 {
		t.Errorf("list: %+v", ps)
	}
	if err := s.DeletePolicy(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetPolicy(ctx, p.ID); err != ErrNotFound {
		t.Errorf("deleted: %v", err)
	}
	if err := s.UpdatePolicy(ctx, got); err != ErrNotFound {
		t.Errorf("update missing: %v", err)
	}
}

func TestAssessmentsLatestAndHistory(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	save := func(dev, pol string, at time.Time, score float64, compliant bool) {
		a := &model.Assessment{DeviceID: dev, PolicyID: pol, Score: score, Compliant: compliant, At: at, Passed: 1,
			Items: []model.ItemResult{{ItemID: "x", Status: model.ItemPass, Severity: "low"}}}
		if err := s.SaveAssessment(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	save("d1", "p1", base, 40, false)
	save("d1", "p1", base.Add(24*time.Hour), 100, true)
	save("d2", "p1", base.Add(24*time.Hour), 60, false)
	save("d1", "p2", base.Add(48*time.Hour), 80, false)

	latest, err := s.LatestAssessments(ctx, "p1", nil)
	if err != nil || len(latest) != 2 {
		t.Fatalf("latest: %v %v", latest, err)
	}
	for _, a := range latest {
		if a.DeviceID == "d1" && (a.Score != 100 || !a.Compliant) {
			t.Errorf("d1 latest should be the newer one: %+v", a)
		}
	}
	latest, _ = s.LatestAssessments(ctx, "", []string{"d1"})
	if len(latest) != 2 {
		t.Errorf("d1 across policies: %d", len(latest))
	}
	all, _ := s.ListAssessments(ctx, AssessmentFilter{DeviceID: "d1", PolicyID: "p1"})
	if len(all) != 2 || all[0].Score != 100 || len(all[0].Items) != 1 {
		t.Errorf("list: %+v", all)
	}
	ranged, _ := s.ListAssessments(ctx, AssessmentFilter{From: base.Add(36 * time.Hour)})
	if len(ranged) != 1 {
		t.Errorf("from filter: %d", len(ranged))
	}
	hist, err := s.ComplianceHistory(ctx, "p1", time.Time{}, time.Time{})
	if err != nil || len(hist) != 2 {
		t.Fatalf("history: %+v %v", hist, err)
	}
	if hist[0].Date != "2026-10-01" || hist[0].Assessed != 1 || hist[0].Compliant != 0 || hist[0].AvgScore != 40 {
		t.Errorf("day 1: %+v", hist[0])
	}
	if hist[1].Date != "2026-10-02" || hist[1].Assessed != 2 || hist[1].Compliant != 1 || hist[1].AvgScore != 80 {
		t.Errorf("day 2: %+v", hist[1])
	}
	if h, _ := s.ComplianceHistory(ctx, "p1", base.Add(20*time.Hour), base.Add(30*time.Hour)); len(h) != 1 {
		t.Errorf("history range: %+v", h)
	}
}

func TestCAProfilesAndCertificates(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	p := &model.CAProfile{Name: "corp", Protocol: model.EnrollEST, URL: "https://ca.example.test/.well-known/est", CAFingerprint: "ab", ValidityDays: 365, KeyType: "rsa2048", Options: map[string]string{"label": "printers"}}
	if err := s.CreateCAProfile(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetCAProfile(ctx, p.ID)
	if err != nil || got.Options["label"] != "printers" || got.Protocol != model.EnrollEST || got.KeyType != "rsa2048" {
		t.Fatalf("get: %+v %v", got, err)
	}
	got.Name = "corp2"
	if err := s.UpdateCAProfile(ctx, got); err != nil {
		t.Fatal(err)
	}
	if ps, _ := s.ListCAProfiles(ctx); len(ps) != 1 || ps[0].Name != "corp2" {
		t.Errorf("list: %+v", ps)
	}

	// Issued certificates: key encrypted at rest and only readable through IssuedCertificateKey.
	now := time.Now().UTC().Truncate(time.Second)
	ic := &model.IssuedCertificate{DeviceID: "d1", CAProfileID: p.ID, Protocol: "est", Use: "identity", Subject: "CN=x", Issuer: "CN=ca",
		Serial: "1a", NotBefore: now, NotAfter: now.Add(24 * time.Hour), Fingerprint: "ff", KeyType: "rsa2048", CertPEM: "-----BEGIN CERTIFICATE-----\nAA==\n-----END CERTIFICATE-----\n"}
	const keyPEM = "-----BEGIN PRIVATE KEY-----\nSECRETKEYMATERIAL\n-----END PRIVATE KEY-----\n"
	if err := s.SaveIssuedCertificate(ctx, ic, keyPEM); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err := s.db.QueryRow(`SELECT key_enc FROM issued_certificates WHERE id=?`, ic.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "SECRETKEYMATERIAL") {
		t.Fatal("private key stored in clear")
	}
	if k, err := s.IssuedCertificateKey(ctx, ic.ID); err != nil || k != keyPEM {
		t.Errorf("key round-trip: %q %v", k, err)
	}
	list, _ := s.ListIssuedCertificates(ctx, "d1")
	if len(list) != 1 || list[0].Installed || list[0].CertPEM == "" || list[0].NotAfter != ic.NotAfter {
		t.Errorf("issued list: %+v", list)
	}
	if err := s.MarkCertificateInstalled(ctx, ic.ID, "plugin", now); err != nil {
		t.Fatal(err)
	}
	g, _ := s.GetIssuedCertificate(ctx, ic.ID)
	if !g.Installed || g.InstalledBy != "plugin" || g.InstalledAt == nil {
		t.Errorf("installed: %+v", g)
	}

	// Observed certificates replace per source.
	certs := []model.Certificate{{ID: "tls:443", Use: "identity", Subject: "CN=a", NotAfter: now.Add(time.Hour), SelfSigned: true}, {ID: "tls:631", Use: "identity", Subject: "CN=a", NotAfter: now.Add(time.Hour)}}
	if err := s.SaveDeviceCertificates(ctx, "d1", "probe", certs, now); err != nil {
		t.Fatal(err)
	}
	_ = s.SaveDeviceCertificates(ctx, "d1", "plugin", certs[:1], now)
	obs, _ := s.ListDeviceCertificates(ctx, "d1")
	if len(obs) != 3 {
		t.Fatalf("observed: %d", len(obs))
	}
	_ = s.SaveDeviceCertificates(ctx, "d1", "probe", certs[:1], now)
	obs, _ = s.ListDeviceCertificates(ctx, "d1")
	if len(obs) != 2 {
		t.Errorf("probe source should be replaced: %d", len(obs))
	}
	if all, _ := s.ListDeviceCertificates(ctx, ""); len(all) != 2 {
		t.Errorf("all devices: %d", len(all))
	}

	// Credential secrets decrypt only through CredentialSecret.
	c := &model.Credential{Name: "ca-login", Type: model.CredAdminPassword, Username: "est-user"}
	if err := s.CreateCredential(ctx, c, &model.Secret{Password: "pw!"}); err != nil {
		t.Fatal(err)
	}
	cred, sec, err := s.CredentialSecret(ctx, c.ID)
	if err != nil || cred.Username != "est-user" || sec.Password != "pw!" {
		t.Errorf("credential secret: %+v %+v %v", cred, sec, err)
	}
	if _, _, err := s.CredentialSecret(ctx, "nope"); err != ErrNotFound {
		t.Errorf("missing credential: %v", err)
	}
	if err := s.DeleteCAProfile(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteCAProfile(ctx, p.ID); err != ErrNotFound {
		t.Errorf("double delete: %v", err)
	}
}
