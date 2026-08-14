"use client";

import { useQuery } from "@tanstack/react-query";
import { Cpu } from "lucide-react";
import { readIssueEnginePin, hasIssueEnginePin } from "@multica/core/issues";
import { issueDetailOptions } from "@multica/core/issues/queries";
import { PillButton } from "../../common/pill-button";
import {
  IssueEnginePinPopover,
  useEnginePinSummary,
} from "../../issues/components/issue-engine-pin-popover";
import { useT } from "../../i18n";

// Choose the engine at launch, from the composer.
//
// Sending a chat message that mentions an issue is how work on that issue
// starts, and this pill is the moment before it: pick the runtime (and, on
// openclaw, the openclaw agent) and the choice is written onto THAT issue's
// metadata. From then on every task of that issue is queued and claimed on it —
// the server half already reads the two keys.
//
// The write lands when a value is picked, not when the message is sent, so the
// pill behaves like the project-context pill next to it: what it shows is what
// the issue is set to, right now, whether or not this draft is ever sent.
export function ChatEnginePill({
  wsId,
  issueId,
}: {
  wsId: string;
  /** The single issue this draft mentions — the pill is not rendered without
   *  one, because the pin has to land on a specific issue. */
  issueId: string;
}) {
  const { t } = useT("issues");
  const { data: issue } = useQuery(issueDetailOptions(wsId, issueId));
  const pin = readIssueEnginePin(issue?.metadata);
  const pinned = hasIssueEnginePin(pin);
  const { engineName, modelLabel } = useEnginePinSummary(wsId, pin);

  // The issue has to resolve before the popover can write to it: the metadata
  // endpoints key off the issue's own id, and the mention may carry an
  // identifier ("MUL-123") rather than a uuid.
  if (!issue) return null;

  const label = pinned
    ? t(($) => $.engine_pin.chip_aria, { engine: engineName, model: modelLabel })
    : t(($) => $.engine_pin.chat_pill_unset_aria, { issue: issue.identifier });

  return (
    <IssueEnginePinPopover
      wsId={wsId}
      issueId={issue.id}
      metadata={issue.metadata}
      trigger={
        <PillButton
          aria-label={label}
          title={label}
          className="h-6 max-w-[13rem] border-surface-border bg-surface-raised text-muted-foreground"
        >
          <Cpu className="size-3.5 shrink-0" />
          <span className="truncate">
            {pinned
              ? `${engineName} · ${modelLabel}`
              : t(($) => $.engine_pin.chat_pill_unset)}
          </span>
        </PillButton>
      }
    />
  );
}
