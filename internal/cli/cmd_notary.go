package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/vertex-language/macpkg/notary"
	"github.com/vertex-language/macpkg/notary/client"
	"github.com/vertex-language/macpkg/notary/staple"
	"github.com/vertex-language/macpkg/vfs"
)

func runNotary(ctx context.Context, env Env, args []string) int {
	if len(args) == 0 {
		printNotaryUsage(env.Out)
		return 0
	}
	subcmd := args[0]
	subArgs := args[1:]

	switch subcmd {
	case "help", "--help", "-h":
		printNotaryUsage(env.Out)
		return 0
	case "submit":
		return runNotarySubmit(ctx, env, subArgs)
	case "staple":
		return runStaple(ctx, env, subArgs)
	case "status":
		return runNotaryStatus(ctx, env, subArgs)
	case "log", "logs":
		return runNotaryLogs(ctx, env, subArgs)
	default:
		fmt.Fprintf(env.Err, "unknown notary subcommand: %s\nRun 'macpkg notary --help' for usage.\n", subcmd)
		return 1
	}
}

func printNotaryUsage(w io.Writer) {
	fmt.Fprint(w, `Usage: macpkg notary <subcommand> [flags]

Subcommands:
  submit    Submit an artifact (.zip, .dmg, .pkg) to Apple Notary API v2
  staple    Staple a notarization ticket to .app, .dmg, or .pkg
  status    Check status of an existing submission
  log       Fetch developer log URL for a submission

Run 'macpkg notary <subcommand> --help' for flags.
`)
}

func runNotarySubmit(ctx context.Context, env Env, args []string) int {
	fs := flag.NewFlagSet("notary submit", flag.ContinueOnError)
	fs.SetOutput(env.Err)

	issuer := fs.String("issuer", "", "App Store Connect API Issuer ID (UUID)")
	keyID := fs.String("key-id", "", "Key ID (e.g. 2X9R4NN92Z)")
	keyPath := fs.String("key", "", "Path to AuthKey_<KeyID>.p8 private key file")
	wait := fs.Bool("wait", false, "Wait for notarization to finish")
	timeout := fs.Duration("timeout", 30*time.Minute, "Timeout when waiting")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if fs.NArg() == 0 {
		fmt.Fprintln(env.Err, "error: artifact path is required")
		return 1
	}
	target := fs.Arg(0)

	cfg := notary.Config{
		IssuerID:       *issuer,
		KeyID:          *keyID,
		PrivateKeyPath: *keyPath,
		Target:         target,
		Wait:           *wait,
		Timeout:        *timeout,
		FS:             vfs.RealFS(""),
	}

	sub, err := notary.Submit(ctx, cfg)
	if err != nil {
		fmt.Fprintf(env.Err, "notarization submission failed: %v\n", err)
		return 1
	}

	fmt.Fprintf(env.Out, "Submission ID: %s\nStatus: %s\nTarget: %s\n", sub.ID, sub.Status, sub.Target)
	if sub.LogURL != "" {
		fmt.Fprintf(env.Out, "Logs: %s\n", sub.LogURL)
	}
	return 0
}

func runStaple(ctx context.Context, env Env, args []string) int {
	fs := flag.NewFlagSet("staple", flag.ContinueOnError)
	fs.SetOutput(env.Err)

	ticketPath := fs.String("ticket", "", "Path to raw ticket file (optional)")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if fs.NArg() == 0 {
		fmt.Fprintln(env.Err, "error: target (.app, .dmg, or .pkg) is required")
		return 1
	}
	target := fs.Arg(0)

	cfg := staple.Config{
		Target:     target,
		TicketPath: *ticketPath,
		FS:         vfs.RealFS(""),
	}

	if err := staple.Staple(ctx, cfg); err != nil {
		fmt.Fprintf(env.Err, "error stapling ticket to %s: %v\n", target, err)
		return 1
	}

	fmt.Fprintf(env.Out, "Successfully stapled ticket to %s\n", target)
	return 0
}

func runNotaryStatus(ctx context.Context, env Env, args []string) int {
	fs := flag.NewFlagSet("notary status", flag.ContinueOnError)
	fs.SetOutput(env.Err)

	id := fs.String("id", "", "Submission ID")
	issuer := fs.String("issuer", "", "Issuer ID")
	keyID := fs.String("key-id", "", "Key ID")
	keyPath := fs.String("key", "", "Path to AuthKey_<KeyID>.p8 private key file")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *id == "" || *issuer == "" || *keyID == "" || *keyPath == "" {
		fmt.Fprintln(env.Err, "error: --id, --issuer, --key-id, and --key are required")
		return 1
	}

	keyData, err := os.ReadFile(*keyPath)
	if err != nil {
		fmt.Fprintf(env.Err, "error reading key file: %v\n", err)
		return 1
	}

	c := client.New(*issuer, *keyID, keyData)
	st, err := c.GetStatus(ctx, *id)
	if err != nil {
		fmt.Fprintf(env.Err, "error fetching status: %v\n", err)
		return 1
	}

	fmt.Fprintf(env.Out, "Submission ID: %s\nStatus: %s\nName: %s\nCreated: %s\n",
		st.Data.ID, st.Data.Attributes.Status, st.Data.Attributes.Name, st.Data.Attributes.CreatedDate.Format(time.RFC3339))
	return 0
}

func runNotaryLogs(ctx context.Context, env Env, args []string) int {
	fs := flag.NewFlagSet("notary log", flag.ContinueOnError)
	fs.SetOutput(env.Err)

	id := fs.String("id", "", "Submission ID")
	issuer := fs.String("issuer", "", "Issuer ID")
	keyID := fs.String("key-id", "", "Key ID")
	keyPath := fs.String("key", "", "Path to AuthKey_<KeyID>.p8 private key file")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *id == "" || *issuer == "" || *keyID == "" || *keyPath == "" {
		fmt.Fprintln(env.Err, "error: --id, --issuer, --key-id, and --key are required")
		return 1
	}

	keyData, err := os.ReadFile(*keyPath)
	if err != nil {
		fmt.Fprintf(env.Err, "error reading key file: %v\n", err)
		return 1
	}

	c := client.New(*issuer, *keyID, keyData)
	logsURL, err := c.GetLogs(ctx, *id)
	if err != nil {
		fmt.Fprintf(env.Err, "error fetching logs: %v\n", err)
		return 1
	}

	fmt.Fprintf(env.Out, "Submission Logs URL: %s\n", logsURL)
	return 0
}
