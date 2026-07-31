package telegram

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/integrations/channel"
	"github.com/multica-ai/multica/server/internal/integrations/channel/engine"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// This file is the Telegram ResolverSet: the platform-specific seams the
// channel-agnostic engine.Router runs the inbound pipeline through. Like the
// Slack set it is built entirely on the generic channel_* queries — no new
// query, no schema change — so "adding Telegram" stays "implement Channel +
// register a ResolverSet".
//
// Two things Slack needs and Telegram does not:
//
//   - No team/workspace scoping. A Slack app can be installed into several
//     Slack workspaces and every one of them emits events carrying the same
//     api_app_id, so Slack must additionally match the event's team. A
//     Telegram bot has no such notion: the token IS the bot and the bot is
//     one entity, so the bot id alone routes unambiguously.
//   - No cross-installation binding reuse, for the same reason: there is no
//     "same Slack team, second app" case to spare the user a second link.

// originTelegramChat is the issue.origin_type label for issues created from a
// Telegram conversation.
const originTelegramChat = "telegram_chat"

// NewTelegramResolverSet assembles the set over the generated queries plus a tx
// starter for the shared session service. botID is the numeric bot id — the
// routing key stored in config->>'app_id' — which the caller derives from the
// bot token (Telegram tokens are "<bot_id>:<secret>", see BotIDFromToken).
//
// replier may be nil: the inbound pipeline (route, identity, dedup, session,
// run trigger) is fully functional without it; what is lost is the outbound
// "you need to link your account" prompt.
func NewTelegramResolverSet(q *db.Queries, tx engine.TxStarter, botID string, replier engine.OutboundReplier) engine.ResolverSet {
	return engine.ResolverSet{
		Installation: &installationResolver{q: q, botID: botID},
		Identity:     &identityResolver{q: q},
		Dedup:        &deduper{q: q},
		Session: &sessionBinder{session: engine.NewChatSession(q, tx, TypeTelegram, engine.SessionTitles{
			Group:    "Gruppo Telegram",
			Direct:   "Messaggio diretto Telegram",
			Fallback: "Chat Telegram",
		})},
		Audit:      &auditor{q: q},
		Replier:    replier,
		OriginType: originTelegramChat,
	}
}

var (
	_ engine.InstallationResolver = (*installationResolver)(nil)
	_ engine.IdentityResolver     = (*identityResolver)(nil)
	_ engine.Deduper              = (*deduper)(nil)
	_ engine.SessionBinder        = (*sessionBinder)(nil)
	_ engine.Auditor              = (*auditor)(nil)
)

// bindingConfig is the opaque outbound routing persisted on the chat binding.
// The binding key can be composite (a forum topic inside a group), so the bare
// chat id is kept here for the outbound path to post back to.
type bindingConfig struct {
	ChatID string `json:"chat_id"`
}

// sessionRouting derives the session-isolation key, the outbound config and the
// reply thread from one inbound message.
//
// A direct chat is one continuous session. A group is one session too — unlike
// Slack, where every @mention opens a thread, a plain Telegram group has no
// threads at all, so isolating per message would shred one conversation into
// dozens of sessions. The exception is a forum topic (supergroups with topics
// enabled), which IS a durable thread: there the key carries the topic id so
// two topics are two conversations.
//
// Pure function, so the isolation contract is testable without a database.
func sessionRouting(msg channel.InboundMessage) (bindingKey string, config []byte, replyThread string) {
	chatID := msg.Source.ChatID
	cfg, _ := json.Marshal(bindingConfig{ChatID: chatID})
	if msg.Source.ThreadID == "" {
		return chatID, cfg, ""
	}
	return chatID + ":" + msg.Source.ThreadID, cfg, msg.Source.ThreadID
}

func nullText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// ---- installation routing ----

type installationResolver struct {
	q     *db.Queries
	botID string
}

func (r *installationResolver) ResolveInstallation(ctx context.Context, msg channel.InboundMessage) (engine.ResolvedInstallation, error) {
	if r.botID == "" {
		return engine.ResolvedInstallation{}, engine.ErrInstallationNotFound
	}
	inst, err := r.q.GetChannelInstallationByAppID(ctx, db.GetChannelInstallationByAppIDParams{
		ChannelType: string(TypeTelegram),
		AppID:       r.botID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return engine.ResolvedInstallation{}, engine.ErrInstallationNotFound
		}
		return engine.ResolvedInstallation{}, err
	}
	return engine.ResolvedInstallation{
		ID:              inst.ID,
		WorkspaceID:     inst.WorkspaceID,
		AgentID:         inst.AgentID,
		InstallerUserID: inst.InstallerUserID,
		Active:          inst.Status == "active",
		Platform:        inst,
	}, nil
}

// ---- identity ----

type identityQueries interface {
	GetChannelUserBindingByUserID(ctx context.Context, arg db.GetChannelUserBindingByUserIDParams) (db.ChannelUserBinding, error)
	GetMemberByUserAndWorkspace(ctx context.Context, arg db.GetMemberByUserAndWorkspaceParams) (db.Member, error)
}

