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
		RunE: func(_ *cobra.Command, Args []string) error {
			AgentName := Args[0]

			Content, Err := pluginFor(AgentName)
			if Err != nil {
				return Err
			}

			switch AgentName {
			case "opencode":
				ConfigBase, Err := os.UserConfigDir()
				if Err != nil {
					return fmt.Errorf("resolving config directory: %w", Err)
				}

				PluginPath := filepath.Join(ConfigBase, "opencode", "plugins", "lazy-mcp-inject.ts")
				if Err := writeFile(PluginPath, Content); Err != nil {
					return fmt.Errorf("writing plugin: %w", Err)
				}
				fmt.Printf("plugin written to %s\n", PluginPath)
				return nil

			default:
				return fmt.Errorf("unsupported agent %q — supported agents: %s", AgentName, strings.Join(supportedAgents, ", "))
			}
		},
	}
}
