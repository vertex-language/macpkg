package vfs

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemFS is a thread-safe, purely in-memory virtual filesystem.
type MemFS struct {
	mu    sync.RWMutex
	files map[string]*memEntry
}

type memEntry struct {
	name    string
	data    []byte
	perm    fs.FileMode
	isDir   bool
	modTime time.Time
}

func (e *memEntry) Name() string       { return path.Base(e.name) }
func (e *memEntry) Size() int64        { return int64(len(e.data)) }
func (e *memEntry) Mode() fs.FileMode {
	if e.isDir {
		if e.perm != 0 {
			return e.perm | fs.ModeDir
		}
		return fs.ModeDir | 0755
	}
	if e.perm != 0 {
		return e.perm
	}
	return 0644
}
func (e *memEntry) ModTime() time.Time         { return e.modTime }
func (e *memEntry) IsDir() bool                { return e.isDir }
func (e *memEntry) Sys() any                   { return nil }
func (e *memEntry) Type() fs.FileMode          { return e.Mode().Type() }
func (e *memEntry) Info() (fs.FileInfo, error) { return e, nil }

// NewMemFS initializes a new empty in-memory filesystem.
func NewMemFS() *MemFS {
	m := &MemFS{
		files: make(map[string]*memEntry),
	}
	m.files["."] = &memEntry{name: ".", isDir: true, perm: fs.ModeDir | 0755, modTime: time.Now()}
	return m
}

func cleanPath(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	p = path.Clean(p)
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return "."
	}
	return p
}

// AddFile adds a file with the given content to the in-memory filesystem.
func (m *MemFS) AddFile(name string, data []byte) error {
	return m.WriteFile(name, data, 0644)
}

// AddString adds a file with string content to the filesystem.
func (m *MemFS) AddString(name, content string) error {
	return m.AddFile(name, []byte(content))
}

// AddReader reads all content from r and adds it as a file.
func (m *MemFS) AddReader(name string, r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	return m.AddFile(name, data)
}

// WriteFile writes data to the named file with the specified permissions.
func (m *MemFS) WriteFile(name string, data []byte, perm fs.FileMode) error {
	cleaned := cleanPath(name)
	if cleaned == "." {
		return fmt.Errorf("cannot add file with root path: %s", name)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Ensure parent directories exist
	dir := path.Dir(cleaned)
	for dir != "." && dir != "/" && dir != "" {
		if _, exists := m.files[dir]; !exists {
			m.files[dir] = &memEntry{name: dir, isDir: true, perm: fs.ModeDir | 0755, modTime: time.Now()}
		}
		dir = path.Dir(dir)
	}

	cpy := make([]byte, len(data))
	copy(cpy, data)
	m.files[cleaned] = &memEntry{
		name:    cleaned,
		data:    cpy,
		perm:    perm,
		isDir:   false,
		modTime: time.Now(),
	}
	return nil
}

// MkdirAll creates a directory named path, along with any necessary parents.
func (m *MemFS) MkdirAll(p string, perm fs.FileMode) error {
	cleaned := cleanPath(p)
	if cleaned == "." {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	parts := strings.Split(cleaned, "/")
	cur := ""
	for _, part := range parts {
		if cur == "" {
			cur = part
		} else {
			cur += "/" + part
		}
		if f, exists := m.files[cur]; exists {
			if !f.isDir {
				return errors.New("path component is not a directory")
			}
		} else {
			m.files[cur] = &memEntry{
				name:    cur,
				isDir:   true,
				perm:    perm | fs.ModeDir,
				modTime: time.Now(),
			}
		}
	}
	return nil
}

// Remove deletes a file or directory from the filesystem.
func (m *MemFS) Remove(name string) error {
	cleaned := cleanPath(name)
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.files[cleaned]; !exists {
		return fs.ErrNotExist
	}
	delete(m.files, cleaned)
	return nil
}

// Open opens the named file.
func (m *MemFS) Open(name string) (fs.File, error) {
	cleaned := cleanPath(name)

	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.files[cleaned]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	if entry.isDir {
		var entries []fs.DirEntry
		prefix := cleaned
		if prefix == "." {
			prefix = ""
		} else {
			prefix += "/"
		}

		for k, v := range m.files {
			if k == "." || k == cleaned {
				continue
			}
			if strings.HasPrefix(k, prefix) {
				sub := strings.TrimPrefix(k, prefix)
				if !strings.Contains(sub, "/") {
					entries = append(entries, v)
				}
			}
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Name() < entries[j].Name()
		})
		return &memDirFile{entry: entry, entries: entries}, nil
	}

	return &memFile{
		entry:  entry,
		reader: bytes.NewReader(entry.data),
	}, nil
}

// Stat returns a FileInfo describing the named file.
func (m *MemFS) Stat(name string) (fs.FileInfo, error) {
	cleaned := cleanPath(name)

	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.files[cleaned]
	if !ok {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}
	return entry, nil
}

// ReadFile reads the named file and returns its contents.
func (m *MemFS) ReadFile(name string) ([]byte, error) {
	cleaned := cleanPath(name)

	m.mu.RLock()
	defer m.mu.RUnlock()

	entry, ok := m.files[cleaned]
	if !ok {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrNotExist}
	}
	if entry.isDir {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: errors.New("is a directory")}
	}
	cpy := make([]byte, len(entry.data))
	copy(cpy, entry.data)
	return cpy, nil
}

