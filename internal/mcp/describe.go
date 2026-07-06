package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"lazy-mcp/internal/proxy"
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

func describeHandler(Manager *proxy.Manager) func(context.Context, *mcp.CallToolRequest, DescribeArgs) (*mcp.CallToolResult, any, error) {
	return func(Ctx context.Context, _ *mcp.CallToolRequest, Args DescribeArgs) (*mcp.CallToolResult, any, error) {
		Schema, Err := Manager.Describe(Ctx, Args.Server, Args.Tool)
		if Err != nil {
			return nil, nil, fmt.Errorf("describing tool: %w", Err)
		}

		Out, Err := json.Marshal(Schema)
		if Err != nil {
			return nil, nil, fmt.Errorf("marshalling schema: %w", Err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(Out)}},
		}, nil, nil
	}
}
