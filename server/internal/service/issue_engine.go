package service

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Per-issue engine override (NEB-648).
//
// Engine (runtime) and model normally live on the agent row and therefore
// apply to everything that agent does. These two metadata keys pin a choice to
// a SINGLE issue instead, so the same agent can run on openclaw here and on
// Claude everywhere else without being duplicated:
//
//	multica issue metadata set <issue> --key engine_runtime_id --value <runtime uuid>
//	multica issue metadata set <issue> --key engine_model      --value <model or openclaw agent id>
//
// The keys are namespaced with engine_ because issue.metadata is a free-form
// bag agents already write pipeline state into; a bare "model" would collide
// with a workflow key by accident.
//
// For an openclaw runtime engine_model is NOT a model: the daemon passes it as
// `--agent`, i.e. the id of an openclaw agent that is itself configured with a
// model (server/pkg/agent/openclaw.go).
const (
	IssueEngineRuntimeKey = "engine_runtime_id"
	IssueEngineModelKey   = "engine_model"
)

// IssueEngineOverride is the engine choice pinned to an issue. A zero value
// means "no choice pinned" — the agent's own runtime and model are used.
type IssueEngineOverride struct {
	// RuntimeID is valid only when the issue pins a runtime AND the value
	// parses as a UUID. Validity against the runtime table is a separate,
	// query-backed step (ResolveIssueRuntimeID) — parsing alone proves
	// nothing about whether the runtime exists.
	RuntimeID pgtype.UUID
	Model     string
}

// issueEngineQueries is the slice of the generated query set this file needs,
// so both the service layer (enqueue) and the handler layer (claim) can reuse
// the resolution without either importing the other.
type issueEngineQueries interface {
	GetAgentRuntimeForWorkspace(ctx context.Context, arg db.GetAgentRuntimeForWorkspaceParams) (db.AgentRuntime, error)
}

// ParseIssueEngineOverride reads the pinned engine choice out of an issue's
// metadata blob. It never fails: metadata is written through a free-form CLI
// surface, so anything unparseable is treated as "not pinned" rather than as
// an error that would block a task from being queued.
func ParseIssueEngineOverride(metadata []byte) IssueEngineOverride {
	var out IssueEngineOverride
	bag := util.JSONObjectOrEmpty(metadata)
	if s, ok := bag[IssueEngineRuntimeKey].(string); ok && s != "" {
		var id pgtype.UUID
		if err := id.Scan(s); err == nil {
			out.RuntimeID = id
		}
	}
	if s, ok := bag[IssueEngineModelKey].(string); ok {
		out.Model = s
	}
	return out
}

// ResolveIssueRuntimeID returns the runtime an issue-bound task must be queued
// on: the issue's pinned runtime when it names a real runtime of the issue's
// own workspace, the agent's runtime otherwise.
//
// Fail-safe by construction. agent_task_queue.runtime_id is NOT NULL with an FK
// to agent_runtime, so queueing a task on a runtime id that does not exist
// would make the INSERT fail and the trigger (a comment, an assignment) would
// silently produce no run at all. Every doubt therefore falls back to the
// agent's own runtime — a task on the wrong engine is recoverable, a task that
// was never created is not. Scoping the lookup to issue.WorkspaceID also keeps
// a pinned id from dispatching an agent's brief to another workspace's machine.
func ResolveIssueRuntimeID(ctx context.Context, q issueEngineQueries, issue db.Issue, agentRuntimeID pgtype.UUID) pgtype.UUID {
	ov := ParseIssueEngineOverride(issue.Metadata)
	if !ov.RuntimeID.Valid || ov.RuntimeID == agentRuntimeID {
		return agentRuntimeID
	}
	if _, err := q.GetAgentRuntimeForWorkspace(ctx, db.GetAgentRuntimeForWorkspaceParams{
		ID:          ov.RuntimeID,
		WorkspaceID: issue.WorkspaceID,
	}); err != nil {
		slog.Warn("issue engine override ignored: runtime not usable",
			"issue_id", util.UUIDToString(issue.ID),
			"runtime_id", util.UUIDToString(ov.RuntimeID),
			"error", err)
		return agentRuntimeID
	}
	return ov.RuntimeID
}

// ApplyIssueEngineOverride rewrites the model fields of a claimed task's agent
// payload from the issue's pinned choice. model/thinkingLevel/serviceTier come
// in from the agent row and go back out as the values the daemon must actually
// run with.
//
// Pinning a RUNTIME clears thinking level and service tier: both are
// provider-specific tunings of the agent's own engine and mean nothing on the
// engine the issue selected. The pinned model replaces the agent's for the same
// reason — an empty pinned model is a deliberate "let the selected engine use
// its default", not a gap to fill with a model id from another provider.
func ApplyIssueEngineOverride(metadata []byte, model, thinkingLevel, serviceTier string) (string, string, string) {
	ov := ParseIssueEngineOverride(metadata)
	if ov.RuntimeID.Valid {
		return ov.Model, "", ""
	}
	if ov.Model != "" {
		return ov.Model, thinkingLevel, serviceTier
	}
	return model, thinkingLevel, serviceTier
}
