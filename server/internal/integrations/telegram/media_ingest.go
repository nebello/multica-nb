package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/integrations/channel"
	"github.com/multica-ai/multica/server/internal/integrations/channel/engine"
)

// This is the ingest half of "send a voice note, get work done with it": it
// pulls the file off Telegram's servers and puts it in Multica storage, so the
// agent finds a real audio file in its workdir and can do whatever its skill
// says — transcribe it, summarise it, file an issue from it. Nothing here knows
// what a transcription is; that belongs to the agent, not to the channel layer.

// mediaStorage is the same seam the Feishu resolver uses.
type mediaStorage interface {
	Upload(ctx context.Context, key string, data []byte, contentType string, filename string) (string, error)
	// ObjectURL is the URL a successful upload of key returns — a pure
	// function of configuration, so the intent can be recorded BEFORE the PUT.
	ObjectURL(key string) string
}

// telegramDownloadLimit is the Bot API's hard ceiling on getFile downloads.
// Larger files simply cannot be fetched by a bot: Telegram returns
// "file is too big" and there is no bot-accessible workaround, so the resolver
// skips them with a warning instead of pretending to retry.
//
// ponytail: buffered in memory rather than streamed. At 20 MB per object that
// is a bounded cost, and voice notes are two orders of magnitude smaller. If
// video ever matters here, add the UploadStream path the Feishu resolver has.
const telegramDownloadLimit = 20 << 20

type mediaResolver struct {
	token   string
	storage mediaStorage
	ledger  engine.MediaIntentLedger
	http    *http.Client
	logger  *slog.Logger
}

// NewMediaResolver builds the resolver. A nil storage or ledger disables
// ingest: ResolveMedia then returns the message untouched, which degrades to
// today's text-only behaviour instead of dropping the message.
func NewMediaResolver(botToken string, storage mediaStorage, ledger engine.MediaIntentLedger, logger *slog.Logger) engine.MediaResolver {
	if logger == nil {
		logger = slog.Default()
	}
	return &mediaResolver{
		token:   botToken,
		storage: storage,
		ledger:  ledger,
		http:    &http.Client{Timeout: httpTimeout},
		logger:  logger,
	}
}

// HasMedia is a pure in-memory check: it runs on the ACK path, so it must not
// do I/O. The normalized type already tells us, and the raw payload confirms a
// concrete file id exists.
func (r *mediaResolver) HasMedia(msg channel.InboundMessage) bool {
	if msg.Type == channel.MsgTypeText {
		return false
	}
	return len(mediaFilesOf(msg)) > 0
}

func (r *mediaResolver) ResolveMedia(
	ctx context.Context,
	inst engine.ResolvedInstallation,
	_ engine.ResolvedIdentity,
	_ pgtype.UUID,
	chatMessageID pgtype.UUID,
	msg channel.InboundMessage,
) channel.InboundMessage {
	files := mediaFilesOf(msg)
	if len(files) == 0 {
		return msg
	}
	if r.storage == nil || r.ledger == nil || r.token == "" {
		r.logger.WarnContext(ctx, "telegram media ingest skipped: missing dependency",
			"message_id", msg.MessageID)
		return msg
	}

	for _, f := range files {
		meta, err := r.getFile(ctx, f.fileID)
		if err != nil {
			r.logger.WarnContext(ctx, "telegram media ingest skipped: getFile failed",
				"message_id", msg.MessageID, "error", err)
			continue
		}
		if meta.FileSize > telegramDownloadLimit {
			r.logger.WarnContext(ctx, "telegram media ingest skipped: over the Bot API download limit",
				"message_id", msg.MessageID, "size_bytes", meta.FileSize, "limit_bytes", telegramDownloadLimit)
			continue
		}

		filename := f.filename
		if filename == "" {
			filename = fallbackFilename(f, meta.FilePath)
		}
		key := mediaObjectKey(inst, chatMessageID, f.fileUnique, filename)
		link := r.storage.ObjectURL(key)

		// Record the intent BEFORE anything can be written. Every failure from
		// here on leaves the row for the reconciler; nothing is deleted inline.
		ok, err := r.ledger.RecordPendingMediaObject(ctx, engine.RecordPendingMediaObjectParams{
			StorageKey:     key,
			WorkspaceID:    inst.WorkspaceID,
			ChatMessageID:  chatMessageID,
			StorageURL:     link,
			InstallationID: inst.ID,
		})
		if err != nil {
			r.logger.WarnContext(ctx, "telegram media ingest skipped: intent record failed",
				"message_id", msg.MessageID, "error", err)
			continue
		}
		if !ok {
			// The key has left 'pending': the reconciler owns it, never resurrect.
			r.logger.WarnContext(ctx, "telegram media ingest skipped: key owned by reconciler",
				"message_id", msg.MessageID)
			continue
		}

		body, contentType, err := r.download(ctx, meta.FilePath)
		if err != nil {
			r.logger.WarnContext(ctx, "telegram media download failed",
				"message_id", msg.MessageID, "error", err)
			continue
		}
		if contentType == "" {
			contentType = f.mimeType
		}
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		if _, err := r.storage.Upload(ctx, key, body, contentType, filename); err != nil {
			// The store may still be processing the PUT; deleting here could
			// reorder with it. The intent row covers the object either way.
			r.logger.WarnContext(ctx, "telegram media upload failed",
				"message_id", msg.MessageID, "error", err)
			continue
		}

		msg.MediaRefs = append(msg.MediaRefs, channel.MediaRef{
			Type:       f.kind,
			StorageKey: key,
			StorageURL: link,
			Filename:   filename,
			MimeType:   contentType,
			SizeBytes:  int64(len(body)),
		})
	}
	return msg
}

