package macpkg_test

import (
	"context"
	"testing"

	"github.com/vertex-language/macpkg"
	"github.com/vertex-language/macpkg/app"
	"github.com/vertex-language/macpkg/dmg"
	"github.com/vertex-language/macpkg/pkg"
	"github.com/vertex-language/macpkg/sign"
	"github.com/vertex-language/macpkg/vfs"
)

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		input string
		want  macpkg.Format
	}{
		{"MyApp.app", macpkg.FormatApp},
		{"installer.dmg", macpkg.FormatDMG},
		{"package.pkg", macpkg.FormatPKG},
		{"app", macpkg.FormatApp},
		{"dmg", macpkg.FormatDMG},
		{"pkg", macpkg.FormatPKG},
		{"unknown.txt", ""},
	}

	for _, tt := range tests {
		got := macpkg.DetectFormat(tt.input)
		if got != tt.want {
			t.Errorf("DetectFormat(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBuildAppWithSign(t *testing.T) {
	mem := vfs.NewMemFS()
	cfg := macpkg.Config{
		Format: macpkg.FormatApp,
		App: &app.Config{
			Name:           "FacadeApp",
			Identifier:     "com.example.facade",
			Executable:     "facade",
			ExecutableData: []byte("#!/bin/sh\necho ok\n"),
			Version:        "1.0.0",
		},
		Sign: &sign.Config{
			Hardened: true,
			Force:    true,
		},
		FS: mem,
	}

	res, err := macpkg.Build(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if res.Format != macpkg.FormatApp {
		t.Errorf("expected format app, got %s", res.Format)
	}
	if res.SignResult == nil {
		t.Errorf("expected SignResult to be populated")
	}

	// Verify CodeResources was generated
	cr, err := mem.ReadFile("FacadeApp.app/Contents/_CodeSignature/CodeResources")
	if err != nil || len(cr) == 0 {
		t.Errorf("expected CodeResources file in sealed bundle")
	}
}

func TestBuildDMG(t *testing.T) {
	mem := vfs.NewMemFS()
	_ = mem.MkdirAll("MyApp.app/Contents", 0755)
	_ = mem.WriteFile("MyApp.app/Contents/Info.plist", []byte("<plist/>"), 0644)

	cfg := macpkg.Config{
		Format: macpkg.FormatDMG,
		DMG: &dmg.Config{
			Title:     "TestVolume",
			OutFile:   "output.dmg",
			SourceApp: "MyApp.app",
		},
		FS: mem,
	}

	res, err := macpkg.Build(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Build DMG failed: %v", err)
	}

	if res.Artifact != "output.dmg" {
		t.Errorf("expected artifact output.dmg, got %s", res.Artifact)
	}

	dmgData, err := mem.ReadFile("output.dmg")
	if err != nil || len(dmgData) == 0 {
		t.Fatalf("output.dmg was not generated")
	}
}

func TestBuildPKG(t *testing.T) {
	mem := vfs.NewMemFS()
	_ = mem.MkdirAll("root", 0755)
	_ = mem.WriteFile("root/file.txt", []byte("hello pkg"), 0644)

	cfg := macpkg.Config{
		Format: macpkg.FormatPKG,
		PKG: &pkg.Config{
			Identifier:    "com.example.testpkg",
			Version:       "1.0.0",
			SourcePayload: "root",
			OutFile:       "dist.pkg",
		},
		FS: mem,
	}

	res, err := macpkg.Build(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Build PKG failed: %v", err)
	}

	if res.Artifact != "dist.pkg" {
		t.Errorf("expected artifact dist.pkg, got %s", res.Artifact)
	}

	pkgData, err := mem.ReadFile("dist.pkg")
	if err != nil || len(pkgData) == 0 {
		t.Fatalf("dist.pkg was not generated")
	}
}
