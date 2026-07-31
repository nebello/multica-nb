package telegram

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/integrations/channel"
)

// splitText is the one piece of logic that silently corrupts an agent's reply
// if it is wrong: too long a part and Telegram rejects the whole message, a
// bad boundary and a sentence is cut in half.
func TestSplitTextRespectsLimitAndBoundaries(t *testing.T) {
	t.Run("short text is untouched", func(t *testing.T) {
		got := splitText("ciao", sendLimit)
		if len(got) != 1 || got[0] != "ciao" {
			t.Fatalf("got %q", got)
		}
	})

	t.Run("every part fits the limit", func(t *testing.T) {
		long := strings.Repeat("a", sendLimit*2+17)
		for i, part := range splitText(long, sendLimit) {
			if len(part) > sendLimit {
				t.Fatalf("part %d is %d chars, over the %d limit", i, len(part), sendLimit)
			}
		}
	})

	t.Run("nothing is lost", func(t *testing.T) {
		long := strings.Repeat("parola ", 2000)
		joined := strings.Join(splitText(long, sendLimit), "")
		if strings.ReplaceAll(joined, " ", "") != strings.ReplaceAll(long, " ", "") {
			t.Fatal("splitting dropped or duplicated text")
		}
	})

	t.Run("cuts on a paragraph boundary when there is one", func(t *testing.T) {
		head := strings.Repeat("x", sendLimit-10)
		parts := splitText(head+"\n\n"+strings.Repeat("y", 100), sendLimit)
		if len(parts) != 2 || parts[0] != head {
			t.Fatalf("expected the cut at the blank line, got first part of %d chars", len(parts[0]))
		}
	})
}

func TestIsFreshCommand(t *testing.T) {
	cases := map[string]bool{
		"/fresh":                true,
		"/new":                  true,
		"/fresh@MioBot":         true, // the form Telegram rewrites in groups
		"/fresh riparti da qui": true,
		"/freshen":              false,
		"fresh":                 false,
		"ciao /fresh":           false,
		"":                      false,
	}
	for text, want := range cases {
		if got := isFreshCommand(text); got != want {
			t.Errorf("isFreshCommand(%q) = %v, atteso %v", text, got, want)
		}
	}
}

// Telegram message ids repeat across chats, so the dedup key must carry the
// chat. This test is the guard against "two chats, one dropped message".
func TestCompositeMessageIDIsPerChat(t *testing.T) {
	a := compositeMessageID(111, 7)
	b := compositeMessageID(222, 7)
	if a == b {
		t.Fatal("same message id in two chats produced the same dedup key")
	}
	if got := messageIDPart(a); got != "7" {
		t.Fatalf("messageIDPart(%q) = %q, atteso 7", a, got)
	}
}

func TestNormalizeMapsTheEnvelope(t *testing.T) {
	c := &telegramChannel{botID: 42, botUsername: "MioBot"}

	t.Run("chat privata", func(t *testing.T) {
		msg := &apiMessage{
			MessageID: 5, Text: "ciao",
			From: &apiUser{ID: 9}, Chat: apiChat{ID: -100, Type: "private"},
		}
		got, err := c.normalize(apiUpdate{UpdateID: 1, Message: msg}, msg)
		if err != nil {
			t.Fatal(err)
		}
		if got.Source.ChatType != channel.ChatTypeP2P {
			t.Errorf("ChatType = %q", got.Source.ChatType)
		}
		if got.Source.ChannelType != TypeTelegram {
			t.Errorf("ChannelType = %q", got.Source.ChannelType)
		}
		if got.Type != channel.MsgTypeText || got.Text != "ciao" {
			t.Errorf("Type/Text = %q/%q", got.Type, got.Text)
		}
		if !got.AddressedToBot {
			t.Error("una chat privata è sempre rivolta al bot")
		}
		if got.Source.SenderID != got.Source.SenderStableID {
			t.Error("su Telegram l'id utente è globale: i due campi devono coincidere")
		}
		if !json.Valid(got.Raw) {
			t.Error("Raw non è JSON valido")
		}
	})

	t.Run("gruppo senza menzione", func(t *testing.T) {
		msg := &apiMessage{
			MessageID: 6, Text: "discorso fra umani",
			From: &apiUser{ID: 9}, Chat: apiChat{ID: -200, Type: "supergroup"},
		}
		got, _ := c.normalize(apiUpdate{UpdateID: 2, Message: msg}, msg)
		if got.AddressedToBot {
			t.Error("nessuna menzione: non deve risultare rivolto al bot")
		}
	})

	t.Run("gruppo con risposta al bot", func(t *testing.T) {
		msg := &apiMessage{
			MessageID: 7, Text: "e questo?",
			From: &apiUser{ID: 9}, Chat: apiChat{ID: -200, Type: "group"},
			ReplyToMessage: &apiMessage{
				MessageID: 3, From: &apiUser{ID: 42, IsBot: true}, Chat: apiChat{ID: -200},
			},
		}
		got, _ := c.normalize(apiUpdate{UpdateID: 3, Message: msg}, msg)
		if !got.AddressedToBot {
			t.Error("rispondere a un messaggio del bot lo interpella")
		}
		if got.ReplyTo == nil || got.ReplyTo.MessageID != "-200:3" {
			t.Errorf("ReplyTo = %+v", got.ReplyTo)
		}
	})

	t.Run("immagine con didascalia", func(t *testing.T) {
		msg := &apiMessage{
			MessageID: 8, Caption: "guarda qui",
			Photo: []apiFile{{FileID: "abc"}},
			From:  &apiUser{ID: 9}, Chat: apiChat{ID: -100, Type: "private"},
		}
		got, _ := c.normalize(apiUpdate{UpdateID: 4, Message: msg}, msg)
		if got.Type != channel.MsgTypeImage {
			t.Errorf("Type = %q, atteso image", got.Type)
		}
		if got.Text != "guarda qui" {
			t.Errorf("la didascalia deve diventare il testo, got %q", got.Text)
		}
	})
}

