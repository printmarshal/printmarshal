package firmware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"printmarshal/internal/model"
	"printmarshal/internal/store"
)

// PackageMeta describes an uploaded firmware image.
type PackageMeta struct {
	Vendor     string
	Models     []string
	Version    string
	MinVersion string
	Notes      string
	FileName   string
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// safeFileName keeps a file name usable on disk and free of path elements.
func safeFileName(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = unsafeName.ReplaceAllString(name, "_")
	name = strings.Trim(name, "._")
	if name == "" {
		return "firmware.bin"
	}
	return name
}

// RegisterPackage stores an uploaded image under <DataDir>/firmware/<id>/<file>
// with its SHA-256 and registers the metadata.
func (s *Service) RegisterPackage(ctx context.Context, meta PackageMeta, r io.Reader, by string) (*model.FirmwarePackage, error) {
	meta.Version = strings.TrimSpace(meta.Version)
	if meta.Version == "" {
		return nil, invalidf("version is required")
	}
	models := []string{}
	for _, m := range meta.Models {
		if m = strings.TrimSpace(m); m != "" {
			models = append(models, m)
		}
	}
	p := &model.FirmwarePackage{ID: newID(), Vendor: strings.TrimSpace(meta.Vendor), Models: models, Version: meta.Version,
		MinVersion: strings.TrimSpace(meta.MinVersion), FileName: safeFileName(meta.FileName), Notes: strings.TrimSpace(meta.Notes),
		CreatedBy: by, CreatedAt: time.Now().UTC()}
	dir := filepath.Join(s.Dir(), p.ID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	// StoragePath is kept relative to the firmware root so a backup restored
	// into another data directory (or a container) still finds its images
	// (code review H-018); PackagePath resolves it, and older absolute
	// values too.
	p.StoragePath = filepath.ToSlash(filepath.Join(p.ID, p.FileName))
	f, err := os.OpenFile(s.PackagePath(p), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o640)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), r)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	if n == 0 {
		_ = os.RemoveAll(dir)
		return nil, invalidf("firmware file is empty")
	}
	p.Size, p.SHA256 = n, hex.EncodeToString(h.Sum(nil))
	if err := s.Store.CreateFirmwarePackage(ctx, p); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	return p, nil
}

// GetPackage returns a package by id.
func (s *Service) GetPackage(ctx context.Context, id string) (*model.FirmwarePackage, error) {
	return s.Store.GetFirmwarePackage(ctx, id)
}

// ListPackages returns every registered package.
func (s *Service) ListPackages(ctx context.Context) ([]*model.FirmwarePackage, error) {
	return s.Store.ListFirmwarePackages(ctx)
}

// DeletePackage removes a package and its file. It is refused while an
// unfinished campaign references the package.
func (s *Service) DeletePackage(ctx context.Context, id string) error {
	p, err := s.Store.GetFirmwarePackage(ctx, id)
	if err != nil {
		return err
	}
	active, err := s.Store.ListCampaigns(ctx, store.CampaignFilter{PackageID: id, Active: true})
	if err != nil {
		return err
	}
	if len(active) > 0 {
		names := make([]string, 0, len(active))
		for _, c := range active {
			names = append(names, fmt.Sprintf("%s (%s)", c.Name, c.Status))
		}
		return conflictf("package is referenced by unfinished campaign(s): %s", strings.Join(names, ", "))
	}
	if err := s.Store.DeleteFirmwarePackage(ctx, id); err != nil {
		return err
	}
	if p.StoragePath != "" {
		_ = os.RemoveAll(filepath.Dir(s.PackagePath(p)))
	}
	return nil
}

// PackagePath is the on-disk location of a package's image: StoragePath
// resolved against the current firmware root. Absolute paths written by
// earlier versions are honoured when they still exist and otherwise rebased
// onto the root by package id and file name.
func (s *Service) PackagePath(p *model.FirmwarePackage) string {
	if p == nil || p.StoragePath == "" {
		return ""
	}
	if filepath.IsAbs(p.StoragePath) {
		if _, err := os.Stat(p.StoragePath); err == nil {
			return p.StoragePath
		}
		return filepath.Join(s.Dir(), p.ID, filepath.Base(p.StoragePath))
	}
	return filepath.Join(s.Dir(), filepath.FromSlash(p.StoragePath))
}

// VerifyPackage recomputes the stored file's SHA-256 and compares it with the
// registered digest. It runs before every delivery.
func (s *Service) VerifyPackage(p *model.FirmwarePackage) error {
	if p == nil || p.StoragePath == "" {
		return fmt.Errorf("package has no stored file")
	}
	f, err := os.Open(s.PackagePath(p))
	if err != nil {
		return fmt.Errorf("package file: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, p.SHA256) {
		return fmt.Errorf("package %s failed integrity check: stored file sha256 %s differs from registered %s", p.FileName, got[:12], p.SHA256[:12])
	}
	return nil
}

func newID() string { return uuid.NewString() }
