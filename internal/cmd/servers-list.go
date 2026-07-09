package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"lazy-mcp/internal/proxy"
)

func newServersListCmd() *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the gated servers and their descriptions",
		Long: "List the gated (lazy-loaded) MCP servers and their descriptions.\n\n" +
			"Default output is YAML for readability. Use -o json for the machine-readable\n" +
			"form the opencode plugin parses to inject the server list into the agent.",
		RunE: func(_ *cobra.Command, _ []string) error {
			if output != "yaml" && output != "json" {
				return fmt.Errorf("invalid output format %q: expected yaml or json", output)
			}

			config, err := proxy.LoadConfig()
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}

			servers := proxy.NewManager(config).Servers()

			var rendered []byte
			if output == "json" {
				// Stable, non-nil array so the plugin always gets valid JSON.
				if servers == nil {
					servers = []proxy.ServerInfo{}
				}
				rendered, err = json.Marshal(servers)
			} else {
				rendered, err = yaml.Marshal(servers)
			}
			if err != nil {
				return fmt.Errorf("rendering servers: %w", err)
			}

			fmt.Print(string(rendered))
			if output == "json" {
				fmt.Println()
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "yaml", "Output format: yaml or json")

	return cmd
}
