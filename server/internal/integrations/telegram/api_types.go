package telegram

// Bot API payload subset this adapter reads. Only the fields the adapter
// actually uses are declared: everything else stays in the raw JSON that
// travels in channel.InboundMessage.Raw, which is where platform-specific
// data belongs per the channel package's boundary rule.

type apiUpdate struct {
	UpdateID      int64       `json:"update_id"`
	Message       *apiMessage `json:"message,omitempty"`
	EditedMessage *apiMessage `json:"edited_message,omitempty"`
}

type apiMessage struct {
	MessageID       int64  `json:"message_id"`
	MessageThreadID int64  `json:"message_thread_id,omitempty"`
	IsTopicMessage  bool   `json:"is_topic_message,omitempty"`
	Date            int64  `json:"date"`
	Text            string `json:"text,omitempty"`
	Caption         string `json:"caption,omitempty"`

	From *apiUser `json:"from,omitempty"`
	Chat apiChat  `json:"chat"`

	ReplyToMessage *apiMessage `json:"reply_to_message,omitempty"`

	Entities        []apiEntity `json:"entities,omitempty"`
	CaptionEntities []apiEntity `json:"caption_entities,omitempty"`

	// Attachment markers, read only to classify the message type.
	Photo    []apiFile `json:"photo,omitempty"`
	Document *apiFile  `json:"document,omitempty"`
	Audio    *apiFile  `json:"audio,omitempty"`
	Voice    *apiFile  `json:"voice,omitempty"`
	Video    *apiFile  `json:"video,omitempty"`
}

type apiUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	Username  string `json:"username,omitempty"`
}

type apiChat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"` // private | group | supergroup | channel
}

type apiEntity struct {
	Type   string   `json:"type"`
	Offset int      `json:"offset"`
	Length int      `json:"length"`
	User   *apiUser `json:"user,omitempty"`
}

type apiFile struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	FileSize int64  `json:"file_size,omitempty"`
}
