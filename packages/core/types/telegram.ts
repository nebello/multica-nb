/** The single Telegram bot binding a self-hosted deployment can hold.
 *
 * Wire shape mirrors `TelegramInstallationResponse` in
 * `server/internal/handler/telegram.go`. There is deliberately no token field:
 * the bot token lives in the server's environment, never in the row, so nothing
 * here is secret. New fields the backend adds MUST default to optional so older
 * desktop builds keep parsing the response — see CLAUDE.md → API Compatibility. */
export interface TelegramInstallation {
  id: string;
  workspace_id: string;
  agent_id: string;
  /** Denormalised name of the bound agent; empty if that agent has vanished. */
  agent_name?: string;
  /** Numeric bot id, derived server-side from the token in use. */
  bot_id: string;
  /** e.g. "m_nebello_bot" — refreshed from getMe when the poll loop connects. */
  bot_username?: string;
  installer_user_id: string;
  status: "active" | "revoked" | string;
  installed_at: string;
  updated_at: string;
}

export interface GetTelegramInstallationResponse {
  /** Whether MULTICA_TELEGRAM_BOT_TOKEN is set on the server. When false the
   * whole section hides: it is an operator-level switch a user cannot flip. */
  configured: boolean;
  /** Present whenever configured, even with no binding yet. */
  bot_id?: string;
  /** Null when the token is set but no agent has been bound — a legitimate
   * state, not an error. */
  installation: TelegramInstallation | null;
  /** The bot's routing slot is global to the server; true when it belongs to a
   * different Multica workspace, which the panel must explain rather than
   * silently offering a bind that will 409. */
  owned_by_other_workspace?: boolean;
}

export interface BindTelegramAgentRequest {
  agent_id: string;
}
