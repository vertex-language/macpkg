package macpkg

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/vertex-language/macpkg/app"
	"github.com/vertex-language/macpkg/dmg"
	"github.com/vertex-language/macpkg/notary"
	"github.com/vertex-language/macpkg/notary/staple"
	"github.com/vertex-language/macpkg/pkg"
	"github.com/vertex-language/macpkg/sign"
	"github.com/vertex-language/macpkg/vfs"
)

// Format represents an Apple packaging format.
type Format string

const (
	FormatApp Format = "app"
	FormatDMG Format = "dmg"
	FormatPKG Format = "pkg"
)

// DetectFormat detects format from file extension, path, or bare format name.
func DetectFormat(p string) Format {
	lower := strings.ToLower(strings.TrimSpace(p))
	switch lower {
	case "app":
		return FormatApp
	case "dmg":
		return FormatDMG
	case "pkg":
		return FormatPKG
	}
	switch {
	case strings.HasSuffix(lower, ".app"):
		return FormatApp
	case strings.HasSuffix(lower, ".dmg"):
		return FormatDMG
	case strings.HasSuffix(lower, ".pkg"):
		return FormatPKG
	default:
		ext := strings.TrimPrefix(filepath.Ext(lower), ".")
		switch ext {
		case "app":
			return FormatApp
		case "dmg":
			return FormatDMG
		case "pkg":
			return FormatPKG
		default:
			return ""
		}
	}
}

// Config is the unified build configuration.
type Config struct {
	Format Format         `json:"format"`
	App    *app.Config    `json:"app,omitempty"`
	DMG    *dmg.Config    `json:"dmg,omitempty"`
	PKG    *pkg.Config    `json:"pkg,omitempty"`
	Sign   *sign.Config   `json:"sign,omitempty"`
	Notary *notary.Config `json:"notary,omitempty"`
	FS     vfs.FS         `json:"-"`
}

// BuildResult encapsulates the output of any build step.
type BuildResult struct {
	Format     Format             `json:"format"`
	Artifact   string             `json:"artifact"`
	Bundle     *app.Bundle        `json:"bundle,omitempty"`
	DMGResult  *dmg.BuildResult   `json:"dmg_result,omitempty"`
	PKGResult  *pkg.BuildResult   `json:"pkg_result,omitempty"`
	SignResult *sign.Result       `json:"sign_result,omitempty"`
	NotarySub  *notary.Submission `json:"notary_submission,omitempty"`
}

// Build executes the packaging workflow specified in cfg.
func Build(ctx context.Context, cfg Config) (*BuildResult, error) {
	fsys := cfg.FS
	if fsys == nil {
		fsys = vfs.RealFS("")
	}

	result := &BuildResult{
		Format: cfg.Format,
	}

	switch cfg.Format {
	case FormatApp:
		if cfg.App == nil {
			return nil, fmt.Errorf("macpkg: app configuration is required for format 'app'")
		}
		cfg.App.FS = fsys
		bundle, err := app.Assemble(ctx, *cfg.App)
		if err != nil {
			return nil, fmt.Errorf("macpkg: app assembly failed: %w", err)
		}
		result.Bundle = bundle
		result.Artifact = bundle.Path

	case FormatDMG:
		if cfg.DMG == nil {
			return nil, fmt.Errorf("macpkg: dmg configuration is required for format 'dmg'")
		}
		cfg.DMG.FS = fsys
		dmgRes, err := dmg.Build(ctx, *cfg.DMG)
		if err != nil {
			return nil, fmt.Errorf("macpkg: dmg build failed: %w", err)
		}
		result.DMGResult = dmgRes
		result.Artifact = dmgRes.OutputFile

	case FormatPKG:
		if cfg.PKG == nil {
			return nil, fmt.Errorf("macpkg: pkg configuration is required for format 'pkg'")
		}
		cfg.PKG.FS = fsys
		pkgRes, err := pkg.Build(ctx, *cfg.PKG)
		if err != nil {
			return nil, fmt.Errorf("macpkg: pkg build failed: %w", err)
		}
		result.PKGResult = pkgRes
		result.Artifact = pkgRes.OutputFile

	default:
		return nil, fmt.Errorf("macpkg: unknown format '%s'", cfg.Format)
	}

	// Code signing step (if configured)
	if cfg.Sign != nil {
		cfg.Sign.FS = fsys
		if cfg.Sign.Target == "" {
			cfg.Sign.Target = result.Artifact
		}
		signRes, err := sign.Sign(ctx, *cfg.Sign)
		if err != nil {
			return nil, fmt.Errorf("macpkg: signing failed: %w", err)
		}
		result.SignResult = signRes
	}

	// Notarization step (if configured)
	if cfg.Notary != nil {
		cfg.Notary.FS = fsys
		if cfg.Notary.Target == "" {
			cfg.Notary.Target = result.Artifact
		}
		sub, err := notary.Submit(ctx, *cfg.Notary)
		if err != nil {
			return nil, fmt.Errorf("macpkg: notarization failed: %w", err)
		}
		result.NotarySub = sub

		// Staple if wait was true and succeeded
		if cfg.Notary.Wait && sub.Status == "Accepted" {
			_ = staple.Staple(ctx, staple.Config{
				Target: result.Artifact,
				FS:     fsys,
			})
		}
	}

	return result, nil
}

// Convenience helpers
func RealFS(root string) vfs.FS { return vfs.RealFS(root) }
func MemFS() vfs.FS             { return vfs.NewMemFS() }

func Sign(ctx context.Context, cfg sign.Config) (*sign.Result, error) {
	return sign.Sign(ctx, cfg)
}

func Notarize(ctx context.Context, cfg notary.Config) (*notary.Submission, error) {
	return notary.Submit(ctx, cfg)
}

func Staple(ctx context.Context, cfg staple.Config) error {
	return staple.Staple(ctx, cfg)
}
