package bom_test

import (
	"testing"

	"github.com/vertex-language/macpkg/pkg/bom"
)

func TestGenerateBOM(t *testing.T) {
	entries := []bom.FileEntry{
		{Path: "./Applications", Mode: 0755, UID: 0, GID: 80, Size: 4096},
		{Path: "./Applications/App.app", Mode: 0755, UID: 0, GID: 80, Size: 4096},
		{Path: "./Applications/App.app/Contents/MacOS/app", Mode: 0755, UID: 0, GID: 80, Size: 10240},
	}

	data, err := bom.Generate(entries)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if !bom.Validate(data) {
		t.Fatalf("invalid BOM signature")
	}
}
