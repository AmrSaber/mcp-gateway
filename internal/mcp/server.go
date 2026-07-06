// Package mcp wires the lazy-mcp proxy's meta-tools onto an MCP server. These
// are thin controllers: each handler validates/adapts and delegates to the
// proxy.Manager service.
package mcp

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"lazy-mcp/internal/proxy"
)

// Register wires the three meta-tools onto Server, backed by Manager.
func Register(Server *mcp.Server, Manager *proxy.Manager) {
	mcp.AddTool(Server, searchTool(), searchHandler(Manager))
	mcp.AddTool(Server, describeTool(), describeHandler(Manager))
	mcp.AddTool(Server, callTool(), callHandler(Manager))
}
