package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcp-gateway/internal/proxy"
)

// DescribeArgs are the inputs to mcp_describe.
type DescribeArgs struct {
	Server string `json:"server" jsonschema:"the gated server the tool belongs to (from mcp_search results)."`
	Tool   string `json:"tool" jsonschema:"the tool name to describe (from mcp_search results)."`
}

func describeTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "mcp_describe",
		Description: "Get the full input schema (parameters) for a specific lazy-loaded tool, " +
			"identified by its server and name (as returned by mcp_search). " +
			"Use this before mcp_call to learn what arguments the tool expects.",
	}
}

func describeHandler(manager *proxy.Manager) func(context.Context, *mcp.CallToolRequest, DescribeArgs) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, args DescribeArgs) (*mcp.CallToolResult, any, error) {
		schema, err := manager.Describe(ctx, args.Server, args.Tool)
		if err != nil {
			return nil, nil, fmt.Errorf("describing tool: %w", err)
		}

		out, err := json.Marshal(schema)
		if err != nil {
			return nil, nil, fmt.Errorf("marshalling schema: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(out)}},
		}, nil, nil
	}
}
