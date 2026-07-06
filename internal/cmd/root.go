// Package cmd contains the lazy-mcp CLI commands. These are thin controllers:
// they parse args and delegate to the proxy service.
package cmd

import (
	"bytes"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// NewRootCmd builds the root cobra command.
func NewRootCmd(Version string) *cobra.Command {
	Root := &cobra.Command{
		Use:     "lazy-mcp",
		Short:   "Lazy-loading MCP proxy — gate heavy MCP servers behind a small meta-tool surface",
		Version: resolveVersion(Version),
	}

	Root.AddCommand(newAgentCmd())
	Root.AddCommand(newServersCmd())

	return Root
}

// resolveVersion returns the injected build version, falls back to git
// describe, and appends '+' if the working tree is dirty.
func resolveVersion(Injected string) string {
	if Injected != "" {
		return Injected
	}

	if _, Err := os.Stat(".git"); Err != nil {
		return "unknown"
	}

	Tag, Err := exec.Command("git", "describe", "--tags").Output()
	if Err != nil {
		return "unknown"
	}

	Tag = bytes.TrimSpace(Tag)

	Status, _ := exec.Command("git", "status", "--porcelain").Output()
	if len(bytes.TrimSpace(Status)) > 0 {
		Tag = append(Tag, '+')
	}

	return string(Tag)
}
