package coderesources

import (
	"crypto/sha1"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/vertex-language/macpkg/app/plist"
	"github.com/vertex-language/macpkg/vfs"
)

// Generate scans a bundle's Contents/ directory and generates CodeResources XML plist bytes.
func Generate(bundleContentsDir string, fsys vfs.FS) ([]byte, error) {
	if fsys == nil {
		fsys = vfs.RealFS("")
	}

	filesMap := make(map[string]any)
	files2Map := make(map[string]any)

	err := fsys.WalkDir(bundleContentsDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel := strings.TrimPrefix(p, bundleContentsDir)
		rel = strings.TrimPrefix(rel, "/")
		// Never hash _CodeSignature itself
		if strings.HasPrefix(rel, "_CodeSignature") {
			return nil
		}

		data, err := fsys.ReadFile(p)
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}

		h1 := sha1.Sum(data)
		h256 := sha256.Sum256(data)

		filesMap[rel] = h1[:]
		files2Map[rel] = map[string]any{
			"hash":  h1[:],
			"hash2": h256[:],
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan bundle resources: %w", err)
	}

	doc := map[string]any{
		"files":  filesMap,
		"files2": files2Map,
		"rules": map[string]any{
			"^.*": true,
		},
		"rules2": map[string]any{
			"^.*": true,
		},
	}

	return plist.MarshalXML(doc)
}

// SealBundle generates and writes Contents/_CodeSignature/CodeResources.
func SealBundle(bundlePath string, fsys vfs.FS) error {
	if fsys == nil {
		fsys = vfs.RealFS("")
	}
	contentsDir := path.Join(bundlePath, "Contents")
	codeSigDir := path.Join(contentsDir, "_CodeSignature")
	if err := fsys.MkdirAll(codeSigDir, 0755); err != nil {
		return fmt.Errorf("mkdir _CodeSignature: %w", err)
	}

	resBytes, err := Generate(contentsDir, fsys)
	if err != nil {
		return err
	}

	codeResourcesPath := path.Join(codeSigDir, "CodeResources")
	return fsys.WriteFile(codeResourcesPath, resBytes, 0644)
}
