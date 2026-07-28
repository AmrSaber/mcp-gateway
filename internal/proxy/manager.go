// Package proxy handles coordination and communication with the fronted MCP servers.
package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/sync/singleflight"
)

// Manager owns the downstream MCP servers: it spawns their subprocesses,
// connects over stdio, caches the sessions for the process lifetime, and holds
// the collected tool list. All heavy lifting lives here; the cmd/ and mcp/
// controllers are thin wrappers over this.
type Manager struct {
	config *Config

	lock     sync.Mutex
	sessions map[string]*Downstream // keyed by server name

	// connecting dedupes concurrent ensure() calls for the same server so a
	// server is connected (subprocess spawned) at most once, even under
	// parallel Start / concurrent requests.
	connecting singleflight.Group
}

// Downstream is one connected (or connectable) fronted server.
type Downstream struct {
	name    string
	session *mcp.ClientSession
	tools   []*mcp.Tool
}

// ToolRef is a search result: a tool paired with its owning server.
//
// Matched lists which of the caller's query terms hit this tool. MatchedFields
// lists which fields those hits landed in ("name", "description", "input
// schema"). Together they let the agent judge relevance and know where to look:
// a hit in "input schema" means the capability is buried in a parameter (mega-
// tools that wrap many sub-operations often hide routes/options in a parameter
// description) — the agent should call mcp_describe to see it, since the schema
// is deliberately NOT returned here (keeps results lean).
type ToolRef struct {
	Server        string   `json:"server"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Matched       []string `json:"matched"`
	MatchedFields []string `json:"matchedFields"`
}

// ServerInfo is one entry for `servers list` and plugin injection.
type ServerInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// NewManager builds a manager from config. It does not spawn anything yet —
// call Start to connect eager servers.
func NewManager(config *Config) *Manager {
	return &Manager{
		config:   config,
		sessions: make(map[string]*Downstream),
	}
}

// Start connects all enabled servers whose spawn mode is eager. Lazy servers
// are connected on first use (see ensure).
func (manager *Manager) Start(ctx context.Context) error {
	var wg sync.WaitGroup
	var errsLock sync.Mutex
	var errs []error

	for name, srv := range manager.config.Servers {
		if !srv.isEnabled() || srv.Spawn == spawnLazy {
			continue
		}

		wg.Go(func() {
			if _, err := manager.ensure(ctx, name); err != nil {
				errsLock.Lock()
				errs = append(errs, fmt.Errorf("connecting %q: %w", name, err))
				errsLock.Unlock()
			}
		})
	}

	wg.Wait()
	return errors.Join(errs...)
}

// Close shuts down every connected downstream, killing subprocesses.
func (manager *Manager) Close() {
	manager.lock.Lock()
	defer manager.lock.Unlock()

	for _, down := range manager.sessions {
		if down.session != nil {
			_ = down.session.Close()
		}
	}

	manager.sessions = make(map[string]*Downstream)
}

// Servers returns the enabled servers' name + description, for `servers list`
// and plugin injection. Independent of connection state.
func (manager *Manager) Servers() []ServerInfo {
	var out []ServerInfo
	for name, srv := range manager.config.Servers {
		if !srv.isEnabled() {
			continue
		}

		out = append(out, ServerInfo{Name: name, Description: srv.Description})
	}

	return out
}

// Search limits: results default to DefaultSearchLimit and the caller may raise
// the limit up to MaxSearchLimit. Asking for more fails rather than silently
// clamping, so the caller knows their request wasn't honoured.
const (
	DefaultSearchLimit = 5
	MaxSearchLimit     = 25
)

// Field weights for scoring (binary per field: a term either hits a field or it
// does not — occurrence count is ignored, as repetition in tool metadata
// signals verbosity, not relevance).
const (
	scoreName        = 6
	scoreDescription = 3
	scoreSchema      = 1
)

// toolScore is the internal ranking record for one matched tool.
type toolScore struct {
	Ref          ToolRef
	TermsMatched int // distinct query terms that hit (primary sort key)
	Score        int // field-weighted score (tiebreak)
}

// Search finds fronted tools matching the query terms, ranked by relevance.
//
// Queries must contain at least one non-blank term — an empty query fails
// rather than returning everything, since dumping all tools would defeat the
// gateway's whole point (keeping tools out of the agent's context). Limit
// defaults to DefaultSearchLimit when <= 0 and may
// not exceed MaxSearchLimit (exceeding it is an error, not a silent clamp). When
// ServerFilter is non-empty, results are limited to that server.
//
// Ranking is breadth-first: tools are sorted by the number of distinct query
// terms they match (coverage of the caller's intent), then by a field-weighted
// score (name 6 > description 3 > schema 1) as a tiebreak, then by name for
// stability. Matching considers name + description + serialized input schema so
// capabilities buried in parameter schemas are still found; the schema itself is
// not returned (see ToolRef).
func (manager *Manager) Search(ctx context.Context, queries []string, serverFilter string, limit int) ([]ToolRef, error) {
	terms := normalizeTerms(queries)
	if len(terms) == 0 {
		return nil, fmt.Errorf("search requires at least one non-blank query term")
	}

	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	if limit > MaxSearchLimit {
		return nil, fmt.Errorf("limit %d exceeds the maximum of %d", limit, MaxSearchLimit)
	}

	if err := manager.ensureAll(ctx); err != nil {
		return nil, err
	}

	manager.lock.Lock()
	defer manager.lock.Unlock()

	var scored []toolScore
	for name, down := range manager.sessions {
		if serverFilter != "" && name != serverFilter {
			continue
		}
		scored = append(scored, scoreTools(name, down.tools, terms)...)
	}

	sortByRelevance(scored)

	if len(scored) > limit {
		scored = scored[:limit]
	}

	out := make([]ToolRef, len(scored))
	for i, s := range scored {
		out[i] = s.Ref
	}
	return out, nil
}

// normalizeTerms lowercases, trims, and drops blank query terms.
func normalizeTerms(queries []string) []string {
	terms := make([]string, 0, len(queries))
	for _, q := range queries {
		if trimmed := strings.TrimSpace(strings.ToLower(q)); trimmed != "" {
			terms = append(terms, trimmed)
		}
	}
	return terms
}

// scoreTools scores every tool of one server against the (already normalized)
// terms and returns only those matching at least one term.
func scoreTools(server string, tools []*mcp.Tool, terms []string) []toolScore {
	var out []toolScore
	for _, tool := range tools {
		if s, ok := scoreTool(server, tool, terms); ok {
			out = append(out, s)
		}
	}
	return out
}

// scoreTool computes the relevance of one tool against the terms. It matches
// each term independently against three fields — name, description, and the
// serialized input schema — and awards field weights (binary per field). It
// returns the ToolRef plus the ranking keys, and Ok=false if nothing matched.
func scoreTool(server string, tool *mcp.Tool, terms []string) (toolScore, bool) {
	name := strings.ToLower(tool.Name)
	desc := strings.ToLower(tool.Description)
	schema := strings.ToLower(serializeSchema(tool))

	var matched []string
	score := 0
	hitName, hitDesc, hitSchema := false, false, false

	for _, term := range terms {
		inName := strings.Contains(name, term)
		inDesc := strings.Contains(desc, term)
		inSchema := strings.Contains(schema, term)

		if !inName && !inDesc && !inSchema {
			continue
		}

		matched = append(matched, term)
		if inName {
			score += scoreName
			hitName = true
		}
		if inDesc {
			score += scoreDescription
			hitDesc = true
		}
		if inSchema {
			score += scoreSchema
			hitSchema = true
		}
	}

	if len(matched) == 0 {
		return toolScore{}, false
	}

	// Field labels in fixed name→description→schema order.
	var fields []string
	if hitName {
		fields = append(fields, "name")
	}
	if hitDesc {
		fields = append(fields, "description")
	}
	if hitSchema {
		fields = append(fields, "input schema")
	}

	return toolScore{
		Ref: ToolRef{
			Server:        server,
			Name:          tool.Name,
			Description:   tool.Description,
			Matched:       matched,
			MatchedFields: fields,
		},
		TermsMatched: len(matched),
		Score:        score,
	}, true
}

// serializeSchema renders a tool's input schema to a string for matching, so
// capabilities documented in parameter descriptions are searchable.
func serializeSchema(tool *mcp.Tool) string {
	if tool.InputSchema == nil {
		return ""
	}
	if raw, err := json.Marshal(tool.InputSchema); err == nil {
		return string(raw)
	}
	return ""
}

// sortByRelevance orders results breadth-first: more distinct terms matched
// first, then higher field-weighted score, then name for a stable order.
func sortByRelevance(scored []toolScore) {
	sort.SliceStable(scored, func(i, j int) bool {
		a, b := scored[i], scored[j]
		if a.TermsMatched != b.TermsMatched {
			return a.TermsMatched > b.TermsMatched
		}
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		return a.Ref.Name < b.Ref.Name
	})
}

// Describe returns the full input schema of one tool on one server.
func (manager *Manager) Describe(ctx context.Context, server, tool string) (any, error) {
	down, err := manager.ensure(ctx, server)
	if err != nil {
		return nil, err
	}

	for _, t := range down.tools {
		if t.Name == tool {
			return t.InputSchema, nil
		}
	}

	return nil, fmt.Errorf("tool %q not found on server %q", tool, server)
}

// Call invokes a downstream tool and returns its raw result.
func (manager *Manager) Call(ctx context.Context, server, tool string, args any) (*mcp.CallToolResult, error) {
	down, err := manager.ensure(ctx, server)
	if err != nil {
		return nil, err
	}

	return down.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      tool,
		Arguments: args,
	})
}

// ensureAll connects every enabled server not yet connected. Used by Search so
// a broad query sees the full catalog even for lazy servers.
func (manager *Manager) ensureAll(ctx context.Context) error {
	for name, srv := range manager.config.Servers {
		if !srv.isEnabled() {
			continue
		}
		if _, err := manager.ensure(ctx, name); err != nil {
			return fmt.Errorf("connecting %q: %w", name, err)
		}
	}
	return nil
}

// ensure returns a connected downstream, connecting it on first use. Safe to
// call repeatedly and concurrently; the connection is cached and concurrent
// callers for the same server share a single connect via singleflight.
func (manager *Manager) ensure(ctx context.Context, name string) (*Downstream, error) {
	manager.lock.Lock()
	down, ok := manager.sessions[name]
	manager.lock.Unlock()
	if ok {
		return down, nil
	}

	// Slow path: connect outside the map lock so different servers connect in
	// parallel, and dedupe concurrent connects of the same server.
	res, err, _ := manager.connecting.Do(name, func() (any, error) {
		// Re-check under lock: a prior singleflight winner may have populated it.
		manager.lock.Lock()
		if down, ok := manager.sessions[name]; ok {
			manager.lock.Unlock()
			return down, nil
		}

		srv, ok := manager.config.Servers[name]
		manager.lock.Unlock()

		if !ok {
			return nil, fmt.Errorf("unknown server %q", name)
		}
		if !srv.isEnabled() {
			return nil, fmt.Errorf("server %q is disabled", name)
		}

		down, err := connect(ctx, name, srv)
		if err != nil {
			return nil, err
		}

		manager.lock.Lock()
		defer manager.lock.Unlock()
		manager.sessions[name] = down
		return down, nil
	})

	if err != nil {
		return nil, err
	}

	return res.(*Downstream), nil
}

// connect performs the MCP handshake and lists tools over the server's
// transport: a spawned stdio subprocess for local servers, or streamable HTTP
// for remote (URL) servers. It is a var so tests can stub it without spawning
// real subprocesses.
var connect = func(ctx context.Context, name string, srv ServerConfig) (*Downstream, error) {
	connectCtx, cancel := context.WithTimeout(ctx, srv.Timeout.orDefault())
	defer cancel()

	transport, err := transportFor(ctx, srv.Server)
	if err != nil {
		return nil, fmt.Errorf("configuring transport for %q: %w", name, err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "mcp-gateway", Version: "0.1.0"}, nil)

	session, err := client.Connect(connectCtx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connecting to %q: %w", name, err)
	}

	listed, err := session.ListTools(connectCtx, nil)
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("listing tools for %q: %w", name, err)
	}

	return &Downstream{
		name:    name,
		session: session,
		tools:   listed.Tools,
	}, nil
}

// transportFor builds the MCP transport for a server spec: a CommandTransport
// spawning a stdio subprocess for local servers, or a StreamableClientTransport
// for remote (URL) servers.
//
// It resolves {env:...}/{cmd:...} interpolation in two phases (see interpolate):
// Environment values are resolved first against the process env, the resolved
// map is then merged over the process env, and that merged env is used both as
// the subprocess environment and as the resolution source for the remaining
// values (command args, headers, oauth). This lets a header reference an
// environment value that is itself computed by a {cmd:...}.
func transportFor(ctx context.Context, spec ServerSpec) (mcp.Transport, error) {
	resolvedEnv, err := interpolateMap(ctx, spec.Environment, os.Environ())
	if err != nil {
		return nil, fmt.Errorf("resolving environment: %w", err)
	}
	merged := mergeEnv(resolvedEnv)

	if spec.isRemote() {
		return remoteTransport(ctx, spec, merged)
	}

	args, err := interpolateSlice(ctx, spec.Command, merged)
	if err != nil {
		return nil, fmt.Errorf("resolving command: %w", err)
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Env = merged
	return &mcp.CommandTransport{Command: cmd}, nil
}
