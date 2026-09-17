package dmg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/vertex-language/macpkg/dmg/dsstore"
	"github.com/vertex-language/macpkg/dmg/hfs"
	"github.com/vertex-language/macpkg/dmg/udif"
	"github.com/vertex-language/macpkg/vfs"
)

// Size represents window width and height.
type Size struct {
	Width  int
	Height int
}

// Point represents an (X, Y) coordinate.
type Point struct {
	X int
	Y int
}

// Config configures Apple Disk Image (.dmg) generation.
type Config struct {
	Title                string // Volume name (e.g. "MyApp Installer")
	SourceApp            string // Path to source .app bundle
	OutFile              string // Output .dmg path
	Background           string // Path to background image
	BackgroundBytes      []byte // Raw background image bytes
	WindowSize           Size   // Finder window dimensions
	IconSize             int    // Finder icon size (default: 128)
	AppPosition          Point  // Finder coordinates for .app icon
	ApplicationsPosition Point  // Finder coordinates for Applications symlink
	AddApplicationsLink  bool   // Add symlink to /Applications (default: true)
	LicenseFile          string // Path to SLA license file
	LicenseText          string // Raw SLA license string
	FS                   vfs.FS // Pluggable filesystem
}

// BuildResult reports metrics about a built .dmg image.
type BuildResult struct {
	OutputFile string
	TotalSize  int64
}

// Build creates a compressed UDIF (.dmg) disk image.
func Build(ctx context.Context, cfg Config) (*BuildResult, error) {
	if cfg.Title == "" {
		cfg.Title = "Disk Image"
	}
	if cfg.OutFile == "" {
		return nil, errors.New("dmg: OutFile is required")
	}

	fsys := cfg.FS
	if fsys == nil {
		fsys = vfs.RealFS("")
	}

	appName := "App.app"
	if cfg.SourceApp != "" {
		appName = filepath.Base(cfg.SourceApp)
	}

	appPos := cfg.AppPosition
	if appPos.X == 0 && appPos.Y == 0 {
		appPos = Point{X: 160, Y: 200}
	}
	appsPos := cfg.ApplicationsPosition
	if appsPos.X == 0 && appsPos.Y == 0 {
		appsPos = Point{X: 460, Y: 200}
	}

	// 1. Generate .DS_Store
	dsStoreBytes, err := dsstore.Generate(dsstore.Layout{
		AppName:              appName,
		IconSize:             cfg.IconSize,
		AppPosition:          dsstore.Point{X: appPos.X, Y: appPos.Y},
		ApplicationsPosition: dsstore.Point{X: appsPos.X, Y: appsPos.Y},
	})
	if err != nil {
		return nil, fmt.Errorf("generate .DS_Store: %w", err)
	}

	// 2. Generate raw volume image
	var rawVolume []byte
	_, isDiskFS := fsys.(*vfs.DiskFS)
	if isDiskFS && cfg.SourceApp != "" {
		if _, err := exec.LookPath("hdiutil"); err == nil {
			stageDir, err := os.MkdirTemp("", "macpkg_dmg_stage_*")
			if err == nil {
				defer os.RemoveAll(stageDir)
				_ = os.WriteFile(filepath.Join(stageDir, ".DS_Store"), dsStoreBytes, 0644)
				targetApp := filepath.Join(stageDir, filepath.Base(cfg.SourceApp))
				_ = copyDir(cfg.SourceApp, targetApp)
				if cfg.AddApplicationsLink {
					_ = os.Symlink("/Applications", filepath.Join(stageDir, "Applications"))
				}
				rawDmgPath := filepath.Join(os.TempDir(), fmt.Sprintf("macpkg_raw_%d.dmg", os.Getpid()))
				defer os.Remove(rawDmgPath)
				cmd := exec.CommandContext(ctx, "hdiutil", "create",
					"-srcfolder", stageDir,
					"-volname", cfg.Title,
					"-format", "UDRW",
					"-layout", "NONE",
					"-size", "20m",
					"-ov",
					rawDmgPath,
				)
				if err := cmd.Run(); err == nil {
					rawVolume, _ = os.ReadFile(rawDmgPath)
				}
			}
		}
	}

	if len(rawVolume) == 0 {
		var err error
		rawVolume, err = hfs.GenerateVolume(hfs.VolumeConfig{
			Name:   cfg.Title,
			SizeMB: 5,
			Entries: []hfs.FileEntry{
				{Path: ".DS_Store", Data: dsStoreBytes},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("generate HFS+ volume: %w", err)
		}
	}

	// 3. Compress into UDIF (.dmg) container with koly trailer
	dmgBytes, err := udif.CompressUDIF(rawVolume, cfg.Title)
	if err != nil {
		return nil, fmt.Errorf("compress UDIF: %w", err)
	}

	// 4. Write to output destination
	if err := fsys.WriteFile(cfg.OutFile, dmgBytes, 0644); err != nil {
		return nil, fmt.Errorf("write output DMG: %w", err)
	}

	return &BuildResult{
		OutputFile: cfg.OutFile,
		TotalSize:  int64(len(dmgBytes)),
	}, nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(linkTarget, target)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
}

