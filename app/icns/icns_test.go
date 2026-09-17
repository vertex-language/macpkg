package icns_test

import (
	"bytes"
	"testing"

	"github.com/vertex-language/macpkg/app/icns"
)

func TestIconEncodeDecode(t *testing.T) {
	fakePNG := []byte("\x89PNG\r\n\x1a\nfake-png-data")
	ico := icns.FromPNG(fakePNG)

	data, err := ico.Bytes()
	if err != nil {
		t.Fatalf("ico.Bytes(): %v", err)
	}

	if len(data) < 8 {
		t.Fatalf("encoded icns too short: %d", len(data))
	}

	decoded, err := icns.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("icns.Decode: %v", err)
	}

	if len(decoded.Chunks) != 4 {
		t.Errorf("expected 4 chunks, got %d", len(decoded.Chunks))
	}

	for _, c := range decoded.Chunks {
		if !bytes.Equal(c.Data, fakePNG) {
			t.Errorf("chunk %s data mismatch", c.Type)
		}
	}
}
