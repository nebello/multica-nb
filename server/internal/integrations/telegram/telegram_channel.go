package telegram

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/multica-ai/multica/server/internal/integrations/channel"
)

const (
	apiBase = "https://api.telegram.org"

	// pollTimeout is how long Telegram holds getUpdates open with no
	// traffic. 50s keeps the connection count low while staying well under
	// typical proxy idle limits.
	pollTimeout = 50 * time.Second

	// httpTimeout must exceed pollTimeout, otherwise every idle long poll
	// would look like a network failure and the supervisor would reconnect
	// in a loop.
	httpTimeout = pollTimeout + 15*time.Second

	// sendLimit is Telegram's hard cap on a text message. Longer replies are
	// split rather than rejected — an agent's answer is frequently longer.
	sendLimit = 4096
)

// ChannelDeps is what the factory needs from the application. BotToken is
// read from the environment by the caller (see cmd/server/router.go), never
// from the installation row.
type ChannelDeps struct {
	BotToken string
	Logger   *slog.Logger
	// HTTPClient is injectable so tests can serve the Bot API locally.
	HTTPClient *http.Client
}

// telegramChannel is ONE installation's long-poll loop plus its outbound
// sender. The engine.Supervisor builds one per active Telegram installation
// and owns the lease and reconnect lifecycle; Connect blocks until the run
// context is cancelled or polling fails unrecoverably.
type telegramChannel struct {
	token       string
	botID       int64
	botUsername string
	handler     channel.InboundHandler
	logger      *slog.Logger
	http        *http.Client

	// offset is the getUpdates cursor: Telegram redelivers everything from
	// the last unconfirmed update until an offset acknowledges it, which is
	// what makes at-least-once delivery (and the engine's dedup layer)
	// necessary rather than optional.
	offset int64
}

var _ channel.Channel = (*telegramChannel)(nil)

func (c *telegramChannel) Type() channel.Type { return TypeTelegram }

// Capabilities declares what this adapter honours today: text, quote-reply,
// and inbound attachments including voice notes (see media_ingest.go). Threads
// are deliberately absent — only forum topics thread on Telegram, and a plain
// group has no threading at all, so declaring CapThreadReply would make callers
// thread replies that cannot be threaded.
func (c *telegramChannel) Capabilities() channel.Capability {
	return channel.CapText | channel.CapQuoteReply | channel.CapAttachment | channel.CapVoice
}

// Disconnect is a no-op: the poll loop's whole lifetime is scoped to Connect,
// which returns when its context is cancelled. Mirrors slackChannel.
func (c *telegramChannel) Disconnect(ctx context.Context) error { return nil }

// Connect learns the bot identity, then polls until ctx is cancelled.
//
// Returning nil on cancellation and an error on a dropped link is the
// contract the supervisor reads: nil means "graceful shutdown, stay down",
// error means "this attempt failed, reconnect under backoff".
func (c *telegramChannel) Connect(ctx context.Context) error {
	if c.handler == nil {
		return errors.New("telegram: inbound handler not configured")
	}
	if c.token == "" {
		return errors.New("telegram: bot token not configured")
	}

	me, err := c.getMe(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("telegram: getMe: %w", err)
	}
	c.botID, c.botUsername = me.ID, me.Username
	c.logger.Info("telegram: connected", "bot_id", me.ID, "bot_username", me.Username)

	for {
		if ctx.Err() != nil {
			return nil
		}
		updates, err := c.getUpdates(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			// A 409 means someone else is polling this bot, or a webhook is
			// registered. Reconnecting cannot fix it, but the supervisor's
			// backoff at least stops the log from flooding.
			return fmt.Errorf("telegram: getUpdates: %w", err)
		}
		for _, u := range updates {
			if u.UpdateID >= c.offset {
				c.offset = u.UpdateID + 1
			}
			msg := u.Message
			if msg == nil {
				msg = u.EditedMessage
			}
			if msg == nil || msg.From == nil {
				continue
			}
			if msg.From.IsBot {
				// Includes this bot's own messages echoed back in groups.
				continue
			}
			inbound, err := c.normalize(u, msg)
			if err != nil {
				c.logger.Warn("telegram: normalize failed", "update_id", u.UpdateID, "error", err)
				continue
			}
			if err := c.handler(ctx, inbound); err != nil {
				// The engine already logs and persists drops; a handler error
				// must not abort the receive loop or the rest of this batch.
				c.logger.Warn("telegram: handler failed",
					"update_id", u.UpdateID, "chat_id", inbound.Source.ChatID, "error", err)
			}
		}
	}
}

