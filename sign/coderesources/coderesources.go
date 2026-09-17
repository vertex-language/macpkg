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

// DefaultRules returns the standard macOS rules and rules2 dictionaries for CodeResources.
func DefaultRules() (map[string]any, map[string]any) {
	rules := map[string]any{
		"^Resources/": true,
		"^Resources/.*\\.lproj/": map[string]any{
			"optional": true,
			"weight":   1000.0,
		},
		"^Resources/.*\\.lproj/locversion.plist$": map[string]any{
			"omit":   true,
			"weight": 1100.0,
		},
		"^Resources/Base\\.lproj/": map[string]any{
			"weight": 1010.0,
		},
		"^version.plist$": true,
	}

	rules2 := map[string]any{
		".*\\.dSYM($|/)": map[string]any{
			"weight": 11.0,
		},
		"^(.*/)?\\.DS_Store$": map[string]any{
			"omit":   true,
			"weight": 2000.0,
		},
		"^(Frameworks|SharedFrameworks|PlugIns|Plug-ins|XPCServices|Helpers|MacOS|Library/(Automator|Spotlight|LoginItems))/": map[string]any{
			"nested": true,
			"weight": 10.0,
		},
		"^.*": true,
		"^Info\\.plist$": map[string]any{
			"omit":   true,
			"weight": 20.0,
		},
		"^PkgInfo$": map[string]any{
			"omit":   true,
			"weight": 20.0,
		},
		"^Resources/": map[string]any{
			"weight": 20.0,
		},
		"^Resources/.*\\.lproj/": map[string]any{
			"optional": true,
			"weight":   1000.0,
		},
		"^Resources/.*\\.lproj/locversion.plist$": map[string]any{
			"omit":   true,
			"weight": 1100.0,
		},
		"^Resources/Base\\.lproj/": map[string]any{
			"weight": 1010.0,
		},
		"^[^/]+$": map[string]any{
			"nested": true,
			"weight": 10.0,
		},
		"^embedded\\.provisionprofile$": map[string]any{
			"weight": 20.0,
		},
		"^version\\.plist$": map[string]any{
			"weight": 20.0,
		},
	}
	return rules, rules2
}

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

		// Omit _CodeSignature, Info.plist, PkgInfo, and executables in MacOS/ (bound by CodeDirectory)
		if strings.HasPrefix(rel, "_CodeSignature") || rel == "Info.plist" || rel == "PkgInfo" || strings.HasPrefix(rel, "MacOS/") {
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

	rules, rules2 := DefaultRules()
	doc := map[string]any{
		"files":  filesMap,
		"files2": files2Map,
		"rules":  rules,
		"rules2": rules2,
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
