package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/multica-ai/multica/server/internal/integrations/telegram"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// TelegramInstallationResponse is the wire shape for the single Telegram
// installation a self-hosted deployment can have.
//
// There is no token field and never should be: this adapter keeps the bot token
// in the process environment, not in the row (see telegram.installConfig). What
// the panel needs is only "which bot, bound to which agent, and is it live".
type TelegramInstallationResponse struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	AgentID     string `json:"agent_id"`
	// AgentName is denormalised so the panel can name the bound agent without a
	// second round-trip. Empty when the agent row has since disappeared.
	AgentName       string `json:"agent_name,omitempty"`
	BotID           string `json:"bot_id"`
	BotUsername     string `json:"bot_username,omitempty"`
	InstallerUserID string `json:"installer_user_id"`
	Status          string `json:"status"`
	InstalledAt     string `json:"installed_at"`
	UpdatedAt       string `json:"updated_at"`
}

// BindTelegramAgentRequest moves the bot to a different agent. Only the target
// agent is client-supplied — the bot identity comes from the server's token.
type BindTelegramAgentRequest struct {
	AgentID string `json:"agent_id"`
}

func (h *Handler) telegramInstallationToResponse(r *http.Request, row db.ChannelInstallation) TelegramInstallationResponse {
	cfg := telegram.DecodePublicConfig(row.Config)
	out := TelegramInstallationResponse{
		ID:              uuidToString(row.ID),
		WorkspaceID:     uuidToString(row.WorkspaceID),
		AgentID:         uuidToString(row.AgentID),
		BotID:           cfg.AppID,
		BotUsername:     cfg.BotUsername,
		InstallerUserID: uuidToString(row.InstallerUserID),
		Status:          row.Status,
		InstalledAt:     row.InstalledAt.Time.UTC().Format(time.RFC3339),
		UpdatedAt:       row.UpdatedAt.Time.UTC().Format(time.RFC3339),
	}
	// Best-effort: a missing agent leaves the name blank rather than failing the
	// read, so a dangling binding is still visible (and fixable) in the panel.
	if agent, err := h.Queries.GetAgentInWorkspace(r.Context(), db.GetAgentInWorkspaceParams{
		ID:          row.AgentID,
		WorkspaceID: row.WorkspaceID,
	}); err == nil {
		out.AgentName = agent.Name
	}
	return out
}

// GetTelegramInstallation (GET /api/workspaces/{id}/telegram/installation) is
// member-visible so the Integrations tab renders for non-admins.
//
// `configured` reflects the server's MULTICA_TELEGRAM_BOT_TOKEN, which is the
// integration's only switch. `installation` is null when the token is set but no
// agent has been bound yet — a legitimate state, not an error: the deployment
// can speak Telegram, nobody has said which agent answers.
func (h *Handler) GetTelegramInstallation(w http.ResponseWriter, r *http.Request) {
	if h.TelegramBotID == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"configured":   false,
			"installation": nil,
		})
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "workspace id")
	if !ok {
		return
	}
	row, err := h.Queries.GetChannelInstallationByAppID(r.Context(), db.GetChannelInstallationByAppIDParams{
		ChannelType: string(telegram.TypeTelegram),
		AppID:       h.TelegramBotID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusOK, map[string]any{
				"configured":   true,
				"bot_id":       h.TelegramBotID,
				"installation": nil,
			})
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load telegram installation")
		return
	}
	// The routing slot is global to the deployment (unique on
	// (channel_type, app_id)), so a row can belong to a DIFFERENT workspace than
	// the one being viewed. Report it as unbound here rather than leaking another
	// workspace's agent id — same reasoning as Slack's cross-workspace guard.
	if uuidToString(row.WorkspaceID) != uuidToString(wsUUID) {
		writeJSON(w, http.StatusOK, map[string]any{
			"configured":               true,
			"bot_id":                   h.TelegramBotID,
			"installation":             nil,
			"owned_by_other_workspace": true,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured":   true,
		"bot_id":       h.TelegramBotID,
		"installation": h.telegramInstallationToResponse(r, row),
	})
}

