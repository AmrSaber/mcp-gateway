package cmd

import "github.com/spf13/cobra"

func newServersCmd() *cobra.Command {
	Cmd := &cobra.Command{
		Use:   "servers",
		Short: "Inspect the gated (lazy-loaded) MCP servers",
	}

	Cmd.AddCommand(newServersListCmd())

	return Cmd
}
