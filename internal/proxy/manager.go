package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Manager owns the downstream MCP servers: it spawns their subprocesses,
// connects over stdio, caches the sessions for the process lifetime, and holds
// the collected tool list. All heavy lifting lives here; the cmd/ and mcp/
// controllers are thin wrappers over this.
type Manager struct {
	Config *Config

	Mu       sync.Mutex
	Sessions map[string]*Downstream // keyed by server name
}

// Downstream is one connected (or connectable) gated server.
type Downstream struct {
	Name    string
	Config  ServerConfig
	Session *mcp.ClientSession
	Tools   []*mcp.Tool
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
func NewManager(Config *Config) *Manager {
	return &Manager{
		Config:   Config,
		Sessions: make(map[string]*Downstream),
	}
}

// Start connects all enabled servers whose spawn mode is eager. Lazy servers
// are connected on first use (see ensure).
func (Mgr *Manager) Start(Ctx context.Context) error {
	for Name, Srv := range Mgr.Config.Servers {
		if !Srv.IsEnabled() {
			continue
		}
		if Srv.Spawn == SpawnLazy {
			continue
		}
		if _, Err := Mgr.ensure(Ctx, Name); Err != nil {
			return fmt.Errorf("connecting %q: %w", Name, Err)
		}
	}
	return nil
}

// Close shuts down every connected downstream, killing subprocesses.
func (Mgr *Manager) Close() {
	Mgr.Mu.Lock()
	defer Mgr.Mu.Unlock()

	for _, Down := range Mgr.Sessions {
		if Down.Session != nil {
			_ = Down.Session.Close()
		}
	}
	Mgr.Sessions = make(map[string]*Downstream)
}

// Servers returns the enabled servers' name + description, for `servers list`
// and plugin injection. Independent of connection state.
func (Mgr *Manager) Servers() []ServerInfo {
	var Out []ServerInfo
	for Name, Srv := range Mgr.Config.Servers {
		if !Srv.IsEnabled() {
			continue
		}
		Out = append(Out, ServerInfo{Name: Name, Description: Srv.Description})
	}
	return Out
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

// Search finds gated tools matching the query terms, ranked by relevance.
//
// Queries must contain at least one non-blank term — an empty query fails
// rather than returning everything, since dumping all tools would defeat the
// point of lazy-loading. Limit defaults to DefaultSearchLimit when <= 0 and may
// not exceed MaxSearchLimit (exceeding it is an error, not a silent clamp). When
// ServerFilter is non-empty, results are limited to that server.
//
// Ranking is breadth-first: tools are sorted by the number of distinct query
// terms they match (coverage of the caller's intent), then by a field-weighted
// score (name 6 > description 3 > schema 1) as a tiebreak, then by name for
// stability. Matching considers name + description + serialized input schema so
// capabilities buried in parameter schemas are still found; the schema itself is
// not returned (see ToolRef).
func (Mgr *Manager) Search(Ctx context.Context, Queries []string, ServerFilter string, Limit int) ([]ToolRef, error) {
	Terms := normalizeTerms(Queries)
	if len(Terms) == 0 {
		return nil, fmt.Errorf("search requires at least one non-blank query term")
	}

	if Limit <= 0 {
		Limit = DefaultSearchLimit
	}
	if Limit > MaxSearchLimit {
		return nil, fmt.Errorf("limit %d exceeds the maximum of %d", Limit, MaxSearchLimit)
	}

	if Err := Mgr.ensureAll(Ctx); Err != nil {
		return nil, Err
	}

	Mgr.Mu.Lock()
	defer Mgr.Mu.Unlock()

	var Scored []toolScore
	for Name, Down := range Mgr.Sessions {
		if ServerFilter != "" && Name != ServerFilter {
			continue
		}
		Scored = append(Scored, scoreTools(Name, Down.Tools, Terms)...)
	}

	sortByRelevance(Scored)

	if len(Scored) > Limit {
		Scored = Scored[:Limit]
	}

	Out := make([]ToolRef, len(Scored))
	for I, S := range Scored {
		Out[I] = S.Ref
	}
	return Out, nil
}

// normalizeTerms lowercases, trims, and drops blank query terms.
func normalizeTerms(Queries []string) []string {
	Terms := make([]string, 0, len(Queries))
	for _, Q := range Queries {
		if Trimmed := strings.TrimSpace(strings.ToLower(Q)); Trimmed != "" {
			Terms = append(Terms, Trimmed)
		}
	}
	return Terms
}

// scoreTools scores every tool of one server against the (already normalized)
// terms and returns only those matching at least one term.
func scoreTools(Server string, Tools []*mcp.Tool, Terms []string) []toolScore {
	var Out []toolScore
	for _, Tool := range Tools {
		if S, Ok := scoreTool(Server, Tool, Terms); Ok {
			Out = append(Out, S)
		}
	}
	return Out
}

// scoreTool computes the relevance of one tool against the terms. It matches
// each term independently against three fields — name, description, and the
// serialized input schema — and awards field weights (binary per field). It
// returns the ToolRef plus the ranking keys, and Ok=false if nothing matched.
func scoreTool(Server string, Tool *mcp.Tool, Terms []string) (toolScore, bool) {
	Name := strings.ToLower(Tool.Name)
	Desc := strings.ToLower(Tool.Description)
	Schema := strings.ToLower(serializeSchema(Tool))

	var Matched []string
	Score := 0
	HitName, HitDesc, HitSchema := false, false, false

	for _, Term := range Terms {
		InName := strings.Contains(Name, Term)
		InDesc := strings.Contains(Desc, Term)
		InSchema := strings.Contains(Schema, Term)

		if !InName && !InDesc && !InSchema {
			continue
		}

		Matched = append(Matched, Term)
		if InName {
			Score += scoreName
			HitName = true
		}
		if InDesc {
			Score += scoreDescription
			HitDesc = true
		}
		if InSchema {
			Score += scoreSchema
			HitSchema = true
		}
	}

	if len(Matched) == 0 {
		return toolScore{}, false
	}

	// Field labels in fixed name→description→schema order.
	var Fields []string
	if HitName {
		Fields = append(Fields, "name")
	}
	if HitDesc {
		Fields = append(Fields, "description")
	}
	if HitSchema {
		Fields = append(Fields, "input schema")
	}

	return toolScore{
		Ref: ToolRef{
			Server:        Server,
			Name:          Tool.Name,
			Description:   Tool.Description,
			Matched:       Matched,
			MatchedFields: Fields,
		},
		TermsMatched: len(Matched),
		Score:        Score,
	}, true
}

// serializeSchema renders a tool's input schema to a string for matching, so
// capabilities documented in parameter descriptions are searchable.
func serializeSchema(Tool *mcp.Tool) string {
	if Tool.InputSchema == nil {
		return ""
	}
	if Raw, Err := json.Marshal(Tool.InputSchema); Err == nil {
		return string(Raw)
	}
	return ""
}

// sortByRelevance orders results breadth-first: more distinct terms matched
// first, then higher field-weighted score, then name for a stable order.
func sortByRelevance(Scored []toolScore) {
	sort.SliceStable(Scored, func(I, J int) bool {
		A, B := Scored[I], Scored[J]
		if A.TermsMatched != B.TermsMatched {
			return A.TermsMatched > B.TermsMatched
		}
		if A.Score != B.Score {
			return A.Score > B.Score
		}
		return A.Ref.Name < B.Ref.Name
	})
}

// Describe returns the full input schema of one tool on one server.
func (Mgr *Manager) Describe(Ctx context.Context, Server, Tool string) (any, error) {
	Down, Err := Mgr.ensure(Ctx, Server)
	if Err != nil {
		return nil, Err
	}

	for _, T := range Down.Tools {
		if T.Name == Tool {
			return T.InputSchema, nil
		}
	}

	return nil, fmt.Errorf("tool %q not found on server %q", Tool, Server)
}

// Call invokes a downstream tool and returns its raw result.
func (Mgr *Manager) Call(Ctx context.Context, Server, Tool string, Args any) (*mcp.CallToolResult, error) {
	Down, Err := Mgr.ensure(Ctx, Server)
	if Err != nil {
		return nil, Err
	}

	return Down.Session.CallTool(Ctx, &mcp.CallToolParams{
		Name:      Tool,
		Arguments: Args,
	})
}

// ensureAll connects every enabled server not yet connected. Used by Search so
// a broad query sees the full catalog even for lazy servers.
func (Mgr *Manager) ensureAll(Ctx context.Context) error {
	for Name, Srv := range Mgr.Config.Servers {
		if !Srv.IsEnabled() {
			continue
		}
		if _, Err := Mgr.ensure(Ctx, Name); Err != nil {
			return fmt.Errorf("connecting %q: %w", Name, Err)
		}
	}
	return nil
}

// ensure returns a connected downstream, connecting it on first use. Safe to
// call repeatedly; connection is cached.
func (Mgr *Manager) ensure(Ctx context.Context, Name string) (*Downstream, error) {
	Mgr.Mu.Lock()
	defer Mgr.Mu.Unlock()

	if Down, Ok := Mgr.Sessions[Name]; Ok {
		return Down, nil
	}

	Srv, Ok := Mgr.Config.Servers[Name]
	if !Ok {
		return nil, fmt.Errorf("unknown server %q", Name)
	}
	if !Srv.IsEnabled() {
		return nil, fmt.Errorf("server %q is disabled", Name)
	}

	Down, Err := connect(Ctx, Name, Srv)
	if Err != nil {
		return nil, Err
	}

	Mgr.Sessions[Name] = Down
	return Down, nil
}

// connect spawns the subprocess, performs the MCP handshake, and lists tools.
func connect(Ctx context.Context, Name string, Srv ServerConfig) (*Downstream, error) {
	ConnectCtx, Cancel := context.WithTimeout(Ctx, Srv.Timeout.OrDefault())
	defer Cancel()

	Cmd := exec.CommandContext(Ctx, Srv.Command[0], Srv.Command[1:]...)
	Cmd.Env = mergeEnv(Srv.Environment)

	Client := mcp.NewClient(&mcp.Implementation{Name: "lazy-mcp", Version: "0.1.0"}, nil)
	Transport := &mcp.CommandTransport{Command: Cmd}

	Session, Err := Client.Connect(ConnectCtx, Transport, nil)
	if Err != nil {
		return nil, fmt.Errorf("connecting to %q: %w", Name, Err)
	}

	Listed, Err := Session.ListTools(ConnectCtx, nil)
	if Err != nil {
		_ = Session.Close()
		return nil, fmt.Errorf("listing tools for %q: %w", Name, Err)
	}

	return &Downstream{
		Name:    Name,
		Config:  Srv,
		Session: Session,
		Tools:   Listed.Tools,
	}, nil
}
