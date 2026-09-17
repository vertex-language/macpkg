package pkg

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vertex-language/macpkg/pkg/bom"
	"github.com/vertex-language/macpkg/pkg/cpio"
	"github.com/vertex-language/macpkg/pkg/packageinfo"
	"github.com/vertex-language/macpkg/pkg/xar"
	"github.com/vertex-language/macpkg/vfs"
)

// Config specifies input parameters for compiling a macOS Flat Package (.pkg).
type Config struct {
	Identifier      string            // Package bundle identifier (e.g. "com.example.app.pkg")
	Version         string            // Package version (e.g. "1.0.0")
	InstallLocation string            // Target directory (default: "/Applications")
	SourcePayload   string            // Directory or .app bundle to harvest
	PayloadFiles    map[string][]byte // In-memory payload files
	OutFile         string            // Output .pkg destination path
	ScriptsDir      string            // Optional path to preinstall/postinstall scripts
	Scripts         map[string][]byte // In-memory scripts
	FS              vfs.FS            // Pluggable filesystem interface
}

// BuildResult reports metrics about a successfully built .pkg installer.
type BuildResult struct {
	OutputFile string
	TotalSize  int64
	FilesCount int
}

// Build creates a valid macOS Flat Package (.pkg).
func Build(ctx context.Context, cfg Config) (*BuildResult, error) {
	if cfg.Identifier == "" {
		return nil, errors.New("pkg: Identifier is required")
	}
	if cfg.OutFile == "" {
		return nil, errors.New("pkg: OutFile is required")
	}
	if cfg.Version == "" {
		cfg.Version = "1.0.0"
	}
	if cfg.InstallLocation == "" {
		cfg.InstallLocation = "/Applications"
	}

	fsys := cfg.FS
	if fsys == nil {
		fsys = vfs.RealFS("")
	}

	// 1. Gather payload files
	files := make(map[string][]byte)
	for k, v := range cfg.PayloadFiles {
		clean := filepath.Clean(k)
		clean = strings.TrimPrefix(clean, "./")
		clean = strings.TrimPrefix(clean, "/")
		files[clean] = v
	}

	if cfg.SourcePayload != "" {
		baseDir := cfg.SourcePayload
		if filepath.Ext(cfg.SourcePayload) == ".app" {
			baseDir = filepath.Dir(cfg.SourcePayload)
		}
		err := fsys.WalkDir(cfg.SourcePayload, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				data, err := fsys.ReadFile(p)
				if err != nil {
					return err
				}
				rel, err := filepath.Rel(baseDir, p)
				if err != nil {
					rel = p
				}
				rel = filepath.Clean(rel)
				rel = strings.TrimPrefix(rel, "./")
				rel = strings.TrimPrefix(rel, "/")
				files[rel] = data
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk SourcePayload %s: %w", cfg.SourcePayload, err)
		}
	}

	if len(files) == 0 {
		return nil, errors.New("pkg: no payload files provided")
	}

	// Sort file paths for deterministic packaging
	var sortedPaths []string
	for p := range files {
		sortedPaths = append(sortedPaths, p)
	}
	sort.Strings(sortedPaths)

	// Collect unique directories for cpio archive
	dirSet := make(map[string]bool)
	for _, p := range sortedPaths {
		parent := filepath.Dir(p)
		for parent != "." && parent != "/" && parent != "" {
			dirSet["./"+parent] = true
			parent = filepath.Dir(parent)
		}
		if pDir := filepath.Dir(p); pDir != "." && pDir != "/" && pDir != "" {
			dirSet["./"+pDir] = true
		}
	}

	var sortedDirs []string
	for d := range dirSet {
		sortedDirs = append(sortedDirs, d)
	}
	sort.Strings(sortedDirs)

	var cpioEntries []cpio.FileEntry
	// Root directory "."
	cpioEntries = append(cpioEntries, cpio.FileEntry{
		Name: ".",
		Mode: 0040755,
		UID:  0,
		GID:  80,
	})
	// Subdirectories
	for _, d := range sortedDirs {
		cpioEntries = append(cpioEntries, cpio.FileEntry{
			Name: d,
			Mode: 0040755,
			UID:  0,
			GID:  80,
		})
	}

	// Files
	var bomEntries []bom.FileEntry
	totalPayloadBytes := uint64(0)

	for _, p := range sortedPaths {
		data := files[p]
		size := uint64(len(data))
		totalPayloadBytes += size

		mode := uint32(0100644)
		base := filepath.Base(p)
		if filepath.Ext(p) == "" || base == "PkgInfo" {
			mode = 0100755
		}
		if len(data) >= 4 {
			magic := binary.BigEndian.Uint32(data[:4])
			// Mach-O magics: MH_MAGIC(0xFEEDFACE), MH_MAGIC_64(0xFEEDFACF), MH_CIGAM(0xCEFAEDFE), MH_CIGAM_64(0xCFFAEDFE), FAT(0xCAFEBABE)
			if magic == 0xFEEDFACE || magic == 0xFEEDFACF || magic == 0xCEFAEDFE || magic == 0xCFFAEDFE || magic == 0xCAFEBABE {
				mode = 0100755
			}
		}

		cpioPath := "./" + p
		cpioEntries = append(cpioEntries, cpio.FileEntry{
			Name: cpioPath,
			Mode: mode,
			UID:  0,
			GID:  80,
			Data: data,
		})

		bomEntries = append(bomEntries, bom.FileEntry{
			Path: cpioPath,
			Mode: uint16(mode),
			UID:  0,
			GID:  80,
			Size: size,
			Data: data,
		})
	}

	// 2. Generate BOM
	bomBytes, err := bom.Generate(bomEntries)
	if err != nil {
		return nil, fmt.Errorf("generate BOM: %w", err)
	}

	// 3. Generate cpio Payload (.cpio.gz)
	payloadBytes, err := cpio.ArchiveGz(cpioEntries)
	if err != nil {
		return nil, fmt.Errorf("generate cpio Payload: %w", err)
	}

	// 4. Generate PackageInfo XML
	pkgInfoXML, err := packageinfo.Generate(packageinfo.Info{
		Identifier:      cfg.Identifier,
		Version:         cfg.Version,
		InstallLocation: cfg.InstallLocation,
		Auth:            "root",
		Payload: packageinfo.Payload{
			InstallKBytes: (totalPayloadBytes + 1023) / 1024,
			NumberOfFiles: uint64(len(sortedDirs) + len(sortedPaths) + 1), // dirs + files + root
		},
	})
	if err != nil {
		return nil, fmt.Errorf("generate PackageInfo: %w", err)
	}

	// 5. Assemble files into XAR archive
	xarFiles := []xar.FileEntry{
		{Name: "PackageInfo", Data: pkgInfoXML},
		{Name: "Payload", Data: payloadBytes},
		{Name: "Bom", Data: bomBytes},
	}

	// Handle scripts if present
	if len(cfg.Scripts) > 0 {
		var scriptEntries []cpio.FileEntry
		for name, scriptData := range cfg.Scripts {
			scriptEntries = append(scriptEntries, cpio.FileEntry{
				Name: name,
				Mode: 0100755,
				Data: scriptData,
			})
		}
		scriptsBytes, err := cpio.ArchiveGz(scriptEntries)
		if err != nil {
			return nil, fmt.Errorf("generate Scripts archive: %w", err)
		}
		xarFiles = append(xarFiles, xar.FileEntry{Name: "Scripts", Data: scriptsBytes})
	}

	pkgBytes, err := xar.Archive(xarFiles)
	if err != nil {
		return nil, fmt.Errorf("archive XAR .pkg: %w", err)
	}

	// 6. Write to destination
	if err := fsys.WriteFile(cfg.OutFile, pkgBytes, 0644); err != nil {
		return nil, fmt.Errorf("write output PKG %s: %w", cfg.OutFile, err)
	}

	return &BuildResult{
		OutputFile: cfg.OutFile,
		TotalSize:  int64(len(pkgBytes)),
		FilesCount: len(files),
	}, nil
}
