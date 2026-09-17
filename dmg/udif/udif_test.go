package udif_test

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/vertex-language/macpkg/dmg/udif"
)

func TestCompressUDIF(t *testing.T) {
	rawFS := make([]byte, 1024*1024) // 1MB raw data
	copy(rawFS[1024:], []byte("HFS+ TEST VOLUME CONTENT"))

	dmgBytes, err := udif.CompressUDIF(rawFS, "TestVolume")
	if err != nil {
		t.Fatalf("CompressUDIF: %v", err)
	}

	if len(dmgBytes) < 512 {
		t.Fatalf("DMG too short: %d", len(dmgBytes))
	}

	// Verify koly trailer at the end
	trailer := dmgBytes[len(dmgBytes)-512:]
	if string(trailer[0:4]) != "koly" {
		t.Fatalf("expected koly magic, got %q", string(trailer[0:4]))
	}

	version := binary.BigEndian.Uint32(trailer[4:8])
	if version != 4 {
		t.Errorf("expected version 4, got %d", version)
	}

	xmlOffset := binary.BigEndian.Uint64(trailer[216:224])
	xmlLength := binary.BigEndian.Uint64(trailer[224:232])
	if xmlOffset+xmlLength > uint64(len(dmgBytes)-512) {
		t.Errorf("XML bounds out of range: offset=%d len=%d total=%d", xmlOffset, xmlLength, len(dmgBytes))
	}

	xmlData := string(dmgBytes[xmlOffset : xmlOffset+xmlLength])
	if !strings.Contains(xmlData, "TestVolume") {
		t.Errorf("XML missing volume name: %s", xmlData)
	}
	if !strings.Contains(xmlData, "<key>blkx</key>") {
		t.Errorf("XML missing blkx key: %s", xmlData)
	}
}
