"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Send, Trash2 } from "lucide-react";
import { Button } from "@multica/ui/components/ui/button";
import { Card, CardContent } from "@multica/ui/components/ui/card";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@multica/ui/components/ui/select";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@multica/ui/components/ui/alert-dialog";
import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceId } from "@multica/core/hooks";
import { agentListOptions, memberListOptions } from "@multica/core/workspace/queries";
import { telegramInstallationOptions, telegramKeys } from "@multica/core/telegram";
import { api } from "@multica/core/api";
import { useT } from "../../i18n";

// TelegramTab is the workspace settings panel for the Telegram bot.
//
// It differs from SlackTab in one structural way, and the difference is the
// whole reason this panel exists rather than living on the Agent page: a
// self-hosted deployment runs ONE bot, whose token is a server env var. There is
// no per-agent install to start, only a single question — which agent answers
// it — so the agent picker belongs here. Slack, where each agent brings its own
// app, correctly puts the picker on the agent instead.
export function TelegramTab() {
  const { t } = useT("settings");
  const wsId = useWorkspaceId();
  const qc = useQueryClient();
  const user = useAuthStore((s) => s.user);

  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const currentMember = members.find((m) => m.user_id === user?.id) ?? null;
  const canManage = currentMember?.role === "owner" || currentMember?.role === "admin";

  const { data, isLoading } = useQuery(telegramInstallationOptions(wsId));
  // agentListOptions asks for include_archived, so filter here: binding the bot
  // to an archived agent would leave messages arriving at something that cannot
  // answer. The currently bound agent is kept even if archived, so a binding
  // that drifted into that state stays visible instead of silently emptying the
  // selector.
  const { data: allAgents = [] } = useQuery(agentListOptions(wsId));

  const configured = data?.configured === true;
  const installation = data?.installation ?? null;
  const takenElsewhere = data?.owned_by_other_workspace === true;
  const agents = allAgents.filter(
    (a) => !a.archived_at || a.id === installation?.agent_id,
  );

  const [pending, setPending] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [confirmDisconnect, setConfirmDisconnect] = useState(false);
  const [disconnecting, setDisconnecting] = useState(false);

  // The selector shows the pending choice while it is being made, otherwise
  // whatever the server says is bound. Reading through to the query means a
  // rebind from another tab is reflected without local bookkeeping.
  const selected = pending ?? installation?.agent_id ?? "";
  const dirty = !!pending && pending !== installation?.agent_id;

  async function handleSave() {
    if (!pending || saving) return;
    setSaving(true);
    try {
      await api.bindTelegramAgent(wsId, pending);
      await qc.invalidateQueries({ queryKey: telegramKeys.installation(wsId) });
      setPending(null);
      toast.success(t(($) => $.telegram.toast_bound));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : t(($) => $.telegram.toast_bind_failed));
    } finally {
      setSaving(false);
    }
  }

  async function handleDisconnect() {
    if (disconnecting) return;
    setDisconnecting(true);
    try {
      await api.disconnectTelegram(wsId);
      await qc.invalidateQueries({ queryKey: telegramKeys.installation(wsId) });
      setPending(null);
      toast.success(t(($) => $.telegram.toast_disconnected));
    } catch (e) {
      toast.error(e instanceof Error ? e.message : t(($) => $.telegram.toast_disconnect_failed));
    } finally {
      setDisconnecting(false);
      setConfirmDisconnect(false);
    }
  }

  if (isLoading) {
    return <p className="text-sm text-muted-foreground">{t(($) => $.telegram.loading)}</p>;
  }

  // No token on the server: this is an operator-level switch a user cannot flip,
  // so say where it lives instead of offering a control that would 503.
  if (!configured) {
    return (
      <div className="space-y-2">
        <p className="text-sm text-muted-foreground">
          {t(($) => $.telegram.not_enabled_description_prefix)}{" "}
          <code className="rounded bg-muted px-1 py-0.5 text-xs">MULTICA_TELEGRAM_BOT_TOKEN</code>{" "}
          {t(($) => $.telegram.not_enabled_description_suffix)}
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">{t(($) => $.telegram.page_description)}</p>

      <Card>
        <CardContent className="space-y-4 p-4">
          <div className="flex items-center gap-3">
            <div className="flex size-9 shrink-0 items-center justify-center rounded-md bg-muted">
              <Send className="size-4" />
            </div>
            <div className="min-w-0">
              <p className="truncate text-sm font-medium">
                {installation?.bot_username
                  ? `@${installation.bot_username}`
                  : t(($) => $.telegram.bot_unknown_username)}
              </p>
              <p className="truncate text-xs text-muted-foreground">
                {t(($) => $.telegram.bot_id_label, { id: data?.bot_id ?? "—" })}
                {installation?.status === "revoked" && ` · ${t(($) => $.telegram.revoked_badge)}`}
              </p>
            </div>
          </div>

          {takenElsewhere ? (
            <p className="text-sm text-muted-foreground">
              {t(($) => $.telegram.owned_by_other_workspace)}
            </p>
          ) : (
            <div className="space-y-2">
              <p className="text-sm font-medium">{t(($) => $.telegram.bound_agent_label)}</p>
              <div className="flex flex-wrap items-center gap-2">
                <Select
                  // `items` is required by this Select: it is the value→label
                  // map the trigger reads, so without it the closed control
                  // would render the raw agent UUID.
                  items={agents.map((a) => ({ value: a.id, label: a.name }))}
                  value={selected}
                  onValueChange={(v) => v && setPending(v)}
                  disabled={!canManage || saving}
                >
                  <SelectTrigger size="sm" className="w-64">
                    <SelectValue placeholder={t(($) => $.telegram.pick_agent_placeholder)}>
                      {agents.find((a) => a.id === selected)?.name ?? null}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    {agents.map((a) => (
                      <SelectItem key={a.id} value={a.id}>
                        {a.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {canManage && dirty && (
                  <Button size="sm" onClick={handleSave} disabled={saving}>
                    {saving
                      ? t(($) => $.telegram.saving)
                      : installation
                        ? t(($) => $.telegram.save_rebind)
                        : t(($) => $.telegram.save_bind)}
                  </Button>
                )}
                {canManage && installation && installation.status !== "revoked" && !dirty && (
                  <Button
                    size="sm"
                    variant="ghost"
                    onClick={() => setConfirmDisconnect(true)}
                    disabled={disconnecting}
                  >
                    <Trash2 className="mr-1 size-3.5" />
                    {t(($) => $.telegram.disconnect)}
                  </Button>
                )}
              </div>
              <p className="text-xs text-muted-foreground">
                {t(($) => $.telegram.propagation_hint)}
              </p>
            </div>
          )}
        </CardContent>
      </Card>

      <AlertDialog open={confirmDisconnect} onOpenChange={setConfirmDisconnect}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t(($) => $.telegram.disconnect_confirm_title)}</AlertDialogTitle>
            <AlertDialogDescription>
              {t(($) => $.telegram.disconnect_confirm_description)}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel disabled={disconnecting}>
              {t(($) => $.telegram.disconnect_confirm_cancel)}
            </AlertDialogCancel>
            <AlertDialogAction onClick={handleDisconnect} disabled={disconnecting}>
              {disconnecting ? t(($) => $.telegram.disconnecting) : t(($) => $.telegram.disconnect)}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
