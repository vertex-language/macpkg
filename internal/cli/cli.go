package cli

import (
	"context"
	"fmt"
	"io"
	"os"
)

// Version is the current version of the macpkg toolchain.
const Version = "1.0.0"

// Env encapsulates process execution context.
type Env struct {
	Args []string
	Dir  string
	Env  []string
	In   io.Reader
	Out  io.Writer
	Err  io.Writer
}

// Run executes the CLI using standard OS arguments and terminates the process.
func Run() {
	os.Exit(Main(context.Background(), Env{
		Args: os.Args[1:],
		Dir:  ".",
		Env:  os.Environ(),
		In:   os.Stdin,
		Out:  os.Stdout,
		Err:  os.Stderr,
	}))
}

// Main is the primary entry point for the 'macpkg' CLI.
func Main(ctx context.Context, env Env) int {
	if len(env.Args) == 0 {
		printUsage(env.Out)
		return 0
	}

	cmd := env.Args[0]
	args := env.Args[1:]

	switch cmd {
	case "help", "--help", "-h":
		printUsage(env.Out)
		return 0
	case "version", "--version", "-v":
		fmt.Fprintf(env.Out, "macpkg v%s - Pure-Go macOS Packaging & Signing Toolchain\n", Version)
		return 0

	// Format namespaces
	case "app":
		return runApp(ctx, env, args)
	case "dmg":
		return runDMG(ctx, env, args)
	case "pkg":
		return runPKG(ctx, env, args)
	case "sign":
		return runSign(ctx, env, args)
	case "notary":
		return runNotary(ctx, env, args)
	case "staple":
		return runStaple(ctx, env, args)

	default:
		fmt.Fprintf(env.Err, "unknown command: %s\nRun 'macpkg --help' for usage.\n", cmd)
		return 1
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintf(w, `macpkg v%s - Pure-Go macOS Packaging & Signing Toolchain

Usage:
  macpkg <command> [subcommand] [flags]

Commands:
  app       Assemble macOS Application Bundles (.app)
  dmg       Build Apple Disk Images (.dmg)
  pkg       Compile macOS Flat Packages (.pkg)
  sign      Code-sign Mach-O binaries and bundles
  notary    Apple Notarization REST API v2 and stapler
  staple    Staple notarization ticket to .app, .dmg, or .pkg
  version   Print macpkg version
  help      Display help information

Format Subcommands:
  macpkg app build [flags]
  macpkg dmg build [flags]
  macpkg pkg build [flags]
  macpkg notary submit [flags] <artifact>
  macpkg notary staple [flags] <artifact>
  macpkg notary status [flags]
  macpkg notary log [flags]

Examples:
  macpkg app build --name MyApp --id com.example.app --bin ./myapp
  macpkg dmg build --title "MyApp Installer" --app MyApp.app --out MyApp.dmg
  macpkg pkg build --id com.example.app.pkg --payload MyApp.app --out MyApp.pkg
  macpkg sign --target MyApp.app --identity "-" --hardened
  macpkg notary submit --issuer <id> --key-id <id> --key <auth.p8> --wait MyApp.dmg

Run 'macpkg <command> --help' for details on each subcommand.
`, Version)
}
