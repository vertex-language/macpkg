package udif

import (
	"bytes"
	"compress/zlib"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
)

// Magic constants for UDIF.
const (
	KolyMagic = "koly"
	MishMagic = "mish"

	SectorSize = 512
	ChunkSize  = 2048 * SectorSize // 1 MB chunks

	TypeZeroFill   = uint32(0x00000000)
	TypeRaw        = uint32(0x00000001)
	TypeIgnored    = uint32(0x00000002)
	TypeUDZO       = uint32(0x80000005) // zlib Deflate
	TypeTerminator = uint32(0xFFFFFFFF)
)

// Chunk represents a single block chunk in a mish block.
type Chunk struct {
	Type             uint32
	Comment          uint32
	SectorNumber     uint64
	SectorCount      uint64
	CompressedOffset uint64
	CompressedLength uint64
}

// CompressUDIF compresses raw filesystem bytes into a streaming UDIF (.dmg) payload.
func CompressUDIF(raw []byte, volumeName string) ([]byte, error) {
	var dataFork bytes.Buffer
	var chunks []Chunk

	totalSectors := (uint64(len(raw)) + SectorSize - 1) / SectorSize
	paddedLen := totalSectors * SectorSize
	paddedRaw := make([]byte, paddedLen)
	copy(paddedRaw, raw)

	curSector := uint64(0)
	for curSector < totalSectors {
		sectorsInChunk := uint64(ChunkSize / SectorSize)
		if curSector+sectorsInChunk > totalSectors {
			sectorsInChunk = totalSectors - curSector
		}

		startByte := curSector * SectorSize
		endByte := startByte + sectorsInChunk*SectorSize
		chunkData := paddedRaw[startByte:endByte]

		// Check if chunk is all zeros
		allZero := true
		for _, b := range chunkData {
			if b != 0 {
				allZero = false
				break
			}
		}

		offset := uint64(dataFork.Len())
		if allZero {
			chunks = append(chunks, Chunk{
				Type:             TypeIgnored,
				Comment:          0,
				SectorNumber:     curSector,
				SectorCount:      sectorsInChunk,
				CompressedOffset: offset,
				CompressedLength: 0,
			})
		} else {
			var zbuf bytes.Buffer
			zw := zlib.NewWriter(&zbuf)
			_, _ = zw.Write(chunkData)
			_ = zw.Close()
			compBytes := zbuf.Bytes()

			if len(compBytes) < len(chunkData) {
				dataFork.Write(compBytes)
				chunks = append(chunks, Chunk{
					Type:             TypeUDZO,
					Comment:          0,
					SectorNumber:     curSector,
					SectorCount:      sectorsInChunk,
					CompressedOffset: offset,
					CompressedLength: uint64(len(compBytes)),
				})
			} else {
				dataFork.Write(chunkData)
				chunks = append(chunks, Chunk{
					Type:             TypeRaw,
					Comment:          0,
					SectorNumber:     curSector,
					SectorCount:      sectorsInChunk,
					CompressedOffset: offset,
					CompressedLength: uint64(len(chunkData)),
				})
			}
		}

		curSector += sectorsInChunk
	}

	// Terminator chunk
	chunks = append(chunks, Chunk{
		Type:             TypeTerminator,
		Comment:          0,
		SectorNumber:     totalSectors,
		SectorCount:      0,
		CompressedOffset: uint64(dataFork.Len()),
		CompressedLength: 0,
	})

	// Build mish block
	mishData, err := buildMishBlock(totalSectors, chunks)
	if err != nil {
		return nil, fmt.Errorf("build mish block: %w", err)
	}

	// Build XML property list
	xmlPlist := buildXMLPlist(mishData, volumeName)

	// Combine data fork + XML plist
	var out bytes.Buffer
	out.Write(dataFork.Bytes())
	xmlOffset := uint64(out.Len())
	out.Write([]byte(xmlPlist))
	xmlLength := uint64(len(xmlPlist))

	// Build 512-byte koly trailer
	kolyBlock, err := buildKolyTrailer(uint64(dataFork.Len()), xmlOffset, xmlLength, totalSectors)
	if err != nil {
		return nil, fmt.Errorf("build koly trailer: %w", err)
	}
	out.Write(kolyBlock)

	return out.Bytes(), nil
}

