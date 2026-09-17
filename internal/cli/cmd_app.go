package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/vertex-language/macpkg/app"
	"github.com/vertex-language/macpkg/vfs"
)

func runApp(ctx context.Context, env Env, args []string) int {
	if len(args) == 0 {
		printAppUsage(env.Out)
		return 0
	}
	subcmd := args[0]
	subArgs := args[1:]

	switch subcmd {
	case "help", "--help", "-h":
		printAppUsage(env.Out)
		return 0
	case "build":
		return runAppBuild(ctx, env, subArgs)
	default:
		fmt.Fprintf(env.Err, "unknown app subcommand: %s\nRun 'macpkg app --help' for usage.\n", subcmd)
		return 1
	}
}

func printAppUsage(w io.Writer) {
	fmt.Fprint(w, `Usage: macpkg app <subcommand> [flags]

Subcommands:
  build     Assemble a .app bundle from binary and resources

Flags for 'build':
  --name        Application display name (required)
  --id          Bundle identifier, e.g. com.example.myapp (required)
  --version     Bundle version string (default: 1.0.0)
  --build       Bundle build number (default: 1)
  --bin         Path to executable binary (required)
  --icon        Path to application icon (.icns or .png)
  --category    Application category (e.g. public.app-category.developer-tools)
  --min-os      Minimum macOS version (default: 11.0)
  --out         Output bundle path (default: <name>.app)

Example:
  macpkg app build --name MyApp --id com.example.myapp --bin ./bin/myapp
`)
}

func runAppBuild(ctx context.Context, env Env, args []string) int {
	fs := flag.NewFlagSet("app build", flag.ContinueOnError)
	fs.SetOutput(env.Err)

	name := fs.String("name", "", "Application name")
	id := fs.String("id", "", "Bundle identifier")
	ver := fs.String("version", "1.0.0", "Version string")
	bld := fs.String("build", "1", "Build number")
	bin := fs.String("bin", "", "Path to executable binary")
	icon := fs.String("icon", "", "Path to icon file")
	cat := fs.String("category", "", "Category")
	minOS := fs.String("min-os", "11.0", "Minimum OS version")
	out := fs.String("out", "", "Output directory")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *name == "" || *id == "" {
		fmt.Fprintln(env.Err, "error: --name and --id are required")
		return 1
	}

	cfg := app.Config{
		Name:         *name,
		Identifier:   *id,
		Version:      *ver,
		Build:        *bld,
		SourceBinary: *bin,
		SourceIcon:   *icon,
		Category:     *cat,
		MinOS:        *minOS,
		OutDir:       *out,
		FS:           vfs.RealFS(""),
	}

	bundle, err := app.Assemble(ctx, cfg)
	if err != nil {
		fmt.Fprintf(env.Err, "error assembling .app bundle: %v\n", err)
		return 1
	}

	fmt.Fprintf(env.Out, "Successfully assembled %s (%d files, %d bytes)\n", bundle.Path, len(bundle.Files), bundle.TotalSize)
	return 0
}

var _ = os.Args
