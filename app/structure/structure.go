package structure

import (
	"fmt"
	"io/fs"
	"path"

	"github.com/vertex-language/macpkg/app/icns"
	"github.com/vertex-language/macpkg/app/plist"
	"github.com/vertex-language/macpkg/vfs"
)

// PkgInfoContent is the standard 8-byte content for macOS GUI application packages.
const PkgInfoContent = "APPL????"

// BundleLayout defines the input configuration for populating a .app directory tree.
type BundleLayout struct {
	OutDir         string
	Name           string
	Identifier     string
	Version        string
	Build          string
	ExecutableName string
	ExecutableData []byte
	IconName       string
	IconData       []byte // PNG or ICNS
	MinOS          string
	Category       string
	ExtraFiles     map[string][]byte
	FS             vfs.FS
}

// Assemble writes the complete .app directory structure to the virtual filesystem.
func Assemble(layout BundleLayout) ([]string, error) {
	fsys := layout.FS
	if fsys == nil {
		fsys = vfs.RealFS("")
	}

	outDir := layout.OutDir
	if outDir == "" {
		outDir = layout.Name + ".app"
	}

	contentsDir := path.Join(outDir, "Contents")
	macosDir := path.Join(contentsDir, "MacOS")
	resourcesDir := path.Join(contentsDir, "Resources")

	if err := fsys.MkdirAll(macosDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir MacOS: %w", err)
	}
	if err := fsys.MkdirAll(resourcesDir, 0755); err != nil {
		return nil, fmt.Errorf("mkdir Resources: %w", err)
	}

	var createdFiles []string

	// 1. PkgInfo
	pkgInfoPath := path.Join(contentsDir, "PkgInfo")
	if err := fsys.WriteFile(pkgInfoPath, []byte(PkgInfoContent), 0644); err != nil {
		return nil, fmt.Errorf("write PkgInfo: %w", err)
	}
	createdFiles = append(createdFiles, pkgInfoPath)

	// 2. Executable
	execName := layout.ExecutableName
	if execName == "" {
		execName = layout.Name
	}
	execPath := path.Join(macosDir, execName)
	if len(layout.ExecutableData) > 0 {
		if err := fsys.WriteFile(execPath, layout.ExecutableData, 0755); err != nil {
			return nil, fmt.Errorf("write executable: %w", err)
		}
		createdFiles = append(createdFiles, execPath)
	}

	// 3. Icon
	iconName := layout.IconName
	if iconName == "" {
		iconName = "AppIcon.icns"
	}
	if len(layout.IconData) > 0 {
		var icnsBytes []byte
		// If icon is raw PNG (starts with \x89PNG), wrap it into ICNS chunks
		if len(layout.IconData) >= 8 && string(layout.IconData[1:4]) == "PNG" {
			ico := icns.FromPNG(layout.IconData)
			var err error
			icnsBytes, err = ico.Bytes()
			if err != nil {
				return nil, fmt.Errorf("encode ICNS: %w", err)
			}
		} else {
			icnsBytes = layout.IconData
		}
		iconPath := path.Join(resourcesDir, iconName)
		if err := fsys.WriteFile(iconPath, icnsBytes, 0644); err != nil {
			return nil, fmt.Errorf("write icon: %w", err)
		}
		createdFiles = append(createdFiles, iconPath)
	}

	// 4. Info.plist
	minOS := layout.MinOS
	if minOS == "" {
		minOS = "11.0"
	}
	ver := layout.Version
	if ver == "" {
		ver = "1.0.0"
	}
	bld := layout.Build
	if bld == "" {
		bld = ver
	}

	plistMap := map[string]any{
		"CFBundlePackageType":             "APPL",
		"CFBundleInfoDictionaryVersion":   "6.0",
		"CFBundleName":                    layout.Name,
		"CFBundleDisplayName":             layout.Name,
		"CFBundleIdentifier":              layout.Identifier,
		"CFBundleVersion":                 bld,
		"CFBundleShortVersionString":      ver,
		"CFBundleExecutable":              execName,
		"LSMinimumSystemVersion":          minOS,
		"NSHighResolutionCapable":         true,
	}
	if len(layout.IconData) > 0 {
		plistMap["CFBundleIconFile"] = iconName
	}
	if layout.Category != "" {
		plistMap["LSApplicationCategoryType"] = layout.Category
	}

	plistBytes, err := plist.MarshalXML(plistMap)
	if err != nil {
		return nil, fmt.Errorf("marshal Info.plist: %w", err)
	}
	plistPath := path.Join(contentsDir, "Info.plist")
	if err := fsys.WriteFile(plistPath, plistBytes, 0644); err != nil {
		return nil, fmt.Errorf("write Info.plist: %w", err)
	}
	createdFiles = append(createdFiles, plistPath)

	// 5. Extra files
	for relPath, data := range layout.ExtraFiles {
		target := path.Join(contentsDir, relPath)
		if err := fsys.WriteFile(target, data, fs.FileMode(0644)); err != nil {
			return nil, fmt.Errorf("write extra file %s: %w", relPath, err)
		}
		createdFiles = append(createdFiles, target)
	}

	return createdFiles, nil
}
