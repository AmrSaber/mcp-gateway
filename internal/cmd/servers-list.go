package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"lazy-mcp/internal/proxy"
)

func newServersListCmd() *cobra.Command {
	var Output string

	Cmd := &cobra.Command{
		Use:   "list",
		Short: "List the gated servers and their descriptions",
		Long: "List the gated (lazy-loaded) MCP servers and their descriptions.\n\n" +
			"Default output is YAML for readability. Use -o json for the machine-readable\n" +
			"form the opencode plugin parses to inject the server list into the agent.",
		RunE: func(_ *cobra.Command, _ []string) error {
			if Output != "yaml" && Output != "json" {
				return fmt.Errorf("invalid output format %q: expected yaml or json", Output)
			}

			Config, Err := proxy.LoadConfig()
			if Err != nil {
				return fmt.Errorf("loading config: %w", Err)
			}

			Servers := proxy.NewManager(Config).Servers()

			var Rendered []byte
			if Output == "json" {
				// Stable, non-nil array so the plugin always gets valid JSON.
				if Servers == nil {
					Servers = []proxy.ServerInfo{}
				}
				Rendered, Err = json.Marshal(Servers)
			} else {
				Rendered, Err = yaml.Marshal(Servers)
			}
			if Err != nil {
				return fmt.Errorf("rendering servers: %w", Err)
			}

			fmt.Print(string(Rendered))
			if Output == "json" {
				fmt.Println()
			}
			return nil
		},
	}

	Cmd.Flags().StringVarP(&Output, "output", "o", "yaml", "Output format: yaml or json")

	return Cmd
}
