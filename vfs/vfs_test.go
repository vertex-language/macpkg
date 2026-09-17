package vfs_test

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/vertex-language/macpkg/vfs"
)

func TestMemFS_MSICompat(t *testing.T) {
	mem := vfs.NewMemFS()
	data := []byte("hello world")
	if err := mem.WriteFile("dir/sub/test.txt", data, 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	read, err := mem.ReadFile("dir/sub/test.txt")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(read) != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", string(read))
	}

	file, err := mem.Open("dir/sub/test.txt")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer file.Close()

	buf, err := io.ReadAll(file)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if string(buf) != "hello world" {
		t.Errorf("expected 'hello world', got '%s'", string(buf))
	}
}

func TestMemFS_MSIXCompat(t *testing.T) {
	m := vfs.NewMemFS()

	if err := m.AddFile("hello.txt", []byte("world")); err != nil {
		t.Fatalf("AddFile: %v", err)
	}
	if err := m.AddFile("assets/icon.png", []byte("fake-png-data")); err != nil {
		t.Fatalf("AddFile nested: %v", err)
	}

	// Read back
	data, err := m.ReadFile("hello.txt")
	if err != nil {
		t.Fatalf("ReadFile hello.txt: %v", err)
	}
	if string(data) != "world" {
		t.Fatalf("expected 'world', got %q", string(data))
	}

	// Read nested
	nested, err := m.ReadFile("assets/icon.png")
	if err != nil {
		t.Fatalf("ReadFile assets/icon.png: %v", err)
	}
	if string(nested) != "fake-png-data" {
		t.Fatalf("expected 'fake-png-data', got %q", string(nested))
	}

	// ReadDir
	entries, err := m.ReadDir("assets")
	if err != nil {
		t.Fatalf("ReadDir assets: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "icon.png" {
		t.Fatalf("unexpected entries: %+v", entries)
	}

	// Files list
	files, err := m.Files()
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	if len(files) != 2 || files[0] != "assets/icon.png" || files[1] != "hello.txt" {
		t.Fatalf("unexpected files: %+v", files)
	}

	// AddReader
	buf := bytes.NewBufferString("stream content")
	if err := m.AddReader("stream.txt", buf); err != nil {
		t.Fatalf("AddReader: %v", err)
	}
	sdata, _ := m.ReadFile("stream.txt")
	if string(sdata) != "stream content" {
		t.Fatalf("unexpected stream content: %s", string(sdata))
	}

	// AddString
	if err := m.AddString("str.txt", "string content"); err != nil {
		t.Fatalf("AddString: %v", err)
	}
	strData, _ := m.ReadFile("str.txt")
	if string(strData) != "string content" {
		t.Fatalf("unexpected string content: %s", string(strData))
	}

	// Stat
	fi, err := m.Stat("hello.txt")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if fi.Size() != 5 {
		t.Fatalf("expected size 5, got %d", fi.Size())
	}
	if fi.IsDir() {
		t.Fatalf("expected regular file, got directory")
	}

	// Remove
	if err := m.Remove("hello.txt"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := m.Stat("hello.txt"); err == nil {
		t.Fatalf("expected error after remove")
	}

	// Walk
	var walked []string
	err = m.Walk(func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		walked = append(walked, path)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if len(walked) == 0 {
		t.Fatalf("expected walked files, got none")
	}
}

func TestDiskFS(t *testing.T) {
	tempDir := t.TempDir()

	subDir := filepath.Join(tempDir, "sub")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "test.txt"), []byte("disk content"), 0644); err != nil {
		t.Fatal(err)
	}

	dfs, err := vfs.DirFS(tempDir)
	if err != nil {
		t.Fatalf("DirFS: %v", err)
	}

	data, err := dfs.ReadFile("sub/test.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "disk content" {
		t.Fatalf("expected 'disk content', got %q", string(data))
	}

	files, err := dfs.Files()
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	if len(files) != 1 || files[0] != "sub/test.txt" {
		t.Fatalf("unexpected files list: %+v", files)
	}
}
