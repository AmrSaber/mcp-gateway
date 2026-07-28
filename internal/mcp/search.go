package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcp-gateway/internal/proxy"
)

// SearchArgs are the inputs to mcp_search.
type SearchArgs struct {
	Query  []string `json:"query" jsonschema:"one or more search keywords (required, non-empty); a tool matches if its name, description, or input schema contains ANY term (case-insensitive). Pass individual keywords rather than a full sentence, e.g. [\"event\", \"meeting\", \"calendar\"]."`
	Server string   `json:"server,omitempty" jsonschema:"optional: restrict results to this server."`
	Limit  int      `json:"limit,omitempty" jsonschema:"optional: max results to return. Defaults to 5; may be raised up to 25. Values above 25 are rejected."`
}

func searchTool() *mcp.Tool {
	return &mcp.Tool{
		Name: "mcp_search",
		Description: "Search for tools provided by the MCP servers fronted by the gateway. " +
			"These servers' tools are kept out of your context to save tokens — use this to discover them. " +
			"Pass 'query' as a non-empty list of keywords (not a full sentence); a tool matches if any term " +
			"appears in its name, description, or input schema. " +
			"Results are ranked by how many of your keywords match (broadest coverage first), then by where " +
			"they matched (name > description > schema), and capped at 5 by default (raise 'limit' up to 25). " +
			"Each result is {server, name, description, matched, matchedFields}: 'matched' is which of your " +
			"keywords hit, 'matchedFields' is where they hit. " +
			"NOTE: the input schema is NOT returned here. If 'matchedFields' includes \"input schema\" (or you " +
			"need a tool's parameters), call mcp_describe next — mega-tools often bury their real capabilities " +
			"(routes, options) in parameter descriptions. Then call mcp_call to run the tool.",
	}
}

func searchHandler(manager *proxy.Manager) func(context.Context, *mcp.CallToolRequest, SearchArgs) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, args SearchArgs) (*mcp.CallToolResult, any, error) {
		hits, err := manager.Search(ctx, args.Query, args.Server, args.Limit)
		if err != nil {
			return nil, nil, fmt.Errorf("searching tools: %w", err)
		}

		out, err := json.Marshal(hits)
		if err != nil {
			return nil, nil, fmt.Errorf("marshalling results: %w", err)
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: string(out)}},
		}, nil, nil
	}
}