func buildMishBlock(totalSectors uint64, chunks []Chunk) ([]byte, error) {
	var buf bytes.Buffer

	// Mish Header (204 bytes before chunks)
	buf.WriteString(MishMagic)                         // 4 bytes: 'mish'
	_ = binary.Write(&buf, binary.BigEndian, uint32(1)) // Version 1
	_ = binary.Write(&buf, binary.BigEndian, uint64(0)) // SectorNumber
	_ = binary.Write(&buf, binary.BigEndian, totalSectors)
	_ = binary.Write(&buf, binary.BigEndian, uint64(0))          // DataOffset
	_ = binary.Write(&buf, binary.BigEndian, uint32(2056))       // BuffersNeeded
	_ = binary.Write(&buf, binary.BigEndian, uint32(0xfffffffe)) // BlockDescriptors
	buf.Write(make([]byte, 24))                                  // Reserved 24 bytes

	// Checksum (136 bytes: type uint32, size uint32, data 128 bytes)
	_ = binary.Write(&buf, binary.BigEndian, uint32(0))
	_ = binary.Write(&buf, binary.BigEndian, uint32(0))
	buf.Write(make([]byte, 128))

	// Number of chunks
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(chunks)))

	// Chunks (40 bytes each)
	for _, c := range chunks {
		_ = binary.Write(&buf, binary.BigEndian, c.Type)
		_ = binary.Write(&buf, binary.BigEndian, c.Comment)
		_ = binary.Write(&buf, binary.BigEndian, c.SectorNumber)
		_ = binary.Write(&buf, binary.BigEndian, c.SectorCount)
		_ = binary.Write(&buf, binary.BigEndian, c.CompressedOffset)
		_ = binary.Write(&buf, binary.BigEndian, c.CompressedLength)
	}

	return buf.Bytes(), nil
}

func buildXMLPlist(mishData []byte, volumeName string) string {
	b64Mish := base64.StdEncoding.EncodeToString(mishData)
	dummyPlst := base64.StdEncoding.EncodeToString(make([]byte, 512))
	if volumeName == "" {
		volumeName = "whole disk"
	}
	partName := fmt.Sprintf("%s (Apple_HFS : 0)", volumeName)
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>resource-fork</key>
	<dict>
		<key>blkx</key>
		<array>
			<dict>
				<key>Attributes</key>
				<string>0x0050</string>
				<key>Data</key>
				<data>%s</data>
				<key>ID</key>
				<string>0</string>
				<key>Name</key>
				<string>%s</string>
			</dict>
		</array>
		<key>plst</key>
		<array>
			<dict>
				<key>Attributes</key>
				<string>0x0050</string>
				<key>Data</key>
				<data>%s</data>
				<key>ID</key>
				<string>0</string>
				<key>Name</key>
				<string></string>
			</dict>
		</array>
	</dict>
</dict>
</plist>
`, b64Mish, partName, dummyPlst)
}

func buildKolyTrailer(dataForkLen, xmlOffset, xmlLength, totalSectors uint64) ([]byte, error) {
	buf := make([]byte, 512)

	// Magic
	copy(buf[0:4], []byte(KolyMagic))
	// Version 4
	binary.BigEndian.PutUint32(buf[4:8], 4)
	// HeaderSize 512
	binary.BigEndian.PutUint32(buf[8:12], 512)
	// Flags 1
	binary.BigEndian.PutUint32(buf[12:16], 1)
	// RunningDataForkOffset
	binary.BigEndian.PutUint64(buf[16:24], 0)
	// DataForkOffset
	binary.BigEndian.PutUint64(buf[24:32], 0)
	// DataForkLength
	binary.BigEndian.PutUint64(buf[32:40], dataForkLen)
	// RsrcForkOffset & Length (0)
	binary.BigEndian.PutUint64(buf[40:48], 0)
	binary.BigEndian.PutUint64(buf[48:56], 0)
	// SegmentNumber & Count
	binary.BigEndian.PutUint32(buf[56:60], 1)
	binary.BigEndian.PutUint32(buf[60:64], 1)
	// SegmentID (16 bytes random)
	_, _ = io.ReadFull(rand.Reader, buf[64:80])

	// DataForkChecksum (0 = None)
	binary.BigEndian.PutUint32(buf[80:84], 0)
	binary.BigEndian.PutUint32(buf[84:88], 0)

	// XMLOffset & XMLLength
	binary.BigEndian.PutUint64(buf[216:224], xmlOffset)
	binary.BigEndian.PutUint64(buf[224:232], xmlLength)

	// Master Checksum (0 = None)
	binary.BigEndian.PutUint32(buf[352:356], 0)
	binary.BigEndian.PutUint32(buf[356:360], 0)

	// ImageVariant 2 = UDZO
	binary.BigEndian.PutUint32(buf[488:492], 2)
	// SectorCount
	binary.BigEndian.PutUint64(buf[492:500], totalSectors)

	return buf, nil
}
