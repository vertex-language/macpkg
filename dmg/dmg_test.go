package dmg_test

import (
	"context"
	"testing"

	"github.com/vertex-language/macpkg/dmg"
	"github.com/vertex-language/macpkg/vfs"
)

func TestBuild_MemFS(t *testing.T) {
	ctx := context.Background()
	mem := vfs.NewMemFS()

	res, err := dmg.Build(ctx, dmg.Config{
		Title:       "TestApp Installer",
		SourceApp:   "TestApp.app",
		OutFile:     "dist/TestApp.dmg",
		IconSize:    128,
		FS:          mem,
	})
	if err != nil {
		t.Fatalf("dmg.Build: %v", err)
	}

	if res.OutputFile != "dist/TestApp.dmg" {
		t.Errorf("expected dist/TestApp.dmg, got %s", res.OutputFile)
	}
	if res.TotalSize <= 512 {
		t.Errorf("expected dmg size > 512, got %d", res.TotalSize)
	}

	dmgData, err := mem.ReadFile("dist/TestApp.dmg")
	if err != nil {
		t.Fatalf("read dist/TestApp.dmg: %v", err)
	}

	// Verify koly trailer
	trailer := dmgData[len(dmgData)-512:]
	if string(trailer[:4]) != "koly" {
		t.Fatalf("expected koly trailer, got %s", string(trailer[:4]))
	}
}
