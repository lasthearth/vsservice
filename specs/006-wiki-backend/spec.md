# Feature Specification: Wiki backend (articles, revisions, public reads)

**Feature Branch**: `006-wiki-backend`
**Created**: 2026-10-12
**Status**: Implemented (first slice)
**Input**: User description: "Давай теперь делать бэк под все это дело" — the site already has a
wiki editor and public pages running on `localStorage`; this slice moves the articles to the server.

## Why

The frontend wiki was built against `localStorage` because there was no backend. That makes it a
local draft tool: articles are invisible to everyone except the browser that wrote them, they do not
survive a cache wipe, and the public `/wiki` page is empty for every visitor. Nothing about the page
model changes here — the frontend already speaks this shape.

## User Scenarios & Testing

### User Story 1 - Read the wiki without signing in (Priority: P1)

A visitor opens `/wiki` and a published article, without a token.

**Acceptance**
1. **Given** published articles exist, **When** an anonymous caller lists them, **Then** only
   published articles are returned, newest edit first.
2. **Given** a draft exists, **When** an anonymous caller lists or fetches it, **Then** it is not
   returned (`NOT_FOUND` for a direct fetch).
3. **Given** a published article, **When** it is fetched by slug, **Then** its body and metadata
   come back.

### User Story 2 - Write a page as an editor (Priority: P1)

An editor creates, edits and deletes articles.

**Acceptance**
1. **Given** an editor scope, **When** an article is created without a slug, **Then** the slug is
   derived from the title by transliteration.
2. **Given** a taken slug, **When** another article is created or moved onto it, **Then**
   `ALREADY_EXISTS` is returned.
3. **Given** an edit based on an outdated version, **When** the version no longer matches, **Then**
   `FAILED_PRECONDITION` is returned and the stored article is untouched.
4. **Given** a valid edit, **When** it is saved, **Then** the version advances by one.

### User Story 3 - Undo a bad edit (Priority: P2)

An editor reviews history and restores an earlier state.

**Acceptance**
1. **Given** an edited article, **When** its revisions are listed, **Then** the state replaced by
   each edit is returned, newest first.
2. **Given** a revision, **When** it is restored, **Then** the article returns to that state, the
   replaced state is itself saved as a revision, and the version advances.
3. **Given** more than 40 edits, **Then** only the newest 40 revisions are kept.

## Requirements

- **FR-001** Articles live in MongoDB in `wiki_articles`; revisions are stored inside the article
  document, so a save and its history entry are one atomic write.
- **FR-002** `slug` is unique across live articles (unique index) and is the public address.
- **FR-003** The domain owns validation: title required, slug slug-shaped, section and status from
  their known sets. The service does not restate those rules.
- **FR-004** Editing requires the version the editor loaded; a mismatch is reported, not applied.
- **FR-005** Reading published articles is public; every mutating method and the revision history
  require the `wiki:write` scope.
- **FR-006** Search is a literal substring match over title, slug, lead and tags; regex
  metacharacters in the query are escaped, so a query cannot shape the pattern.
- **FR-007** List filters (section, status, tag, search) compose into one query, and `total_size`
  describes the same result set as the page.

## Out of scope (next slices)

- `UPLOAD_PURPOSE_WIKI` in the media service; covers today reuse the news purpose.
- Per-article view counts, full-text search, and a rendered-HTML cache.
- Draft visibility for a caller who holds `wiki:write` on a public read: the scope is checked by
  the interceptor, and the service currently treats "has a token" as "may see drafts".

## Success Criteria

- `go test ./internal/wiki/...` passes: domain rules (slug, defaults, version guard, revision
  pruning, restore) and service rules (auth, slug collisions, revision on edit, draft visibility).
- `go vet ./internal/wiki/...` is clean.
- `go build ./internal/wiki/... ./internal/server/...` succeeds.
- The auth coverage check accepts every new method: reads are listed in `publicMethods`, writes
  carry a scope.