func TestCapabilitiesDeclareOnlyWhatIsHonoured(t *testing.T) {
	c := &telegramChannel{}
	caps := c.Capabilities()
	if !caps.Has(channel.CapText) || !caps.Has(channel.CapQuoteReply) {
		t.Fatalf("capabilities = %s", caps)
	}
	// I media ora si risolvono (media_ingest.go), quindi vanno dichiarati.
	if !caps.Has(channel.CapAttachment) || !caps.Has(channel.CapVoice) {
		t.Error("allegati e vocali sono implementati ma non dichiarati")
	}
	// I thread restano fuori: solo i topic dei forum sono thread veri.
	if caps.Has(channel.CapThreadReply) {
		t.Error("CapThreadReply dichiarata ma un gruppo Telegram non ha thread")
	}
}

// sessionRouting decides what counts as "one conversation". Getting it wrong
// either shreds a group chat into a session per message or merges two forum
// topics into one — both are silent, both are bad.
func TestSessionRoutingIsolation(t *testing.T) {
	msg := func(chatID, threadID string, ct channel.ChatType) channel.InboundMessage {
		return channel.InboundMessage{Source: channel.Source{ChatID: chatID, ThreadID: threadID, ChatType: ct}}
	}

	t.Run("una chat diretta è una sessione continua", func(t *testing.T) {
		k1, cfg, thread := sessionRouting(msg("555", "", channel.ChatTypeP2P))
		k2, _, _ := sessionRouting(msg("555", "", channel.ChatTypeP2P))
		if k1 != "555" || k1 != k2 {
			t.Fatalf("chiavi %q e %q", k1, k2)
		}
		if thread != "" {
			t.Errorf("nessun thread atteso, got %q", thread)
		}
		var bc bindingConfig
		if err := json.Unmarshal(cfg, &bc); err != nil || bc.ChatID != "555" {
			t.Errorf("config di binding = %s (err %v)", cfg, err)
		}
	})

	t.Run("un gruppo senza topic resta una sola sessione", func(t *testing.T) {
		a, _, _ := sessionRouting(msg("-100", "", channel.ChatTypeGroup))
		b, _, _ := sessionRouting(msg("-100", "", channel.ChatTypeGroup))
		if a != b || a != "-100" {
			t.Fatalf("due messaggi nello stesso gruppo hanno dato %q e %q", a, b)
		}
	})

	t.Run("due topic dello stesso gruppo sono due sessioni", func(t *testing.T) {
		a, _, ta := sessionRouting(msg("-100", "7", channel.ChatTypeGroup))
		b, _, _ := sessionRouting(msg("-100", "9", channel.ChatTypeGroup))
		if a == b {
			t.Fatal("topic diversi hanno prodotto la stessa chiave di sessione")
		}
		if ta != "7" {
			t.Errorf("thread di risposta = %q, atteso 7", ta)
		}
	})
}

func TestBotIDFromToken(t *testing.T) {
	if got := BotIDFromToken("123456789:AAHscrittosegretissimo"); got != "123456789" {
		t.Errorf("token valido → %q", got)
	}
	for _, bad := range []string{"", "senzaduepunti", ":soloseg", "abc:def", "12ab34:xyz"} {
		if got := BotIDFromToken(bad); got != "" {
			t.Errorf("BotIDFromToken(%q) = %q, atteso vuoto", bad, got)
		}
	}
	// La metà segreta non deve mai uscire dalla funzione.
	if strings.Contains(BotIDFromToken("42:supersegreto"), "supersegreto") {
		t.Fatal("il segreto è finito nel valore di ritorno")
	}
}
