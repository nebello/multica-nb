package telegram

import "encoding/json"

// PublicConfig is the non-secret view of a Telegram installation's config, for
// the management API. Every field here is already public information: the bot
// id is half of a token that Telegram itself exposes in the bot's username
// lookup, and the username is what users type to mention it.
//
// Unlike Slack and Lark there is nothing to redact — this adapter deliberately
// keeps no credential in the row (see installConfig). The type exists anyway so
// the handler never has to reach into an unexported struct, and so a future
// field that IS secret has an obvious place NOT to go.
type PublicConfig struct {
	AppID       string `json:"app_id"`
	BotUsername string `json:"bot_username,omitempty"`
}

// DecodePublicConfig parses a stored installation config. A malformed or empty
// blob yields a zero PublicConfig rather than an error: the caller is rendering
// a settings panel, and a row it cannot fully parse should still show what it
// can instead of failing the whole request.
func DecodePublicConfig(raw []byte) PublicConfig {
	var cfg installConfig
	_ = json.Unmarshal(raw, &cfg)
	return PublicConfig{AppID: cfg.AppID, BotUsername: cfg.BotUsername}
}

// EncodeConfig builds the config blob for an install / rebind. botID is
// authoritative: it comes from the token the process is actually running with
// (BotIDFromToken), never from client input, so the routing key in
// config->>'app_id' cannot be made to disagree with the live credential.
//
// botUsername is best-effort decoration for group-mention detection before the
// first getMe round-trip. Passing it empty is fine — Connect refreshes it.
func EncodeConfig(botID, botUsername string) ([]byte, error) {
	return json.Marshal(installConfig{AppID: botID, BotUsername: botUsername})
}