## Second slice: moderation

Players do not edit the wiki; they propose. The rules above still describe the
article side, which is now the editor's direct path.

### User Story 4 - Propose an article (Priority: P1)

**Acceptance**
1. **Given** a signed-in player, **When** they submit a new article, **Then** it is stored as a
   pending suggestion, no article is created, and it does not appear in any public list.
2. **Given** a player who already has 5 proposals waiting, **When** they submit another,
   **Then** `RESOURCE_EXHAUSTED` is returned.
3. **Given** a proposal that breaks the article rules (no title), **When** it is submitted,
   **Then** it is refused at the door, not in review.

### User Story 5 - Propose an edit to an existing article (Priority: P1)

**Acceptance**
1. **Given** a published article, **When** a player proposes a change to it, **Then** the article
   is unchanged until review.
2. **Given** an approved edit proposal, **Then** it is applied through the article domain, the
   replaced state is saved as a revision, and the editor who approved it is recorded.

### User Story 6 - Review the queue (Priority: P1)

**Acceptance**
1. **Given** a reviewer, **When** they list proposals, **Then** they see every proposal.
2. **Given** a player, **When** they list proposals, **Then** they see only their own.
3. **Given** an approved proposal, **Then** an article exists and the proposal records who
   approved it and which article it produced.
4. **Given** a rejected proposal, **Then** the reason is stored and shown to the author.
5. **Given** an already reviewed proposal, **When** it is reviewed again, **Then**
   `FAILED_PRECONDITION` is returned: two editors cannot apply the same proposal twice.

### User Story 7 - An edit proposal does not erase a later save (Priority: P1)

**Acceptance**
1. **Given** a player who read version 3 of an article, **When** they propose a change, **Then**
   the proposal records that version and the text they saw.
2. **Given** a proposal whose recorded version is older than the stored article, **When** it is
   submitted, **Then** `FAILED_PRECONDITION` is returned and nothing is queued.
3. **Given** an accepted proposal, **When** an editor saves the article before the review, **Then**
   approving it returns `ABORTED`, the article is unchanged and the proposal stays in the queue.
4. **Given** the same proposal, **When** the reviewer approves it with `allow_stale`, **Then** it
   is applied and the state it replaced is saved as a revision.
5. **Given** a proposal written before base versions were kept, **Then** it counts as stale and
   needs the same confirmation to be applied.

### Requirements (moderation)

- **FR-101** A suggestion is a separate entity (`wiki_suggestions`), not a draft article: it has
  an author, a reviewer, a state and an outcome of its own.
- **FR-102** `SubmitSuggestion` and `ListSuggestions` need a token; `ApproveSuggestion` and
  `RejectSuggestion` need `wiki:review`; the direct article write paths need `wiki:write`.
- **FR-103** An approved proposal is applied through the article domain, so it can never produce a
  state an editor could not have written directly.
- **FR-104** A rejection requires a reason and changes nothing else.
- **FR-105** Reviewing is guarded by the stored document version, so two editors deciding the same
  proposal cannot both win.
- **FR-106** An edit proposal records the version of the article its author read (`base_version`)
  and the text they saw (`base`); the base is stored without the article's revision history.
- **FR-107** A proposal whose `base_version` is already older than the stored article is refused
  when it is submitted. One the article moves past afterwards is refused at approval with
  `ABORTED` — a code of its own, because the reviewer can still act on it — unless the reviewer
  passes `allow_stale`, which is them confirming they saw what would be replaced.
- **FR-108** A proposal with no recorded base version cannot be checked and counts as stale, so it
  needs the same explicit confirmation.
- **FR-109** The author is notified once their proposal is decided — approved or refused — and a
  notification that cannot be sent is logged, never rolled back: the decision is already stored, and
  losing it because a message failed would cost more than the missing message.

### Out of scope (next slices)

- Reporting/abuse handling and per-author proposals history in the profile.
