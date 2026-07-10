// mcp-gateway-inject — opencode plugin: teach the agent about mcp-gateway, the
// gateway that fronts the other MCP servers, once (system prompt) and feed it
// the live server list every turn (messages), so it always knows which servers
// exist behind the gateway and what they do — without their full tool schemas
// ever entering context.
//
// Two hooks, deliberately split (mirrors the jumper plugin pattern):
//
//   experimental.chat.system.transform — push a STATIC primer into the system
//     prompt: what mcp-gateway is and the search → describe → call workflow. This
//     stays valid across the whole session.
//
//   experimental.chat.messages.transform — push the live server list (name +
//     description) fetched from `mcp-gateway servers list -o json`. This hook
//     fires per outgoing LLM request and mutates only that payload — never
//     written back to stored history — so the model sees exactly one fresh copy
//     per turn and nothing accumulates.
//
// Why in-place mutation: opencode passes the same messages array reference on
// to model-message conversion; reassigning output.messages is a no-op. We push
// onto an existing message's parts array in place.
//
// Best-effort throughout — if mcp-gateway is missing, errors, or returns nothing,
// we inject nothing and never disrupt the request.
//
// Opt-out: MCP_GATEWAY_INJECT=false|0|no suppresses both injections.
//
// NOTE (Stage 1): this plugin only INJECTS the server list. The proxy itself is
// added to opencode via opencode.json manually. Stage 2 will experiment with
// registering the proxy from here via client.mcp.add().

import type { Plugin } from '@opencode-ai/plugin';

const INJECT_ENABLED = !['false', '0', 'no'].includes((process.env.MCP_GATEWAY_INJECT ?? '').toLowerCase());

const BLOCK_OPEN = '<mcp-gateway-servers>';
const BLOCK_CLOSE = '</mcp-gateway-servers>';

const SYSTEM_PRIMER = [
  'mcp-gateway is a single gateway that fronts many MCP servers. Their tools are not loaded into your',
  'context up front (that would cost too many tokens) — you reach them on demand through the gateway.',
  `The servers available behind the gateway are injected each turn in a \`${BLOCK_OPEN}\` block (name: description).`,
  'To use any of their tools:',
  '1. `mcp_search` with a list of keywords (not a sentence) to discover tools. Results are ranked',
  '   primarily by HOW MANY of your keywords a tool matches (broadest coverage first), then by where',
  '   they matched. So pass several distinct keywords for the concept — a tool matching more of them',
  '   ranks higher. Each result includes `matched` (which keywords hit) and `matchedFields` (where they hit).',
  "2. If a result's `matchedFields` includes \"input schema\", or you need the tool's parameters,",
  '   call `mcp_describe` — search does NOT return schemas, and mega-tools hide capabilities',
  '   (routes, options) in their parameter descriptions.',
  '3. `mcp_call` to run it, passing server + tool + args.',
  'Do not conclude a capability is missing from an empty search — try broader/alternative keywords first.',
].join('\n');

export const McpGatewayInject: Plugin = async ({ $ }) => {
  // Fork `mcp-gateway servers list -o json` and render it as a compact
  // `name: description` block. Returns '' on any failure or when empty.
  async function renderServers(): Promise<string> {
    try {
      const res = await $`mcp-gateway servers list -o json`.quiet().nothrow();
      if (res.exitCode !== 0) return '';

      const parsed = JSON.parse(res.stdout.toString());
      if (!Array.isArray(parsed) || parsed.length === 0) return '';

      const lines = parsed
        .filter((s: any) => s?.name)
        .map((s: any) => (s.description ? `- ${s.name}: ${s.description}` : `- ${s.name}`));
      if (lines.length === 0) return '';

      return [BLOCK_OPEN, ...lines, BLOCK_CLOSE].join('\n');
    } catch {
      return '';
    }
  }

  return {
    'experimental.chat.system.transform': async (_input, output) => {
      if (!INJECT_ENABLED) return;
      output.system.push(SYSTEM_PRIMER);
    },

    'experimental.chat.messages.transform': async (_input, output) => {
      if (!INJECT_ENABLED) return;
      try {
        const messages = output.messages;
        if (!Array.isArray(messages) || messages.length === 0) return;

        const target = [...messages].reverse().find((m) => m?.info?.role === 'user');
        if (!target) return;

        const block = await renderServers();
        if (!block) return;

        target.parts.push({
          id: `mcp-gateway-inject-${Date.now()}`,
          sessionID: target.info.sessionID,
          messageID: target.info.id,
          type: 'text',
          text: block,
          synthetic: true,
        } as any);
      } catch {
        // best-effort — never disrupt the request
      }
    },
  };
};

export default McpGatewayInject;
