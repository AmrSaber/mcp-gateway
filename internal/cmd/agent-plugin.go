package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"mcp-gateway/assets"
)

func newAgentPluginCmd() *cobra.Command {
	var path string

	cmd := &cobra.Command{
		Use:       "plugin <agent-name>",
		Short:     fmt.Sprintf("Print the mcp-gateway plugin for an agent to stdout, or write it to a file (%s)", strings.Join(supportedAgents, ", ")),
		Args:      cobra.ExactArgs(1),
		ValidArgs: supportedAgents,
		ValidArgsFunction: func(_ *cobra.Command, _ []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
			return supportedAgents, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(_ *cobra.Command, args []string) error {
			content, err := pluginFor(args[0])
			if err != nil {
				return err
			}

			if path == "" {
				fmt.Print(content)
				return nil
			}
			if err := writeFile(path, content); err != nil {
				return fmt.Errorf("writing plugin file: %w", err)
			}
			fmt.Printf("plugin written to %s\n", path)
			return nil
		},
	}

	cmd.Flags().StringVar(&path, "path", "", "Write the plugin to this file path (created if needed) instead of stdout")

	return cmd
}

// pluginFor returns the plugin source for a supported agent.
func pluginFor(agentName string) (string, error) {
	switch agentName {
	case "opencode":
		return assets.OpencodePlugin, nil
	default:
		return "", fmt.Errorf("unsupported agent %q — supported agents: %s", agentName, strings.Join(supportedAgents, ", "))
	}
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
