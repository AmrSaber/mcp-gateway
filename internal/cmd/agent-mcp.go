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

func runAgentMCP(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	config, err := proxy.LoadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	manager := proxy.NewManager(config)
	if err := manager.Start(ctx); err != nil {
		return fmt.Errorf("starting downstream servers: %w", err)
	}
	defer manager.Close()

	server := mcp.NewServer(&mcp.Implementation{Name: "lazy-mcp", Version: "0.1.0"}, nil)
	mcptools.Register(server, manager)

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("mcp server error: %w", err)
	}

	return nil
}
