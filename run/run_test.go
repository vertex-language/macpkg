package run

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"
)

func TestMockRunner(t *testing.T) {
	ctx := context.Background()
	mock := NewMock()

	mock.SetHandler("signtool", func(args []string, stdout, stderr io.Writer) error {
		stdout.Write([]byte("Successfully signed: package.msi\n"))
		return nil
	})

	var out, errOut bytes.Buffer
	err := mock.Run(ctx, "signtool", []string{"sign", "/f", "cert.pfx", "package.msi"}, "/tmp", []string{"ENV=1"}, &out, &errOut)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mock.Invocations) != 1 {
		t.Fatalf("expected 1 invocation, got %d", len(mock.Invocations))
	}
	inv := mock.Invocations[0]
	if inv.Cmd != "signtool" {
		t.Errorf("cmd mismatch: %s", inv.Cmd)
	}
	if inv.Dir != "/tmp" {
		t.Errorf("dir mismatch: %s", inv.Dir)
	}
	if !strings.Contains(out.String(), "Successfully signed") {
		t.Errorf("expected stdout message, got %q", out.String())
	}

	// Test failing handler
	mock.SetHandler("fail", func(args []string, stdout, stderr io.Writer) error {
		return errors.New("simulated failure")
	})
	if err := mock.Run(ctx, "fail", nil, "", nil, &out, &errOut); err == nil {
		t.Errorf("expected error from failing handler, got nil")
	}
}

func TestOSRunner(t *testing.T) {
	ctx := context.Background()
	runner := OS()

	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go binary not found in PATH")
	}

	var out, errOut bytes.Buffer
	err = runner.Run(ctx, goBin, []string{"version"}, "", nil, &out, &errOut)
	if err != nil {
		t.Fatalf("run go version failed: %v", err)
	}

	if !strings.Contains(out.String(), "go version") {
		t.Errorf("expected go version output, got %q", out.String())
	}
}
