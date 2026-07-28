package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AmrSaber/mcp-gateway/internal/proxy"
)

// ServersArgs are the inputs to mcp_servers. It takes none.
type ServersArgs struct{}

func serversTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "mcp_servers",
		Description: "List the MCP servers fronted by the gateway, one `name: description` per line. " +
			"Use this to discover what's available, then mcp_search to find tools on them. " +
			"Only needed when the server list has not already been injected into your context.",
	}
}

func serversHandler(manager *proxy.Manager) func(context.Context, *mcp.CallToolRequest, ServersArgs) (*mcp.CallToolResult, any, error) {
	return func(_ context.Context, _ *mcp.CallToolRequest, _ ServersArgs) (*mcp.CallToolResult, any, error) {
		var b strings.Builder
		for _, s := range manager.Servers() {
			fmt.Fprintf(&b, "- %s: %s\n", s.Name, s.Description)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: b.String()}},
		}, nil, nil
	}
}
