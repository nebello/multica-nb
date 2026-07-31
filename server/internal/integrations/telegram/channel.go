// Package telegram is the Telegram Bot API adapter for the channel
// foundation. It implements channel.Channel and registers itself under
// TypeTelegram, so the core never learns Telegram's payload shape — see
// server/internal/integrations/channel/doc.go for the contract.
//
// Two deliberate differences from the Slack and Feishu adapters:
//
//  1. No SDK. The whole surface this adapter needs is three HTTP calls
//     (getMe, getUpdates, sendMessage), so it uses net/http directly rather
//     than adding a module dependency. One less line in go.mod is one less
//     rebase conflict on a fork that follows upstream.
//
//  2. Long polling, not a socket or a webhook. getUpdates blocks on
//     Telegram's side for up to `pollTimeout`, which is exactly the shape
//     channel.Channel.Connect asks for ("establish the link and BLOCK").
//     It also means a self-hosted instance needs no publicly reachable
//     webhook URL and no signature verification.
package telegram

import (
	"strings"

	"github.com/multica-ai/multica/server/internal/integrations/channel"
)

// TypeTelegram is this adapter's platform discriminator, registered in the
// channel Registry. It lives here and not in the channel package for the
// same reason TypeSlack does: the core has no business knowing the list of
// platforms that exist.
const TypeTelegram channel.Type = "telegram"

// installConfig is the channel_installation.config shape for a Telegram
// installation.
//
// The bot token is NOT stored here. Slack and Lark keep encrypted tokens in
// the installation row because they serve many installations across many
// workspaces, each with its own app; a self-hosted instance runs one bot, so
// the token lives in the environment like every other server secret and this
// row stays free of secret material. AppID is the bot's numeric id, which is
// what the unique index on (channel_type, config->>'app_id') fences on.
type installConfig struct {
	AppID string `json:"app_id"`
	// BotUsername lets group-mention detection work before the first getMe
	// round-trip; it is refreshed from getMe at Connect time.
	BotUsername string `json:"bot_username,omitempty"`
}

// BotIDFromToken extracts the numeric bot id from a Bot API token, whose
// documented shape is "<bot_id>:<secret>". It is the routing key stored in
// config->>'app_id', and deriving it from the token means the installation row
// never has to be trusted to agree with the credential actually in use.
//
// The secret half is never returned, so a caller cannot leak it by logging the
// result.
func BotIDFromToken(token string) string {
	i := strings.Index(token, ":")
	if i <= 0 {
		return ""
	}
	id := token[:i]
	for _, r := range id {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return id
}
