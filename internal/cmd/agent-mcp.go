package cmd

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	mcptools "lazy-mcp/internal/mcp"
	"lazy-mcp/internal/proxy"
)

func newAgentMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start the lazy-mcp proxy server (stdio)",
		RunE:  runAgentMCP,
	}
}

func runAgentMCP(Cmd *cobra.Command, _ []string) error {
	Ctx := Cmd.Context()
	if Ctx == nil {
		Ctx = context.Background()
	}

	Config, Err := proxy.LoadConfig()
	if Err != nil {
		return fmt.Errorf("loading config: %w", Err)
	}

	Manager := proxy.NewManager(Config)
	if Err := Manager.Start(Ctx); Err != nil {
		return fmt.Errorf("starting downstream servers: %w", Err)
	}
	defer Manager.Close()

	Server := mcp.NewServer(&mcp.Implementation{Name: "lazy-mcp", Version: "0.1.0"}, nil)
	mcptools.Register(Server, Manager)

	if Err := Server.Run(Ctx, &mcp.StdioTransport{}); Err != nil {
		return fmt.Errorf("mcp server error: %w", Err)
	}

	return nil
}
