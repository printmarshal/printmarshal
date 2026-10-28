package security

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"printmarshal/internal/engine"
	"printmarshal/internal/jobs"
	"printmarshal/internal/model"
	"printmarshal/internal/security/enroll"
	"printmarshal/internal/store"
)

// Expiry thresholds for cert:expiry alerts.
const (
	ExpiryWarningDays  = 30
	ExpiryCriticalDays = 7
)

// DeviceCertificates is everything known about a device's certificates.
type DeviceCertificates struct {
	DeviceID string `json:"deviceId"`
	// Installed is what drivers implementing manage_certs report.
	Installed []model.DeviceCertificate `json:"installed"`
	// Observed is what the TLS probe saw on the device's HTTPS/IPPS ports.
	Observed []model.DeviceCertificate `json:"observed"`
	// Issued are certificates PrintMarshal obtained from a CA for the device.
	Issued []*model.IssuedCertificate `json:"issued"`
	// CanInstall reports whether any driver can install certificates on it.
	CanInstall bool        `json:"canInstall"`
	Drivers    []string    `json:"drivers,omitempty"`
	TLS        []TLSReport `json:"tls,omitempty"`
}

// ListCertificates lists a device's certificates live: every manage_certs
// driver plus the TLS probe, stored as observations, plus the issued records.
func (s *Service) ListCertificates(ctx context.Context, dev *model.Device) (*DeviceCertificates, error) {
	out := &DeviceCertificates{DeviceID: dev.ID, Installed: []model.DeviceCertificate{}, Observed: []model.DeviceCertificate{}, Issued: []*model.IssuedCertificate{}}
	creds := s.creds(ctx, dev)
	now := s.Now()
	if s.Engine != nil {
		for _, drv := range s.Engine.Resolve(dev, model.CapManageCerts) {
			cm, ok := drv.(engine.CertManager)
			if !ok {
				continue
			}
			out.CanInstall = true
			out.Drivers = append(out.Drivers, drv.Name())
			certs, err := cm.ListCertificates(ctx, dev, creds)
			if err != nil {
				s.warn("list certificates", dev, drv.Name(), err)
				continue
			}
			_ = s.Store.SaveDeviceCertificates(ctx, dev.ID, drv.Name(), certs, now)
			for _, c := range certs {
				out.Installed = append(out.Installed, model.DeviceCertificate{DeviceID: dev.ID, Source: drv.Name(), SeenAt: now, Certificate: c})
			}
		}
	}
	if s.TLSProbe != nil && dev.Address != "" {
		out.TLS = s.TLSProbe(ctx, dev.Address, DefaultTLSPorts)
		var leafs []model.Certificate
		for _, r := range out.TLS {
			if r.Leaf != nil {
				leafs = append(leafs, *r.Leaf)
				out.Observed = append(out.Observed, model.DeviceCertificate{DeviceID: dev.ID, Source: "probe", SeenAt: now, Certificate: *r.Leaf})
			}
		}
		if len(leafs) > 0 {
			_ = s.Store.SaveDeviceCertificates(ctx, dev.ID, "probe", leafs, now)
		}
	}
	issued, err := s.Store.ListIssuedCertificates(ctx, dev.ID)
	if err != nil {
		return nil, err
	}
	out.Issued = issued
	return out, nil
}

// StoredCertificates returns the last stored observations without touching the device.
func (s *Service) StoredCertificates(ctx context.Context, deviceID string) (*DeviceCertificates, error) {
	out := &DeviceCertificates{DeviceID: deviceID, Installed: []model.DeviceCertificate{}, Observed: []model.DeviceCertificate{}, Issued: []*model.IssuedCertificate{}}
	certs, err := s.Store.ListDeviceCertificates(ctx, deviceID)
	if err != nil {
		return nil, err
	}
	for _, c := range certs {
		if c.Source == "probe" {
			out.Observed = append(out.Observed, c)
		} else {
			out.Installed = append(out.Installed, c)
		}
	}
	out.Issued, err = s.Store.ListIssuedCertificates(ctx, deviceID)
	return out, err
}

// EnrollRequest asks for a certificate from a CA profile for one device.
type EnrollRequest struct {
	CAProfileID string   `json:"caProfileId"`
	Use         string   `json:"use"` // identity (default) | ca
	CommonName  string   `json:"commonName,omitempty"`
	SANs        []string `json:"sans,omitempty"`
	DryRun      bool     `json:"dryRun"`
}

