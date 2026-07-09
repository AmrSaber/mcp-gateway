package mcp

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"lazy-mcp/internal/proxy"
)

// CallArgs are the inputs to mcp_call.
type CallArgs struct {
	Server string         `json:"server" jsonschema:"the gated server the tool belongs to (from mcp_search results)."`
	Tool   string         `json:"tool" jsonschema:"the tool name to invoke (from mcp_search results)."`
	Args   map[string]any `json:"args,omitempty" jsonschema:"arguments to pass to the tool, matching its input schema (see mcp_describe)."`
}

func callTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "mcp_call",
		Description: "Invoke a lazy-loaded tool on a gated server, identified by server and name " +
			"(as returned by mcp_search). Pass the tool's arguments in 'args' " +
			"(see mcp_describe for its schema). Returns the tool's result verbatim.",
	}
}

func callHandler(manager *proxy.Manager) func(context.Context, *mcp.CallToolRequest, CallArgs) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, args CallArgs) (*mcp.CallToolResult, any, error) {
		result, err := manager.Call(ctx, args.Server, args.Tool, args.Args)
		if err != nil {
			return nil, nil, fmt.Errorf("calling tool: %w", err)
		}

		return result, nil, nil
	}
}
