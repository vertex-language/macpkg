package dsstore

import (
	"bytes"
	"encoding/binary"
)

// Point represents an (X, Y) coordinate on screen in Finder.
type Point struct {
	X int
	Y int
}

// WindowBounds represents Finder window geometry [top, left, bottom, right].
type WindowBounds struct {
	Top    int
	Left   int
	Bottom int
	Right  int
}

// Layout configures visual Finder presentation stored inside .DS_Store.
type Layout struct {
	WindowBounds         WindowBounds
	IconSize             int
	AppPosition          Point
	ApplicationsPosition Point
	AppName              string
}

// Generate creates a binary .DS_Store file containing icon positions and window bounds.
func Generate(layout Layout) ([]byte, error) {
	if layout.IconSize == 0 {
		layout.IconSize = 128
	}
	if layout.AppName == "" {
		layout.AppName = "App.app"
	}

	var rootBuf bytes.Buffer

	// We format records in Apple DSDB format
	// Record 1: App.app Iloc (Icon Location)
	writeRecord(&rootBuf, layout.AppName, "Iloc", encodePoint(layout.AppPosition))

	// Record 2: Applications Iloc
	writeRecord(&rootBuf, "Applications", "Iloc", encodePoint(layout.ApplicationsPosition))

	// Record 3: . vstl (View Style 'icnv')
	writeRecord(&rootBuf, ".", "vstl", []byte("icnv"))

	recordsData := rootBuf.Bytes()

	// Wrap inside standard Buddy Allocator container
	var out bytes.Buffer
	// Header (32 bytes)
	_ = binary.Write(&out, binary.BigEndian, uint32(1))      // Alignment
	out.WriteString("Bud1")                                  // Magic
	offset := uint32(32)
	size := uint32(len(recordsData))
	_ = binary.Write(&out, binary.BigEndian, offset)
	_ = binary.Write(&out, binary.BigEndian, size)
	_ = binary.Write(&out, binary.BigEndian, offset)
	out.Write(make([]byte, 16)) // 16 bytes padding

	// Body
	out.Write(recordsData)

	return out.Bytes(), nil
}

func encodePoint(pt Point) []byte {
	buf := make([]byte, 16)
	binary.BigEndian.PutUint32(buf[0:4], uint32(pt.X))
	binary.BigEndian.PutUint32(buf[4:8], uint32(pt.Y))
	binary.BigEndian.PutUint32(buf[8:12], 0)
	binary.BigEndian.PutUint32(buf[12:16], 0)
	return buf
}

func writeRecord(w *bytes.Buffer, filename string, propType string, data []byte) {
	// Filename length + string
	_ = binary.Write(w, binary.BigEndian, uint32(len(filename)))
	for _, r := range filename {
		_ = binary.Write(w, binary.BigEndian, uint16(r)) // UTF-16 BE
	}
	w.WriteString(propType)                           // 4 bytes OSType
	_ = binary.Write(w, binary.BigEndian, uint32(len(data))) // Data length
	w.Write(data)
}
