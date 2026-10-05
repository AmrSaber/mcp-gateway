// mcp-gateway-inject teaches OpenCode how to discover tools behind the gateway
// without loading their schemas into every model request.
//
// The context hook adds a static primer and a fresh server list to the outgoing
// request. Neither is written into the session history. If mcp-gateway is
// unavailable, the primer remains and the server list is omitted.
//
// Opt-out: MCP_GATEWAY_INJECT=false|0|no suppresses both injections.

const INJECT_ENABLED = !["false", "0", "no"].includes(
  (process.env.MCP_GATEWAY_INJECT ?? "").toLowerCase(),
);

const BLOCK_OPEN = "<mcp-gateway-servers>";
const BLOCK_CLOSE = "</mcp-gateway-servers>";
const SYSTEM_PRIMER = [
  "mcp-gateway is a single gateway that fronts many MCP servers. Their tools are not loaded into your",
  "context up front — you reach them on demand through the gateway.",
  `The servers available behind the gateway are injected each turn in a \`${BLOCK_OPEN}\` block (name: description).`,
  "To use any of their tools:",
  "1. `mcp_search` with a list of keywords (not a sentence) to discover tools. Results are ranked",
  "   primarily by HOW MANY of your keywords match, then by where they matched (name > description > schema).",
  "2. If a result's `matchedFields` includes \"input schema\", or you need the tool's parameters,",
  "   call `mcp_describe` — search does NOT return schemas.",
  "3. `mcp_call` to run it, passing server + tool + args.",
  "Do not conclude a capability is missing from an empty search — try broader or alternative keywords first.",
].join("\n");

async function renderServers(): Promise<string> {
  try {
    const result = await Bun.$`mcp-gateway servers list -o json`
      .quiet()
      .nothrow();
    if (result.exitCode !== 0) return "";

    const servers = JSON.parse(result.stdout.toString());
    if (!Array.isArray(servers) || servers.length === 0) return "";

    const lines = servers
      .filter((server: { name?: unknown }) => typeof server.name === "string")
      .map((server: { name: string; description?: unknown }) =>
        typeof server.description === "string"
          ? `- ${server.name}: ${server.description}`
          : `- ${server.name}`,
      );
    return lines.length ? [BLOCK_OPEN, ...lines, BLOCK_CLOSE].join("\n") : "";
  } catch {
    return "";
  }
}

export default {
  id: "mcp-gateway-inject",
  async setup(ctx) {
    await ctx.session.hook("context", async (event) => {
      if (!INJECT_ENABLED) return;

      event.system.push({ type: "text", text: SYSTEM_PRIMER });

      const serverList = await renderServers();
      if (serverList) event.system.push({ type: "text", text: serverList });
    });
  },
};
