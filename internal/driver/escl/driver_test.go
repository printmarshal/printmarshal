package escl

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"printmarshal/internal/model"
)

type fakeTransport struct{ files map[string]string }

func (f fakeTransport) Get(_ context.Context, url string) ([]byte, error) {
	for suffix, path := range f.files {
		if strings.HasSuffix(url, suffix) && strings.HasPrefix(url, "http://198.51.100.5/eSCL") {
			return os.ReadFile(path)
		}
	}
	return nil, errors.New("404")
}

func TestIdentifyFromRealCapabilities(t *testing.T) {
	d := New(fakeTransport{map[string]string{
		"ScannerCapabilities": "../../../testdata/fixtures/hp-m283fdw-escl-ScannerCapabilities.xml",
		"ScannerStatus":       "../../../testdata/fixtures/hp-m283fdw-escl-ScannerStatus.xml",
	}}, DefaultOptions())
	id, err := d.Identify(context.Background(), "198.51.100.5", model.Creds{})
	if err != nil {
		t.Fatal(err)
	}
	if id.DeviceType != "mfp" || id.Make != "HP" || id.Model != "HP Color LaserJet MFP M283fdw" || id.Serial != "VNB0TEST01" || id.UUID != "564e4230-5445-5354-3031-02005e0a1b2c" {
		t.Errorf("identity = %+v", id)
	}
	st, err := d.ScannerStatus(context.Background(), "198.51.100.5")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "Idle" || st.AdfState != "ScannerAdfEmpty" {
		t.Errorf("status = %+v", st)
	}
	if _, err := d.Identify(context.Background(), "10.0.0.1", model.Creds{}); err == nil {
		t.Error("a host without eSCL must not identify")
	}
}
