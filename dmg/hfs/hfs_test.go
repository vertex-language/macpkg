package hfs_test

import (
	"testing"

	"github.com/vertex-language/macpkg/dmg/hfs"
)

func TestGenerateVolume(t *testing.T) {
	raw, err := hfs.GenerateVolume(hfs.VolumeConfig{
		Name:   "TestAppInstaller",
		SizeMB: 5,
	})
	if err != nil {
		t.Fatalf("GenerateVolume: %v", err)
	}

	if len(raw) != 5*1024*1024 {
		t.Fatalf("expected 5MB volume, got %d", len(raw))
	}

	if !hfs.ValidateHeader(raw) {
		t.Fatalf("invalid HFS+ volume header signature")
	}
}