type identityResolver struct{ q identityQueries }

func (r *identityResolver) ResolveSender(ctx context.Context, inst engine.ResolvedInstallation, msg channel.InboundMessage) (engine.ResolvedIdentity, error) {
	binding, err := r.q.GetChannelUserBindingByUserID(ctx, db.GetChannelUserBindingByUserIDParams{
		InstallationID: inst.ID,
		ChannelUserID:  msg.Source.SenderID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The engine turns this into the outbound "link your account"
			// prompt; it is the normal first-contact path, not an error.
			return engine.ResolvedIdentity{}, engine.ErrSenderUnbound
		}
		return engine.ResolvedIdentity{}, err
	}
	// A binding does not prove membership (there is no FK), and a member can
	// have left since linking, so re-check on every message.
	if _, err := r.q.GetMemberByUserAndWorkspace(ctx, db.GetMemberByUserAndWorkspaceParams{
		UserID:      binding.MulticaUserID,
		WorkspaceID: inst.WorkspaceID,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return engine.ResolvedIdentity{}, engine.ErrSenderNotMember
		}
		return engine.ResolvedIdentity{}, err
	}
	return engine.ResolvedIdentity{UserID: binding.MulticaUserID}, nil
}

// ---- dedup ----

// Telegram redelivers every update until an offset acknowledges it, so this
// layer is what stops a reconnect mid-batch from running the same message
// twice.
type deduper struct{ q *db.Queries }

func (r *deduper) Claim(ctx context.Context, installationID pgtype.UUID, messageID string) (pgtype.UUID, error) {
	claim, err := r.q.ClaimChannelInboundDedup(ctx, db.ClaimChannelInboundDedupParams{
		InstallationID: installationID,
		MessageID:      messageID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return pgtype.UUID{}, engine.ErrDuplicate
		}
		return pgtype.UUID{}, err
	}
	return claim.ClaimToken, nil
}

func (r *deduper) Mark(ctx context.Context, installationID pgtype.UUID, messageID string, claimToken pgtype.UUID) error {
	_, err := r.q.MarkChannelInboundDedupProcessed(ctx, db.MarkChannelInboundDedupProcessedParams{
		InstallationID: installationID,
		MessageID:      messageID,
		ClaimToken:     claimToken,
	})
	return err
}

func (r *deduper) Release(ctx context.Context, installationID pgtype.UUID, messageID string, claimToken pgtype.UUID) error {
	_, err := r.q.ReleaseChannelInboundDedup(ctx, db.ReleaseChannelInboundDedupParams{
		InstallationID: installationID,
		MessageID:      messageID,
		ClaimToken:     claimToken,
	})
	return err
}

// ---- session bind / append ----

type sessionBinder struct{ session *engine.ChatSession }

func (r *sessionBinder) EnsureSession(ctx context.Context, p engine.EnsureSessionParams) (pgtype.UUID, error) {
	bindingKey, config, _ := sessionRouting(p.Message)
	return r.session.EnsureSession(ctx, engine.EnsureSessionInput{
		WorkspaceID:    p.Installation.WorkspaceID,
		AgentID:        p.Installation.AgentID,
		InstallationID: p.Installation.ID,
		Sender:         p.Sender,
		BindingKey:     bindingKey,
		BindingConfig:  config,
		ChatType:       p.Message.Source.ChatType,
	})
}

func (r *sessionBinder) AppendMessage(ctx context.Context, p engine.AppendParams) (engine.AppendResult, error) {
	_, _, replyThread := sessionRouting(p.Message)
	return r.session.AppendUserMessage(ctx, engine.AppendInput{
		SessionID:      p.SessionID,
		Sender:         p.Sender,
		InstallationID: p.InstallationID,
		Body:           p.Message.Text,
		// Telegram text is not enriched, so the command source is the body.
		CommandText:         p.Message.Text,
		MessageID:           p.Message.MessageID,
		ThreadID:            replyThread,
		ClaimToken:          p.ClaimToken,
		MediaPendingSeconds: p.MediaPendingSeconds,
	})
}

func (r *sessionBinder) BindMedia(ctx context.Context, p engine.BindMediaParams) error {
	return r.session.BindMediaRefs(ctx, engine.BindMediaInput{
		MessageID:   p.MessageID,
		SessionID:   p.SessionID,
		WorkspaceID: p.WorkspaceID,
		Sender:      p.Sender,
		MediaRefs:   p.MediaRefs,
	})
}

// ---- audit ----

type auditor struct{ q *db.Queries }

func (r *auditor) RecordDrop(ctx context.Context, instID pgtype.UUID, msg channel.InboundMessage, reason engine.DropReason) error {
	return r.q.RecordChannelInboundDrop(ctx, db.RecordChannelInboundDropParams{
		ChannelType:      string(TypeTelegram),
		EventType:        "message",
		DropReason:       string(reason),
		InstallationID:   instID,
		ChannelChatID:    nullText(msg.Source.ChatID),
		ChannelEventID:   nullText(msg.EventID),
		ChannelMessageID: nullText(msg.MessageID),
	})
}
