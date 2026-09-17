package icns

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Magic is the 4-byte identifier at the start of an Apple Icon Image file.
const Magic = "icns"

// Common OSType chunk identifiers for embedded PNG images.
const (
	TypeIcon128x128   = "ic07" // 128x128 PNG
	TypeIcon256x256   = "ic08" // 256x256 PNG
	TypeIcon512x512   = "ic09" // 512x512 PNG
	TypeIcon1024x1024 = "ic10" // 1024x1024 or 512x512@2x Retina PNG
	TypeIcon16x16_2x  = "ic11" // 16x16@2x PNG
	TypeIcon32x32_2x  = "ic12" // 32x32@2x PNG
	TypeIcon128x128_2x = "ic13" // 128x128@2x PNG
	TypeIcon256x256_2x = "ic14" // 256x256@2x PNG
)

// Chunk represents a single OSType entry within an .icns file.
type Chunk struct {
	Type string // 4-byte OSType (e.g., "ic07")
	Data []byte // Raw PNG or icon bytes
}

// Icon represents an in-memory Apple Icon Image container.
type Icon struct {
	Chunks []Chunk
}

// New creates an empty Icon container.
func New() *Icon {
	return &Icon{}
}

// FromPNG creates an Icon container populated with modern PNG chunk entries.
func FromPNG(pngData []byte) *Icon {
	ico := New()
	// Populate standard modern OSTypes so macOS Finder picks it up at all resolutions
	types := []string{TypeIcon128x128, TypeIcon256x256, TypeIcon512x512, TypeIcon1024x1024}
	for _, t := range types {
		ico.AddChunk(t, pngData)
	}
	return ico
}

// AddChunk appends an OSType chunk to the icon.
func (ico *Icon) AddChunk(osType string, data []byte) {
	ico.Chunks = append(ico.Chunks, Chunk{
		Type: osType,
		Data: data,
	})
}

// Encode writes the .icns binary stream to w.
func (ico *Icon) Encode(w io.Writer) error {
	totalSize := uint32(8) // 4 bytes magic + 4 bytes total size
	for _, chunk := range ico.Chunks {
		totalSize += uint32(8 + len(chunk.Data))
	}

	// Write magic
	if _, err := w.Write([]byte(Magic)); err != nil {
		return err
	}

	// Write total size
	if err := binary.Write(w, binary.BigEndian, totalSize); err != nil {
		return err
	}

	// Write chunks
	for _, chunk := range ico.Chunks {
		if len(chunk.Type) != 4 {
			return fmt.Errorf("invalid chunk OSType length: %q", chunk.Type)
		}
		if _, err := w.Write([]byte(chunk.Type)); err != nil {
			return err
		}
		chunkLen := uint32(8 + len(chunk.Data))
		if err := binary.Write(w, binary.BigEndian, chunkLen); err != nil {
			return err
		}
		if _, err := w.Write(chunk.Data); err != nil {
			return err
		}
	}
	return nil
}

// Bytes returns the serialized .icns byte slice.
func (ico *Icon) Bytes() ([]byte, error) {
	var buf bytes.Buffer
	if err := ico.Encode(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Decode parses an .icns binary stream into an Icon.
func Decode(r io.Reader) (*Icon, error) {
	header := make([]byte, 8)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	if string(header[0:4]) != Magic {
		return nil, errors.New("invalid icns magic: expected 'icns'")
	}

	totalSize := binary.BigEndian.Uint32(header[4:8])
	if totalSize < 8 {
		return nil, fmt.Errorf("invalid icns total size: %d", totalSize)
	}

	remaining := int64(totalSize - 8)
	ico := New()

	for remaining > 0 {
		chunkHeader := make([]byte, 8)
		if _, err := io.ReadFull(r, chunkHeader); err != nil {
			return nil, fmt.Errorf("read chunk header: %w", err)
		}
		osType := string(chunkHeader[0:4])
		chunkSize := binary.BigEndian.Uint32(chunkHeader[4:8])
		if chunkSize < 8 {
			return nil, fmt.Errorf("invalid chunk size for %s: %d", osType, chunkSize)
		}

		dataLen := int64(chunkSize - 8)
		data := make([]byte, dataLen)
		if _, err := io.ReadFull(r, data); err != nil {
			return nil, fmt.Errorf("read chunk data for %s: %w", osType, err)
		}

		ico.AddChunk(osType, data)
		remaining -= int64(chunkSize)
	}

	return ico, nil
}