// EnrollCertificate queues an enrollment (job "security.enroll"): generate a
// key and CSR, obtain the certificate from the CA profile, record it, and
// install it through a manage_certs driver when one exists.
func (s *Service) EnrollCertificate(ctx context.Context, deviceID string, req EnrollRequest, actor string) (*model.Job, error) {
	if _, err := s.Store.GetDevice(ctx, deviceID); err != nil {
		return nil, err
	}
	if _, err := s.Store.GetCAProfile(ctx, req.CAProfileID); err != nil {
		return nil, fmt.Errorf("CA profile: %w", err)
	}
	switch req.Use {
	case "", "identity":
		req.Use = "identity"
	case "ca":
	default:
		return nil, fmt.Errorf("use must be identity or ca")
	}
	sans := make([]any, len(req.SANs))
	for i, sn := range req.SANs {
		sans[i] = sn
	}
	return s.Jobs.Submit(ctx, &model.Job{Type: JobEnroll, CreatedBy: actor,
		Target: map[string]any{"deviceIds": []any{deviceID}, "caProfileId": req.CAProfileID},
		Params: map[string]any{"caProfileId": req.CAProfileID, "use": req.Use, "commonName": req.CommonName, "sans": sans, "dryRun": req.DryRun}})
}

func (s *Service) runEnroll(ctx context.Context, jc *jobs.Context) error {
	devs, err := s.ResolveTarget(ctx, targetFromJob(jc.Job))
	if err != nil {
		return err
	}
	req := EnrollRequest{CAProfileID: jobString(jc.Job, "caProfileId"), Use: jobString(jc.Job, "use"), CommonName: jobString(jc.Job, "commonName"), DryRun: jobBool(jc.Job, "dryRun", true)}
	if sans, ok := jc.Job.Params["sans"].([]any); ok {
		for _, sn := range sans {
			if str, ok := sn.(string); ok {
				req.SANs = append(req.SANs, str)
			}
		}
	}
	profile, err := s.Store.GetCAProfile(ctx, req.CAProfileID)
	if err != nil {
		return fmt.Errorf("CA profile: %w", err)
	}
	s.forEachDevice(ctx, jc, devs, 10*time.Minute, func(ctx context.Context, d *model.Device) (string, string, map[string]any) {
		return s.enrollDevice(ctx, d, profile, req, jc.Job.CreatedBy, jc.Job.ID)
	})
	return nil
}

// certManager returns the best manage_certs driver for a device, or nil.
func (s *Service) certManager(dev *model.Device) (engine.CertManager, string) {
	if s.Engine == nil {
		return nil, ""
	}
	for _, drv := range s.Engine.Resolve(dev, model.CapManageCerts) {
		if cm, ok := drv.(engine.CertManager); ok {
			return cm, drv.Name()
		}
	}
	return nil, ""
}

