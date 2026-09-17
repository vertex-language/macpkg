package xar

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
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
	ChecksumAlgorithm = uint32(2) // 2 = SHA-256
)

// FileEntry represents a file in the XAR archive.
type FileEntry struct {
	Name string
	Data []byte
}

// TOC represents the XAR XML Table of Contents.
type TOC struct {
	XMLName      xml.Name  `xml:"xar"`
	TOC          TOCInner  `xml:"toc"`
}

type TOCInner struct {
	CreationDate string    `xml:"creation-time"`
	Checksum     Checksum  `xml:"checksum"`
	Files        []TOCFile `xml:"file"`
}

type Checksum struct {
	Style  string `xml:"style,attr"`
	Offset uint64 `xml:"offset"`
	Size   uint64 `xml:"size"`
}

type TOCFile struct {
	ID   string  `xml:"id,attr"`
	Name string  `xml:"name"`
	Type string  `xml:"type"`
	Data TOCData `xml:"data"`
}

type TOCData struct {
	Length            uint64 `xml:"length"`
	Offset            uint64 `xml:"offset"`
	Size              uint64 `xml:"size"`
	Encoding          *TOCEncoding `xml:"encoding,omitempty"`
	ArchivedChecksum  ChecksumStyle `xml:"archived-checksum"`
	ExtractedChecksum ChecksumStyle `xml:"extracted-checksum"`
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

	for i, f := range files {
		uncompressedSize := uint64(len(f.Data))
		extractedHash := sha256.Sum256(f.Data)

		offset := uint64(heap.Len())
		heap.Write(f.Data)
		archivedSize := uint64(len(f.Data))
		archivedHash := sha256.Sum256(f.Data)

		tocFiles = append(tocFiles, TOCFile{
			ID:   fmt.Sprintf("%d", i+1),
			Name: f.Name,
			Type: "file",
			Data: TOCData{
				Length: archivedSize,
				Offset: offset,
				Size:   uncompressedSize,
				ArchivedChecksum: ChecksumStyle{
					Style: "sha256",
					Value: hex.EncodeToString(archivedHash[:]),
				},
				ExtractedChecksum: ChecksumStyle{
					Style: "sha256",
					Value: hex.EncodeToString(extractedHash[:]),
				},
			},
		})
	}

	// Prepare XML TOC
	tocObj := TOC{
		TOC: TOCInner{
			CreationDate: time.Now().UTC().Format(time.RFC3339),
			Checksum: Checksum{
				Style:  "sha256",
				Offset: 0,
				Size:   32,
			},
			Files: tocFiles,
		},
	}

	xmlBytes, err := xml.MarshalIndent(tocObj, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal TOC XML: %w", err)
	}
	xmlHeader := []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	uncompressedTOC := append(xmlHeader, xmlBytes...)
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

	// Build 28-byte Header
	var out bytes.Buffer
	_ = binary.Write(&out, binary.BigEndian, Magic)
	_ = binary.Write(&out, binary.BigEndian, HeaderSize)
	_ = binary.Write(&out, binary.BigEndian, Version)
	_ = binary.Write(&out, binary.BigEndian, compressedLen)
	_ = binary.Write(&out, binary.BigEndian, uncompressedLen)
	_ = binary.Write(&out, binary.BigEndian, ChecksumAlgorithm)

	// Append compressed TOC + Heap
	out.Write(compressedTOC.Bytes())
	out.Write(heap.Bytes())

	return out.Bytes(), nil
}

// Extract extracts files from a XAR archive stream.
func Extract(data []byte) (map[string][]byte, error) {
	if len(data) < 28 {
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
			return nil, fmt.Errorf("file %s heap bounds overflow", f.Name)
		}
		result[f.Name] = heap[start:end]
	}

	return result, nil
}
