# mcp-gateway

mcp-gateway is a single MCP server that fronts many others. It keeps their tool schemas out of the agent's context: instead of dozens of tools costing 50–70k tokens up front, the agent sees three small meta-tools and pulls in only the schemas it actually needs, on demand. Add as many servers as you like — the context cost stays flat.

Normally every MCP server an agent connects to injects all of its tool schemas at startup, and most are irrelevant to any given session — so the more servers you add, the more context you burn, and useful servers go unconnected just to keep things lean. The gateway removes that trade-off.

## How it works

mcp-gateway runs as a single MCP server (the proxy) in front of all the others. The agent sees only three meta-tools instead of the dozens the fronted servers actually expose:

- `mcp_search({ query, server?, limit? })` — find tools by keyword. `query` is a required, non-empty list of keywords; a tool matches if any term appears in its name, description, or input schema (case-insensitive). Results are ranked **breadth-first**: by how many distinct keywords match, then by where they matched (name > description > schema). Capped at 5 by default; `limit` may be raised up to 25. Returns `{ server, name, description, matched, matchedFields }` — the input schema is **not** included (see `mcp_describe`).
- `mcp_describe({ server, tool })` — full input schema for one tool. Needed when a hit's `matchedFields` includes `"input schema"`: mega-tools bury real capabilities (routes, options) in parameter descriptions, which search matches on but does not return.
- `mcp_call({ server, tool, args })` — invoke a tool, returns its result.

A typical flow is `search` → `call`. Insert a `describe` step when a search hit's `matchedFields` includes `"input schema"` — that signals the capability you matched is buried in a parameter description, which search doesn't return, so you fetch the full schema before calling. Most tasks skip `describe`.

The proxy spawns each fronted server as a persistent subprocess (or connects to a remote one) and proxies calls over its transport, so in-memory server state survives across calls within a session. Eager servers connect in the background at launch, so the gateway answers immediately instead of waiting on every downstream handshake.

A companion opencode plugin injects the list of fronted servers (name + description) into context each turn, so the agent knows what exists without paying for the full tool schemas. It does **not** inject the tools themselves.

## Configuration: `mcp-gateway.json`

Lives in the config dir — `$OPENCODE_CONFIG_DIR` if set, else `$XDG_CONFIG_HOME`, else `~/.config` — as `mcp-gateway.jsonc` (preferred) or `mcp-gateway.json`.

```jsonc
{
  "servers": {
    // Local (stdio subprocess). Secrets pulled at connect time via {cmd:...}.
    "issues-mcp": {
      "description": "Issue tracker: issues, comments, labels, milestones",
      "spawn": "eager",
      "timeout": "30s",
      "enabled": true,
      "server": {
        "command": ["issues-mcp-server"],
        "environment": { "API_TOKEN": "{cmd:kv get issues:token}" }
      }
    },
    // Minimal local server: only command + description are required.
    "docs-mcp": {
      "description": "Docs and knowledge base: search, read pages",
      "server": { "command": ["docs-mcp-server"] }
    },
    // Remote (streamable HTTP) with a static bearer header.
    "sentry": {
      "description": "Sentry: issues, projects, error events",
      "server": {
        "url": "https://mcp.sentry.dev/mcp",
        "headers": { "Authorization": "Bearer {cmd:kv get sentry:token}" }
      }
    },
    // Remote with pre-registered OAuth client credentials, resolved from env.
    "api": {
      "description": "Example API: read/write records",
      "server": {
        "url": "https://api.example.com/mcp",
        "oauth": { "clientId": "{env:API_CLIENT_ID}", "clientSecret": "{cmd:kv get api:secret}" }
      }
    }
  }
}
```

Each server has operational **settings** at the top level and a nested **`server`** transport block. The transport kind is inferred from the block: `command` ⇒ local (stdio subprocess), `url` ⇒ remote (streamable HTTP). Exactly one of the two must be set.

### Settings

| Field | Required | Default | Meaning |
|-------|----------|---------|---------|
| `description` | yes | — | shown in `servers list`, plugin injection, and search |
| `spawn` | no | `eager` | `eager` (connect in the background at launch) or `lazy` (connect on first use). Either way the gateway starts serving immediately. |
| `timeout` | no | `30s` | connect timeout: Go duration (`1h30m12s`) or a bare number of seconds |
| `enabled` | no | `true` | `false` skips the server entirely |
| `server` | yes | — | the transport (see below) |

### Transport (`server`) — local

| Field | Required | Default | Meaning |
|-------|----------|---------|---------|
| `command` | yes | — | argv to spawn the server (local stdio) |
| `environment` | no | — | env vars added on top of the inherited environment |

### Transport (`server`) — remote

| Field | Required | Default | Meaning |
|-------|----------|---------|---------|
| `url` | yes | — | streamable-HTTP endpoint of the remote server |
| `headers` | no | — | headers sent on every request (e.g. a bearer token) |
| `oauth` | no | — | pre-registered OAuth client credentials: `{ "clientId": ..., "clientSecret": ... }` |
| `environment` | no | — | env vars used to resolve `{env:...}` and run `{cmd:...}` (see below) |

OAuth uses the client-credentials grant (service-to-service, no user interaction); the token endpoint and scopes are discovered from server metadata. For API-key servers, prefer `headers`. The interactive (browser) authorization-code flow is not implemented yet — see future work.

### Secret interpolation

Rather than hard-coding secrets, any author-supplied string — `command` args, `environment` values, `headers`, and `oauth` credentials — may contain directives resolved **at connect time**:

- `{env:NAME}` — replaced with `NAME` from the resolution environment. An unset `NAME` is an error.
- `{cmd:...}` — the body is run via `sh -c` and replaced with its trimmed stdout. A non-zero exit is an error.

The resolution environment is the process environment with the server's `environment` map layered on top (**the map wins** on conflict), so `{env:X}` resolves from the map even when `X` is not in the ambient process env. Resolution is two-phase: `environment` values are resolved first (against the process env), then the resolved map feeds everything else — so a header can reference an `environment` value that is itself computed by a `{cmd:...}`.

mcp-gateway never learns about any specific secret store — the author supplies the command, as in the `{cmd:kv get ...}` and `{env:...}` directives in the config example above.

## CLI

```
mcp-gateway agent mcp                     # run the proxy (stdio) — this is the opencode mcp entry
mcp-gateway agent plugin <agent> [--path] # print or write the plugin for an agent (e.g. opencode)
mcp-gateway agent setup <agent>           # install the plugin into the agent's config dir
mcp-gateway servers list [-o yaml|json]   # list fronted servers (yaml default; json for the plugin)
```

## opencode wiring

Add the proxy to `opencode.json` and remove the now-fronted servers from its `mcp` block:

```jsonc
"mcp": {
  "mcp-gateway": { "type": "local", "command": ["mcp-gateway", "agent", "mcp"], "enabled": true }
}
```

Then `mcp-gateway agent setup opencode` to install the injection plugin.
