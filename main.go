// Command agx is one CLI for your AI coding agents and all their accounts:
// plan usage across every account, launching on the account with the most
// headroom, resuming a conversation on the account that holds it, and
// managing session folders. See README.md.
//
// main only wires the process: signal-aware context, the command tree from
// internal/cli, and the exit code. All behaviour lives in packages.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/khanakia/agx/internal/appmeta"
	"github.com/khanakia/agx/internal/cli"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	deps, err := cli.SystemDeps()
	if err != nil {
		report(err)
		return cli.ExitFailure
	}
	err = cli.NewRoot(deps).ExecuteContext(ctx)
	if err != nil && !cli.IsSilent(err) {
		report(err)
	}
	return cli.ExitCode(err)
}

// report prints err to stderr. A failed stderr write has nowhere left to be
// reported, so its error is deliberately dropped.
func report(err error) {
	_, _ = fmt.Fprintln(os.Stderr, appmeta.Name+": "+err.Error())
}
