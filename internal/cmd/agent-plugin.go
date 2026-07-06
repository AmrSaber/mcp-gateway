package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"lazy-mcp/assets"
)

func newAgentPluginCmd() *cobra.Command {
	var Path string

	Cmd := &cobra.Command{
		Use:       "plugin <agent-name>",
		Short:     fmt.Sprintf("Print the lazy-mcp plugin for an agent to stdout, or write it to a file (%s)", strings.Join(supportedAgents, ", ")),
		Args:      cobra.ExactArgs(1),
		ValidArgs: supportedAgents,
		ValidArgsFunction: func(_ *cobra.Command, _ []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
			return supportedAgents, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(_ *cobra.Command, Args []string) error {
			Content, Err := pluginFor(Args[0])
			if Err != nil {
				return Err
			}

			if Path == "" {
				fmt.Print(Content)
				return nil
			}
			if Err := writeFile(Path, Content); Err != nil {
				return fmt.Errorf("writing plugin file: %w", Err)
			}
			fmt.Printf("plugin written to %s\n", Path)
			return nil
		},
	}

	Cmd.Flags().StringVar(&Path, "path", "", "Write the plugin to this file path (created if needed) instead of stdout")

	return Cmd
}

// pluginFor returns the plugin source for a supported agent.
func pluginFor(AgentName string) (string, error) {
	switch AgentName {
	case "opencode":
		return assets.OpencodePlugin, nil
	default:
		return "", fmt.Errorf("unsupported agent %q — supported agents: %s", AgentName, strings.Join(supportedAgents, ", "))
	}
}

func writeFile(Path, Content string) error {
	if Err := os.MkdirAll(filepath.Dir(Path), 0o755); Err != nil {
		return Err
	}
	return os.WriteFile(Path, []byte(Content), 0o644)
}
