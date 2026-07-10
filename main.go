package main

import (
	"os"

	"mcp-gateway/internal/cmd"
)

// version is injected at build time via ldflags: -X main.version=<tag>
var version string

func main() {
	if err := cmd.NewRootCmd(version).Execute(); err != nil {
		os.Exit(1)
	}
}
