package cli

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/vertex-language/macpkg/sign"
	"github.com/vertex-language/macpkg/vfs"
)

func runSign(ctx context.Context, env Env, args []string) int {
	fs := flag.NewFlagSet("sign", flag.ContinueOnError)
	fs.SetOutput(env.Err)

	target := fs.String("target", "", "Target .app bundle, .framework, or Mach-O binary")
	identity := fs.String("identity", "-", "Signing identity ('-' for ad-hoc)")
	certPath := fs.String("cert", "", "Path to PEM certificate file")
	keyPath := fs.String("key", "", "Path to PEM private key file")
	ident := fs.String("id", "", "Code signing identifier")
	teamID := fs.String("team-id", "", "10-character Apple Developer Team ID")
	ent := fs.String("entitlements", "", "Path to entitlements plist file")
	hardened := fs.Bool("hardened", true, "Enable hardened runtime (CS_RUNTIME)")
	deep := fs.Bool("deep", true, "Recursively sign nested frameworks and helpers")
	force := fs.Bool("force", true, "Overwrite existing signature")

	for _, a := range args {
		if a == "--help" || a == "-h" || a == "help" {
			printSignUsage(env.Out)
			return 0
		}
	}

	if err := fs.Parse(args); err != nil {
		return 0
	}

	targetPath := *target
	if targetPath == "" && fs.NArg() > 0 {
		targetPath = fs.Arg(0)
	}

	if targetPath == "" {
		printSignUsage(env.Out)
		return 1
	}

	cfg := sign.Config{
		Target:       targetPath,
		Identity:     *identity,
		CertPath:     *certPath,
		KeyPath:      *keyPath,
		Identifier:   *ident,
		TeamID:       *teamID,
		Entitlements: *ent,
		Hardened:     *hardened,
		Deep:         *deep,
		Force:        *force,
		FS:           vfs.RealFS(""),
	}

	res, err := sign.Sign(ctx, cfg)
	if err != nil {
		fmt.Fprintf(env.Err, "error signing target %s: %v\n", targetPath, err)
		return 1
	}

	fmt.Fprintf(env.Out, "Successfully signed %s (Identifier=%s, Format=%s, Signed=%d items)\n",
		res.Target, res.Identifier, res.Format, len(res.SignedList))
	return 0
}

func printSignUsage(w io.Writer) {
	fmt.Fprint(w, `Usage: macpkg sign [flags] <target>

Flags:
  --target         Path to .app bundle, .framework, or Mach-O binary
  --identity       Signing identity ('-' for ad-hoc, default: '-')
  --cert           Path to PEM certificate file
  --key            Path to PEM private key file
  --id             Code signing identifier (default: Info.plist or binary name)
  --team-id        10-character Apple Developer Team ID
  --entitlements   Path to entitlements plist file
  --hardened       Enable hardened runtime (default: true)
  --deep           Recursively sign nested frameworks and helpers (default: true)
  --force          Overwrite existing signature (default: true)

Example:
  macpkg sign --target MyApp.app --identity "-" --hardened
`)
}
