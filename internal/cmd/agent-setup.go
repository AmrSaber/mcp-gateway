package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var supportedAgents = []string{"opencode"}

func newAgentSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "setup <agent-name>",
		Short:     fmt.Sprintf("Install lazy-mcp's integration for a supported agent (%s)", strings.Join(supportedAgents, ", ")),
		Args:      cobra.ExactArgs(1),
		ValidArgs: supportedAgents,
		ValidArgsFunction: func(_ *cobra.Command, _ []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
			return supportedAgents, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(_ *cobra.Command, args []string) error {
			agentName := args[0]

			content, err := pluginFor(agentName)
			if err != nil {
				return err
			}

			switch agentName {
			case "opencode":
				configBase, err := os.UserConfigDir()
				if err != nil {
					return fmt.Errorf("resolving config directory: %w", err)
				}

				pluginPath := filepath.Join(configBase, "opencode", "plugins", "lazy-mcp-inject.ts")
				if err := writeFile(pluginPath, content); err != nil {
					return fmt.Errorf("writing plugin: %w", err)
				}
				fmt.Printf("plugin written to %s\n", pluginPath)
				return nil

			default:
				return fmt.Errorf("unsupported agent %q — supported agents: %s", agentName, strings.Join(supportedAgents, ", "))
			}
		},
	}
}
