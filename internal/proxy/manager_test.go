package proxy

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

// megaTool mimics a broad "read a URL" tool whose real capability (calendar)
// lives ONLY in the input schema, not the name/description — the case that
// name+description-only search would miss.
func megaTool() *mcp.Tool {
	return &mcp.Tool{
		Name:        "read_website",
		Description: "Reads content from websites",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"inputs": map[string]any{
					"type":        "array",
					"description": "URLs, e.g. calendar.example.com/find/USER to get calendar events",
				},
			},
		},
	}
}

func TestScoreToolWeights(t *testing.T) {
	// name hit = 6, description hit = 3, schema hit = 1, per matched term.
	cases := []struct {
		Name       string
		Tool       *mcp.Tool
		Terms      []string
		WantScore  int
		WantTerms  int
		WantFields []string
	}{
		{
			Name:       "name only",
			Tool:       &mcp.Tool{Name: "create_issue", Description: "open a thing"},
			Terms:      []string{"issue"},
			WantScore:  6,
			WantTerms:  1,
			WantFields: []string{"name"},
		},
		{
			Name:       "description only",
			Tool:       &mcp.Tool{Name: "x", Description: "show CI pipelines"},
			Terms:      []string{"pipeline"},
			WantScore:  3,
			WantTerms:  1,
			WantFields: []string{"description"},
		},
		{
			Name:       "schema only",
			Tool:       megaTool(),
			Terms:      []string{"calendar"},
			WantScore:  1,
			WantTerms:  1,
			WantFields: []string{"input schema"},
		},
		{
			Name:       "name and description one term",
			Tool:       &mcp.Tool{Name: "issue_tool", Description: "manage an issue"},
			Terms:      []string{"issue"},
			WantScore:  9, // 6 + 3
			WantTerms:  1,
			WantFields: []string{"name", "description"},
		},
		{
			Name:       "two schema terms",
			Tool:       megaTool(),
			Terms:      []string{"calendar", "calendar.example.com"},
			WantScore:  2, // 1 + 1
			WantTerms:  2,
			WantFields: []string{"input schema"},
		},
	}

	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			s, ok := scoreTool("srv", c.Tool, c.Terms)
			if !ok {
				t.Fatalf("expected a match")
			}
			if s.Score != c.WantScore {
				t.Errorf("score = %d, want %d", s.Score, c.WantScore)
			}
			if s.TermsMatched != c.WantTerms {
				t.Errorf("termsMatched = %d, want %d", s.TermsMatched, c.WantTerms)
			}
			if !equal(s.Ref.MatchedFields, c.WantFields) {
				t.Errorf("matchedFields = %v, want %v", s.Ref.MatchedFields, c.WantFields)
			}
		})
	}
}

func TestScoreToolNoMatch(t *testing.T) {
	if _, ok := scoreTool("srv", &mcp.Tool{Name: "a", Description: "b"}, []string{"nope"}); ok {
		t.Error("expected no match")
	}
}

func TestSortByRelevanceBreadthFirst(t *testing.T) {
	// A tool matching 2 terms (only in schema, low field score) must outrank a
	// tool matching 1 term with a high field score (name). Breadth wins.
	scored := []toolScore{
		{Ref: ToolRef{Name: "name_hit"}, TermsMatched: 1, Score: 6},
		{Ref: ToolRef{Name: "two_schema_hits"}, TermsMatched: 2, Score: 2},
	}
	sortByRelevance(scored)
	if scored[0].Ref.Name != "two_schema_hits" {
		t.Errorf("breadth-first failed: top = %q, want two_schema_hits", scored[0].Ref.Name)
	}
}

func TestSortByRelevanceFieldTiebreak(t *testing.T) {
	// Same breadth → higher field score wins.
	scored := []toolScore{
		{Ref: ToolRef{Name: "schema"}, TermsMatched: 1, Score: 1},
		{Ref: ToolRef{Name: "name"}, TermsMatched: 1, Score: 6},
	}
	sortByRelevance(scored)
	if scored[0].Ref.Name != "name" {
		t.Errorf("field tiebreak failed: top = %q, want name", scored[0].Ref.Name)
	}
}

func TestNormalizeTerms(t *testing.T) {
	got := normalizeTerms([]string{" Calendar ", "", "  ", "MEETING"})
	want := []string{"calendar", "meeting"}
	if !equal(got, want) {
		t.Errorf("normalizeTerms = %v, want %v", got, want)
	}
}

// Both guards fire before any downstream connection, so an empty manager
// exercises them without spawning anything.
func TestSearchRejectsEmptyQuery(t *testing.T) {
	mgr := NewManager(&Config{Servers: map[string]ServerConfig{}})
	if _, err := mgr.Search(context.Background(), []string{"  ", ""}, "", 0); err == nil {
		t.Error("expected error for empty query, got nil")
	}
}

func TestSearchRejectsLimitOverMax(t *testing.T) {
	mgr := NewManager(&Config{Servers: map[string]ServerConfig{}})
	if _, err := mgr.Search(context.Background(), []string{"calendar"}, "", MaxSearchLimit+1); err == nil {
		t.Errorf("expected error for limit > %d, got nil", MaxSearchLimit)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestEnsureDedupesConcurrentConnects verifies concurrent ensure() calls for the
// same server share a single connect (singleflight), so no server is connected
// twice and no downstream leaks. Run with -race to catch map races.
func TestEnsureDedupesConcurrentConnects(t *testing.T) {
	orig := connect
	t.Cleanup(func() { connect = orig })

	var connects atomic.Int32
	connect = func(_ context.Context, name string, _ ServerConfig) (*Downstream, error) {
		connects.Add(1)
		time.Sleep(10 * time.Millisecond) // widen the race window
		return &Downstream{Name: name}, nil
	}

	mgr := NewManager(&Config{Servers: map[string]ServerConfig{"srv": {}}})

	const goroutines = 20
	var wg sync.WaitGroup
	downs := make([]*Downstream, goroutines)
	for i := range goroutines {
		wg.Go(func() {
			d, err := mgr.ensure(context.Background(), "srv")
			if err != nil {
				t.Errorf("ensure: %v", err)
				return
			}
			downs[i] = d
		})
	}
	wg.Wait()

	if got := connects.Load(); got != 1 {
		t.Fatalf("expected exactly 1 connect, got %d", got)
	}
	for i, d := range downs {
		if d != downs[0] {
			t.Fatalf("goroutine %d got a different downstream; cache/dedupe broken", i)
		}
	}
}
