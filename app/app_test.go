package app_test

import (
	"context"
	"strings"
	"testing"

	"github.com/vertex-language/macpkg/app"
	"github.com/vertex-language/macpkg/vfs"
)

func TestAssemble_MemFS(t *testing.T) {
	ctx := context.Background()
	mem := vfs.NewMemFS()

	fakeExec := []byte("\xcf\xfa\xed\xfeMach-O-Executable")
	fakePNG := []byte("\x89PNG\r\n\x1a\nFake-PNG-Icon-Data")

	bundle, err := app.Assemble(ctx, app.Config{
		Name:           "TestApp",
		Identifier:     "com.example.testapp",
		Version:        "1.0.0",
		Build:          "10",
		Executable:     "testapp",
		ExecutableData: fakeExec,
		IconName:       "AppIcon.icns",
		IconData:       fakePNG,
		MinOS:          "11.0",
		Category:       "public.app-category.utilities",
		OutDir:         "TestApp.app",
		FS:             mem,
	})
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	if bundle.Path != "TestApp.app" {
		t.Errorf("expected path TestApp.app, got %s", bundle.Path)
	}

	// Verify Info.plist exists and contains keys
	plistData, err := mem.ReadFile("TestApp.app/Contents/Info.plist")
	if err != nil {
		t.Fatalf("read Info.plist: %v", err)
	}
	if !strings.Contains(string(plistData), "com.example.testapp") {
		t.Errorf("expected identifier in Info.plist, got: %s", string(plistData))
	}
	if !strings.Contains(string(plistData), "testapp") {
		t.Errorf("expected executable in Info.plist, got: %s", string(plistData))
	}

	// Verify PkgInfo
	pkgInfoData, err := mem.ReadFile("TestApp.app/Contents/PkgInfo")
	if err != nil {
		t.Fatalf("read PkgInfo: %v", err)
	}
	if string(pkgInfoData) != "APPL????" {
		t.Errorf("expected APPL????, got %s", string(pkgInfoData))
	}

	// Verify executable
	execData, err := mem.ReadFile("TestApp.app/Contents/MacOS/testapp")
	if err != nil {
		t.Fatalf("read executable: %v", err)
	}
	if string(execData) != string(fakeExec) {
		t.Errorf("executable content mismatch")
	}

	// Verify icon is valid ICNS (magic 'icns')
	iconData, err := mem.ReadFile("TestApp.app/Contents/Resources/AppIcon.icns")
	if err != nil {
		t.Fatalf("read icon: %v", err)
	}
	if len(iconData) < 8 || string(iconData[0:4]) != "icns" {
		t.Errorf("expected valid icns magic, got %s", string(iconData[:4]))
	}
}
