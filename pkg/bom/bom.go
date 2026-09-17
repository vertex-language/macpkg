package bom

import (
	"bytes"
	"encoding/binary"
	"path/filepath"
	"sort"
	"strings"
)

// Magic is the 8-byte signature of Apple BOM files.
const Magic = "BOMStore"

// FileEntry represents a file in the Bill of Materials.
type FileEntry struct {
	Path     string
	Mode     uint16 // e.g. 0755, 0644, or full st_mode (0100755, 0100644)
	UID      uint32
	GID      uint32
	Size     uint64
	Checksum uint32
	Data     []byte // optional, used to calculate size and checksum if not provided
}

// bomChecksumTable is the CRC-32 table for polynomial 0x04C11DB7 (MSB-first),
// used by the POSIX cksum algorithm stored by Apple mkbom.
var bomChecksumTable = func() [256]uint32 {
	var t [256]uint32
	for i := range t {
		c := uint32(i) << 24
		for range 8 {
			if c&0x80000000 != 0 {
				c = (c << 1) ^ 0x04C11DB7
			} else {
				c <<= 1
			}
		}
		t[i] = c
	}
	return t
}()

// Checksum computes the POSIX cksum (CRC-32/CKSUM) of data: the CRC-32 over
// the data followed by the little-endian minimal-byte encoding of its length,
// finally inverted. This matches the checksum Apple's mkbom records per file.
func Checksum(data []byte) uint32 {
	var crc uint32
	for _, b := range data {
		crc = (crc << 8) ^ bomChecksumTable[byte(crc>>24)^b]
	}
	for n := len(data); n != 0; n >>= 8 {
		crc = (crc << 8) ^ bomChecksumTable[byte(crc>>24)^byte(n)]
	}
	return ^crc
}

// Validate checks if data starts with valid BOMStore signature.
func Validate(data []byte) bool {
	if len(data) < 32 {
		return false
	}
	return string(data[0:8]) == Magic
}

type bomPath struct {
	id       uint32
	parentID uint32
	name     string
	isDir    bool
	mode     uint16
	size     uint32
	checksum uint32
}

const (
	bomInfoBlock      = 1
	bomPathsTree      = 2
	bomPathsLeaf      = 3
	bomHLIndexTree    = 4
	bomHLIndexLeaf    = 5
	bomVIndexBlock    = 6
	bomVIndexTree     = 7
	bomVIndexLeaf     = 8
	bomSize64Tree     = 9
	bomSize64Leaf     = 10
	bomFirstPathBlock = 11
)

type node struct {
	name     string
	isDir    bool
	mode     uint16
	size     uint32
	checksum uint32
	children map[string]*node
}

// Generate creates a valid binary Apple BOMStore file from file entries.
func Generate(entries []FileEntry) ([]byte, error) {
	root := collectNodes(entries)
	paths := flattenTree(root)
	return buildBom(paths)
}

func collectNodes(entries []FileEntry) *node {
	root := &node{name: ".", isDir: true, mode: 040755, children: make(map[string]*node)}

	for _, e := range entries {
		clean := filepath.Clean(e.Path)
		clean = strings.TrimPrefix(clean, "./")
		clean = strings.TrimPrefix(clean, "/")
		if clean == "" || clean == "." {
			continue
		}

		parts := strings.Split(clean, "/")
		curr := root
		for i, part := range parts {
			isLeaf := (i == len(parts)-1)
			if isLeaf && (e.Mode&040000 == 0) {
				cksum := e.Checksum
				if cksum == 0 && len(e.Data) > 0 {
					cksum = Checksum(e.Data)
				}
				sz := e.Size
				if sz == 0 && len(e.Data) > 0 {
					sz = uint64(len(e.Data))
				}
				mode := e.Mode
				if mode < 0100000 {
					mode |= 0100000
				}
				curr.children[part] = &node{
					name:     part,
					isDir:    false,
					mode:     mode,
					size:     uint32(sz),
					checksum: cksum,
				}
			} else {
				dirNode, ok := curr.children[part]
				if !ok {
					dirNode = &node{
						name:     part,
						isDir:    true,
						mode:     040755,
						children: make(map[string]*node),
					}
					curr.children[part] = dirNode
				}
				curr = dirNode
			}
		}
	}
	return root
}

