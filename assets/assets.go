// Package assets embeds the agent-integration files shipped by mcp-gateway.
package assets

import _ "embed"

// OpencodePlugin is the mcp-gateway opencode plugin, written to
// ~/.config/opencode/plugins/mcp-gateway-inject.ts by `mcp-gateway agent setup opencode`.
//
//go:embed mcp-gateway-inject.ts
var OpencodePlugin string
