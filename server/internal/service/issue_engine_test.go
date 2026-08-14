package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

const (
	testRuntimeA = "11111111-1111-1111-1111-111111111111"
	testRuntimeB = "22222222-2222-2222-2222-222222222222"
)

func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(s); err != nil {
		t.Fatalf("scan uuid %q: %v", s, err)
	}
	return id
}

// fakeIssueEngineQueries answers the runtime lookup with a fixed set of ids
// that exist in the queried workspace; anything else is "no rows", the same
// shape a deleted or cross-workspace runtime produces.
type fakeIssueEngineQueries struct {
	existing map[string]bool
	calls    int
}

func (f *fakeIssueEngineQueries) GetAgentRuntimeForWorkspace(_ context.Context, arg db.GetAgentRuntimeForWorkspaceParams) (db.AgentRuntime, error) {
	f.calls++
	if f.existing[util.UUIDToString(arg.ID)] {
		return db.AgentRuntime{ID: arg.ID, WorkspaceID: arg.WorkspaceID}, nil
	}
	return db.AgentRuntime{}, pgx.ErrNoRows
}

func TestResolveIssueRuntimeID(t *testing.T) {
	agentRuntime := mustUUID(t, testRuntimeA)
	pinned := mustUUID(t, testRuntimeB)

	cases := []struct {
		name      string
		metadata  string
		existing  map[string]bool
		want      pgtype.UUID
		wantCalls int
	}{
		{
			name:     "no override falls back to the agent runtime",
			metadata: `{"pr_url":"https://example.test/1"}`,
			want:     agentRuntime,
		},
		{
			name:     "pinned runtime wins when it exists in the workspace",
			metadata: `{"engine_runtime_id":"` + testRuntimeB + `"}`,
			existing: map[string]bool{testRuntimeB: true},
			want:     pinned, wantCalls: 1,
		},
		{
			name:      "unknown runtime is ignored rather than failing the enqueue",
			metadata:  `{"engine_runtime_id":"` + testRuntimeB + `"}`,
			existing:  map[string]bool{},
			want:      agentRuntime,
			wantCalls: 1,
		},
		{
			name:     "garbage value is ignored without a lookup",
			metadata: `{"engine_runtime_id":"deepseek"}`,
			want:     agentRuntime,
		},
		{
			name:     "pinning the agent's own runtime skips the lookup",
			metadata: `{"engine_runtime_id":"` + testRuntimeA + `"}`,
			want:     agentRuntime,
		},
		{
			name:     "empty metadata falls back",
			metadata: ``,
			want:     agentRuntime,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := &fakeIssueEngineQueries{existing: tc.existing}
			issue := db.Issue{Metadata: []byte(tc.metadata)}
			got := ResolveIssueRuntimeID(context.Background(), q, issue, agentRuntime)
			if got != tc.want {
				t.Fatalf("runtime = %v, want %v", got, tc.want)
			}
			if q.calls != tc.wantCalls {
				t.Fatalf("runtime lookups = %d, want %d", q.calls, tc.wantCalls)
			}
		})
	}
}

func TestResolveIssueRuntimeIDIgnoresLookupErrors(t *testing.T) {
	agentRuntime := mustUUID(t, testRuntimeA)
	q := &erroringIssueEngineQueries{}
	issue := db.Issue{Metadata: []byte(`{"engine_runtime_id":"` + testRuntimeB + `"}`)}
	if got := ResolveIssueRuntimeID(context.Background(), q, issue, agentRuntime); got != agentRuntime {
		t.Fatalf("runtime = %v, want the agent default %v", got, agentRuntime)
	}
}

type erroringIssueEngineQueries struct{}

func (erroringIssueEngineQueries) GetAgentRuntimeForWorkspace(context.Context, db.GetAgentRuntimeForWorkspaceParams) (db.AgentRuntime, error) {
	return db.AgentRuntime{}, errors.New("connection refused")
}

func TestApplyIssueEngineOverride(t *testing.T) {
	cases := []struct {
		name                           string
		metadata                       string
		wantModel, wantThink, wantTier string
	}{
		{
			name:      "no override keeps the agent's own tuning",
			metadata:  `{}`,
			wantModel: "claude-opus-5", wantThink: "high", wantTier: "priority",
		},
		{
			name:      "model-only override keeps thinking and tier",
			metadata:  `{"engine_model":"claude-sonnet-5"}`,
			wantModel: "claude-sonnet-5", wantThink: "high", wantTier: "priority",
		},
		{
			name:      "runtime override drops provider-specific tuning",
			metadata:  `{"engine_runtime_id":"` + testRuntimeB + `","engine_model":"deepseek"}`,
			wantModel: "deepseek",
		},
		{
			name:     "runtime override without a model runs the engine's default",
			metadata: `{"engine_runtime_id":"` + testRuntimeB + `"}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model, think, tier := ApplyIssueEngineOverride([]byte(tc.metadata), "claude-opus-5", "high", "priority")
			if model != tc.wantModel || think != tc.wantThink || tier != tc.wantTier {
				t.Fatalf("got (%q, %q, %q), want (%q, %q, %q)",
					model, think, tier, tc.wantModel, tc.wantThink, tc.wantTier)
			}
		})
	}
}