// BindTelegramAgent (PUT /api/workspaces/{id}/telegram/installation) points the
// bot at an agent — the first call installs, a later one moves it. Admin-only at
// the router.
//
// Upsert keyed on (channel_type, config->>'app_id') is deliberate: one bot token
// means one routing slot, so rebinding must MOVE the existing row's agent_id
// rather than insert a second row and trip the unique index. That is exactly
// what Slack does with a team id, and the wrong key here (workspace+agent+type)
// would silently leave the old binding alive alongside the new one.
//
// The Supervisor picks the change up on its next scan (PollInterval, 30s by
// default) — no restart, but not instant either.
func (h *Handler) BindTelegramAgent(w http.ResponseWriter, r *http.Request) {
	if h.TelegramBotID == "" {
		writeError(w, http.StatusServiceUnavailable, "telegram integration not enabled")
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "workspace id")
	if !ok {
		return
	}
	var body BindTelegramAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	agentIDStr := strings.TrimSpace(body.AgentID)
	if agentIDStr == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}
	agentUUID, ok := parseUUIDOrBadRequest(w, agentIDStr, "agent_id")
	if !ok {
		return
	}
	agent, err := h.Queries.GetAgentInWorkspace(r.Context(), db.GetAgentInWorkspaceParams{
		ID:          agentUUID,
		WorkspaceID: wsUUID,
	})
	if err != nil {
		writeError(w, http.StatusNotFound, "agent not found in this workspace")
		return
	}
	initiatorUUID, ok := parseUUIDOrBadRequest(w, userID, "user id")
	if !ok {
		return
	}
	// Carry the known username forward so group-mention detection keeps working
	// between the rebind and the next getMe.
	var username string
	if existing, err := h.Queries.GetChannelInstallationByAppID(r.Context(), db.GetChannelInstallationByAppIDParams{
		ChannelType: string(telegram.TypeTelegram),
		AppID:       h.TelegramBotID,
	}); err == nil {
		if uuidToString(existing.WorkspaceID) != uuidToString(wsUUID) {
			writeError(w, http.StatusConflict, "this bot is already connected to a different workspace on this server")
			return
		}
		username = telegram.DecodePublicConfig(existing.Config).BotUsername
	} else if !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to load telegram installation")
		return
	}
	cfg, err := telegram.EncodeConfig(h.TelegramBotID, username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode installation config")
		return
	}
	row, err := h.Queries.UpsertChannelInstallationByAppID(r.Context(), db.UpsertChannelInstallationByAppIDParams{
		WorkspaceID:     wsUUID,
		AgentID:         agentUUID,
		ChannelType:     string(telegram.TypeTelegram),
		Config:          cfg,
		InstallerUserID: initiatorUUID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The WHERE clause on the conflict update fenced it: the slot belongs
			// to another Multica workspace on this server.
			writeError(w, http.StatusConflict, "this bot is already connected to a different workspace on this server")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to bind telegram bot")
		return
	}
	out := h.telegramInstallationToResponse(r, row)
	if out.AgentName == "" {
		out.AgentName = agent.Name
	}
	writeJSON(w, http.StatusOK, out)
}

// DisconnectTelegram (DELETE /api/workspaces/{id}/telegram/installation) flips
// status to 'revoked' so the Supervisor drops the poll loop. The row survives:
// re-binding an agent flips it back to 'active' through the same upsert, which
// keeps the installation's history rather than churning ids. Admin-only.
func (h *Handler) DisconnectTelegram(w http.ResponseWriter, r *http.Request) {
	if h.TelegramBotID == "" {
		writeError(w, http.StatusServiceUnavailable, "telegram integration not enabled")
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "workspace id")
	if !ok {
		return
	}
	row, err := h.Queries.GetChannelInstallationByAppID(r.Context(), db.GetChannelInstallationByAppIDParams{
		ChannelType: string(telegram.TypeTelegram),
		AppID:       h.TelegramBotID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "telegram installation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to load telegram installation")
		return
	}
	// Workspace-scoped so one workspace cannot revoke another's binding.
	if uuidToString(row.WorkspaceID) != uuidToString(wsUUID) {
		writeError(w, http.StatusNotFound, "telegram installation not found")
		return
	}
	if err := h.Queries.SetChannelInstallationStatus(r.Context(), db.SetChannelInstallationStatusParams{
		ID:     row.ID,
		Status: "revoked",
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to disconnect telegram bot")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
