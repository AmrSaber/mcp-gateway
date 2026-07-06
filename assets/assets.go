// Package assets embeds the agent-integration files shipped by lazy-mcp.
package assets

import _ "embed"

// OpencodePlugin is the lazy-mcp opencode plugin, written to
// ~/.config/opencode/plugins/lazy-mcp-inject.ts by `lazy-mcp agent setup opencode`.
//
//go:embed lazy-mcp-inject.ts
var OpencodePlugin string
