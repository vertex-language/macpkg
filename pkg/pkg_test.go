package pkg_test

import (
	"context"
	"testing"

	"github.com/vertex-language/macpkg/pkg"
	"github.com/vertex-language/macpkg/pkg/bom"
	"github.com/vertex-language/macpkg/pkg/cpio"
	"github.com/vertex-language/macpkg/pkg/xar"
	"github.com/vertex-language/macpkg/vfs"
)

func TestBuild_MemFS(t *testing.T) {
	ctx := context.Background()
	mem := vfs.NewMemFS()

	payloadFiles := map[string][]byte{
		"App.app/Contents/PkgInfo":        []byte("APPL????"),
		"App.app/Contents/Info.plist":     []byte("<plist><dict></dict></plist>"),
		"App.app/Contents/MacOS/app_exec": []byte("RAW MACHO BINARY"),
	}

	scripts := map[string][]byte{
		"postinstall": []byte("#!/bin/sh\necho installed\nexit 0\n"),
	}

	res, err := pkg.Build(ctx, pkg.Config{
		Identifier:      "com.example.testapp.pkg",
		Version:         "1.0.0",
		InstallLocation: "/Applications",
		PayloadFiles:    payloadFiles,
		Scripts:         scripts,
		OutFile:         "dist/TestApp.pkg",
		FS:              mem,
	})
	if err != nil {
		t.Fatalf("pkg.Build: %v", err)
	}

	if res.OutputFile != "dist/TestApp.pkg" {
		t.Errorf("expected dist/TestApp.pkg, got %s", res.OutputFile)
	}
	if res.FilesCount != 3 {
		t.Errorf("expected 3 files, got %d", res.FilesCount)
	}

	pkgBytes, err := mem.ReadFile("dist/TestApp.pkg")
	if err != nil {
		t.Fatalf("read dist/TestApp.pkg: %v", err)
	}

	// Extract XAR container
	extracted, err := xar.Extract(pkgBytes)
	if err != nil {
		t.Fatalf("xar.Extract: %v", err)
	}

	// Verify XAR entries: PackageInfo, Payload, Bom, Scripts
	if _, ok := extracted["PackageInfo"]; !ok {
		t.Errorf("missing PackageInfo in XAR")
	}
	if _, ok := extracted["Bom"]; !ok {
		t.Errorf("missing Bom in XAR")
	}
	if _, ok := extracted["Payload"]; !ok {
		t.Errorf("missing Payload in XAR")
	}
	if _, ok := extracted["Scripts"]; !ok {
		t.Errorf("missing Scripts in XAR")
	}

	// Verify BOM header
	if !bom.Validate(extracted["Bom"]) {
		t.Errorf("extracted Bom is not a valid BOMStore")
	}

	// Verify Payload extracts via cpio
	cpioEntries, err := cpio.ExtractGz(extracted["Payload"])
	if err != nil {
		t.Fatalf("cpio.ExtractGz(Payload): %v", err)
	}

	foundEntries := make(map[string]bool)
	for _, e := range cpioEntries {
		foundEntries[e.Name] = true
	}

	for p := range payloadFiles {
		expectedPath := "./" + p
		if !foundEntries[expectedPath] {
			t.Errorf("expected %s in Payload cpio entries", expectedPath)
		}
	}
}
