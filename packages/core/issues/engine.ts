import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { useWorkspaceId } from "../hooks";
import { onIssueMetadataChanged } from "./ws-updaters";
import type { Issue } from "../types";

/**
 * Per-issue engine pin (NEB-648).
 *
 * Engine (runtime) and model normally live on the agent and apply to
 * everything it does. These two metadata keys pin a choice to ONE issue: every
 * task of that issue runs on that engine, whoever picks it up, while the same
 * agent keeps its own engine everywhere else.
 *
 * The keys are read by the server at enqueue and at claim
 * (`server/internal/service/issue_engine.go`) — they are a contract, not a
 * client-side convention, so they are spelled out here once and imported.
 *
 * `engineModel` is a model id for most runtimes, but an openclaw AGENT id when
 * the pinned runtime's provider is `openclaw`: the daemon passes it as
 * `--agent`, and which model answers is decided inside openclaw. The UI must
 * label it accordingly — see `isOpenclawProvider`.
 */
export const ISSUE_ENGINE_RUNTIME_KEY = "engine_runtime_id";
export const ISSUE_ENGINE_MODEL_KEY = "engine_model";

export type IssueEnginePin = {
  /** Pinned runtime id, or null when the issue follows the agent's engine. */
  runtimeId: string | null;
  /** Pinned model / openclaw agent id. Empty means "the engine's default". */
  model: string;
};

/**
 * Read the pin off an issue. Non-string values cannot occur through the
 * selector, but metadata is a free-form bag any agent can write, so anything
 * that is not a non-empty string reads as "not pinned" rather than as a value
 * the UI would render as `[object Object]`.
 */
export function readIssueEnginePin(issue: Pick<Issue, "metadata"> | undefined): IssueEnginePin {
  const bag = issue?.metadata ?? {};
  const runtime = bag[ISSUE_ENGINE_RUNTIME_KEY];
  const model = bag[ISSUE_ENGINE_MODEL_KEY];
  return {
    runtimeId: typeof runtime === "string" && runtime !== "" ? runtime : null,
    model: typeof model === "string" ? model : "",
  };
}

/** The one provider whose "model" field is not a model. */
export function isOpenclawProvider(provider: string | undefined): boolean {
  return provider === "openclaw";
}

/**
 * Write or clear one engine key.
 *
 * Not optimistic: the enqueue path reads these keys, so showing a pin the
 * server may have rejected would tell the user their next run goes somewhere
 * it does not. The server answers with the full post-write bag, which is
 * applied through the same updater the `issue_metadata:changed` event uses —
 * so a write and a remote change converge on one code path.
 */
export function useSetIssueEngineKey() {
  const qc = useQueryClient();
  const wsId = useWorkspaceId();
  return useMutation({
    mutationFn: ({ issueId, key, value }: { issueId: string; key: string; value: string | null }) =>
      value === null
        ? api.deleteIssueMetadataKey(issueId, key)
        : api.setIssueMetadataKey(issueId, key, value),
    // Serialized per issue: clearing the engine clears two keys back to back,
    // and the second response must not be overtaken by the first.
    scope: { id: `issue-engine:${wsId}` },
    onSuccess: (data, { issueId }) => {
      onIssueMetadataChanged(qc, wsId, issueId, data.metadata ?? {});
    },
  });
}
