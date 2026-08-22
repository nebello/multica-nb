// Which issue a chat draft is about, read from its `mention://issue/<id>`
// links. The chat composer needs this to offer the per-issue engine pin: the
// pin is written on the issue itself, so it can only be offered when the draft
// names exactly one.
//
// The mention picker inserts a real issue UUID; a bare "MUL-123" typed by hand
// is rewritten to `mention://issue/MUL-123` on render only, never in the draft
// the composer holds. Identifiers are accepted anyway — the metadata endpoints
// resolve either form through loadIssueForUser.
const ISSUE_MENTION_RE = /\[[^\]]*\]\(mention:\/\/issue\/([^)\s]+)\)/g;

/**
 * The single issue a draft mentions, or null when it mentions none or several.
 *
 * Several is deliberately null rather than "the first one": the pin changes
 * which engine an issue runs on from now on, and guessing which of three
 * mentioned issues the user meant would silently move the wrong one.
 */
export function soleMentionedIssueId(markdown: string): string | null {
  const ids = new Set<string>();
  for (const match of markdown.matchAll(ISSUE_MENTION_RE)) {
    const id = match[1];
    if (id) ids.add(decodeURIComponent(id));
  }
  if (ids.size !== 1) return null;
  return [...ids][0] ?? null;
}
