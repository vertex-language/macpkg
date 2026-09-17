package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"github.com/vertex-language/macpkg/dmg"
	"github.com/vertex-language/macpkg/vfs"
)

func runDMG(ctx context.Context, env Env, args []string) int {
	if len(args) == 0 {
		printDMGUsage(env.Out)
		return 0
	}
	subcmd := args[0]
	subArgs := args[1:]

	switch subcmd {
	case "help", "--help", "-h":
		printDMGUsage(env.Out)
		return 0
	case "build":
		return runDMGBuild(ctx, env, subArgs)
	default:
		fmt.Fprintf(env.Err, "unknown dmg subcommand: %s\nRun 'macpkg dmg --help' for usage.\n", subcmd)
		return 1
	}
}

func printDMGUsage(w io.Writer) {
	fmt.Fprint(w, `Usage: macpkg dmg <subcommand> [flags]

Subcommands:
  build     Build a compressed Apple Disk Image (.dmg)

Flags for 'build':
  --title        Volume label displayed in Finder (default: App Name)
  --app          Path to .app bundle to package (required)
  --out          Output .dmg file path (default: <Title>.dmg)
  --bg           Path to background PNG image
  --icon-size    Finder icon size in points (default: 128)
  --license      Path to software license agreement (SLA) text file
  --no-apps-link Do not create a symlink to /Applications

Example:
  macpkg dmg build --title "MyApp Installer" --app dist/MyApp.app --out dist/MyApp.dmg
`)
}

func runDMGBuild(ctx context.Context, env Env, args []string) int {
	fs := flag.NewFlagSet("dmg build", flag.ContinueOnError)
	fs.SetOutput(env.Err)

	title := fs.String("title", "", "Volume label")
	appPath := fs.String("app", "", "Path to .app bundle")
	out := fs.String("out", "", "Output .dmg path")
	bg := fs.String("bg", "", "Background image")
	iconSize := fs.Int("icon-size", 128, "Icon size")
	license := fs.String("license", "", "SLA license file")
	noAppsLink := fs.Bool("no-apps-link", false, "Omit Applications link")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *appPath == "" {
		fmt.Fprintln(env.Err, "error: --app is required")
		return 1
	}

	if *title == "" {
		*title = filepath.Base(*appPath)
	}
	if *out == "" {
		*out = *title + ".dmg"
	}

	cfg := dmg.Config{
		Title:                *title,
		SourceApp:            *appPath,
		OutFile:              *out,
		Background:           *bg,
		IconSize:             *iconSize,
		LicenseFile:          *license,
		AddApplicationsLink:  !*noAppsLink,
		WindowSize:           dmg.Size{Width: 640, Height: 480},
		AppPosition:          dmg.Point{X: 180, Y: 240},
		ApplicationsPosition: dmg.Point{X: 460, Y: 240},
		FS:                   vfs.RealFS(""),
	}

	res, err := dmg.Build(ctx, cfg)
	if err != nil {
		fmt.Fprintf(env.Err, "error building .dmg: %v\n", err)
		return 1
	}

	fmt.Fprintf(env.Out, "Successfully built %s (%d bytes)\n", res.OutputFile, res.TotalSize)
	return 0
}
