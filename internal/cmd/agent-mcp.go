package cmd

import (
	"context"
	"fmt"
	"log"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"

	mcptools "mcp-gateway/internal/mcp"
	"mcp-gateway/internal/proxy"
)

func newAgentMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start the mcp-gateway proxy server (stdio)",
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
	// Connect eager servers in the background so the gateway answers MCP
	// requests immediately instead of blocking startup on every downstream
	// handshake. Servers not yet connected are ensured (and deduped against
	// this background work) on first use. Connection errors surface when a
	// server is first touched rather than at boot, so one broken downstream
	// no longer blocks the whole gateway.
	go func() {
		if err := manager.Start(ctx); err != nil {
			log.Printf("connecting downstream servers: %v", err)
		}
	}()
	defer manager.Close()

	server := mcp.NewServer(&mcp.Implementation{Name: "mcp-gateway", Version: "0.1.0"}, nil)
	mcptools.Register(server, manager)

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		return fmt.Errorf("mcp server error: %w", err)
	}

	return nil
}