func (s *Service) enrollDevice(ctx context.Context, dev *model.Device, profile *model.CAProfile, req EnrollRequest, actor, jobID string) (string, string, map[string]any) {
	cn := req.CommonName
	sans := req.SANs
	if cn == "" {
		cn = nonEmpty(dev.Hostname, dev.Address)
	}
	if len(sans) == 0 {
		if dev.Hostname != "" {
			sans = append(sans, dev.Hostname)
		}
		if dev.Address != "" {
			sans = append(sans, dev.Address)
		}
	}
	if profile.Protocol == model.EnrollACME {
		// dns-01 can only validate DNS names (see enroll.ACMEClient).
		dns, _ := enroll.SplitSANs(sans)
		sans = dns
		if net.ParseIP(cn) != nil {
			if len(dns) == 0 {
				return "error", "acme: the device has no DNS name to validate with dns-01; set a hostname or pass DNS SANs", nil
			}
			cn = dns[0]
		}
	}
	keyType := nonEmpty(profile.KeyType, enroll.KeyRSA2048)
	if profile.Protocol == model.EnrollSCEP && !strings.HasPrefix(keyType, "rsa") {
		return "error", "scep requires an RSA key type on the CA profile (RFC 8894)", nil
	}
	key, err := enroll.GenerateKey(keyType)
	if err != nil {
		return "error", err.Error(), nil
	}
	csr, err := enroll.NewCSR(key, cn, sans)
	if err != nil {
		return "error", err.Error(), nil
	}
	cm, driver := s.certManager(dev)
	data := map[string]any{"dryRun": req.DryRun, "caProfileId": profile.ID, "protocol": profile.Protocol, "use": req.Use, "commonName": cn, "sans": sans, "keyType": keyType, "installDriver": driver, "canInstall": cm != nil}
	if req.DryRun {
		msg := fmt.Sprintf("would enroll CN=%s (%s) with %s %s", cn, strings.Join(sans, ","), profile.Protocol, profile.Name)
		if cm == nil {
			msg += "; no driver can install certificates on this device, so the certificate would be kept for manual installation"
		} else {
			msg += "; would install via " + driver
		}
		return "ok", msg, data
	}

	cfg := enroll.Config{Profile: profile, Notify: func(m string) { s.Log.Info("security: enroll", "device", dev.ID, "msg", m) }}
	if profile.CredentialID != "" {
		cred, sec, err := s.Store.CredentialSecret(ctx, profile.CredentialID)
		if err != nil {
			return "error", "CA profile credential: " + err.Error(), data
		}
		cfg.Username, cfg.Password, cfg.AccountKeyPEM = cred.Username, nonEmpty(sec.Password, sec.Community), sec.PEM
	}
	if profile.Protocol == model.EnrollACME {
		wait, _ := time.ParseDuration(profile.Options["dnsWait"])
		cfg.DNS = &enroll.Manual{Wait: wait, Notify: cfg.Notify}
	}
	client, err := enroll.NewClient(cfg)
	if err != nil {
		return "error", err.Error(), data
	}
	res, err := client.Enroll(ctx, csr, key)
	if err != nil {
		var dnsErr *enroll.ErrDNSRecordRequired
		if errors.As(err, &dnsErr) {
			data["dnsRecord"] = map[string]any{"name": dnsErr.FQDN, "type": "TXT", "value": dnsErr.Value}
		}
		_ = s.Store.AddAudit(ctx, &model.AuditEntry{Actor: actor, Action: "security.enroll", Target: dev.ID,
			After: map[string]any{"caProfileId": profile.ID, "commonName": cn, "jobId": jobID}, Result: "failed: " + err.Error()})
		return "error", err.Error(), data
	}
	keyPEM, err := enroll.EncodeKeyPEM(key)
	if err != nil {
		return "error", err.Error(), data
	}
	leaf := CertificateFromX509(res.Cert, req.Use, "")
	issued := &model.IssuedCertificate{DeviceID: dev.ID, CAProfileID: profile.ID, Protocol: string(profile.Protocol), Use: req.Use,
		Subject: leaf.Subject, Issuer: leaf.Issuer, Serial: leaf.Serial, NotBefore: leaf.NotBefore, NotAfter: leaf.NotAfter, Fingerprint: leaf.Fingerprint,
		KeyType: keyType, CertPEM: enroll.EncodeCertPEM(res.Cert), ChainPEM: enroll.EncodeCertPEM(res.Chain...), JobID: jobID, CreatedAt: s.Now()}
	if err := s.Store.SaveIssuedCertificate(ctx, issued, keyPEM); err != nil {
		return "error", "certificate issued but could not be stored: " + err.Error(), data
	}
	data["issuedId"], data["fingerprint"], data["notAfter"], data["subject"] = issued.ID, issued.Fingerprint, issued.NotAfter, issued.Subject
	_ = s.Store.AddAudit(ctx, &model.AuditEntry{Actor: actor, Action: "security.enroll", Target: dev.ID,
		After: map[string]any{"caProfileId": profile.ID, "issuedId": issued.ID, "subject": issued.Subject, "fingerprint": issued.Fingerprint, "notAfter": issued.NotAfter, "jobId": jobID}, Result: "issued"})
	s.emit("security.certificate.issued", issued)

	if cm == nil {
		return "ok", fmt.Sprintf("certificate issued (%s, expires %s) but no driver can install certificates on this device; retrieve it from GET /api/devices/%s/certificates and install it manually",
			issued.Subject, issued.NotAfter.Format("2006-01-02"), dev.ID), data
	}
	if err := s.install(ctx, dev, issued, keyPEM, cm, driver, actor, jobID); err != nil {
		data["installError"] = err.Error()
		return "error", fmt.Sprintf("certificate issued (%s) but installing via %s failed: %s; it stays retrievable for manual installation", issued.Subject, driver, err), data
	}
	data["installed"] = true
	return "ok", fmt.Sprintf("certificate issued (%s, expires %s) and installed via %s", issued.Subject, issued.NotAfter.Format("2006-01-02"), driver), data
}

// install hands an issued certificate (and its key) to a manage_certs driver.
func (s *Service) install(ctx context.Context, dev *model.Device, issued *model.IssuedCertificate, keyPEM string, cm engine.CertManager, driver, actor, jobID string) error {
	creds := s.creds(ctx, dev)
	_, err := cm.InstallCertificate(ctx, dev, creds, model.CertRequest{Use: issued.Use, CommonName: issued.Subject, CertPEM: issued.CertPEM, KeyPEM: keyPEM, ChainPEM: issued.ChainPEM}, model.WriteOptions{})
	result := "ok"
	if err != nil {
		result = "failed: " + err.Error()
	}
	_ = s.Store.AddAudit(ctx, &model.AuditEntry{Actor: actor, Action: "security.certificate.install", Target: dev.ID,
		After: map[string]any{"issuedId": issued.ID, "driver": driver, "fingerprint": issued.Fingerprint, "jobId": jobID}, Result: result})
	if err != nil {
		return err
	}
	now := s.Now()
	issued.Installed, issued.InstalledBy, issued.InstalledAt = true, driver, &now
	_ = s.Store.MarkCertificateInstalled(ctx, issued.ID, driver, now)
	s.emit("security.certificate.installed", issued)
	return nil
}

