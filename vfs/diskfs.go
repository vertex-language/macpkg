package vfs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// DiskFS wraps an operating system directory tree as a vfs.FS.
type DiskFS struct {
	root string
}

// RealFS returns an FS backed by the OS filesystem rooted at baseDir.
// If baseDir is empty, paths are used directly.
func RealFS(baseDir string) FS {
	return &DiskFS{root: baseDir}
}

// DirFS creates a new DiskFS rooted at the specified OS directory.
func DirFS(root string) (*DiskFS, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root directory: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("stat root directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root path is not a directory: %s", abs)
	}

	return &DiskFS{root: abs}, nil
}

func (d *DiskFS) resolve(p string) string {
	if d.root == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(d.root, p)
}

// Open opens the named file.
func (d *DiskFS) Open(name string) (fs.File, error) {
	return os.Open(d.resolve(name))
}

// Stat returns a FileInfo describing the named file.
func (d *DiskFS) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(d.resolve(name))
}

// ReadFile reads the named file and returns its contents.
func (d *DiskFS) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(d.resolve(name))
}

// ReadDir reads the named directory.
func (d *DiskFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return os.ReadDir(d.resolve(name))
}

// WriteFile writes data to the named file with the specified permissions.
func (d *DiskFS) WriteFile(name string, data []byte, perm fs.FileMode) error {
	resolved := d.resolve(name)
	if err := os.MkdirAll(filepath.Dir(resolved), 0755); err != nil {
		return err
	}
	return os.WriteFile(resolved, data, perm)
}

// MkdirAll creates a directory named path, along with any necessary parents.
func (d *DiskFS) MkdirAll(path string, perm fs.FileMode) error {
	return os.MkdirAll(d.resolve(path), perm)
}

// Walk walks the file tree rooted at root.
func (d *DiskFS) Walk(fn fs.WalkDirFunc) error {
	start := d.root
	if start == "" {
		start = "."
	}
	return filepath.WalkDir(start, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(start, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		return fn(rel, d, nil)
	})
}

// WalkDir walks the file tree rooted at root.
func (d *DiskFS) WalkDir(root string, fn fs.WalkDirFunc) error {
	return filepath.WalkDir(d.resolve(root), fn)
}

// Files returns a sorted list of all regular file paths in the filesystem.
func (d *DiskFS) Files() ([]string, error) {
	var files []string
	start := d.root
	if start == "" {
		start = "."
	}
	err := filepath.WalkDir(start, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			rel, err := filepath.Rel(start, path)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}
