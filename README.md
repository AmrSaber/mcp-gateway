# mcp-gateway

mcp-gateway is a single MCP server that fronts many others. It keeps their tool
schemas out of the agent's context: instead of dozens of tools costing 50–70k
tokens up front, the agent sees three small meta-tools and pulls in only the
schemas it actually needs, on demand. Add as many servers as you like — the
context cost stays flat.

## The problem

Every MCP server an agent connects to injects all of its tool schemas into the
LLM context at startup. A couple of large servers can eat 50–70k tokens before
the first message — and most of those tools are irrelevant to any given session.
The more servers you add, the worse it gets, so useful tools go unconnected just
to keep the context lean.

## How it works

mcp-gateway runs as a single MCP server (the proxy) in front of all the others.
The agent sees only three meta-tools instead of the dozens the fronted servers
actually expose:

- `mcp_search({ query, server?, limit? })` — find tools by keyword.
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
- `mcp_call({ server, tool, args })` — invoke a tool, returns its result.

A typical flow is `search` → `describe` (only when a hit's `matchedFields`
includes `"input schema"`) → `call`. Most tasks skip `describe` entirely.

The proxy spawns each fronted server as a persistent subprocess and proxies
calls to it over stdio, so in-memory server state survives across calls within a
session.

A companion opencode plugin injects the list of fronted servers (name +
description) into context each turn, so the agent knows what exists without
paying for the full tool schemas. It does **not** inject the tools themselves.

## Configuration: `mcp-gateway.json`

Lives next to `opencode.json` in the opencode config dir
(`$OPENCODE_CONFIG_DIR`, else `$XDG_CONFIG_HOME/opencode`, else
`~/.config/opencode`).

```jsonc
{
  "servers": {
    "issues-mcp": {
      "description": "Issue tracker: issues, comments, labels, milestones",
      "spawn": "eager",
      "timeout": "30s",
      "enabled": true,
      "server": {
        "command": ["sh", "-c", "API_TOKEN=\"$(get-secret issues:token)\" exec issues-mcp-server"],
        "environment": { "API_URL": "https://issues.example.com/api" }
      }
    },
    "docs-mcp": {
      "description": "Docs and knowledge base: search, read pages",
      "server": { "command": ["docs-mcp-server"] }
    },
    "sentry": {
      "description": "Sentry: issues, projects, error events",
      "server": {
        "url": "https://mcp.sentry.dev/mcp",
        "headers": { "Authorization": "Bearer {{token}}" }
      }
    }
  }
}
```

Each server has operational **settings** at the top level and a nested
**`server`** transport block. The transport kind is inferred from the block:
`command` ⇒ local (stdio subprocess), `url` ⇒ remote (streamable HTTP). Exactly
one of the two must be set.

### Settings

| Field | Required | Default | Meaning |
|-------|----------|---------|---------|
| `description` | yes | — | shown in `servers list`, plugin injection, and search |
| `spawn` | no | `eager` | `eager` (connect at startup) or `lazy` (connect on first use) |
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

OAuth uses the client-credentials grant (service-to-service, no user
interaction); the token endpoint and scopes are discovered from server metadata.
For API-key servers, prefer `headers`. The interactive (browser)
authorization-code flow is not implemented yet — see future work.

### Secret interpolation

Rather than hard-coding secrets, any author-supplied string — `command` args,
`environment` values, `headers`, and `oauth` credentials — may contain
directives resolved **at connect time**:

- `{env:NAME}` — replaced with `NAME` from the resolution environment. An unset
  `NAME` is an error.
- `{cmd:...}` — the body is run via `sh -c` and replaced with its trimmed
  stdout. A non-zero exit is an error.

The resolution environment is the process environment with the server's
`environment` map layered on top (**the map wins** on conflict), so `{env:X}`
resolves from the map even when `X` is not in the ambient process env. Resolution
is two-phase: `environment` values are resolved first (against the process env),
then the resolved map feeds everything else — so a header can reference an
`environment` value that is itself computed by a `{cmd:...}`.

mcp-gateway never learns about any specific secret store: the author supplies the
command. For example, pulling a GitHub PAT from a `kv` CLI for the hosted GitHub
MCP server:

```jsonc
"github": {
  "description": "GitHub: repos, issues, pull requests, actions",
  "server": {
    "url": "https://api.githubcopilot.com/mcp/",
    "headers": { "Authorization": "Bearer {cmd:kv get github:pat}" }
  }
}
```

## CLI

```
mcp-gateway agent mcp                     # run the proxy (stdio) — this is the opencode mcp entry
mcp-gateway agent plugin <agent> [--path] # print or write the plugin for an agent (e.g. opencode)
mcp-gateway agent setup <agent>           # install the plugin into the agent's config dir
mcp-gateway servers list [-o yaml|json]   # list fronted servers (yaml default; json for the plugin)
```

## opencode wiring

Add the proxy to `opencode.json` and remove the now-fronted servers from its
`mcp` block:

```jsonc
"mcp": {
  "mcp-gateway": { "type": "local", "command": ["mcp-gateway", "agent", "mcp"], "enabled": true }
}
```

Then `mcp-gateway agent setup opencode` to install the injection plugin.

## Future work

- **Per-server tool allow/deny scoping.** Restrict which of a server's tools are
  exposed (e.g. block destructive tools). Intended shape: an opencode-style
  ordered pattern → `allow`/`deny` mapping with glob keys and last-match-wins
  semantics, e.g. `{ "*": "deny", "get_*": "allow", "delete_item": "allow" }`.
  A blocked tool would be both invisible to `mcp_search` and rejected by
  `mcp_call`. Deferred: not a priority and non-trivial to implement/test.
- **Remote (URL) servers.** Supported via a `server.url` transport block
  (streamable HTTP), with static `headers` and pre-registered client-credentials
  `oauth`. Still deferred: the **interactive OAuth flow** (browser
  authorization-code grant with dynamic client registration and on-disk token
  storage, as opencode does with `opencode mcp auth`). mcp-gateway runs headless as
  a stdio subprocess, so this needs its own `mcp-gateway mcp auth <server>` command
  and token store.
- **Stage 2: plugin-registered proxy.** Experiment with the plugin adding the
  proxy via `client.mcp.add()` so it need not live in `opencode.json`.
