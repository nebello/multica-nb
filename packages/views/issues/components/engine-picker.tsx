"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Cpu } from "lucide-react";
import { useWorkspaceId } from "@multica/core/hooks";
import { runtimeListOptions } from "@multica/core/runtimes";
import { runtimeDisplayLabel } from "@multica/core/runtimes";
import {
  ISSUE_ENGINE_MODEL_KEY,
  ISSUE_ENGINE_RUNTIME_KEY,
  isOpenclawProvider,
  readIssueEnginePin,
  useSetIssueEngineKey,
} from "@multica/core/issues";
import type { AgentRuntime, Issue } from "@multica/core/types";
import { ModelPicker } from "../../agents/components/inspector/model-picker";
import { PropRow } from "../../common/prop-row";
import { PickerItem, PropertyPicker } from "./pickers";
import { PICKER_TRIGGER_CLASS } from "./pickers/property-picker";
import { useT } from "../../i18n";

/**
 * The issue's pinned ENGINE — what the next run will use, not what past runs
 * used. The execution log below already reports the model each run consumed;
 * these rows are the setting, so they live among the other properties and read
 * as a choice, never as a receipt (NEB-648).
 *
 * Two rows rather than one control: the model catalog belongs to the runtime,
 * so it can only be offered once a runtime is chosen, and it stays absent
 * while the issue follows the agent's own engine.
 */
export function IssueEngineRows({
  issue,
  defaultOpen = false,
}: {
  issue: Issue;
  defaultOpen?: boolean;
}) {
  const { t } = useT("issues");
  const wsId = useWorkspaceId();
  const [open, setOpen] = useState(defaultOpen);
  const pin = readIssueEnginePin(issue);
  const setEngineKey = useSetIssueEngineKey();

  const runtimesQuery = useQuery(runtimeListOptions(wsId));
  const runtimes: AgentRuntime[] = runtimesQuery.data ?? [];
  const pinned = runtimes.find((r) => r.id === pin.runtimeId);

  // A pinned runtime the workspace no longer lists (deleted, or not visible to
  // this member) still has to render as something: showing an empty chip would
  // read as "no engine pinned", which is the opposite of the truth — the
  // server will still route this issue's tasks to that id if it exists.
  const runtimeLabel = pinned
    ? runtimeDisplayLabel(pinned)
    : pin.runtimeId
      ? t(($) => $.detail.engine_unknown_runtime)
      : t(($) => $.detail.engine_agent_default);

  const selectRuntime = async (runtimeId: string | null) => {
    setOpen(false);
    if (runtimeId === pin.runtimeId) return;
    await setEngineKey.mutateAsync({ issueId: issue.id, key: ISSUE_ENGINE_RUNTIME_KEY, value: runtimeId });
    // The model belongs to the engine that declared it: carrying it across a
    // switch would send one runtime's model id to another, and on openclaw an
    // agent id that does not exist there. Clearing means "the new engine's
    // default" until the user picks again.
    if (pin.model) {
      await setEngineKey.mutateAsync({ issueId: issue.id, key: ISSUE_ENGINE_MODEL_KEY, value: null });
    }
  };

  const setModel = async (model: string) => {
    await setEngineKey.mutateAsync({
      issueId: issue.id,
      key: ISSUE_ENGINE_MODEL_KEY,
      value: model === "" ? null : model,
    });
  };

  return (
    <>
      <PropRow label={t(($) => $.detail.prop_engine)}>
        <PropertyPicker
          open={open}
          onOpenChange={setOpen}
          align="start"
          width="w-auto min-w-[14rem] max-w-xs"
          tooltip={t(($) => $.detail.engine_tooltip)}
          triggerRender={
            <button
              type="button"
              className={PICKER_TRIGGER_CLASS}
              aria-label={t(($) => $.detail.prop_engine)}
            />
          }
          trigger={
            <>
              <Cpu className="size-3.5 shrink-0 text-muted-foreground" aria-hidden="true" />
              <span className={`min-w-0 truncate ${pin.runtimeId ? "" : "text-muted-foreground"}`}>
                {runtimeLabel}
              </span>
            </>
          }
        >
          <PickerItem
            selected={pin.runtimeId === null}
            emptyValue
            onClick={() => void selectRuntime(null)}
          >
            <span className="truncate text-muted-foreground">
              {t(($) => $.detail.engine_agent_default)}
            </span>
          </PickerItem>
          {runtimes.map((r) => (
            <PickerItem
              key={r.id}
              selected={r.id === pin.runtimeId}
              onClick={() => void selectRuntime(r.id)}
              tooltip={r.status === "online" ? undefined : t(($) => $.detail.engine_runtime_offline)}
            >
              <span className="min-w-0 flex-1 truncate text-left">{runtimeDisplayLabel(r)}</span>
              {r.status !== "online" && (
                <span className="shrink-0 text-micro text-muted-foreground">
                  {t(($) => $.detail.engine_runtime_offline)}
                </span>
              )}
            </PickerItem>
          ))}
        </PropertyPicker>
      </PropRow>

      {pin.runtimeId !== null && (
        <PropRow
          label={
            isOpenclawProvider(pinned?.provider)
              ? t(($) => $.detail.prop_engine_openclaw_agent)
              : t(($) => $.detail.prop_engine_model)
          }
        >
          {/* A pinned engine with no model is complete, not half-filled: it
              means "use whatever that engine runs by default", which is what
              the server does with an empty value. Hence the muted placeholder
              instead of a warning. */}
          <ModelPicker
            runtimeId={pin.runtimeId}
            runtimeOnline={pinned?.status === "online"}
            value={pin.model}
            onChange={setModel}
          />
        </PropRow>
      )}
    </>
  );
}
