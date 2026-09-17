package sign

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"strings"

	"github.com/vertex-language/macpkg/app/plist"
	"github.com/vertex-language/macpkg/sign/coderesources"
	"github.com/vertex-language/macpkg/sign/macho"
	"github.com/vertex-language/macpkg/vfs"
)

// Config configures codesigning for a binary or bundle.
type Config struct {
	Target       string // Path to .app bundle, .framework, or Mach-O executable
	Identity     string // "-" for ad-hoc, or certificate subject name
	CertPath     string // Path to PEM certificate file (optional)
	KeyPath      string // Path to PEM private key file (optional)
	CertPEM      []byte // Raw PEM certificate bytes (optional)
	KeyPEM       []byte // Raw PEM private key bytes (optional)
	Identifier   string // Code signing identifier (defaults to CFBundleIdentifier or file base)
	TeamID       string // Team ID (e.g. ABCDE12345)
	Entitlements string // Path to entitlements plist file or raw XML string
	Hardened     bool   // Enable hardened runtime (CS_RUNTIME)
	Force        bool   // Overwrite existing signature
	Deep         bool   // Recursively sign nested frameworks, dylibs, and helper apps
	FS           vfs.FS // Virtual filesystem (nil => RealFS)
}

// Result describes the outcome of a signing operation.
type Result struct {
	Target     string   `json:"target"`
	Identifier string   `json:"identifier"`
	Format     string   `json:"format"`
	IsBundle   bool     `json:"is_bundle"`
	SignedList []string `json:"signed_list"`
}

// Sign signs a Mach-O binary or application bundle according to cfg.
func Sign(ctx context.Context, cfg Config) (*Result, error) {
	if cfg.Target == "" {
		return nil, fmt.Errorf("sign: target path is required")
	}

	fsys := cfg.FS
	if fsys == nil {
		fsys = vfs.RealFS("")
	}

	// Prepare identity
	var id *macho.Identity
	if len(cfg.CertPEM) > 0 && len(cfg.KeyPEM) > 0 {
		var err error
		id, err = macho.LoadIdentityPEMFromBytes(cfg.CertPEM, cfg.KeyPEM)
		if err != nil {
			return nil, fmt.Errorf("sign: parse identity bytes: %w", err)
		}
	} else if cfg.CertPath != "" && cfg.KeyPath != "" {
		certBytes, err := fsys.ReadFile(cfg.CertPath)
		if err != nil {
			return nil, fmt.Errorf("sign: read cert file %s: %w", cfg.CertPath, err)
		}
		keyBytes, err := fsys.ReadFile(cfg.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("sign: read key file %s: %w", cfg.KeyPath, err)
		}
		id, err = macho.LoadIdentityPEMFromBytes(certBytes, keyBytes)
		if err != nil {
			return nil, fmt.Errorf("sign: load identity: %w", err)
		}
	}

	// Prepare entitlements
	var entitlementsBytes []byte
	if cfg.Entitlements != "" {
		trimmed := strings.TrimSpace(cfg.Entitlements)
		if strings.HasPrefix(trimmed, "<") {
			entitlementsBytes = []byte(cfg.Entitlements)
		} else {
			data, err := fsys.ReadFile(cfg.Entitlements)
			if err != nil {
				return nil, fmt.Errorf("sign: read entitlements file %s: %w", cfg.Entitlements, err)
			}
			entitlementsBytes = data
		}
	}

	// Check if target is a bundle directory
	info, err := fsys.Stat(cfg.Target)
	if err != nil {
		return nil, fmt.Errorf("sign: stat target %s: %w", cfg.Target, err)
	}

	isBundle := info.IsDir() || strings.HasSuffix(cfg.Target, ".app") || strings.HasSuffix(cfg.Target, ".framework")

	res := &Result{
		Target:     cfg.Target,
		IsBundle:   isBundle,
		SignedList: make([]string, 0),
	}

	if isBundle {
		return signBundle(ctx, cfg, fsys, id, entitlementsBytes, res)
	}

	// Single binary
	signOpts := macho.Options{
		Identifier:   cfg.Identifier,
		TeamID:       cfg.TeamID,
		Identity:     id,
		Force:        true,
		Hardened:     cfg.Hardened,
		Entitlements: entitlementsBytes,
	}

	binBytes, err := fsys.ReadFile(cfg.Target)
	if err != nil {
		return nil, fmt.Errorf("sign: read binary %s: %w", cfg.Target, err)
	}

	signed, err := macho.SignImage(binBytes, signOpts)
	if err != nil {
		return nil, fmt.Errorf("sign: %w", err)
	}

	if err := fsys.WriteFile(cfg.Target, signed, 0755); err != nil {
		return nil, fmt.Errorf("sign: write binary %s: %w", cfg.Target, err)
	}

	parsed, _ := macho.Parse(signed)
	format := "Mach-O"
	if parsed != nil {
		format = parsed.FormatString()
	}
	res.Format = format
	res.Identifier = cfg.Identifier
	if res.Identifier == "" {
		res.Identifier = filepath.Base(cfg.Target)
	}
	res.SignedList = append(res.SignedList, cfg.Target)
	return res, nil
}

