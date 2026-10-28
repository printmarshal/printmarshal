package engine

import (
	"context"

	"printmarshal/internal/model"
)

// Write-side capability interfaces (spec §6). Drivers implement the subset
// they declare in Capabilities(); the engine dispatches by capability and
// never by vendor. Every write takes WriteOptions so a dry run is always
// possible (spec §3 principle 4).

// ConfigReader serves read_config: a normalised configuration document.
type ConfigReader interface {
	ReadConfig(ctx context.Context, d *model.Device, creds model.Creds, sel model.ConfigSelector) (*model.ConfigDocument, error)
}

// ConfigWriter serves write_config. It must honour opts.DryRun (no device I/O
// that changes state), report per-setting results, and fill Rollback with the
// previous values of everything it wrote.
type ConfigWriter interface {
	WriteConfig(ctx context.Context, d *model.Device, creds model.Creds, changes []model.SettingChange, opts model.WriteOptions) (*model.WriteResult, error)
}

// SecurityReader serves read_security: security-relevant settings as a
// ConfigDocument restricted to the security/protocols areas.
type SecurityReader interface {
	ReadSecurity(ctx context.Context, d *model.Device, creds model.Creds) (*model.ConfigDocument, error)
}

// SecurityWriter serves write_security with the same contract as ConfigWriter.
type SecurityWriter interface {
	WriteSecurity(ctx context.Context, d *model.Device, creds model.Creds, changes []model.SettingChange, opts model.WriteOptions) (*model.WriteResult, error)
}

// CertManager serves manage_certs.
type CertManager interface {
	ListCertificates(ctx context.Context, d *model.Device, creds model.Creds) ([]model.Certificate, error)
	InstallCertificate(ctx context.Context, d *model.Device, creds model.Creds, req model.CertRequest, opts model.WriteOptions) (*model.CertResult, error)
}

// FirmwareUpdater serves firmware_update. Implementations must check
// compatibility (vendor/model/min version) before delivery and must treat
// opts.DryRun as "validate and report only".
type FirmwareUpdater interface {
	UpdateFirmware(ctx context.Context, d *model.Device, creds model.Creds, pkg *model.FirmwarePackage, opts model.WriteOptions) (*model.FirmwareResult, error)
}

// Resolve returns the drivers that serve cap for d, best first, as a typed
// helper for the write-side services.
func (e *Engine) Resolve(d *model.Device, cap model.Capability) []Driver {
	return e.Registry.For(d, cap)
}
