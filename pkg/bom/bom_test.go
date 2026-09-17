package bom_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/vertex-language/macpkg/pkg/bom"
)

func TestGenerateBOM(t *testing.T) {
	entries := []bom.FileEntry{
		{Path: "SampleApp.app/Contents/MacOS/SampleApp", Mode: 0100755, Data: []byte("TEST BINARY PAYLOAD")},
		{Path: "SampleApp.app/Contents/Info.plist", Mode: 0100644, Data: []byte("<?xml version=\"1.0\"?><plist></plist>")},
		{Path: "SampleApp.app/Contents/PkgInfo", Mode: 0100644, Data: []byte("APPL????")},
	}

	data, err := bom.Generate(entries)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if !bom.Validate(data) {
		t.Fatalf("invalid BOM signature")
	}

	// On macOS, verify that native /usr/bin/lsbom parses the BOM without errors
	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("lsbom"); err == nil {
			tmpBom := filepath.Join(t.TempDir(), "test.bom")
			if err := os.WriteFile(tmpBom, data, 0644); err != nil {
				t.Fatalf("write tmpBom: %v", err)
			}
			out, err := exec.Command("lsbom", tmpBom).CombinedOutput()
			if err != nil {
				t.Fatalf("lsbom failed on generated BOM: %v, output: %s", err, string(out))
			}
			t.Logf("lsbom output:\n%s", string(out))
		}
	}
}
