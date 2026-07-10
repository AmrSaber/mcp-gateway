// Package mcp wires the mcp-gateway proxy's meta-tools onto an MCP server. These
// are thin controllers: each handler validates/adapts and delegates to the
// proxy.Manager service.
package mcp

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mcp-gateway/internal/proxy"
)

// Register wires the three meta-tools onto Server, backed by Manager.
func Register(server *mcp.Server, manager *proxy.Manager) {
	mcp.AddTool(server, searchTool(), searchHandler(manager))
	mcp.AddTool(server, describeTool(), describeHandler(manager))
	mcp.AddTool(server, callTool(), callHandler(manager))
}