func signBundle(ctx context.Context, cfg Config, fsys vfs.FS, id *macho.Identity, entitlements []byte, res *Result) (*Result, error) {
	contentsDir := path.Join(cfg.Target, "Contents")

	// 1. Deep signing: nested frameworks, dylibs, helper tools
	if cfg.Deep {
		nestedDirs := []string{
			path.Join(contentsDir, "Frameworks"),
			path.Join(contentsDir, "PlugIns"),
			path.Join(contentsDir, "Helpers"),
			path.Join(contentsDir, "XPCServices"),
		}
		for _, nd := range nestedDirs {
			_ = fsys.WalkDir(nd, func(p string, d fs.DirEntry, err error) error {
				if err != nil || d == nil || d.IsDir() {
					return nil
				}
				ext := strings.ToLower(filepath.Ext(p))
				if ext == ".dylib" || ext == ".so" || (!strings.Contains(filepath.Base(p), ".") && d.Type().IsRegular()) {
					raw, readErr := fsys.ReadFile(p)
					if readErr != nil {
						return nil
					}
					// Verify it's a Mach-O
					if !isMachO(raw) {
						return nil
					}
					nestedOpts := macho.Options{
						Identifier: filepath.Base(p),
						TeamID:     cfg.TeamID,
						Identity:   id,
						Force:      true,
						Hardened:   cfg.Hardened,
					}
					signed, sErr := macho.SignImage(raw, nestedOpts)
					if sErr == nil {
						_ = fsys.WriteFile(p, signed, 0755)
						res.SignedList = append(res.SignedList, p)
					}
				}
				return nil
			})
		}
	}

	// 2. Seal CodeResources
	if err := coderesources.SealBundle(cfg.Target, fsys); err != nil {
		return nil, fmt.Errorf("sign: seal bundle: %w", err)
	}
	res.SignedList = append(res.SignedList, path.Join(contentsDir, "_CodeSignature", "CodeResources"))

	// 3. Inspect Info.plist
	infoPlistPath := path.Join(contentsDir, "Info.plist")
	var execName string
	var bundleIdent string
	if plistData, err := fsys.ReadFile(infoPlistPath); err == nil {
		if val, ok := plist.Lookup(plistData, "CFBundleExecutable"); ok {
			execName = fmt.Sprint(val)
		}
		if val, ok := plist.Lookup(plistData, "CFBundleIdentifier"); ok {
			bundleIdent = fmt.Sprint(val)
		}
	}

	if cfg.Identifier != "" {
		res.Identifier = cfg.Identifier
	} else if bundleIdent != "" {
		res.Identifier = bundleIdent
	} else {
		res.Identifier = filepath.Base(cfg.Target)
	}

	// 4. Locate and sign main executable
	macosDir := path.Join(contentsDir, "MacOS")
	var mainExecPath string
	if execName != "" {
		mainExecPath = path.Join(macosDir, execName)
	} else {
		entries, err := fsys.ReadDir(macosDir)
		if err == nil && len(entries) > 0 {
			mainExecPath = path.Join(macosDir, entries[0].Name())
		}
	}

	if mainExecPath != "" {
		binData, err := fsys.ReadFile(mainExecPath)
		if err != nil {
			return nil, fmt.Errorf("sign: read main executable %s: %w", mainExecPath, err)
		}

		if isMachO(binData) {
			signOpts := macho.Options{
				Identifier:   res.Identifier,
				TeamID:       cfg.TeamID,
				Identity:     id,
				Force:        true,
				Hardened:     cfg.Hardened,
				Entitlements: entitlements,
			}

			signed, err := macho.SignImage(binData, signOpts)
			if err != nil {
				return nil, fmt.Errorf("sign main executable %s: %w", mainExecPath, err)
			}

			if err := fsys.WriteFile(mainExecPath, signed, 0755); err != nil {
				return nil, fmt.Errorf("sign: write executable %s: %w", mainExecPath, err)
			}

			parsed, _ := macho.Parse(signed)
			if parsed != nil {
				res.Format = parsed.FormatString()
			}
			res.SignedList = append(res.SignedList, mainExecPath)
		}
	}

	return res, nil
}

func isMachO(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	magic := []byte{data[0], data[1], data[2], data[3]}
	// MH_MAGIC, MH_CIGAM, MH_MAGIC_64, MH_CIGAM_64, FAT_MAGIC, FAT_CIGAM
	return bytes.Equal(magic, []byte{0xfe, 0xed, 0xfa, 0xce}) ||
		bytes.Equal(magic, []byte{0xce, 0xfa, 0xed, 0xfe}) ||
		bytes.Equal(magic, []byte{0xfe, 0xed, 0xfa, 0xcf}) ||
		bytes.Equal(magic, []byte{0xcf, 0xfa, 0xed, 0xfe}) ||
		bytes.Equal(magic, []byte{0xca, 0xfe, 0xba, 0xbe}) ||
		bytes.Equal(magic, []byte{0xbe, 0xba, 0xfe, 0xca})
}
