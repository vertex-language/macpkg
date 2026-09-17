package xar_test

import (
	"bytes"
	"testing"

	"github.com/vertex-language/macpkg/pkg/xar"
)

func TestXAR_RoundTrip(t *testing.T) {
	files := []xar.FileEntry{
		{Name: "PackageInfo", Data: []byte("<pkg-info/>")},
		{Name: "Payload", Data: []byte("FAKE GZIPPED CPIO DATA")},
		{Name: "Bom", Data: []byte("BOMStore\x00FAKE BOM")},
	}

	archiveBytes, err := xar.Archive(files)
	if err != nil {
		t.Fatalf("xar.Archive: %v", err)
	}

	if len(archiveBytes) < 28 {
		t.Fatalf("archive too short: %d", len(archiveBytes))
	}

	extracted, err := xar.Extract(archiveBytes)
	if err != nil {
		t.Fatalf("xar.Extract: %v", err)
	}

	if len(extracted) != 3 {
		t.Errorf("expected 3 files, got %d", len(extracted))
	}

	for _, f := range files {
		data, ok := extracted[f.Name]
		if !ok {
			t.Errorf("missing file %s in extracted", f.Name)
			continue
		}
		if !bytes.Equal(data, f.Data) {
			t.Errorf("content mismatch for %s", f.Name)
		}
	}
}
