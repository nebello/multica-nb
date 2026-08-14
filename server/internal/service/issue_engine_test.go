package service

import (
	"context"
	"encoding/json"
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

// recordingMetadataQueries captures the metadata writes inheritance performs
// and echoes back a child whose bag contains them, so the test can assert on
// the returned row as well as on the calls.
type recordingMetadataQueries struct {
	written map[string]string
	err     error
}

func (r *recordingMetadataQueries) SetIssueMetadataKey(_ context.Context, arg db.SetIssueMetadataKeyParams) (db.Issue, error) {
	if r.err != nil {
		return db.Issue{}, r.err
	}
	if r.written == nil {
		r.written = map[string]string{}
	}
	var value string
	if err := json.Unmarshal(arg.Value, &value); err != nil {
		return db.Issue{}, err
	}
	r.written[arg.Key] = value
	bag, _ := json.Marshal(r.written)
	return db.Issue{ID: arg.ID, WorkspaceID: arg.WorkspaceID, Metadata: bag}, nil
}

func TestInheritIssueEngine(t *testing.T) {
	child := db.Issue{ID: mustUUID(t, testRuntimeA), WorkspaceID: mustUUID(t, testRuntimeB)}

	t.Run("parent pinning nothing writes nothing", func(t *testing.T) {
		q := &recordingMetadataQueries{}
		parent := db.Issue{Metadata: []byte(`{"pr_url":"https://example.test/1"}`)}
		got, err := InheritIssueEngine(context.Background(), q, parent, child)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if len(q.written) != 0 {
			t.Fatalf("wrote %v, want no writes", q.written)
		}
		if len(got.Metadata) != 0 {
			t.Fatalf("child metadata = %s, want untouched", got.Metadata)
		}
	})

	t.Run("engine and model are both copied down", func(t *testing.T) {
		q := &recordingMetadataQueries{}
		parent := db.Issue{Metadata: []byte(`{"engine_runtime_id":"` + testRuntimeB + `","engine_model":"deepseek"}`)}
		got, err := InheritIssueEngine(context.Background(), q, parent, child)
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if q.written[IssueEngineRuntimeKey] != testRuntimeB || q.written[IssueEngineModelKey] != "deepseek" {
			t.Fatalf("wrote %v, want the parent's engine and model", q.written)
		}
		// The returned child must carry the inherited keys: the assigned task is
		// queued from this row inside the same transaction.
		if ov := ParseIssueEngineOverride(got.Metadata); util.UUIDToString(ov.RuntimeID) != testRuntimeB || ov.Model != "deepseek" {
			t.Fatalf("returned child resolves to %+v, want the inherited engine", ov)
		}
	})

	t.Run("a parent pinning only the engine copies only the engine", func(t *testing.T) {
		q := &recordingMetadataQueries{}
		parent := db.Issue{Metadata: []byte(`{"engine_runtime_id":"` + testRuntimeB + `"}`)}
		if _, err := InheritIssueEngine(context.Background(), q, parent, child); err != nil {
			t.Fatalf("err = %v", err)
		}
		if _, present := q.written[IssueEngineModelKey]; present {
			t.Fatalf("wrote %v, want no model key", q.written)
		}
	})

	t.Run("a failed copy fails the create instead of half-inheriting", func(t *testing.T) {
		q := &recordingMetadataQueries{err: errors.New("connection refused")}
		parent := db.Issue{Metadata: []byte(`{"engine_runtime_id":"` + testRuntimeB + `"}`)}
		if _, err := InheritIssueEngine(context.Background(), q, parent, child); err == nil {
			t.Fatal("err = nil, want the write failure surfaced")
		}
	})
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
