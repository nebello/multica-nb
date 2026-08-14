import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import { onIssueMetadataChanged } from "./ws-updaters";
import type { IssueMetadata } from "../types";

// Per-issue engine pin (NEB-648 / NEB-656).
//
// Engine (runtime) and model normally live on the agent row and apply to
// everything that agent does. These two reserved metadata keys pin a choice to
// a SINGLE issue instead, so the same agent runs on openclaw here and on Claude
// everywhere else without being duplicated. The server reads them at enqueue
// (runtime) and at claim (model) — see server/internal/service/issue_engine.go,
// which owns the same two constants. Keep both sides in sync.
//
// An issue WITHOUT these keys behaves exactly as before: the agent's own
// runtime and model are used.
export const ISSUE_ENGINE_RUNTIME_KEY = "engine_runtime_id";
export const ISSUE_ENGINE_MODEL_KEY = "engine_model";

/**
 * The engine choice pinned to an issue. `runtimeId` null means "not pinned";
 * `model` empty means "let the selected engine pick its own default", which is
 * a deliberate state, not a gap.
 *
 * For an openclaw runtime `model` is NOT a model: the daemon passes it to
 * `--agent`, i.e. the id of an openclaw agent that is itself configured with a
 * model (server/pkg/agent/openclaw.go). Any UI that shows this value must label
 * it accordingly — see `isOpenclawProvider`.
 */
export interface IssueEnginePin {
  runtimeId: string | null;
  model: string;
}

/**
 * Read the pin out of an issue's metadata bag. Never throws: metadata is a
 * free-form surface agents and the CLI write into, so a non-string value is
 * read as "not pinned" rather than as an error.
 */
export function readIssueEnginePin(
  metadata: IssueMetadata | undefined,
): IssueEnginePin {
  const runtime = metadata?.[ISSUE_ENGINE_RUNTIME_KEY];
  const model = metadata?.[ISSUE_ENGINE_MODEL_KEY];
  return {
    runtimeId: typeof runtime === "string" && runtime !== "" ? runtime : null,
    model: typeof model === "string" ? model : "",
  };
}

/** True when the issue carries any engine pin at all. */
export function hasIssueEnginePin(pin: IssueEnginePin): boolean {
  return pin.runtimeId !== null || pin.model !== "";
}

/**
 * openclaw is the one provider whose "model" field is an agent id, so the
 * pickers and the header chip must relabel themselves for it. Matched on the
 * provider slug the daemon registers the runtime with.
 */
export function isOpenclawProvider(provider: string | undefined): boolean {
  return provider === "openclaw";
}

/**
 * Write / clear the pin on one issue.
 *
 * Mutations are per key because the metadata bag is shared with the pipeline
 * keys agents write concurrently — a whole-blob PUT would race them. Selecting
 * a runtime also drops the model: model ids do not carry across engines, and an
 * openclaw agent id on a Claude runtime would be dispatched as a model name.
 *
 * No optimistic patch: the write leaves the user on the same screen but decides
 * where the NEXT run executes, so the chip must show what the server actually
 * stored. The response carries the whole bag back and is pushed through the
 * same cache updater the `issue_metadata:changed` event uses.
 */
export function useSetIssueEnginePin(wsId: string, issueId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (pin: IssueEnginePin): Promise<IssueMetadata> => {
      let metadata: IssueMetadata;
      if (pin.runtimeId) {
        metadata = await api.setIssueMetadataKey(
          issueId,
          ISSUE_ENGINE_RUNTIME_KEY,
          pin.runtimeId,
        );
      } else {
        metadata = await api.deleteIssueMetadataKey(
          issueId,
          ISSUE_ENGINE_RUNTIME_KEY,
        );
      }
      metadata = pin.model
        ? await api.setIssueMetadataKey(
            issueId,
            ISSUE_ENGINE_MODEL_KEY,
            pin.model,
          )
        : await api.deleteIssueMetadataKey(issueId, ISSUE_ENGINE_MODEL_KEY);
      return metadata;
    },
    onSuccess: (metadata) => {
      onIssueMetadataChanged(qc, wsId, issueId, metadata);
    },
  });
}
