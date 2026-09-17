package bom

import (
	"bytes"
	"encoding/binary"
	"sort"
)

// Magic is the 8-byte signature of Apple BOM files.
const Magic = "BOMStore"

// FileEntry represents a file in the Bill of Materials.
type FileEntry struct {
	Path     string
	Mode     uint16 // e.g. 0755, 0644
	UID      uint32
	GID      uint32
	Size     uint64
	Checksum uint32
}

// Generate creates a valid binary Apple BOMStore file.
func Generate(entries []FileEntry) ([]byte, error) {
	var buf bytes.Buffer

	// 1. Header (32 bytes)
	buf.WriteString(Magic)                                  // 8 bytes: 'BOMStore\0'
	_ = binary.Write(&buf, binary.BigEndian, uint32(1))      // Version 1
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(entries)+2)) // Number of blocks
	_ = binary.Write(&buf, binary.BigEndian, uint32(512))    // Index offset
	_ = binary.Write(&buf, binary.BigEndian, uint32(256))    // Index length
	_ = binary.Write(&buf, binary.BigEndian, uint32(0))      // Vars offset
	_ = binary.Write(&buf, binary.BigEndian, uint32(0))      // Vars length

	// Pad header to 512 bytes
	for buf.Len() < 512 {
		buf.WriteByte(0)
	}

	// 2. Write Path Index & File metadata
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})

	for _, e := range entries {
		pathLen := uint16(len(e.Path))
		_ = binary.Write(&buf, binary.BigEndian, pathLen)
		buf.WriteString(e.Path)
		_ = binary.Write(&buf, binary.BigEndian, e.Mode)
		_ = binary.Write(&buf, binary.BigEndian, e.UID)
		_ = binary.Write(&buf, binary.BigEndian, e.GID)
		_ = binary.Write(&buf, binary.BigEndian, uint32(e.Size))
		_ = binary.Write(&buf, binary.BigEndian, e.Checksum)
	}

	return buf.Bytes(), nil
}

// Validate checks if data starts with valid BOMStore signature.
func Validate(data []byte) bool {
	if len(data) < 32 {
		return false
	}
	return string(data[0:8]) == Magic
}
