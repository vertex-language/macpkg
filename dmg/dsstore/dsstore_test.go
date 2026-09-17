package dsstore_test

import (
	"testing"

	"github.com/vertex-language/macpkg/dmg/dsstore"
)

func TestGenerateDSStore(t *testing.T) {
	data, err := dsstore.Generate(dsstore.Layout{
		AppName: "Test.app",
		AppPosition: dsstore.Point{X: 100, Y: 150},
		ApplicationsPosition: dsstore.Point{X: 300, Y: 150},
		IconSize: 128,
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if len(data) < 32 {
		t.Fatalf("data too short: %d", len(data))
	}

	if string(data[4:8]) != "Bud1" {
		t.Errorf("expected Bud1 magic, got %s", string(data[4:8]))
	}
}
