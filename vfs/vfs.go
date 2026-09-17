package vfs

import (
	"io/fs"
)

// FS represents a virtual filesystem used for hermetic testing, packaging input, and disk operations.
type FS interface {
	fs.FS
	fs.StatFS
	fs.ReadFileFS
	fs.ReadDirFS

	// Walk walks the file tree rooted at root (or current directory), calling fn for each file or directory.
	Walk(fn fs.WalkDirFunc) error

	// WalkDir walks the file tree rooted at root, calling fn for each file or directory.
	WalkDir(root string, fn fs.WalkDirFunc) error

	// Files returns a sorted list of all regular file paths in the filesystem.
	Files() ([]string, error)

	// WriteFile writes data to the named file with the specified permissions.
	WriteFile(name string, data []byte, perm fs.FileMode) error

	// MkdirAll creates a directory named path, along with any necessary parents.
	MkdirAll(path string, perm fs.FileMode) error
}