// ReadDir reads the named directory.
func (m *MemFS) ReadDir(name string) ([]fs.DirEntry, error) {
	file, err := m.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	rdf, ok := file.(fs.ReadDirFile)
	if !ok {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: errors.New("not a directory")}
	}
	return rdf.ReadDir(-1)
}

// Walk walks the file tree rooted at root (defaulting to ".").
func (m *MemFS) Walk(fn fs.WalkDirFunc) error {
	return fs.WalkDir(m, ".", fn)
}

// WalkDir walks the file tree rooted at root, calling fn for each file or directory.
func (m *MemFS) WalkDir(root string, fn fs.WalkDirFunc) error {
	cleaned := cleanPath(root)
	return fs.WalkDir(m, cleaned, fn)
}

// Files returns a sorted list of all regular file paths in the filesystem.
func (m *MemFS) Files() ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []string
	for k, v := range m.files {
		if !v.isDir {
			result = append(result, k)
		}
	}
	sort.Strings(result)
	return result, nil
}

type memFile struct {
	entry  *memEntry
	reader *bytes.Reader
}

func (f *memFile) Stat() (fs.FileInfo, error) { return f.entry, nil }
func (f *memFile) Read(b []byte) (int, error)  { return f.reader.Read(b) }
func (f *memFile) Close() error               { return nil }
func (f *memFile) Seek(offset int64, whence int) (int64, error) {
	return f.reader.Seek(offset, whence)
}
func (f *memFile) ReadAt(b []byte, off int64) (int, error) {
	return f.reader.ReadAt(b, off)
}

type memDirFile struct {
	entry   *memEntry
	entries []fs.DirEntry
	offset  int
}

func (d *memDirFile) Stat() (fs.FileInfo, error) { return d.entry, nil }
func (d *memDirFile) Read([]byte) (int, error)   { return 0, errors.New("cannot read from directory") }
func (d *memDirFile) Close() error               { return nil }

func (d *memDirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if n <= 0 {
		res := d.entries[d.offset:]
		d.offset = len(d.entries)
		return res, nil
	}
	if d.offset >= len(d.entries) {
		return nil, io.EOF
	}
	end := d.offset + n
	if end > len(d.entries) {
		end = len(d.entries)
	}
	res := d.entries[d.offset:end]
	d.offset = end
	return res, nil
}
