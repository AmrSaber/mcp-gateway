# lazy-mcp

A lazy-loading MCP proxy. It hides the tool schemas of heavy MCP servers from
the agent's context to save tokens, exposing instead a small fixed surface of
meta-tools the agent uses to discover and call the hidden tools on demand.

## The problem

Every MCP server an agent connects to injects all of its tool schemas into the
LLM context at startup. A couple of large servers can eat 50–70k tokens before
the first message. Most of those tools are irrelevant to any given session.

## How it works

`lazy-mcp` runs as a single MCP server (the proxy). The agent sees only three
meta-tools instead of the ~dozens of tools the gated servers actually expose:

- `mcp_search({ query, server?, limit? })` — find gated tools by keyword.
  `query` is a required, non-empty list of keywords; a tool matches if any term
  appears in its name, description, or input schema (case-insensitive). Results
  are ranked **breadth-first**: by how many distinct keywords match (coverage of
  intent), then by where they matched (name 6 > description 3 > schema 1). Capped
  at 5 by default; `limit` may be raised up to 25 (higher is rejected). An empty
  query is rejected — the point is to narrow, not dump every tool. Returns
  `{ server, name, description, matched, matchedFields }` — `matched` is which
  keywords hit, `matchedFields` is where. The input schema is **not** returned
  (see `mcp_describe`).
- `mcp_describe({ server, tool })` — full input schema for one tool. Needed when
  a search hit's `matchedFields` includes `"input schema"`: mega-tools that wrap
  many sub-operations bury their real capabilities — routes, options — in
  parameter descriptions, which search matches on but does not return.
- `mcp_call({ server, tool, args })` — invoke a gated tool, returns its result.

The proxy spawns each gated server as a persistent subprocess and proxies calls
to it over stdio, so in-memory server state survives across calls within a
session.

A companion opencode plugin injects the list of gated servers (name +
description) into context each turn, so the agent knows what exists without
paying for the full tool schemas. It does **not** inject the tools themselves.

## Configuration: `lazy-mcp.json`

Lives next to `opencode.json` in the opencode config dir
(`$OPENCODE_CONFIG_DIR`, else `$XDG_CONFIG_HOME/opencode`, else
`~/.config/opencode`).

```jsonc
{
  "servers": {
    "issues-mcp": {
      "command": ["sh", "-c", "API_TOKEN=\"$(get-secret issues:token)\" exec issues-mcp-server"],
      "environment": { "API_URL": "https://issues.example.com/api" },
      "spawn": "eager",
      "timeout": "30s",
      "enabled": true,
      "description": "Issue tracker: issues, comments, labels, milestones"
    },
    "docs-mcp": {
      "command": ["docs-mcp-server"],
      "description": "Docs and knowledge base: search, read pages"
    }
  }
}
```

| Field | Required | Default | Meaning |
|-------|----------|---------|---------|
| `command` | yes | — | argv to spawn the server (local stdio) |
| `environment` | no | — | env vars added on top of the inherited environment |
| `spawn` | no | `eager` | `eager` (connect at startup) or `lazy` (connect on first use) |
| `timeout` | no | `30s` | connect timeout: Go duration (`1h30m12s`) or a bare number of seconds |
| `enabled` | no | `true` | `false` skips the server entirely |
| `description` | no | — | shown in `servers list`, plugin injection, and search |

## CLI

```
lazy-mcp agent mcp                     # run the proxy (stdio) — this is the opencode mcp entry
lazy-mcp agent plugin <agent> [--path] # print or write the plugin for an agent (e.g. opencode)
lazy-mcp agent setup <agent>           # install the plugin into the agent's config dir
lazy-mcp servers list [-o yaml|json]   # list gated servers (yaml default; json for the plugin)
```

## opencode wiring

Add the proxy to `opencode.json` and remove the gated servers from its `mcp`
block:

```jsonc
"mcp": {
  "lazy-mcp": { "type": "local", "command": ["lazy-mcp", "agent", "mcp"], "enabled": true }
}
```

Then `lazy-mcp agent setup opencode` to install the injection plugin.

## Future work

- **Per-server tool allow/deny scoping.** Restrict which of a server's tools are
  exposed (e.g. block destructive tools). Intended shape: an opencode-style
  ordered pattern → `allow`/`deny` mapping with glob keys and last-match-wins
  semantics, e.g. `{ "*": "deny", "get_*": "allow", "delete_item": "allow" }`.
  A blocked tool would be both invisible to `mcp_search` and rejected by
  `mcp_call`. Deferred: not a priority and non-trivial to implement/test.
- **Remote (URL) servers.** Only local stdio servers are supported today.
- **Stage 2: plugin-registered proxy.** Experiment with the plugin adding the
  proxy via `client.mcp.add()` so it need not live in `opencode.json`.
