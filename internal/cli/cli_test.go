package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vertex-language/macpkg/internal/cli"
)

func runCLI(args ...string) (int, string, string) {
	var out, err bytes.Buffer
	code := cli.Main(context.Background(), cli.Env{
		Args: args,
		Dir:  ".",
		In:   strings.NewReader(""),
		Out:  &out,
		Err:  &err,
	})
	return code, out.String(), err.String()
}

func TestCLIHelpAndVersion(t *testing.T) {
	code, out, _ := runCLI("--help")
	if code != 0 || !strings.Contains(out, "macpkg v") {
		t.Errorf("expected help output, got code %d, out: %s", code, out)
	}

	code, out, _ = runCLI("--version")
	if code != 0 || !strings.Contains(out, "macpkg v1.0.0") {
		t.Errorf("expected version output, got code %d, out: %s", code, out)
	}
}

func TestSubcommandHelp(t *testing.T) {
	subcmds := []string{"app", "dmg", "pkg", "sign", "notary"}
	for _, sc := range subcmds {
		code, out, _ := runCLI(sc, "--help")
		if code != 0 || len(out) == 0 {
			t.Errorf("failed help for %s: code %d", sc, code)
		}
	}
}

func TestUnknownCommand(t *testing.T) {
	code, _, err := runCLI("nonexistent")
	if code != 1 || !strings.Contains(err, "unknown command") {
		t.Errorf("expected error for unknown command, got %d, err: %s", code, err)
	}
}

func TestCLIAppBuild(t *testing.T) {
	tmp := t.TempDir()
	binPath := filepath.Join(tmp, "mybin")
	_ = os.WriteFile(binPath, []byte("#!/bin/sh\necho test\n"), 0755)

	outApp := filepath.Join(tmp, "TestCLI.app")
	code, out, err := runCLI("app", "build",
		"--name", "TestCLI",
		"--id", "com.example.testcli",
		"--bin", binPath,
		"--out", outApp,
	)
	if code != 0 {
		t.Fatalf("app build failed: %s", err)
	}
	if !strings.Contains(out, "Successfully assembled") {
		t.Errorf("unexpected output: %s", out)
	}

	infoPath := filepath.Join(outApp, "Contents", "Info.plist")
	if _, statErr := os.Stat(infoPath); statErr != nil {
		t.Fatalf("Info.plist not found in %s", outApp)
	}
}

func TestCLIDMGBuild(t *testing.T) {
	tmp := t.TempDir()
	appPath := filepath.Join(tmp, "Demo.app")
	_ = os.MkdirAll(filepath.Join(appPath, "Contents"), 0755)
	_ = os.WriteFile(filepath.Join(appPath, "Contents", "Info.plist"), []byte("<plist/>"), 0644)

	outDMG := filepath.Join(tmp, "Demo.dmg")
	code, out, err := runCLI("dmg", "build",
		"--title", "Demo",
		"--app", appPath,
		"--out", outDMG,
	)
	if code != 0 {
		t.Fatalf("dmg build failed: %s", err)
	}
	if !strings.Contains(out, "Successfully built") {
		t.Errorf("unexpected output: %s", out)
	}
	if _, statErr := os.Stat(outDMG); statErr != nil {
		t.Fatalf("out dmg not found: %v", statErr)
	}
}

func TestCLIPKGBuild(t *testing.T) {
	tmp := t.TempDir()
	rootPayload := filepath.Join(tmp, "root")
	_ = os.MkdirAll(rootPayload, 0755)
	_ = os.WriteFile(filepath.Join(rootPayload, "app.bin"), []byte("payload"), 0644)

	outPKG := filepath.Join(tmp, "Demo.pkg")
	code, out, err := runCLI("pkg", "build",
		"--id", "com.example.demo.pkg",
		"--payload", rootPayload,
		"--out", outPKG,
	)
	if code != 0 {
		t.Fatalf("pkg build failed: %s", err)
	}
	if !strings.Contains(out, "Successfully compiled") {
		t.Errorf("unexpected output: %s", out)
	}
	if _, statErr := os.Stat(outPKG); statErr != nil {
		t.Fatalf("out pkg not found: %v", statErr)
	}
}
