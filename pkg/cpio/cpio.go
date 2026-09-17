package cpio

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
)

// Magic is the 6-byte POSIX odc (old character) format identifier used by macOS installer packages.
const Magic = "070707"

// MagicNewASCII is the 6-byte SVR4 portable cpio format identifier.
const MagicNewASCII = "070701"

// FileEntry represents a file or directory in a cpio stream.
type FileEntry struct {
	Name string
	Mode uint32 // e.g. 0100755 for regular file, 0040755 for directory
	UID  uint32
	GID  uint32
	Data []byte
}

// Archive builds an uncompressed odc cpio archive stream (macOS standard).
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
	if err := writeFileEntry(&buf, uint32(len(files)+1), trailer); err != nil {
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

	nlink := uint32(1)
	if f.Mode&0040000 != 0 {
		nlink = 2
	}

	// 76-byte ASCII header for POSIX odc format:
	// dev(6), ino(6), mode(6), uid(6), gid(6), nlink(6), rdev(6), mtime(11), namesize(6), filesize(11)
	header := fmt.Sprintf(
		"070707%06o%06o%06o%06o%06o%06o%06o%011o%06o%011o",
		0,
		ino,
		f.Mode,
		f.UID,
		f.GID,
		nlink,
		0,
		0,
		namesize,
		filesize,
	)

	if len(header) != 76 {
		return fmt.Errorf("unexpected odc header length: %d", len(header))
	}

	w.WriteString(header)
	w.WriteString(nameWithNull)
	if filesize > 0 {
		w.Write(f.Data)
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

// Extract parses an uncompressed cpio stream (supporting both odc and new ASCII formats).
func Extract(data []byte) ([]FileEntry, error) {
	var entries []FileEntry
	offset := 0

	for offset+76 <= len(data) {
		magic := string(data[offset : offset+6])

		if magic == Magic {
			// POSIX odc (76-byte header)
			var namesize, filesize uint32
			_, _ = fmt.Sscanf(string(data[offset+59:offset+65]), "%o", &namesize)
			_, _ = fmt.Sscanf(string(data[offset+65:offset+76]), "%o", &filesize)

			offset += 76
			if offset+int(namesize) > len(data) {
				break
			}

			name := string(data[offset : offset+int(namesize)-1]) // strip NUL
			offset += int(namesize)

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
			}

			entries = append(entries, FileEntry{
				Name: name,
				Data: fileData,
			})
		} else if magic == MagicNewASCII {
			// SVR4 new ASCII (110-byte header with 4-byte padding)
			if offset+110 > len(data) {
				break
			}
			var namesize, filesize uint32
			_, _ = fmt.Sscanf(string(data[offset+94:offset+102]), "%x", &namesize)
			_, _ = fmt.Sscanf(string(data[offset+54:offset+62]), "%x", &filesize)

			offset += 110
			if offset+int(namesize) > len(data) {
				break
			}

			name := string(data[offset : offset+int(namesize)-1])
			offset += int(namesize)
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
		} else {
			break
		}
	}

	return entries, nil
}
