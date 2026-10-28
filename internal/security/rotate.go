package security

import (
	"context"
	"errors"
	"fmt"
	"time"

	"printmarshal/internal/engine"
	"printmarshal/internal/jobs"
	"printmarshal/internal/model"
)

// RotateSNMPCommunity queues a rotation of the SNMP read community on every
// target device (job "security.rotate", kind snmpCommunity).
func (s *Service) RotateSNMPCommunity(ctx context.Context, t Target, opts Options, actor string) (*model.Job, error) {
	return s.Rotate(ctx, model.RotateSNMPCommunity, t, opts, actor)
}

// RotateAdminPassword queues a rotation of the administrator password on
// every target device (job "security.rotate", kind adminPassword).
func (s *Service) RotateAdminPassword(ctx context.Context, t Target, opts Options, actor string) (*model.Job, error) {
	return s.Rotate(ctx, model.RotateAdminPassword, t, opts, actor)
}

// Rotate queues a credential rotation. A new random secret is generated per
// device, written through write_security (falling back to write_config) and,
// only when the device accepted it, stored as a device-scoped credential
// that outranks the previous one. The secret never appears in results, logs
// or the audit log. Dry run reports which devices would change.
func (s *Service) Rotate(ctx context.Context, kind string, t Target, opts Options, actor string) (*model.Job, error) {
	if kind != model.RotateSNMPCommunity && kind != model.RotateAdminPassword {
		return nil, fmt.Errorf("unknown rotation kind %q", kind)
	}
	if _, err := s.ResolveTarget(ctx, t); err != nil {
		return nil, err
	}
	target := t.toMap()
	target["kind"] = kind
	return s.Jobs.Submit(ctx, &model.Job{Type: JobRotate, CreatedBy: actor, Target: target,
		Params: map[string]any{"kind": kind, "dryRun": opts.DryRun}})
}

func (s *Service) runRotate(ctx context.Context, jc *jobs.Context) error {
	kind := jobString(jc.Job, "kind")
	devs, err := s.ResolveTarget(ctx, targetFromJob(jc.Job))
	if err != nil {
		return err
	}
	dryRun := jobBool(jc.Job, "dryRun", true)
	s.forEachDevice(ctx, jc, devs, 2*time.Minute, func(ctx context.Context, d *model.Device) (string, string, map[string]any) {
		return s.rotateDevice(ctx, d, kind, dryRun, jc.Job.CreatedBy, jc.Job.ID)
	})
	return nil
}

// RotationResult is a device's rotation outcome (never the secret).
type RotationResult struct {
	DeviceID     string `json:"deviceId"`
	Kind         string `json:"kind"`
	DryRun       bool   `json:"dryRun"`
	Status       string `json:"status"` // rotated | would_rotate | skipped | failed
	Driver       string `json:"driver,omitempty"`
	CredentialID string `json:"credentialId,omitempty"`
	Message      string `json:"message,omitempty"`
}