// Send posts one reply, splitting on Telegram's 4096-character cap. The
// SendResult carries the LAST part's message id: that is the message a
// subsequent quote-reply should anchor to.
func (c *telegramChannel) Send(ctx context.Context, out channel.OutboundMessage) (channel.SendResult, error) {
	if strings.TrimSpace(out.Text) == "" {
		return channel.SendResult{}, errors.New("telegram: refusing to send an empty message")
	}
	var last channel.SendResult
	for i, part := range splitText(out.Text, sendLimit) {
		payload := map[string]any{
			"chat_id": out.ChatID,
			"text":    part,
			// No parse_mode: agent output is arbitrary Markdown and
			// Telegram rejects the whole message on a single unbalanced
			// entity. Plain text always delivers.
			"link_preview_options": map[string]any{"is_disabled": true},
		}
		if out.ThreadID != "" {
			if id, err := strconv.ParseInt(out.ThreadID, 10, 64); err == nil {
				payload["message_thread_id"] = id
			}
		}
		// Quote only the first part, so a split reply reads as one answer
		// rather than N quotes of the same question.
		if i == 0 && out.ReplyTo != "" {
			if id, err := strconv.ParseInt(messageIDPart(out.ReplyTo), 10, 64); err == nil {
				payload["reply_parameters"] = map[string]any{
					"message_id":                  id,
					"allow_sending_without_reply": true,
				}
			}
		}
		var sent apiMessage
		if err := c.call(ctx, "sendMessage", payload, &sent); err != nil {
			return last, fmt.Errorf("telegram: sendMessage: %w", err)
		}
		last = channel.SendResult{MessageID: compositeMessageID(sent.Chat.ID, sent.MessageID)}
	}
	return last, nil
}

// normalize translates one Telegram update into the core envelope. Anything
// platform-specific that the adapter itself may need later stays in Raw.
func (c *telegramChannel) normalize(u apiUpdate, msg *apiMessage) (channel.InboundMessage, error) {
	raw, err := json.Marshal(u)
	if err != nil {
		return channel.InboundMessage{}, fmt.Errorf("marshal raw update: %w", err)
	}

	chatType := channel.ChatTypeGroup
	if msg.Chat.Type == "private" {
		chatType = channel.ChatTypeP2P
	}

	text := msg.Text
	if text == "" {
		text = msg.Caption
	}

	var reply *channel.ReplyCtx
	if msg.ReplyToMessage != nil {
		reply = &channel.ReplyCtx{
			MessageID: compositeMessageID(msg.ReplyToMessage.Chat.ID, msg.ReplyToMessage.MessageID),
		}
	}

	threadID := ""
	if msg.IsTopicMessage && msg.MessageThreadID != 0 {
		threadID = strconv.FormatInt(msg.MessageThreadID, 10)
	}

	return channel.InboundMessage{
		EventID:   strconv.FormatInt(u.UpdateID, 10),
		MessageID: compositeMessageID(msg.Chat.ID, msg.MessageID),
		Source: channel.Source{
			ChannelType: TypeTelegram,
			ChatID:      strconv.FormatInt(msg.Chat.ID, 10),
			ChatType:    chatType,
			SenderID:    strconv.FormatInt(msg.From.ID, 10),
			// Telegram user ids are globally unique and stable, so the
			// per-installation id IS the cross-installation identity.
			SenderStableID: strconv.FormatInt(msg.From.ID, 10),
			ThreadID:       threadID,
		},
		Type:           msgType(msg),
		Text:           strings.TrimSpace(text),
		ReplyTo:        reply,
		AddressedToBot: c.addressedToBot(msg, text),
		ForceFresh:     isFreshCommand(text),
		Raw:            raw,
	}, nil
}

// addressedToBot is meaningless for private chats (the core ignores it there)
// and in groups means: mentions the bot by @username, or replies to one of
// the bot's own messages.
func (c *telegramChannel) addressedToBot(msg *apiMessage, text string) bool {
	if msg.Chat.Type == "private" {
		return true
	}
	if msg.ReplyToMessage != nil && msg.ReplyToMessage.From != nil &&
		c.botID != 0 && msg.ReplyToMessage.From.ID == c.botID {
		return true
	}
	if c.botUsername == "" {
		return false
	}
	needle := "@" + strings.ToLower(c.botUsername)
	for _, e := range append(msg.Entities, msg.CaptionEntities...) {
		if e.Type == "mention" && strings.Contains(strings.ToLower(text), needle) {
			return true
		}
		// A bot added to a group as an administrator can be addressed via a
		// text_mention entity carrying its user object instead of a @handle.
		if e.Type == "text_mention" && e.User != nil && e.User.ID == c.botID {
			return true
		}
	}
	return false
}

