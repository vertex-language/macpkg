package xar

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"time"
)

// Magic and Header constants for XAR.
const (
	Magic             = uint32(0x78617221) // "xar!"
	HeaderSize        = uint16(28)
	Version           = uint16(1)
	ChecksumAlgorithm = uint32(1)  // 1 = SHA-1 (Apple standard for macOS flat .pkg)
	ChecksumSize      = uint64(20) // SHA-1 digest size in bytes
)

// FileEntry represents a file in the XAR archive.
type FileEntry struct {
	Name string
	Data []byte
}

// TOC represents the XAR XML Table of Contents.
type TOC struct {
	XMLName xml.Name `xml:"xar"`
	TOC     TOCInner `xml:"toc"`
}

type TOCInner struct {
	Checksum     Checksum  `xml:"checksum"`
	CreationDate string    `xml:"creation-time,omitempty"`
	Files        []TOCFile `xml:"file"`
}

type Checksum struct {
	Style  string `xml:"style,attr"`
	Size   uint64 `xml:"size"`
	Offset uint64 `xml:"offset"`
}

type TOCFile struct {
	ID   string  `xml:"id,attr"`
	Name string  `xml:"name"`
	Type string  `xml:"type"`
	Data TOCData `xml:"data"`
}

type TOCData struct {
	ArchivedChecksum  ChecksumStyle `xml:"archived-checksum"`
	ExtractedChecksum ChecksumStyle `xml:"extracted-checksum"`
	Size              uint64        `xml:"size"`
	Offset            uint64        `xml:"offset"`
	Encoding          TOCEncoding   `xml:"encoding"`
	Length            uint64        `xml:"length"`
}

type TOCEncoding struct {
	Style string `xml:"style,attr"`
}

type ChecksumStyle struct {
	Style string `xml:"style,attr"`
	Value string `xml:",chardata"`
}

// Archive builds a complete XAR binary stream from a list of files.
func Archive(files []FileEntry) ([]byte, error) {
	var heap bytes.Buffer
	var tocFiles []TOCFile

	// Heap offset 0..ChecksumSize is reserved for the compressed-TOC checksum.
	// File data starts at offset ChecksumSize (20 bytes for SHA-1).
	for i, f := range files {
		uncompressedSize := uint64(len(f.Data))
		fileHash := sha1.Sum(f.Data)
		hashHex := hex.EncodeToString(fileHash[:])

		offset := ChecksumSize + uint64(heap.Len())
		heap.Write(f.Data)
		archivedSize := uint64(len(f.Data))

		tocFiles = append(tocFiles, TOCFile{
			ID:   fmt.Sprintf("%d", i+1),
			Name: f.Name,
			Type: "file",
			Data: TOCData{
				ArchivedChecksum: ChecksumStyle{
					Style: "sha1",
					Value: hashHex,
				},
				ExtractedChecksum: ChecksumStyle{
					Style: "sha1",
					Value: hashHex,
				},
				Size:   uncompressedSize,
				Offset: offset,
				Encoding: TOCEncoding{
					Style: "application/octet-stream",
				},
				Length: archivedSize,
			},
		})
	}

	// Prepare XML TOC
	tocObj := TOC{
		TOC: TOCInner{
			Checksum: Checksum{
				Style:  "sha1",
				Size:   ChecksumSize,
				Offset: 0,
			},
			CreationDate: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
			Files:        tocFiles,
		},
	}

	xmlBytes, err := xml.MarshalIndent(tocObj, "", " ")
	if err != nil {
		return nil, fmt.Errorf("marshal TOC XML: %w", err)
	}
	xmlHeader := []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	uncompressedTOC := append(xmlHeader, xmlBytes...)
	uncompressedTOC = append(uncompressedTOC, '\n')
	uncompressedLen := uint64(len(uncompressedTOC))

	// Compress TOC with zlib
	var compressedTOC bytes.Buffer
	zw := zlib.NewWriter(&compressedTOC)
	if _, err := zw.Write(uncompressedTOC); err != nil {
		return nil, fmt.Errorf("compress TOC: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("close zlib TOC: %w", err)
	}
	compressedLen := uint64(compressedTOC.Len())

	// Compute checksum of compressed TOC
	tocChecksum := sha1.Sum(compressedTOC.Bytes())

	// Build 28-byte Header
	var out bytes.Buffer
	_ = binary.Write(&out, binary.BigEndian, Magic)
	_ = binary.Write(&out, binary.BigEndian, HeaderSize)
	_ = binary.Write(&out, binary.BigEndian, Version)
	_ = binary.Write(&out, binary.BigEndian, compressedLen)
	_ = binary.Write(&out, binary.BigEndian, uncompressedLen)
	_ = binary.Write(&out, binary.BigEndian, ChecksumAlgorithm)

	// Append compressed TOC + Heap (TOC checksum + file data)
	out.Write(compressedTOC.Bytes())
	out.Write(tocChecksum[:])
	out.Write(heap.Bytes())

	return out.Bytes(), nil
}

// Extract extracts files from a XAR archive stream.
func Extract(data []byte) (map[string][]byte, error) {
	if len(data) < int(HeaderSize) {
		return nil, errors.New("data too short for XAR header")
	}

	magic := binary.BigEndian.Uint32(data[0:4])
	if magic != Magic {
		return nil, fmt.Errorf("invalid XAR magic: 0x%08X", magic)
	}

	headerSize := binary.BigEndian.Uint16(data[4:6])
	compressedTOCLen := binary.BigEndian.Uint64(data[8:16])

	tocStart := int(headerSize)
	tocEnd := tocStart + int(compressedTOCLen)
	if tocEnd > len(data) {
		return nil, errors.New("TOC length out of bounds")
	}

	zr, err := zlib.NewReader(bytes.NewReader(data[tocStart:tocEnd]))
	if err != nil {
		return nil, fmt.Errorf("zlib reader: %w", err)
	}
	defer zr.Close()

	tocXML, err := io.ReadAll(zr)
	if err != nil {
		return nil, fmt.Errorf("read TOC XML: %w", err)
	}

	var toc TOC
	if err := xml.Unmarshal(tocXML, &toc); err != nil {
		return nil, fmt.Errorf("unmarshal TOC: %w", err)
	}

	heapStart := tocEnd
	heap := data[heapStart:]

	result := make(map[string][]byte)
	for _, f := range toc.TOC.Files {
		start := int(f.Data.Offset)
		end := start + int(f.Data.Length)
		if end > len(heap) {
			return nil, fmt.Errorf("file %s heap bounds overflow (offset=%d len=%d heap=%d)", f.Name, start, f.Data.Length, len(heap))
		}
		result[f.Name] = heap[start:end]
	}

	return result, nil
}
