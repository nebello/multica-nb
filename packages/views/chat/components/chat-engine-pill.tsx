"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Cpu } from "lucide-react";
import { issueDetailOptions } from "@multica/core/issues";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@multica/ui/components/ui/popover";
import { PillButton } from "../../common/pill-button";
import {
  IssueEngineRows,
  useIssueEnginePin,
} from "../../issues/components/engine-picker";
import { useT } from "../../i18n";

/**
 * Choose the engine at launch, from the composer (NEB-668).
 *
 * Sending a chat message that mentions an issue is how work on that issue
 * starts, and this pill is the moment before it: pick the runtime (and, on
 * openclaw, the openclaw agent) and the choice is written onto THAT issue's
 * metadata, which the server already reads at enqueue and at claim.
 *
 * The popover hosts the issue sidebar's own Engine rows rather than a second
 * control of its own: one panel, one pair of keys, one label — a chat-side copy
 * would be free to drift from the row the issue shows.
 *
 * The write lands when a value is picked, not when the message is sent, so the
 * pill behaves like the project-context pill next to it: what it shows is what
 * the issue is set to, right now, whether or not this draft is ever sent.
 */
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
  const [open, setOpen] = useState(false);
  const { data: issue } = useQuery(issueDetailOptions(wsId, issueId));
  const { runtimeLabel } = useIssueEnginePin(issue);

  // The issue has to resolve before the rows can write to it: they key off the
  // issue's own id, and the mention may carry an identifier ("MUL-123") rather
  // than a uuid.
  if (!issue) return null;

  const label = t(($) => $.detail.engine_chat_pill, {
    issue: issue.identifier,
  });

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <PillButton
            aria-label={label}
            title={label}
            // A floor as well as a ceiling: the composer's bottom row is
            // shared with the agent pill, and a runtime name like
            // "MacBook Pro · OpenClaw" otherwise shrinks to four letters in
            // the narrow floating chat. Full text stays in the title/aria.
            className="h-6 min-w-24 max-w-[11rem] border-surface-border bg-surface-raised text-muted-foreground"
          />
        }
      >
        <Cpu className="size-3.5 shrink-0" />
        <span className="truncate">{runtimeLabel}</span>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-auto min-w-64 gap-1.5 p-2">
        {/* Which issue the pin lands on, stated: the composer can hold several
            mentions before one survives, and the pill only appears for the
            last one standing. */}
        <div className="px-2 text-caption text-muted-foreground">{label}</div>
        <div className="grid grid-cols-[auto_1fr] gap-x-2 gap-y-0.5">
          <IssueEngineRows issue={issue} />
        </div>
      </PopoverContent>
    </Popover>
  );
}