// InstallIssued queues installation of an already issued certificate (job
// "security.install"), for devices that gained a manage_certs driver later.
func (s *Service) InstallIssued(ctx context.Context, deviceID, issuedID string, actor string) (*model.Job, error) {
	issued, err := s.Store.GetIssuedCertificate(ctx, issuedID)
	if err != nil {
		return nil, err
	}
	if issued.DeviceID != deviceID {
		return nil, fmt.Errorf("certificate %s: %w", issuedID, store.ErrNotFound)
	}
	return s.Jobs.Submit(ctx, &model.Job{Type: JobInstall, CreatedBy: actor,
		Target: map[string]any{"deviceIds": []any{deviceID}, "issuedId": issuedID}, Params: map[string]any{"issuedId": issuedID}})
}

func (s *Service) runInstall(ctx context.Context, jc *jobs.Context) error {
	devs, err := s.ResolveTarget(ctx, targetFromJob(jc.Job))
	if err != nil {
		return err
	}
	issued, err := s.Store.GetIssuedCertificate(ctx, jobString(jc.Job, "issuedId"))
	if err != nil {
		return err
	}
	keyPEM, err := s.Store.IssuedCertificateKey(ctx, issued.ID)
	if err != nil {
		return err
	}
	s.forEachDevice(ctx, jc, devs, 5*time.Minute, func(ctx context.Context, d *model.Device) (string, string, map[string]any) {
		cm, driver := s.certManager(d)
		if cm == nil {
			return "skipped", "no driver can install certificates on this device; install it manually", map[string]any{"issuedId": issued.ID}
		}
		if err := s.install(ctx, d, issued, keyPEM, cm, driver, jc.Job.CreatedBy, jc.Job.ID); err != nil {
			return "error", err.Error(), map[string]any{"issuedId": issued.ID, "driver": driver}
		}
		return "ok", "installed via " + driver, map[string]any{"issuedId": issued.ID, "driver": driver}
	})
	return nil
}

// CheckExpiry raises cert:expiry alerts from the stored certificate
// observations and issued certificates: warning when an identity
// certificate expires within 30 days, critical within 7 days or already
// expired; cleared otherwise. It does no network I/O, so main can call it
// on a schedule cheaply; assess jobs call it first.
func (s *Service) CheckExpiry(ctx context.Context) error {
	now := s.Now()
	worst := map[string]*model.Certificate{} // device -> soonest-expiring identity cert
	consider := func(deviceID string, c model.Certificate) {
		if c.Use != "" && c.Use != "identity" || c.NotAfter.IsZero() {
			return
		}
		if cur, ok := worst[deviceID]; !ok || c.NotAfter.Before(cur.NotAfter) {
			cc := c
			worst[deviceID] = &cc
		}
	}
	observed, err := s.Store.ListDeviceCertificates(ctx, "")
	if err != nil {
		return err
	}
	for _, c := range observed {
		consider(c.DeviceID, c.Certificate)
	}
	issued, err := s.Store.ListIssuedCertificates(ctx, "")
	if err != nil {
		return err
	}
	for _, c := range issued {
		if c.Installed {
			consider(c.DeviceID, model.Certificate{Use: c.Use, Subject: c.Subject, NotAfter: c.NotAfter})
		}
	}
	devs, err := s.Store.ListDevices(ctx, store.DeviceFilter{IncludeDecommissioned: true})
	if err != nil {
		return err
	}
	for _, dev := range devs {
		c, ok := worst[dev.ID]
		if !ok {
			s.clearAlerts(ctx, dev, "cert:expiry", nil)
			continue
		}
		days := int(c.NotAfter.Sub(now).Hours() / 24)
		var sev model.Severity
		var desc string
		switch {
		case c.NotAfter.Before(now):
			sev, desc = model.SeverityCritical, fmt.Sprintf("Identity certificate expired on %s (%s)", c.NotAfter.Format("2006-01-02"), c.Subject)
		case days <= ExpiryCriticalDays:
			sev, desc = model.SeverityCritical, fmt.Sprintf("Identity certificate expires in %d days on %s (%s)", days, c.NotAfter.Format("2006-01-02"), c.Subject)
		case days <= ExpiryWarningDays:
			sev, desc = model.SeverityWarning, fmt.Sprintf("Identity certificate expires in %d days on %s (%s)", days, c.NotAfter.Format("2006-01-02"), c.Subject)
		default:
			s.clearAlerts(ctx, dev, "cert:expiry", nil)
			continue
		}
		s.raiseAlert(ctx, dev, "cert:expiry", sev, desc)
	}
	return nil
}