// mediaFile is one downloadable object referenced by a message.
type mediaFile struct {
	fileID     string
	fileUnique string
	filename   string
	mimeType   string
	kind       channel.MsgType
}

// mediaFilesOf decodes the raw update and lists what can be fetched. Photos
// arrive as a size ladder; only the largest is taken, which is the last entry
// per the Bot API.
func mediaFilesOf(msg channel.InboundMessage) []mediaFile {
	if len(msg.Raw) == 0 {
		return nil
	}
	var u apiUpdate
	if err := json.Unmarshal(msg.Raw, &u); err != nil {
		return nil
	}
	m := u.Message
	if m == nil {
		m = u.EditedMessage
	}
	if m == nil {
		return nil
	}
	var out []mediaFile
	add := func(f *apiFile, kind channel.MsgType) {
		if f == nil || f.FileID == "" {
			return
		}
		out = append(out, mediaFile{
			fileID:     f.FileID,
			fileUnique: f.FileUniqueID,
			filename:   f.FileName,
			mimeType:   f.MimeType,
			kind:       kind,
		})
	}
	add(m.Voice, channel.MsgTypeAudio)
	add(m.Audio, channel.MsgTypeAudio)
	add(m.Document, channel.MsgTypeFile)
	add(m.Video, channel.MsgTypeVideo)
	if n := len(m.Photo); n > 0 {
		add(&m.Photo[n-1], channel.MsgTypeImage)
	}
	return out
}

// mediaObjectKey derives the object key from the CHAT message the object binds
// to, not from the platform message: the same Telegram update can be ingested
// twice (a reclaimable dedup row), and a shared key would run the second ingest
// into the first one's ledger row — possibly a tombstone, which silently drops
// the media. One key per chat message keeps the two ingests independent.
func mediaObjectKey(inst engine.ResolvedInstallation, chatMessageID pgtype.UUID, fileUnique, filename string) string {
	ws := uuidString(inst.WorkspaceID)
	cm := uuidString(chatMessageID)
	name := path.Base(filename)
	if name == "" || name == "." || name == "/" {
		name = "file"
	}
	if fileUnique == "" {
		fileUnique = "single"
	}
	return fmt.Sprintf("channel/telegram/%s/%s/%s-%s", ws, cm, sanitize(fileUnique), sanitize(name))
}

func uuidString(u pgtype.UUID) string {
	if !u.Valid {
		return "unknown"
	}
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// sanitize keeps storage keys boring: letters, digits, dot, dash, underscore.
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// fallbackFilename names the object when Telegram supplies none — which is the
// normal case for a voice note. The extension is taken from the remote path so
// a transcriber downstream can tell an .oga from an .mp3.
func fallbackFilename(f mediaFile, remotePath string) string {
	ext := path.Ext(remotePath)
	if ext == "" {
		switch f.kind {
		case channel.MsgTypeAudio:
			ext = ".oga" // Telegram voice notes are OPUS in an OGG container
		case channel.MsgTypeImage:
			ext = ".jpg"
		case channel.MsgTypeVideo:
			ext = ".mp4"
		default:
			ext = ".bin"
		}
	}
	switch f.kind {
	case channel.MsgTypeAudio:
		return "vocale" + ext
	case channel.MsgTypeImage:
		return "immagine" + ext
	case channel.MsgTypeVideo:
		return "video" + ext
	default:
		return "allegato" + ext
	}
}

// ---- Bot API file access ----

type apiFileMeta struct {
	FileID   string `json:"file_id"`
	FileSize int64  `json:"file_size"`
	FilePath string `json:"file_path"`
}

func (r *mediaResolver) getFile(ctx context.Context, fileID string) (apiFileMeta, error) {
	var meta apiFileMeta
	c := &telegramChannel{token: r.token, http: r.http, logger: r.logger}
	err := c.call(ctx, "getFile", map[string]any{"file_id": fileID}, &meta)
	return meta, err
}

// download fetches the object from the file endpoint, which is a different host
// path from the method endpoint and carries the token in the URL.
func (r *mediaResolver) download(ctx context.Context, filePath string) ([]byte, string, error) {
	if filePath == "" {
		return nil, "", fmt.Errorf("telegram: getFile returned no file_path")
	}
	endpoint := apiBase + "/file/bot" + url.PathEscape(r.token) + "/" + filePath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := r.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		// The path contains the token, so it is never echoed into the error.
		return nil, "", fmt.Errorf("telegram: file download returned http %d", resp.StatusCode)
	}
	// LimitReader is the belt to getFile's braces: a lying file_size cannot
	// make this read unbounded.
	body, err := io.ReadAll(io.LimitReader(resp.Body, telegramDownloadLimit+1))
	if err != nil {
		return nil, "", err
	}
	if len(body) > telegramDownloadLimit {
		return nil, "", fmt.Errorf("telegram: file exceeds the %d byte download limit", telegramDownloadLimit)
	}
	return body, resp.Header.Get("Content-Type"), nil
}
