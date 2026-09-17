package pkg

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"

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
		files[k] = v
	}

	if cfg.SourcePayload != "" {
		err := fsys.WalkDir(cfg.SourcePayload, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				data, err := fsys.ReadFile(p)
				if err != nil {
					return err
				}
				files[p] = data
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

	// 2. Build BOM entries & cpio entries
	var bomEntries []bom.FileEntry
	var cpioEntries []cpio.FileEntry

	totalPayloadBytes := uint64(0)
	for filePath, data := range files {
		size := uint64(len(data))
		totalPayloadBytes += size
		mode := uint32(0100644)
		if path.Ext(filePath) == "" || path.Base(filePath) == "PkgInfo" {
			mode = uint32(0100755)
		}

		bomEntries = append(bomEntries, bom.FileEntry{
			Path: "./" + filePath,
			Mode: uint16(mode & 0777),
			UID:  0,
			GID:  80, // admin
			Size: size,
		})

		cpioEntries = append(cpioEntries, cpio.FileEntry{
			Name: filePath,
			Mode: mode,
			UID:  0,
			GID:  80,
			Data: data,
		})
	}

	// Generate BOM
	bomBytes, err := bom.Generate(bomEntries)
	if err != nil {
		return nil, fmt.Errorf("generate BOM: %w", err)
	}

	// Generate cpio Payload (.cpio.gz)
	payloadBytes, err := cpio.ArchiveGz(cpioEntries)
	if err != nil {
		return nil, fmt.Errorf("generate cpio Payload: %w", err)
	}

	// 3. Generate PackageInfo XML
	pkgInfoXML, err := packageinfo.Generate(packageinfo.Info{
		Identifier:      cfg.Identifier,
		Version:         cfg.Version,
		InstallLocation: cfg.InstallLocation,
		Auth:            "root",
		Payload: packageinfo.Payload{
			InstallKBytes: (totalPayloadBytes + 1023) / 1024,
			NumberOfFiles: uint64(len(files)),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("generate PackageInfo: %w", err)
	}

	// 4. Assemble files into XAR archive
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

	// 5. Write to destination
	if err := fsys.WriteFile(cfg.OutFile, pkgBytes, 0644); err != nil {
		return nil, fmt.Errorf("write output PKG %s: %w", cfg.OutFile, err)
	}

	return &BuildResult{
		OutputFile: cfg.OutFile,
		TotalSize:  int64(len(pkgBytes)),
		FilesCount: len(files),
	}, nil
}
