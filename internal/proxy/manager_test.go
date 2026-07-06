package proxy

import (
	"context"
	"os"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func writeFile(t *testing.T, Path, Content string) {
	t.Helper()
	if Err := os.WriteFile(Path, []byte(Content), 0o644); Err != nil {
		t.Fatalf("writing %s: %v", Path, Err)
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
	Cases := []struct {
		Name         string
		Tool         *mcp.Tool
		Terms        []string
		WantScore    int
		WantTerms    int
		WantFields   []string
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

	for _, C := range Cases {
		t.Run(C.Name, func(t *testing.T) {
			S, Ok := scoreTool("srv", C.Tool, C.Terms)
			if !Ok {
				t.Fatalf("expected a match")
			}
			if S.Score != C.WantScore {
				t.Errorf("score = %d, want %d", S.Score, C.WantScore)
			}
			if S.TermsMatched != C.WantTerms {
				t.Errorf("termsMatched = %d, want %d", S.TermsMatched, C.WantTerms)
			}
			if !equal(S.Ref.MatchedFields, C.WantFields) {
				t.Errorf("matchedFields = %v, want %v", S.Ref.MatchedFields, C.WantFields)
			}
		})
	}
}

func TestScoreToolNoMatch(t *testing.T) {
	if _, Ok := scoreTool("srv", &mcp.Tool{Name: "a", Description: "b"}, []string{"nope"}); Ok {
		t.Error("expected no match")
	}
}

func TestSortByRelevanceBreadthFirst(t *testing.T) {
	// A tool matching 2 terms (only in schema, low field score) must outrank a
	// tool matching 1 term with a high field score (name). Breadth wins.
	Scored := []toolScore{
		{Ref: ToolRef{Name: "name_hit"}, TermsMatched: 1, Score: 6},
		{Ref: ToolRef{Name: "two_schema_hits"}, TermsMatched: 2, Score: 2},
	}
	sortByRelevance(Scored)
	if Scored[0].Ref.Name != "two_schema_hits" {
		t.Errorf("breadth-first failed: top = %q, want two_schema_hits", Scored[0].Ref.Name)
	}
}

func TestSortByRelevanceFieldTiebreak(t *testing.T) {
	// Same breadth → higher field score wins.
	Scored := []toolScore{
		{Ref: ToolRef{Name: "schema"}, TermsMatched: 1, Score: 1},
		{Ref: ToolRef{Name: "name"}, TermsMatched: 1, Score: 6},
	}
	sortByRelevance(Scored)
	if Scored[0].Ref.Name != "name" {
		t.Errorf("field tiebreak failed: top = %q, want name", Scored[0].Ref.Name)
	}
}

func TestNormalizeTerms(t *testing.T) {
	Got := normalizeTerms([]string{" Calendar ", "", "  ", "MEETING"})
	Want := []string{"calendar", "meeting"}
	if !equal(Got, Want) {
		t.Errorf("normalizeTerms = %v, want %v", Got, Want)
	}
}

// Both guards fire before any downstream connection, so an empty manager
// exercises them without spawning anything.
func TestSearchRejectsEmptyQuery(t *testing.T) {
	Mgr := NewManager(&Config{Servers: map[string]ServerConfig{}})
	if _, Err := Mgr.Search(context.Background(), []string{"  ", ""}, "", 0); Err == nil {
		t.Error("expected error for empty query, got nil")
	}
}

func TestSearchRejectsLimitOverMax(t *testing.T) {
	Mgr := NewManager(&Config{Servers: map[string]ServerConfig{}})
	if _, Err := Mgr.Search(context.Background(), []string{"calendar"}, "", MaxSearchLimit+1); Err == nil {
		t.Errorf("expected error for limit > %d, got nil", MaxSearchLimit)
	}
}

func equal(A, B []string) bool {
	if len(A) != len(B) {
		return false
	}
	for I := range A {
		if A[I] != B[I] {
			return false
		}
	}
	return true
}