func msgType(msg *apiMessage) channel.MsgType {
	switch {
	case len(msg.Photo) > 0:
		return channel.MsgTypeImage
	case msg.Voice != nil || msg.Audio != nil:
		return channel.MsgTypeAudio
	case msg.Video != nil:
		return channel.MsgTypeVideo
	case msg.Document != nil:
		return channel.MsgTypeFile
	default:
		return channel.MsgTypeText
	}
}

// isFreshCommand recognizes Telegram's command form, including the
// /fresh@botname variant Telegram appends in groups.
func isFreshCommand(text string) bool {
	first := strings.ToLower(strings.TrimSpace(text))
	if i := strings.IndexAny(first, " \n"); i > 0 {
		first = first[:i]
	}
	if at := strings.Index(first, "@"); at > 0 {
		first = first[:at]
	}
	return first == "/fresh" || first == "/new"
}

// compositeMessageID prefixes the chat id because Telegram message ids are
// unique per chat, not per bot — and the engine's dedup keys on
// (installation, MessageID).
func compositeMessageID(chatID, messageID int64) string {
	return strconv.FormatInt(chatID, 10) + ":" + strconv.FormatInt(messageID, 10)
}

func messageIDPart(composite string) string {
	if i := strings.LastIndex(composite, ":"); i >= 0 {
		return composite[i+1:]
	}
	return composite
}

// splitText cuts on a paragraph or line boundary when one is available in the
// last quarter of the window, so a split reply does not break mid-sentence.
func splitText(s string, limit int) []string {
	if len(s) <= limit {
		return []string{s}
	}
	var out []string
	for len(s) > limit {
		cut := limit
		window := s[:limit]
		if i := strings.LastIndex(window, "\n\n"); i > limit*3/4 {
			cut = i + 2
		} else if i := strings.LastIndex(window, "\n"); i > limit*3/4 {
			cut = i + 1
		}
		out = append(out, strings.TrimRight(s[:cut], "\n"))
		s = strings.TrimLeft(s[cut:], "\n")
	}
	if s != "" {
		out = append(out, s)
	}
	return out
}

// --- Bot API plumbing -------------------------------------------------------

func (c *telegramChannel) getMe(ctx context.Context) (apiUser, error) {
	var me apiUser
	err := c.call(ctx, "getMe", nil, &me)
	return me, err
}

func (c *telegramChannel) getUpdates(ctx context.Context) ([]apiUpdate, error) {
	var updates []apiUpdate
	payload := map[string]any{
		"timeout": int(pollTimeout / time.Second),
		// Only message updates: the adapter has nothing to do with polls,
		// reactions or inline queries, and asking for them would burn the
		// offset on updates it silently drops.
		"allowed_updates": []string{"message", "edited_message"},
	}
	if c.offset > 0 {
		payload["offset"] = c.offset
	}
	err := c.call(ctx, "getUpdates", payload, &updates)
	return updates, err
}

// call performs one Bot API method and unwraps the {"ok":…,"result":…}
// envelope into out.
func (c *telegramChannel) call(ctx context.Context, method string, payload any, out any) error {
	var body *strings.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal %s payload: %w", method, err)
		}
		body = strings.NewReader(string(b))
	} else {
		body = strings.NewReader("")
	}

	endpoint := apiBase + "/bot" + url.PathEscape(c.token) + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	var envelope struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Description string          `json:"description"`
		ErrorCode   int             `json:"error_code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("%s: decode response (http %d): %w", method, resp.StatusCode, err)
	}
	if !envelope.OK {
		// The token never reaches the message: Telegram echoes the method,
		// not the credential, and the endpoint is not logged.
		return fmt.Errorf("%s: telegram error %d: %s", method, envelope.ErrorCode, envelope.Description)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(envelope.Result, out)
}

// --- registration -----------------------------------------------------------

// RegisterTelegram registers the factory under TypeTelegram. Adding a
// platform is one registration call, not a change to the core.
func RegisterTelegram(reg *channel.Registry, deps ChannelDeps) {
	reg.Register(TypeTelegram, newTelegramFactory(deps))
}

func newTelegramFactory(deps ChannelDeps) channel.Factory {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	client := deps.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: httpTimeout}
	}
	return func(cfg channel.Config) (channel.Channel, error) {
		var ic installConfig
		if len(cfg.Raw) > 0 {
			if err := json.Unmarshal(cfg.Raw, &ic); err != nil {
				return nil, fmt.Errorf("telegram: decode installation config: %w", err)
			}
		}
		if deps.BotToken == "" {
			return nil, errors.New("telegram: MULTICA_TELEGRAM_BOT_TOKEN is not set")
		}
		return &telegramChannel{
			token:       deps.BotToken,
			botUsername: ic.BotUsername,
			handler:     cfg.Handler,
			logger:      logger,
			http:        client,
		}, nil
	}
}