func flattenTree(root *node) []*bomPath {
	var out []*bomPath
	var nextID uint32 = 1

	rootPath := &bomPath{
		id:       nextID,
		parentID: 0,
		name:     ".",
		isDir:    true,
		mode:     root.mode,
	}
	nextID++
	out = append(out, rootPath)

	var walk func(curr *node, parentID uint32)
	walk = func(curr *node, parentID uint32) {
		names := make([]string, 0, len(curr.children))
		for k := range curr.children {
			names = append(names, k)
		}
		sort.Strings(names)

		for _, name := range names {
			child := curr.children[name]
			p := &bomPath{
				id:       nextID,
				parentID: parentID,
				name:     name,
				isDir:    child.isDir,
				mode:     child.mode,
				size:     child.size,
				checksum: child.checksum,
			}
			nextID++
			out = append(out, p)

			if child.isDir {
				walk(child, p.id)
			}
		}
	}

	walk(root, rootPath.id)
	return out
}

func buildBom(paths []*bomPath) ([]byte, error) {
	n := len(paths)
	blocks := make([][]byte, bomFirstPathBlock+3*n)

	type leafEntry struct {
		parentID       uint32
		name           string
		pi1Idx, fileID uint32
	}
	entries := make([]leafEntry, 0, n)
	for k, p := range paths {
		pi2Idx := bomFirstPathBlock + 3*k
		fileIdx := pi2Idx + 1
		pi1Idx := pi2Idx + 2

		blocks[pi2Idx] = buildBomPathInfo2(p)
		blocks[fileIdx] = buildBomFile(p)
		blocks[pi1Idx] = buildBomPathInfo1(p.id, uint32(pi2Idx))

		entries = append(entries, leafEntry{p.parentID, p.name, uint32(pi1Idx), uint32(fileIdx)})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].parentID != entries[j].parentID {
			return entries[i].parentID < entries[j].parentID
		}
		return entries[i].name < entries[j].name
	})
	leafPairs := make([][2]uint32, len(entries))
	for i, e := range entries {
		leafPairs[i] = [2]uint32{e.pi1Idx, e.fileID}
	}

	blocks[bomInfoBlock] = buildBomInfo(uint32(n) + 1)
	blocks[bomPathsTree] = buildBomTree(bomPathsLeaf, uint32(n), 4096)
	blocks[bomPathsLeaf] = buildBomLeaf(leafPairs)
	blocks[bomHLIndexTree] = buildBomTree(bomHLIndexLeaf, 0, 4096)
	blocks[bomHLIndexLeaf] = buildBomLeaf(nil)
	blocks[bomVIndexBlock] = buildBomVIndex(bomVIndexTree)
	blocks[bomVIndexTree] = buildBomTree(bomVIndexLeaf, 0, 128)
	blocks[bomVIndexLeaf] = buildBomLeaf(nil)
	blocks[bomSize64Tree] = buildBomTree(bomSize64Leaf, 0, 4096)
	blocks[bomSize64Leaf] = buildBomLeaf(nil)

	addrs := make([]uint32, len(blocks))
	var body bytes.Buffer
	cursor := uint32(32)
	for i := 1; i < len(blocks); i++ {
		addrs[i] = cursor
		body.Write(blocks[i])
		cursor += uint32(len(blocks[i]))
	}

	vars := buildBomVars()
	varsOffset := cursor
	cursor += uint32(len(vars))

	index := buildBomIndex(blocks, addrs)
	indexOffset := cursor

	var out bytes.Buffer
	out.WriteString(Magic)
	be := binary.BigEndian
	writeU32 := func(v uint32) { _ = binary.Write(&out, be, v) }
	writeU32(1)
	writeU32(uint32(len(blocks) - 1))
	writeU32(indexOffset)
	writeU32(uint32(len(index)))
	writeU32(varsOffset)
	writeU32(uint32(len(vars)))
	out.Write(body.Bytes())
	out.Write(vars)
	out.Write(index)
	return out.Bytes(), nil
}

