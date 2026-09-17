package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/vertex-language/macpkg/app/structure"
	"github.com/vertex-language/macpkg/vfs"
)

// Config configures .app bundle assembly.
type Config struct {
	Name           string            // Bundle display name (e.g. "MyApp")
	Identifier     string            // CFBundleIdentifier (e.g. "com.example.myapp")
	Version        string            // CFBundleShortVersionString (e.g. "1.0.0")
	Build          string            // CFBundleVersion (e.g. "100")
	Executable     string            // Target executable name (default: Name)
	ExecutableData []byte            // Raw executable content
	SourceBinary   string            // Path to source binary on FS
	IconName       string            // Name of icon file (default: "AppIcon.icns")
	IconData       []byte            // Raw icon data (PNG or ICNS)
	SourceIcon     string            // Path to icon on FS
	MinOS          string            // LSMinimumSystemVersion (default: "11.0")
	Category       string            // LSApplicationCategoryType
	OutDir         string            // Destination directory (default: "<Name>.app")
	ExtraFiles     map[string][]byte // Additional files placed inside Contents/
	FS             vfs.FS            // Pluggable filesystem interface (default: RealFS(""))
}

// Bundle describes an assembled .app bundle.
type Bundle struct {
	Path      string
	Files     []string
	TotalSize int64
}

// Assemble creates a valid .app bundle directory structure based on cfg.
func Assemble(ctx context.Context, cfg Config) (*Bundle, error) {
	if cfg.Name == "" {
		return nil, errors.New("app: Name is required")
	}
	if cfg.Identifier == "" {
		return nil, errors.New("app: Identifier is required")
	}

	fsys := cfg.FS
	if fsys == nil {
		fsys = vfs.RealFS("")
	}

	execData := cfg.ExecutableData
	if len(execData) == 0 && cfg.SourceBinary != "" {
		var err error
		execData, err = fsys.ReadFile(cfg.SourceBinary)
		if err != nil {
			return nil, fmt.Errorf("read SourceBinary %s: %w", cfg.SourceBinary, err)
		}
	}

	iconData := cfg.IconData
	if len(iconData) == 0 && cfg.SourceIcon != "" {
		var err error
		iconData, err = fsys.ReadFile(cfg.SourceIcon)
		if err != nil {
			return nil, fmt.Errorf("read SourceIcon %s: %w", cfg.SourceIcon, err)
		}
	}

	outDir := cfg.OutDir
	if outDir == "" {
		outDir = cfg.Name + ".app"
	}

	files, err := structure.Assemble(structure.BundleLayout{
		OutDir:         outDir,
		Name:           cfg.Name,
		Identifier:     cfg.Identifier,
		Version:        cfg.Version,
		Build:          cfg.Build,
		ExecutableName: cfg.Executable,
		ExecutableData: execData,
		IconName:       cfg.IconName,
		IconData:       iconData,
		MinOS:          cfg.MinOS,
		Category:       cfg.Category,
		ExtraFiles:     cfg.ExtraFiles,
		FS:             fsys,
	})
	if err != nil {
		return nil, err
	}

	var totalSize int64
	for _, f := range files {
		fi, err := fsys.Stat(f)
		if err == nil {
			totalSize += fi.Size()
		}
	}

	return &Bundle{
		Path:      outDir,
		Files:     files,
		TotalSize: totalSize,
	}, nil
}
