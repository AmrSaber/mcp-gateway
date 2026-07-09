package cmd

import "github.com/spf13/cobra"

func newServersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "servers",
		Short: "Inspect the gated (lazy-loaded) MCP servers",
	}

	cmd.AddCommand(newServersListCmd())

	return cmd
}
