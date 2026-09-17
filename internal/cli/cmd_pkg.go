package cli

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/vertex-language/macpkg/pkg"
	"github.com/vertex-language/macpkg/vfs"
)

func runPKG(ctx context.Context, env Env, args []string) int {
	if len(args) == 0 {
		printPKGUsage(env.Out)
		return 0
	}
	subcmd := args[0]
	subArgs := args[1:]

	switch subcmd {
	case "help", "--help", "-h":
		printPKGUsage(env.Out)
		return 0
	case "build":
		return runPKGBuild(ctx, env, subArgs)
	default:
		fmt.Fprintf(env.Err, "unknown pkg subcommand: %s\nRun 'macpkg pkg --help' for usage.\n", subcmd)
		return 1
	}
}

func printPKGUsage(w io.Writer) {
	fmt.Fprint(w, `Usage: macpkg pkg <subcommand> [flags]

Subcommands:
  build     Compile a macOS Flat Package (.pkg) installer

Flags for 'build':
  --id         Package identifier, e.g. com.example.app.pkg (required)
  --version    Package version string (default: 1.0.0)
  --location   Target install location (default: /Applications)
  --payload    Path to source payload directory or .app bundle (required)
  --scripts    Path to optional installer scripts directory
  --out        Output .pkg file path (required)

Example:
  macpkg pkg build --id com.example.app.pkg --payload dist/MyApp.app --out dist/MyApp.pkg
`)
}

func runPKGBuild(ctx context.Context, env Env, args []string) int {
	fs := flag.NewFlagSet("pkg build", flag.ContinueOnError)
	fs.SetOutput(env.Err)

	id := fs.String("id", "", "Package identifier")
	ver := fs.String("version", "1.0.0", "Version string")
	loc := fs.String("location", "/Applications", "Install location")
	payload := fs.String("payload", "", "Source payload directory or bundle")
	scripts := fs.String("scripts", "", "Scripts directory")
	out := fs.String("out", "", "Output .pkg path")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *id == "" || *payload == "" || *out == "" {
		fmt.Fprintln(env.Err, "error: --id, --payload, and --out are required")
		return 1
	}

	cfg := pkg.Config{
		Identifier:      *id,
		Version:         *ver,
		InstallLocation: *loc,
		SourcePayload:   *payload,
		ScriptsDir:      *scripts,
		OutFile:         *out,
		FS:              vfs.RealFS(""),
	}

	res, err := pkg.Build(ctx, cfg)
	if err != nil {
		fmt.Fprintf(env.Err, "error compiling .pkg package: %v\n", err)
		return 1
	}

	fmt.Fprintf(env.Out, "Successfully compiled %s (%d files, %d bytes)\n", res.OutputFile, res.FilesCount, res.TotalSize)
	return 0
}
