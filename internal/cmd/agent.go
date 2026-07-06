package cmd

import "github.com/spf13/cobra"

func newAgentCmd() *cobra.Command {
	Cmd := &cobra.Command{
		Use:   "agent",
		Short: "Commands for agent integration",
	}

	Cmd.AddCommand(newAgentMCPCmd())
	Cmd.AddCommand(newAgentPluginCmd())
	Cmd.AddCommand(newAgentSetupCmd())

	return Cmd
}
