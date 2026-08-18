package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/AmrSaber/mcp-gateway/internal/cmd"
)

// version is injected at build time via ldflags: -X main.version=<tag>
var version string

func main() {
	// Cancel on SIGINT/SIGTERM so server.Run returns and defer Close runs.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := cmd.NewRootCmd(version).ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
