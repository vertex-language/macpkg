package sign_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/vertex-language/macpkg/app"
	"github.com/vertex-language/macpkg/app/plist"
	"github.com/vertex-language/macpkg/sign"
	"github.com/vertex-language/macpkg/vfs"
)

func TestSealBundle(t *testing.T) {
	mem := vfs.NewMemFS()
	cfg := app.Config{
		Name:           "DemoApp",
		Identifier:     "com.example.demo",
		Executable:     "demo",
		ExecutableData: []byte("#!/bin/sh\necho hi\n"),
		Version:        "1.0.0",
		FS:             mem,
	}
	_, err := app.Assemble(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	// Add a bundle resource
	resPath := filepath.Join("DemoApp.app", "Contents", "Resources", "sample.txt")
	if err := mem.WriteFile(resPath, []byte("resource content"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	appPath := "DemoApp.app"
	res, err := sign.Sign(context.Background(), sign.Config{
		Target: appPath,
		FS:     mem,
	})
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	if !res.IsBundle {
		t.Errorf("expected IsBundle=true")
	}

	crPath := filepath.Join(appPath, "Contents", "_CodeSignature", "CodeResources")
	data, err := mem.ReadFile(crPath)
	if err != nil {
		t.Fatalf("CodeResources not written: %v", err)
	}

	val, err := plist.UnmarshalXML(data)
	if err != nil {
		t.Fatalf("unmarshal CodeResources failed: %v", err)
	}

	files, ok := val["files"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'files' dict in CodeResources, got %T", val["files"])
	}

	if _, ok := files["Resources/sample.txt"]; !ok {
		t.Errorf("expected Resources/sample.txt in CodeResources files map")
	}
}

func TestSignRealMachO(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "main.go")
	if err := os.WriteFile(src, []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	binPath := filepath.Join(tmp, "mybin")
	cmd := exec.Command("go", "build", "-o", binPath, src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("skipping real Mach-O test (go build unavailable): %v: %s", err, string(out))
	}

	res, err := sign.Sign(context.Background(), sign.Config{
		Target:     binPath,
		Identifier: "com.example.mybin",
	})
	if err != nil {
		t.Fatalf("Sign real Mach-O: %v", err)
	}

	if res.Identifier != "com.example.mybin" {
		t.Errorf("expected identifier com.example.mybin, got %s", res.Identifier)
	}
}
