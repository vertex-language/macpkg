package cpio

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
)

// Magic is the 6-byte SVR4 portable cpio new ASCII format identifier.
const Magic = "070701"

// FileEntry represents a file or directory in a cpio stream.
type FileEntry struct {
	Name string
	Mode uint32 // e.g. 0100755 for regular file, 0040755 for directory
	UID  uint32
	GID  uint32
	Data []byte
}

// Archive builds an uncompressed cpio archive stream.
func Archive(files []FileEntry) ([]byte, error) {
	var buf bytes.Buffer

	for i, f := range files {
		if err := writeFileEntry(&buf, uint32(i+1), f); err != nil {
			return nil, err
		}
	}

	// Write TRAILER!!! entry
	trailer := FileEntry{
		Name: "TRAILER!!!",
		Mode: 0,
	}
	if err := writeFileEntry(&buf, 0, trailer); err != nil {
		return nil, err
	}

	// Pad final archive to 512-byte boundary
	pad := (512 - (buf.Len() % 512)) % 512
	buf.Write(make([]byte, pad))

	return buf.Bytes(), nil
}

// ArchiveGz builds a gzip-compressed cpio archive stream (standard for macOS .pkg Payload).
func ArchiveGz(files []FileEntry) ([]byte, error) {
	cpioRaw, err := Archive(files)
	if err != nil {
		return nil, err
	}

	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	if _, err := gw.Write(cpioRaw); err != nil {
		return nil, fmt.Errorf("gzip cpio: %w", err)
	}
	if err := gw.Close(); err != nil {
		return nil, fmt.Errorf("close gzip: %w", err)
	}

	return gzBuf.Bytes(), nil
}

func writeFileEntry(w *bytes.Buffer, ino uint32, f FileEntry) error {
	nameWithNull := f.Name + "\x00"
	namesize := uint32(len(nameWithNull))
	filesize := uint32(len(f.Data))

	// 110-byte ASCII header
	header := fmt.Sprintf(
		"%s%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x%08x",
		Magic,
		ino,
		f.Mode,
		f.UID,
		f.GID,
		1, // nlink
		1600000000, // mtime
		filesize,
		0, 0, 0, 0, // dev major/minor, rdev major/minor
		namesize,
		0, // check
	)

	if len(header) != 110 {
		return fmt.Errorf("unexpected header length: %d", len(header))
	}

	w.WriteString(header)
	w.WriteString(nameWithNull)

	// Pad path to 4-byte boundary
	totalHeader := 110 + len(nameWithNull)
	if pad := (4 - (totalHeader % 4)) % 4; pad > 0 {
		w.Write(make([]byte, pad))
	}

	if filesize > 0 {
		w.Write(f.Data)
		// Pad data to 4-byte boundary
		if pad := (4 - (filesize % 4)) % 4; pad > 0 {
			w.Write(make([]byte, pad))
		}
	}

	return nil
}

// ExtractGz decompresses a gzip cpio stream and returns files.
func ExtractGz(data []byte) ([]FileEntry, error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer gr.Close()

	raw, err := io.ReadAll(gr)
	if err != nil {
		return nil, fmt.Errorf("read gzip: %w", err)
	}

	return Extract(raw)
}

// Extract parses an uncompressed cpio stream.
func Extract(data []byte) ([]FileEntry, error) {
	var entries []FileEntry
	offset := 0

	for offset+110 <= len(data) {
		magic := string(data[offset : offset+6])
		if magic != Magic {
			break
		}

		var namesize, filesize uint32
		_, _ = fmt.Sscanf(string(data[offset+94:offset+102]), "%x", &namesize)
		_, _ = fmt.Sscanf(string(data[offset+54:offset+62]), "%x", &filesize)

		offset += 110
		if offset+int(namesize) > len(data) {
			break
		}

		name := string(data[offset : offset+int(namesize)-1]) // strip NUL
		offset += int(namesize)

		// Align to 4 bytes
		if pad := (4 - ((110 + int(namesize)) % 4)) % 4; pad > 0 {
			offset += pad
		}

		if name == "TRAILER!!!" {
			break
		}

		var fileData []byte
		if filesize > 0 {
			if offset+int(filesize) > len(data) {
				break
			}
			fileData = make([]byte, filesize)
			copy(fileData, data[offset:offset+int(filesize)])
			offset += int(filesize)
			if pad := (4 - (int(filesize) % 4)) % 4; pad > 0 {
				offset += pad
			}
		}

		entries = append(entries, FileEntry{
			Name: name,
			Data: fileData,
		})
	}

	return entries, nil
}
