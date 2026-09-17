package cpio_test

import (
	"bytes"
	"testing"

	"github.com/vertex-language/macpkg/pkg/cpio"
)

func TestCpio_RoundTripGz(t *testing.T) {
	orig := []cpio.FileEntry{
		{Name: "Applications", Mode: 0040755, UID: 0, GID: 80},
		{Name: "Applications/App.app", Mode: 0040755, UID: 0, GID: 80},
		{Name: "Applications/App.app/Contents/MacOS/app", Mode: 0100755, UID: 0, GID: 80, Data: []byte("BINARY EXECUTABLE CONTENT")},
	}

	gzData, err := cpio.ArchiveGz(orig)
	if err != nil {
		t.Fatalf("ArchiveGz: %v", err)
	}

	extracted, err := cpio.ExtractGz(gzData)
	if err != nil {
		t.Fatalf("ExtractGz: %v", err)
	}

	if len(extracted) != len(orig) {
		t.Fatalf("expected %d entries, got %d", len(orig), len(extracted))
	}

	for i, o := range orig {
		if extracted[i].Name != o.Name {
			t.Errorf("expected name %s, got %s", o.Name, extracted[i].Name)
		}
		if !bytes.Equal(extracted[i].Data, o.Data) {
			t.Errorf("data mismatch for %s", o.Name)
		}
	}
}