func (s *Service) rotateDevice(ctx context.Context, dev *model.Device, kind string, dryRun bool, actor, jobID string) (string, string, map[string]any) {
	res := RotationResult{DeviceID: dev.ID, Kind: kind, DryRun: dryRun}
	creds := s.creds(ctx, dev)
	var key string
	var credType model.CredentialType
	switch kind {
	case model.RotateSNMPCommunity:
		key, credType = "protocols.snmp.readCommunity", model.CredSNMPCommunity
	case model.RotateAdminPassword:
		key, credType = "security.adminPassword", model.CredAdminPassword
	default:
		return "error", "unknown rotation kind", nil
	}
	if len(s.writers(dev)) == 0 {
		res.Status, res.Message = "skipped", "no driver can write security settings on this device"
		return "skipped", res.Message, asMap(res)
	}
	secret, err := randomSecret(24)
	if err != nil {
		return "error", err.Error(), nil
	}
	if dryRun {
		// A dry run still asks the writer so an unsupported key is reported now.
		results, err := s.applyChanges(ctx, dev, creds, []model.SettingChange{{Key: key, Value: secret}}, model.WriteOptions{DryRun: true})
		if err != nil && !errors.Is(err, engine.ErrUnsupported) {
			res.Status, res.Message = "failed", err.Error()
			return "error", res.Message, asMap(res)
		}
		r := results[key]
		res.Driver = r.Driver
		switch r.Status {
		case model.SettingWouldWrite, model.SettingWritten, model.SettingUnchanged:
			res.Status, res.Message = "would_rotate", fmt.Sprintf("would rotate %s via %s", key, r.Driver)
			return "ok", res.Message, asMap(res)
		default:
			res.Status, res.Message = "skipped", nonEmpty(r.Error, "no driver accepted the setting")
			return "skipped", res.Message, asMap(res)
		}
	}
	// The real write needs a write credential bound to the device (audit A19).
	wc, err := s.writeCreds(ctx, dev)
	if err != nil {
		res.Status, res.Message = "failed", err.Error()
		s.auditRotation(ctx, actor, dev, res, jobID)
		return "error", res.Message, asMap(res)
	}
	results, err := s.applyChanges(ctx, dev, wc, []model.SettingChange{{Key: key, Value: secret}}, model.WriteOptions{})
	if err != nil && len(results) == 0 {
		res.Status, res.Message = "failed", err.Error()
		s.auditRotation(ctx, actor, dev, res, jobID)
		return "error", res.Message, asMap(res)
	}
	r := results[key]
	res.Driver = r.Driver
	if r.Status != model.SettingWritten {
		res.Status, res.Message = "failed", nonEmpty(r.Error, fmt.Sprintf("device reported %s", r.Status))
		if r.Status == model.SettingSkipped {
			res.Status = "skipped"
		}
		s.auditRotation(ctx, actor, dev, res, jobID)
		if res.Status == "skipped" {
			return "skipped", res.Message, asMap(res)
		}
		return "error", res.Message, asMap(res)
	}
	// The device accepted the secret: store it so PrintMarshal keeps access. A
	// rotated read community is a read credential; a rotated admin password
	// authenticates reads and writes on the device's web service, so it is
	// stored as both (audit A19).
	cred := &model.Credential{Name: fmt.Sprintf("rotated %s %s", kind, s.Now().Format("2006-01-02 15:04")), Type: credType,
		Scope: "device:" + dev.ID, Priority: s.nextCredentialPriority(ctx, dev.ID, credType)}
	sec := &model.Secret{}
	switch credType {
	case model.CredSNMPCommunity:
		cred.Purpose = model.PurposeRead
		sec.Community = secret
	case model.CredAdminPassword:
		cred.Purpose = model.PurposeBoth
		cred.Username = nonEmpty(wc.AdminUser, nonEmpty(creds.AdminUser, "admin"))
		sec.Password = secret
	}
	if err := s.Store.CreateCredential(ctx, cred, sec); err != nil {
		res.Status, res.Message = "failed", "device accepted the new secret but storing the credential failed: "+err.Error()+"; the device may need manual recovery"
		s.auditRotation(ctx, actor, dev, res, jobID)
		return "error", res.Message, asMap(res)
	}
	res.Status, res.CredentialID, res.Message = "rotated", cred.ID, fmt.Sprintf("rotated %s via %s; new device-scoped credential %s", kind, r.Driver, cred.ID)
	s.auditRotation(ctx, actor, dev, res, jobID)
	s.emit("security.rotated", res)
	return "ok", res.Message, asMap(res)
}

func (s *Service) auditRotation(ctx context.Context, actor string, dev *model.Device, res RotationResult, jobID string) {
	_ = s.Store.AddAudit(ctx, &model.AuditEntry{Actor: actor, Action: "security.rotate", Target: dev.ID,
		After: map[string]any{"kind": res.Kind, "driver": res.Driver, "credentialId": res.CredentialID, "jobId": jobID}, Result: res.Status})
}

func (s *Service) nextCredentialPriority(ctx context.Context, deviceID string, typ model.CredentialType) int {
	max := 0
	cs, err := s.Store.ListCredentials(ctx)
	if err != nil {
		return 1
	}
	for _, c := range cs {
		if c.Scope == "device:"+deviceID && c.Type == typ && c.Priority >= max {
			max = c.Priority + 1
		}
	}
	return max
}

func asMap(r RotationResult) map[string]any {
	m := map[string]any{"deviceId": r.DeviceID, "kind": r.Kind, "dryRun": r.DryRun, "status": r.Status}
	if r.Driver != "" {
		m["driver"] = r.Driver
	}
	if r.CredentialID != "" {
		m["credentialId"] = r.CredentialID
	}
	return m
}

func nonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
