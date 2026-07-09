package cmd

import "github.com/spf13/cobra"

func newAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Commands for agent integration",
	}

	cmd.AddCommand(newAgentMCPCmd())
	cmd.AddCommand(newAgentPluginCmd())
	cmd.AddCommand(newAgentSetupCmd())

	return cmd
}
