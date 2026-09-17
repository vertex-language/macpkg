package hfs

import (
	"encoding/binary"
	"time"
)

// HFS+ Constants
const (
	SignatureHFSPlus = uint16(0x482B) // 'H+'
	VersionHFSPlus   = uint16(4)
	BlockSize        = uint32(4096)
	SectorSize       = uint32(512)

	// HFS+ epoch begins at Jan 1, 1904 00:00:00 UTC
	// Difference between Unix epoch (1970) and HFS epoch (1904) in seconds:
	HFSEpochOffset = uint32(2082844800)
)

// FileEntry represents a file or directory to insert into the HFS+ volume.
type FileEntry struct {
	Path     string
	Data     []byte
	IsDir    bool
	IsSymlink bool
	LinkTarget string
}

// VolumeConfig defines volume parameters.
type VolumeConfig struct {
	Name    string
	SizeMB  int
	Entries []FileEntry
}

// GenerateVolume builds a raw HFS+ volume image.
func GenerateVolume(cfg VolumeConfig) ([]byte, error) {
	sizeMB := cfg.SizeMB
	if sizeMB < 5 {
		sizeMB = 5 // Minimum 5 MB for standard HFS+ volume
	}

	totalBytes := uint64(sizeMB) * 1024 * 1024
	totalBlocks := uint32(totalBytes / uint64(BlockSize))
	raw := make([]byte, totalBytes)

	now := uint32(time.Now().Unix()) + HFSEpochOffset

	// 1. Write Volume Header at byte 1024 (Sector 2)
	vhOffset := 1024
	writeVolumeHeader(raw[vhOffset:], cfg.Name, totalBlocks, now)

	// 2. Write Alternate Volume Header at EOF - 1024 (2 sectors before EOF)
	altOffset := int(totalBytes) - 1024
	if altOffset > vhOffset {
		writeVolumeHeader(raw[altOffset:], cfg.Name, totalBlocks, now)
	}

	// 3. Write allocation file (Block 1)
	allocOffset := int(BlockSize)
	if allocOffset+4096 <= len(raw) {
		// Mark system blocks as used
		raw[allocOffset] = 0xFF
		raw[allocOffset+1] = 0x03
	}

	return raw, nil
}

func writeVolumeHeader(dst []byte, name string, totalBlocks uint32, now uint32) {
	// Signature (2 bytes: 'H+')
	binary.BigEndian.PutUint16(dst[0:2], SignatureHFSPlus)
	// Version (2 bytes: 4)
	binary.BigEndian.PutUint16(dst[2:4], VersionHFSPlus)
	// Attributes (4 bytes: 0x80002000 unmounted)
	binary.BigEndian.PutUint32(dst[4:8], 0x80002000)
	// LastMountedVersion (4 bytes: '10.0')
	binary.BigEndian.PutUint32(dst[8:12], 0x31302E30)
	// Dates
	binary.BigEndian.PutUint32(dst[16:20], now) // createDate
	binary.BigEndian.PutUint32(dst[20:24], now) // modifyDate
	binary.BigEndian.PutUint32(dst[24:28], now) // backupDate
	binary.BigEndian.PutUint32(dst[28:32], now) // checkedDate

	// File & Folder counts
	binary.BigEndian.PutUint32(dst[32:36], 1)
	binary.BigEndian.PutUint32(dst[36:40], 1)

	// Block size & total blocks
	binary.BigEndian.PutUint32(dst[40:44], BlockSize)
	binary.BigEndian.PutUint32(dst[44:48], totalBlocks)
	binary.BigEndian.PutUint32(dst[48:52], totalBlocks-100) // free blocks

	binary.BigEndian.PutUint32(dst[52:56], 100) // nextAllocation
	binary.BigEndian.PutUint32(dst[56:60], 4096) // rsrcClumpSize
	binary.BigEndian.PutUint32(dst[60:64], 4096) // dataClumpSize
	binary.BigEndian.PutUint32(dst[64:68], 1000) // nextCatalogID

	// Allocation file fork data (at offset 112)
	writeForkData(dst[112:], 4096, 1, 1)

	// Catalog file fork data (at offset 272)
	writeForkData(dst[272:], 4096, 2, 1)
}

func writeForkData(dst []byte, logicalSize uint64, startBlock, blockCount uint32) {
	binary.BigEndian.PutUint64(dst[0:8], logicalSize)
	binary.BigEndian.PutUint32(dst[8:12], 4096)
	binary.BigEndian.PutUint32(dst[12:16], blockCount)
	// First extent descriptor (startBlock, blockCount)
	binary.BigEndian.PutUint32(dst[16:20], startBlock)
	binary.BigEndian.PutUint32(dst[20:24], blockCount)
}

// ValidateHeader checks if data starts with a valid HFS+ Volume Header.
func ValidateHeader(data []byte) bool {
	if len(data) < 1024+512 {
		return false
	}
	sig := binary.BigEndian.Uint16(data[1024 : 1024+2])
	return sig == SignatureHFSPlus
}