func buildBomPathInfo2(p *bomPath) []byte {
	var b bytes.Buffer
	be := binary.BigEndian
	typ := byte(1)
	if p.isDir {
		typ = 2
	}
	b.WriteByte(typ)
	b.WriteByte(1)
	_ = binary.Write(&b, be, uint16(3))
	_ = binary.Write(&b, be, p.mode)
	_ = binary.Write(&b, be, uint32(0))  // uid = 0 (root)
	_ = binary.Write(&b, be, uint32(80)) // gid = 80 (admin)
	_ = binary.Write(&b, be, uint32(0))  // mtime = 0
	_ = binary.Write(&b, be, p.size)
	b.WriteByte(1)
	_ = binary.Write(&b, be, p.checksum)
	_ = binary.Write(&b, be, uint32(0))
	if !p.isDir {
		_ = binary.Write(&b, be, uint32(0))
	}
	return b.Bytes()
}

func buildBomFile(p *bomPath) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.BigEndian, p.parentID)
	b.WriteString(p.name)
	b.WriteByte(0)
	return b.Bytes()
}

func buildBomPathInfo1(id, pathInfo2Block uint32) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.BigEndian, id)
	_ = binary.Write(&b, binary.BigEndian, pathInfo2Block)
	return b.Bytes()
}

func buildBomTree(childBlock, pathCount, blockSize uint32) []byte {
	var b bytes.Buffer
	be := binary.BigEndian
	b.WriteString("tree")
	_ = binary.Write(&b, be, uint32(1))
	_ = binary.Write(&b, be, childBlock)
	_ = binary.Write(&b, be, blockSize)
	_ = binary.Write(&b, be, pathCount)
	b.WriteByte(0)
	return b.Bytes()
}

func buildBomLeaf(pairs [][2]uint32) []byte {
	var b bytes.Buffer
	be := binary.BigEndian
	_ = binary.Write(&b, be, uint16(1))
	_ = binary.Write(&b, be, uint16(len(pairs)))
	_ = binary.Write(&b, be, uint32(0))
	_ = binary.Write(&b, be, uint32(0))
	for _, pr := range pairs {
		_ = binary.Write(&b, be, pr[0])
		_ = binary.Write(&b, be, pr[1])
	}
	return b.Bytes()
}

func buildBomVIndex(treeBlock uint32) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.BigEndian, uint32(1))
	_ = binary.Write(&b, binary.BigEndian, treeBlock)
	b.WriteByte(0)
	return b.Bytes()
}

func buildBomInfo(numPaths uint32) []byte {
	var b bytes.Buffer
	be := binary.BigEndian
	_ = binary.Write(&b, be, uint32(1))
	_ = binary.Write(&b, be, numPaths)
	_ = binary.Write(&b, be, uint32(0))
	return b.Bytes()
}

func buildBomVars() []byte {
	vars := []struct {
		name  string
		block uint32
	}{
		{"BomInfo", bomInfoBlock},
		{"Paths", bomPathsTree},
		{"HLIndex", bomHLIndexTree},
		{"VIndex", bomVIndexBlock},
		{"Size64", bomSize64Tree},
	}
	var b bytes.Buffer
	be := binary.BigEndian
	_ = binary.Write(&b, be, uint32(len(vars)))
	for _, v := range vars {
		_ = binary.Write(&b, be, v.block)
		b.WriteByte(byte(len(v.name)))
		b.WriteString(v.name)
	}
	return b.Bytes()
}

func buildBomIndex(blocks [][]byte, addrs []uint32) []byte {
	var b bytes.Buffer
	be := binary.BigEndian
	_ = binary.Write(&b, be, uint32(len(blocks)))
	for i := range blocks {
		_ = binary.Write(&b, be, addrs[i])
		_ = binary.Write(&b, be, uint32(len(blocks[i])))
	}
	_ = binary.Write(&b, be, uint32(0))
	return b.Bytes()
}
