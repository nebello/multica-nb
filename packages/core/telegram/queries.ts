import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";

/** Query key namespace for the Telegram binding. Singular on purpose: a
 * deployment runs one bot (the token is an env var), so there is no list to
 * key by id — unlike Slack, where a workspace can hold several installations. */
export const telegramKeys = {
  all: (wsId: string) => ["telegram", wsId] as const,
  installation: (wsId: string) => [...telegramKeys.all(wsId), "installation"] as const,
};

export const telegramInstallationOptions = (wsId: string) =>
  queryOptions({
    queryKey: telegramKeys.installation(wsId),
    queryFn: () => api.getTelegramInstallation(wsId),
    enabled: !!wsId,
  });
