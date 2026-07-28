// Package cmd contains the mcp-gateway CLI commands. These are thin controllers:
// they parse args and delegate to the proxy service.
package cmd

import (
	"bytes"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// NewRootCmd builds the root cobra command.
func NewRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:     "mcp-gateway",
		Short:   "MCP gateway — front many MCP servers behind a small meta-tool surface",
		Version: resolveVersion(version),
	}

	root.AddCommand(newAgentCmd())
	root.AddCommand(newServersCmd())

	return root
}

// resolveVersion returns the injected build version, falls back to git
// describe, and appends '+' if the working tree is dirty.
func resolveVersion(injected string) string {
	if injected != "" {
		return injected
	}

	if _, err := os.Stat(".git"); err != nil {
		return "unknown"
	}

	tag, err := exec.Command("git", "describe", "--tags").Output()
	if err != nil {
		return "unknown"
	}

	tag = bytes.TrimSpace(tag)

	status, _ := exec.Command("git", "status", "--porcelain").Output()
	if len(bytes.TrimSpace(status)) > 0 {
		tag = append(tag, '+')
	}

	return string(tag)
}
