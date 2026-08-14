"use client";

import {
  readIssueEnginePin,
  hasIssueEnginePin,
  type IssueEnginePin,
} from "@multica/core/issues";
import type { Issue } from "@multica/core/types";
import { ProviderLogo } from "../../runtimes/components/provider-logo";
import { PillButton } from "../../common/pill-button";
import { useT } from "../../i18n";
import {
  IssueEnginePinPopover,
  useEnginePinSummary,
} from "./issue-engine-pin-popover";

// "What will this issue run on?" — the setting, in the issue header, readable
// BEFORE a run starts.
//
// Deliberately not near the execution log, which answers the other question:
// what each past run actually used (`task_usage`). A consumptive record read as
// a setting is a silent mistake, so the two never share a surface and this one
// says "set" in its own label.
//
// Renders nothing on an issue with no pin — which is every issue until someone
// chooses an engine, and the state in which the agent's own runtime and model
// apply exactly as before.
export function IssueEngineHeaderChip({
  wsId,
  issue,
}: {
  wsId: string;
  issue: Issue;
}) {
  const pin = readIssueEnginePin(issue.metadata);
  if (!hasIssueEnginePin(pin)) return null;
  return (
    <PinnedChip
      wsId={wsId}
      issueId={issue.id}
      metadata={issue.metadata}
      pin={pin}
    />
  );
}

function PinnedChip({
  wsId,
  issueId,
  metadata,
  pin,
}: {
  wsId: string;
  issueId: string;
  metadata: Issue["metadata"];
  pin: IssueEnginePin;
}) {
  const { t } = useT("issues");
  const { runtime, engineName, modelLabel } = useEnginePinSummary(wsId, pin);

  const label = t(($) => $.engine_pin.chip_aria, {
    engine: engineName,
    model: modelLabel,
  });

  return (
    <div className="flex items-center gap-1">
      <IssueEnginePinPopover
        wsId={wsId}
        issueId={issueId}
        metadata={metadata}
        trigger={
          <PillButton
            aria-label={label}
            title={label}
            className="h-7 max-w-[15rem] border-surface-border bg-surface-raised text-muted-foreground"
          >
            {runtime ? (
              <ProviderLogo
                provider={runtime.provider}
                className="size-3.5 shrink-0"
              />
            ) : null}
            <span className="shrink-0 font-medium text-foreground">
              {t(($) => $.engine_pin.chip_prefix)}
            </span>
            {/* One truncating string, not two: a header chip has room for a
                single value, and putting the engine name first would eat the
                model the chip exists to show. The provider logo already says
                which engine this is, so the model / openclaw-agent takes the
                label — and the engine name takes it back when no model is set,
                since "engine default" alone names nothing. The full pair stays
                in the tooltip and the accessible name. */}
            <span className="truncate">{pin.model ? modelLabel : engineName}</span>
          </PillButton>
        }
      />
      {/* Hairline against the header action buttons, matching the live agent
          chip: this is a status segment, not another action. */}
      <span className="h-4 w-px bg-border" aria-hidden="true" />
    </div>
  );
}
