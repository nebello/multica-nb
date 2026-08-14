"use client";

import { useMemo, useState, type ReactElement, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { Check, Loader2, Unplug } from "lucide-react";
import { useAuthStore } from "@multica/core/auth";
import {
  isOpenclawProvider,
  readIssueEnginePin,
  useSetIssueEnginePin,
  type IssueEnginePin,
} from "@multica/core/issues";
import {
  isRuntimeUsableForUser,
  runtimeDisplayLabel,
  runtimeModelsOptions,
} from "@multica/core/runtimes";
import { runtimeListOptions } from "@multica/core/runtimes/queries";
import type { IssueMetadata, RuntimeDevice } from "@multica/core/types";
import {
  Popover,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from "@multica/ui/components/ui/popover";
import { cn } from "@multica/ui/lib/utils";
import { ProviderLogo } from "../../runtimes/components/provider-logo";
import { useT } from "../../i18n";

// The one surface that writes the per-issue engine pin, shared by the chat
// composer (choose at launch) and the issue header chip (see / change / remove
// what is set). Both write the same two metadata keys on the same issue, so
// they are the same panel behind two different triggers rather than two
// pickers that could drift apart.
//
// Deliberately NOT the agent form's RuntimePicker + ModelDropdown: those are
// labelled form fields sized for a settings column, and they always call the
// second field "model" — which is a lie for openclaw, where the value is an
// openclaw AGENT id the daemon passes to `--agent` and openclaw itself decides
// which model answers.

export function IssueEnginePinPopover({
  wsId,
  issueId,
  metadata,
  trigger,
}: {
  wsId: string;
  issueId: string;
  metadata: IssueMetadata | undefined;
  /** Rendered as the popover trigger through Base UI's `render` prop, which
   *  merges its own props (ref, onClick, data-popup-open) into this element. */
  trigger: ReactElement;
}) {
  const { t } = useT("issues");
  const [open, setOpen] = useState(false);
  const currentUserId = useAuthStore((s) => s.user?.id ?? null);
  const pin = readIssueEnginePin(metadata);
  const setPin = useSetIssueEnginePin(wsId, issueId);

  const { data: runtimes = [], isLoading: runtimesLoading } = useQuery({
    ...runtimeListOptions(wsId),
    // The catalog is only needed once the panel is open — mounting it on every
    // issue header would fetch the workspace runtime list on every issue view.
    enabled: open,
  });

  const usableRuntimes = useMemo(
    () => runtimes.filter((r) => isRuntimeUsableForUser(r, currentUserId)),
    [runtimes, currentUserId],
  );
  // A pinned runtime the viewer cannot use still has to render: it is what the
  // issue is actually set to, and hiding it would make the panel disagree with
  // the chip that opened it.
  const selectedRuntime =
    runtimes.find((r) => r.id === pin.runtimeId) ?? null;
  const listedRuntimes = useMemo(
    () =>
      selectedRuntime && !usableRuntimes.some((r) => r.id === selectedRuntime.id)
        ? [selectedRuntime, ...usableRuntimes]
        : usableRuntimes,
    [usableRuntimes, selectedRuntime],
  );

  const openclaw = isOpenclawProvider(selectedRuntime?.provider);

  // Discovery is a round trip to the user's machine: only ask once the panel
  // is open and the pinned runtime is actually reachable.
  const modelsQuery = useQuery(
    runtimeModelsOptions(
      open && selectedRuntime?.status === "online" ? pin.runtimeId : null,
    ),
  );
  const models = useMemo(
    () => modelsQuery.data?.models ?? [],
    [modelsQuery.data],
  );

  const apply = (next: IssueEnginePin) => {
    setPin.mutate(next);
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger render={trigger} />
      <PopoverContent align="end" className="w-80">
        <PopoverHeader>
          <PopoverTitle className="text-body">
            {t(($) => $.engine_pin.title)}
          </PopoverTitle>
          {/* The one line that keeps "what it WILL use" from being read as
              "what it HAS used" — the execution log answers the second. */}
          <PopoverDescription className="text-caption">
            {t(($) => $.engine_pin.description)}
          </PopoverDescription>
        </PopoverHeader>

        <Section label={t(($) => $.engine_pin.runtime_label)}>
          {runtimesLoading && <Pending label={t(($) => $.engine_pin.loading)} />}
          {!runtimesLoading && listedRuntimes.length === 0 && (
            <Empty label={t(($) => $.engine_pin.no_runtimes)} />
          )}
          {listedRuntimes.map((runtime) => (
            <RuntimeRow
              key={runtime.id}
              runtime={runtime}
              selected={runtime.id === pin.runtimeId}
              disabled={setPin.isPending}
              // Switching engine drops the model: ids do not carry across
              // providers, and an openclaw agent id sent to Claude would be
              // dispatched as a model name.
              onSelect={() => apply({ runtimeId: runtime.id, model: "" })}
            />
          ))}
        </Section>

        {selectedRuntime && (
          <Section
            label={
              openclaw
                ? t(($) => $.engine_pin.openclaw_agent_label)
                : t(($) => $.engine_pin.model_label)
            }
            hint={
              openclaw ? t(($) => $.engine_pin.openclaw_agent_hint) : undefined
            }
          >
            <ChoiceRow
              selected={pin.model === ""}
              disabled={setPin.isPending}
              onSelect={() => apply({ runtimeId: pin.runtimeId, model: "" })}
              title={t(($) => $.engine_pin.engine_default)}
            />
            {selectedRuntime.status !== "online" && (
              <Empty label={t(($) => $.engine_pin.runtime_offline)} />
            )}
            {modelsQuery.isLoading && (
              <Pending label={t(($) => $.engine_pin.discovering)} />
            )}
            {modelsQuery.isError && (
              <Empty label={t(($) => $.engine_pin.discovery_failed)} />
            )}
            {/* A pinned value the catalog no longer advertises (an openclaw
                agent that was renamed, a model pulled from the account) still
                needs a selected row, or the panel would show "engine default"
                for an issue that is not on the default. */}
            {pin.model !== "" && !models.some((m) => m.id === pin.model) && (
              <ChoiceRow selected disabled title={pin.model} />
            )}
            {models.map((model) => (
              <ChoiceRow
                key={model.id}
                selected={model.id === pin.model}
                disabled={setPin.isPending}
                onSelect={() =>
                  apply({ runtimeId: pin.runtimeId, model: model.id })
                }
                title={model.label || model.id}
                subtitle={model.label && model.label !== model.id ? model.id : undefined}
              />
            ))}
          </Section>
        )}

        <button
          type="button"
          disabled={setPin.isPending || (!pin.runtimeId && pin.model === "")}
          onClick={() => apply({ runtimeId: null, model: "" })}
          className="flex h-8 items-center justify-center gap-1.5 rounded-md text-caption text-muted-foreground transition-colors hover:bg-accent/60 hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-40"
        >
          <Unplug className="size-3.5 shrink-0" />
          {t(($) => $.engine_pin.remove)}
        </button>
      </PopoverContent>
    </Popover>
  );
}

/**
 * How a pin reads in a collapsed trigger: the engine's name and the
 * model / openclaw-agent line. Shared by the issue header chip and the chat
 * composer pill so the two never describe the same pin differently.
 *
 * The runtime list is fetched only for an issue that actually pins one, so an
 * unpinned issue costs no request.
 */
export function useEnginePinSummary(wsId: string, pin: IssueEnginePin) {
  const { t } = useT("issues");
  const { data: runtimes = [] } = useQuery({
    ...runtimeListOptions(wsId),
    enabled: pin.runtimeId !== null,
  });
  const runtime = runtimes.find((r) => r.id === pin.runtimeId) ?? null;

  const engineName = pin.runtimeId
    ? runtime
      ? runtimeDisplayLabel(runtime)
      : t(($) => $.engine_pin.unknown_runtime)
    : t(($) => $.engine_pin.agent_runtime);

  // openclaw's second value is an agent id, not a model, so it cannot be
  // presented the way a model is presented everywhere else.
  const modelLabel = pin.model
    ? isOpenclawProvider(runtime?.provider)
      ? t(($) => $.engine_pin.chip_openclaw_agent, { agent: pin.model })
      : pin.model
    : t(($) => $.engine_pin.engine_default);

  return { runtime, engineName, modelLabel };
}

function Section({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <div className="flex min-w-0 flex-col">
      <div className="px-1 pb-1 text-caption font-medium uppercase tracking-wide text-muted-foreground">
        {label}
      </div>
      {hint && (
        <div className="px-1 pb-1.5 text-caption text-muted-foreground">
          {hint}
        </div>
      )}
      <div className="flex max-h-52 min-w-0 flex-col overflow-y-auto">
        {children}
      </div>
    </div>
  );
}

function RuntimeRow({
  runtime,
  selected,
  disabled,
  onSelect,
}: {
  runtime: RuntimeDevice;
  selected: boolean;
  disabled: boolean;
  onSelect: () => void;
}) {
  return (
    <ChoiceRow
      selected={selected}
      disabled={disabled}
      onSelect={onSelect}
      title={runtimeDisplayLabel(runtime)}
      leading={
        <span className="relative flex size-4 shrink-0 items-center justify-center">
          <ProviderLogo provider={runtime.provider} className="size-4" />
          <span
            aria-hidden
            className={cn(
              "absolute -bottom-0.5 -right-0.5 size-1.5 rounded-full ring-2 ring-surface-raised",
              runtime.status === "online" ? "bg-success" : "bg-muted-foreground/50",
            )}
          />
        </span>
      }
    />
  );
}

function ChoiceRow({
  selected,
  disabled,
  onSelect,
  title,
  subtitle,
  leading,
}: {
  selected: boolean;
  disabled?: boolean;
  onSelect?: () => void;
  title: string;
  subtitle?: string;
  leading?: ReactNode;
}) {
  return (
    <button
      type="button"
      disabled={disabled || !onSelect}
      onClick={onSelect}
      aria-pressed={selected}
      className={cn(
        "flex min-h-9 w-full min-w-0 items-center gap-2 rounded-md px-2 py-1.5 text-left transition-colors",
        // Selected keeps a weight + color of its own so hovering it never
        // reads as a plain hover row.
        selected ? "bg-accent text-accent-foreground" : "hover:bg-accent/50",
        "focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none",
        "disabled:pointer-events-none",
        disabled && !selected && "opacity-60",
      )}
    >
      {leading}
      <span className="flex min-w-0 flex-1 flex-col">
        <span className={cn("truncate text-body", selected && "font-medium")}>
          {title}
        </span>
        {subtitle && (
          <span className="truncate text-caption text-muted-foreground">
            {subtitle}
          </span>
        )}
      </span>
      {selected && <Check className="size-4 shrink-0 text-primary" />}
    </button>
  );
}

function Pending({ label }: { label: string }) {
  return (
    <div className="flex items-center gap-2 px-2 py-2 text-caption text-muted-foreground">
      <Loader2 className="size-3.5 animate-spin" />
      {label}
    </div>
  );
}

function Empty({ label }: { label: string }) {
  return (
    <div className="px-2 py-2 text-caption text-muted-foreground">{label}</div>
  );
}
