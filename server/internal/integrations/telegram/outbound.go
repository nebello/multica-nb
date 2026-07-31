package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/integrations/channel"
	"github.com/multica-ai/multica/server/internal/integrations/channel/engine"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// outboundQueries is the slice of generated queries the outbound subscriber
// needs. *db.Queries satisfies it.
type outboundQueries interface {
	GetAgentTask(ctx context.Context, id pgtype.UUID) (db.AgentTaskQueue, error)
	TaskHasChannelIngestedMessages(ctx context.Context, taskID pgtype.UUID) (bool, error)
	GetChannelChatSessionBindingBySession(ctx context.Context, arg db.GetChannelChatSessionBindingBySessionParams) (db.ChannelChatSessionBinding, error)
	GetChannelInstallation(ctx context.Context, arg db.GetChannelInstallationParams) (db.ChannelInstallation, error)
}

type replySender interface {
	Send(ctx context.Context, out channel.OutboundMessage) (channel.SendResult, error)
}

// Outbound is the other half of the round trip: on EventChatDone it finds the
// Telegram binding for the finished task's session and posts the agent's reply
// back into the originating chat. Sessions with no Telegram binding are
// ignored, so it coexists with the Feishu and Slack subscribers on the shared
// bus.
type Outbound struct {
	q      outboundQueries
	logger *slog.Logger
	sender replySender
}

// NewOutbound builds the subscriber. The sender reuses the adapter's own Send
// (splitting, quote handling, no parse_mode), so inbound and outbound cannot
// drift apart in how they talk to Telegram.
func NewOutbound(q outboundQueries, botToken string, logger *slog.Logger) *Outbound {
	if logger == nil {
		logger = slog.Default()
	}
	return &Outbound{
		q:      q,
		logger: logger,
		sender: &telegramChannel{
			token:  botToken,
			logger: logger,
			http:   &http.Client{Timeout: 30 * time.Second},
		},
	}
}

// Register subscribes to the chat-done event on the bus.
func (o *Outbound) Register(bus *events.Bus) {
	bus.Subscribe(protocol.EventChatDone, o.handleEvent)
}

func (o *Outbound) handleEvent(e events.Event) {
	// Bus delivery is synchronous: a stuck HTTP call must not wedge the
	// publisher, hence a fresh context with a tight deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := o.processEvent(ctx, e); err != nil {
		o.logger.WarnContext(ctx, "telegram outbound: reply delivery failed",
			"error", err, "chat_session_id", e.ChatSessionID)
	}
}

func (o *Outbound) processEvent(ctx context.Context, e events.Event) error {
	sessionID, err := util.ParseUUID(e.ChatSessionID)
	if err != nil || !sessionID.Valid {
		return nil // issue / autopilot tasks carry no chat session
	}
	binding, err := o.q.GetChannelChatSessionBindingBySession(ctx, db.GetChannelChatSessionBindingBySessionParams{
		ChatSessionID: sessionID,
		ChannelType:   string(TypeTelegram),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // not a Telegram session
		}
		return fmt.Errorf("lookup telegram chat binding: %w", err)
	}
	content := chatDoneContent(e.Payload)
	if content == "" {
		return nil
	}
	// A session that started on Telegram can later be continued from the web
	// app; those replies belong only in Multica. The discriminator is the
	// immutable channel_ingested provenance of the task's input batch, and it
	// fails closed when the origin cannot be established.
	taskID, ok := chatDoneTaskID(e)
	if !ok {
		return nil
	}
	task, err := o.q.GetAgentTask(ctx, taskID)
	if err != nil {
		return fmt.Errorf("load agent task: %w", err)
	}
	deliver, err := engine.TaskInputIsChannelIngested(ctx, o.q, task)
	if err != nil {
		return fmt.Errorf("classify task input origin: %w", err)
	}
	if !deliver {
		return nil
	}
	inst, err := o.q.GetChannelInstallation(ctx, db.GetChannelInstallationParams{
		ID:          binding.InstallationID,
		ChannelType: string(TypeTelegram),
	})
	if err != nil {
		return fmt.Errorf("load telegram installation: %w", err)
	}
	if inst.Status != "active" {
		return nil // disconnected between trigger and reply
	}
	chatID, threadID := outboundTarget(binding)
	if _, err := o.sender.Send(ctx, channel.OutboundMessage{
		ChatID:   chatID,
		Text:     content,
		ThreadID: threadID,
	}); err != nil {
		return fmt.Errorf("post telegram reply: %w", err)
	}
	return nil
}

// outboundTarget recovers the real send target from the binding. channel_chat_id
// may be the composite "chat:topic" isolation key, so the bare chat id is read
// from the binding config; the reply thread is the recorded last_thread_id.
func outboundTarget(b db.ChannelChatSessionBinding) (chatID, threadID string) {
	chatID = b.ChannelChatID
	if len(b.Config) > 0 {
		var cfg bindingConfig
		if err := json.Unmarshal(b.Config, &cfg); err == nil && cfg.ChatID != "" {
			chatID = cfg.ChatID
		}
	}
	if b.LastThreadID.Valid {
		threadID = b.LastThreadID.String
	}
	return chatID, threadID
}

// chatDoneTaskID extracts the task id from the event envelope or its payload,
// in typed and round-tripped map form.
func chatDoneTaskID(e events.Event) (pgtype.UUID, bool) {
	raw := e.TaskID
	if raw == "" {
		switch p := e.Payload.(type) {
		case protocol.ChatDonePayload:
			raw = p.TaskID
		case map[string]any:
			raw, _ = p["task_id"].(string)
		}
	}
	id, err := util.ParseUUID(raw)
	return id, err == nil && id.Valid
}

// chatDoneContent extracts the reply text from an EventChatDone payload.
func chatDoneContent(payload any) string {
	switch p := payload.(type) {
	case protocol.ChatDonePayload:
		return p.Content
	case map[string]any:
		if s, ok := p["content"].(string); ok {
			return s
		}
	}
	return ""
}
