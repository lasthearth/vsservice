// Package model holds the wiki domain: the article, its revisions and the
// rules that keep them consistent.
package model

import "time"

// MaxRevisions bounds the history kept for one article. Editors compare recent
// states; older ones would only grow the document.
const MaxRevisions = 40

// MaxArticleBytes bounds one stored state of an article.
//
// A revision stores the whole article, and the document carries the current
// state plus up to MaxRevisions of them, so 41 states of 300 KB is about 12 MB:
// under MongoDB's 16 MB limit with room left for the write that replaces the
// document. Without a bound one long page makes its own article unwritable,
// because the history can only be pruned by a save that already does not fit.
const MaxArticleBytes = 300_000

// Publication states of an article. A plain string keeps the value readable in
// the database and lets a new state be stored without a migration; the domain
// validates it through IsKnownStatus.
const (
	// StatusDraft is visible to editors only.
	StatusDraft = "draft"
	// StatusPublished is visible to readers.
	StatusPublished = "published"
)

// IsKnownStatus reports whether the value is one the domain accepts.
func IsKnownStatus(s string) bool {
	return s == StatusDraft || s == StatusPublished
}

// Sections of the wiki. A plain string keeps the value readable in the database
// and lets a new section be added without a migration.
const (
	SectionStart       = "start"
	SectionSettlements = "settlements"
	SectionWar         = "war"
	SectionEconomy     = "economy"
	SectionWorld       = "world"
	SectionHelp        = "help"
)

// knownSections lists every section an article may belong to.
var knownSections = map[string]struct{}{
	SectionStart:       {},
	SectionSettlements: {},
	SectionWar:         {},
	SectionEconomy:     {},
	SectionWorld:       {},
	SectionHelp:        {},
}

// IsKnownSection reports whether the value is a section the domain accepts.
func IsKnownSection(s string) bool {
	_, ok := knownSections[s]

	return ok
}

// Cover is the article image and the part of it shown in the strip.
type Cover struct {
	// Url is the image served by the media service.
	Url string
	// Focus is the vertical anchor as a percentage: 0 is the top edge.
	Focus int32
}

// QuickAnswer is a short question and answer pair shown next to the article.
type QuickAnswer struct {
	Question string
	Answer   string
}

// Article is a wiki page together with the revision history that was read with
// it. History travels inside the article document, so a save and the revision
// it produces are written in one operation.
type Article struct {
	Slug        string
	Title       string
	Section     string
	Lead        string
	Cover       *Cover
	Tags        []string
	GameVersion string
	QuickAnswer *QuickAnswer
	Related     []string
	Body        string
	Status      string
	AuthorName  string
	CreatedBy   string
	// Revisions is the history of the article, newest first.
	Revisions []Revision
	// Envelope carries what the stored document owns: the identifier, the
	// timestamps and the version. The repository copies it in when a document is
	// read, because the page shows when it was edited and an editor guards an
	// edit on the version.
	Envelope Envelope
}

// Envelope is the part of a stored document that is not part of the page
// itself: where it lives, when it was written and which revision it is.
type Envelope struct {
	Id        string
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int64
}

// SetVersion records which revision of the stored document this is.
func (e *Envelope) SetVersion(version int64) {
	e.Version = version
}

// Revision is a saved state of an article.
//
// It stores the article as it was, which is what makes a restore possible, and
// it never stores its own history: the revisions of a revision are the history
// of the page.
type Revision struct {
	Id         string
	ArticleId  string
	Article    Article
	Note       string
	AuthorName string
	SavedAt    time.Time
}

// ArticleInput is the editable content of a page as a client sends it.
//
// It carries what an editor writes and nothing else: the identity, the author,
// the timestamps and the history belong to the stored document, so a client
// cannot name them.
type ArticleInput struct {
	Slug        string
	Title       string
	Section     string
	Lead        string
	Cover       *Cover
	Tags        []string
	GameVersion string
	QuickAnswer *QuickAnswer
	Related     []string
	Body        string
	Status      string
}

// Article turns editor input into a page that carries content and no identity.
func (in ArticleInput) Article() Article {
	return Article{
		Slug:        in.Slug,
		Title:       in.Title,
		Section:     in.Section,
		Lead:        in.Lead,
		Cover:       in.Cover,
		Tags:        in.Tags,
		GameVersion: in.GameVersion,
		QuickAnswer: in.QuickAnswer,
		Related:     in.Related,
		Body:        in.Body,
		Status:      in.Status,
	}
}

// WithSlug fills an empty slug from the title, which is the address the page
// gets when the editor did not choose one.
func (in ArticleInput) WithSlug() ArticleInput {
	if in.Slug == "" {
		in.Slug = Slugify(in.Title)
	}

	return in
}

// Input returns the editable content of the page, which is what an edit
// replaces.
func (a *Article) Input() ArticleInput {
	return ArticleInput{
		Slug:        a.Slug,
		Title:       a.Title,
		Section:     a.Section,
		Lead:        a.Lead,
		Cover:       a.Cover,
		Tags:        a.Tags,
		GameVersion: a.GameVersion,
		QuickAnswer: a.QuickAnswer,
		Related:     a.Related,
		Body:        a.Body,
		Status:      a.Status,
	}
}

// NewArticle builds a draft article out of editor input.
//
// The slug falls back to the title when the editor left it empty, the section
// falls back to the first section, and the status defaults to draft so a
// half-written page is never published by accident.
func NewArticle(input ArticleInput, authorName string, now time.Time) (*Article, error) {
	article := input.Article()
	article.AuthorName = authorName
	article.CreatedBy = authorName
	article.Envelope = Envelope{Version: 1, CreatedAt: now, UpdatedAt: now}
	article.Revisions = nil

	article.DeriveSlug()

	if article.Status == "" {
		article.Status = StatusDraft
	}

	if article.Section == "" {
		article.Section = SectionStart
	}

	if err := article.Validate(); err != nil {
		return nil, err
	}

	return &article, nil
}

// DeriveSlug fills an empty slug from the title, which is the address a page
// gets when the editor did not choose one.
func (a *Article) DeriveSlug() {
	if a.Slug == "" {
		a.Slug = Slugify(a.Title)
	}
}

// AssignAuthor records who wrote a page that did not exist before. The
// author also counts as the last editor until someone else edits it.
func (a *Article) AssignAuthor(author string) {
	a.CreatedBy = author
	a.AuthorName = author
}

// Validate checks the invariants an article must satisfy to be stored.
func (a *Article) Validate() error {
	if a.Title == "" {
		return errTitleRequired
	}

	if !IsValidSlug(a.Slug) {
		return errSlugInvalid
	}

	if !IsKnownSection(a.Section) {
		return errSectionUnknown
	}

	if !IsKnownStatus(a.Status) {
		return errStatusUnknown
	}

	if a.size() > MaxArticleBytes {
		return errArticleTooLarge
	}

	return nil
}

// size adds up what one stored state of the article carries.
//
// Revisions are not part of it: a revision stores a state and not the history,
// so counting them here would charge the same text twice.
func (a *Article) size() int {
	size := len(a.Slug) + len(a.Title) + len(a.Section) + len(a.Lead) +
		len(a.Body) + len(a.GameVersion)

	if a.Cover != nil {
		size += len(a.Cover.Url)
	}

	if a.QuickAnswer != nil {
		size += len(a.QuickAnswer.Question) + len(a.QuickAnswer.Answer)
	}

	for _, tag := range a.Tags {
		size += len(tag)
	}

	for _, related := range a.Related {
		size += len(related)
	}

	return size
}

// ApplyEdit replaces the editable fields with editor input, keeping the
// identity, the creation time and the revision history.
//
// The version the caller read is checked against the stored one first, so an
// editor working on a stale page is told to reload instead of overwriting
// someone else's work.
func (a *Article) ApplyEdit(input ArticleInput, authorName string, expectedVersion int64, now time.Time) error {
	if expectedVersion != 0 && expectedVersion != a.Envelope.Version {
		return ErrVersionConflict
	}

	edit := input.Article()
	edit.Envelope = a.Envelope
	edit.Envelope.UpdatedAt = now
	edit.AuthorName = authorName
	edit.CreatedBy = a.CreatedBy
	edit.Envelope.Version = a.Envelope.Version + 1
	edit.Revisions = a.Revisions

	edit.DeriveSlug()

	if edit.Section == "" {
		edit.Section = a.Section
	}

	if edit.Status == "" {
		edit.Status = a.Status
	}

	if err := edit.Validate(); err != nil {
		return err
	}

	*a = edit

	return nil
}

// SaveRevision records the current state as a revision and keeps the history
// bounded.
//
// It is called with the state that is about to be replaced, so restoring any
// revision in the returned history brings the article back.
func (a *Article) SaveRevision(note string, now time.Time) {
	revision := Revision{
		ArticleId:  a.Envelope.Id,
		Article:    a.withoutHistory(),
		Note:       note,
		AuthorName: a.AuthorName,
		SavedAt:    now,
	}

	a.Revisions = PruneRevisions(append([]Revision{revision}, a.Revisions...))
}

// Restore replaces the article with the state stored in the given revision.
//
// The state being replaced is saved first, so a restore can be undone the same
// way as any other edit.
func (a *Article) Restore(revision Revision, authorName string, now time.Time) error {
	if revision.ArticleId != a.Envelope.Id {
		return ErrRevisionNotFound
	}

	a.SaveRevision("Восстановление версии", now)

	restored := revision.Article
	restored.Envelope = a.Envelope
	restored.Envelope.UpdatedAt = now
	restored.AuthorName = authorName
	restored.CreatedBy = a.CreatedBy
	restored.Envelope.Version = a.Envelope.Version + 1
	restored.Revisions = a.Revisions

	if err := restored.Validate(); err != nil {
		return err
	}

	*a = restored

	return nil
}

// HoldRevisions records the revision history that was read with the article.
func (a *Article) HoldRevisions(revisions []Revision) {
	a.Revisions = revisions
}

// AdoptDocument takes the part of a stored document that is not part of the page
// itself: where it lives, when it was written and which revision it is.
//
// The repository calls this when it reads a document, so the domain never has
// to know how a document is addressed.
func (a *Article) AdoptDocument(envelope Envelope) {
	a.Envelope = envelope
}

// Adopt returns an article built from editor input and nothing else.
//
// The service uses it to hand the fields that arrived over the wire to the
// domain without touching them itself; identity and timestamps are not part of
// the input and are applied by the repository from the stored document.
func (a *Article) Adopt(input ArticleInput) {
	envelope := a.Envelope
	*a = input.Article()
	a.Envelope = envelope
}

// Publish marks the article as visible to readers.
func (a *Article) Publish() {
	a.Status = StatusPublished
}

// VisibleTo reports whether a caller may read the article.
//
// A published page is public. A draft is visible only to a caller who may edit
// the wiki, which is what keeps a half-written page off the public site.
func (a *Article) VisibleTo(canEdit bool) bool {
	return canEdit || a.Status == StatusPublished
}

// Touch keeps the model compatible with the shared update helper, which stamps
// the persisted modification time on the stored document itself. The domain
// carries no timestamps: they belong to the document envelope, which the
// repository applies when it converts a document back into an article.
func (a *Article) Touch(_ time.Time) {}

// withoutHistory returns a copy that carries no revision history, which is what
// a revision stores: nesting the whole history inside every revision would grow
// the document without bound.
func (a *Article) withoutHistory() Article {
	snapshot := *a
	snapshot.Revisions = nil

	return snapshot
}

// PruneRevisions keeps the newest revisions and drops the rest.
func PruneRevisions(revisions []Revision) []Revision {
	if len(revisions) <= MaxRevisions {
		return revisions
	}

	return revisions[:MaxRevisions]
}

// Page is a page of articles plus the cursor of the following one.
type Page struct {
	Articles []*Article
	Next     string
	// Total counts every article matching the filter, ignoring pagination.
	Total int64
}

// Add appends an article to the page.
func (p *Page) Add(article *Article) {
	p.Articles = append(p.Articles, article)
}

// Count records how many articles match the filter, ignoring pagination.
func (p *Page) Count(total int64) {
	p.Total = total
}

// Filter narrows a list request.
type Filter struct {
	// Section limits the result to one section when set.
	Section string
	// Status limits the result to one status when set.
	Status string
	// Search is a case-insensitive substring matched against title, slug, lead
	// and tags when set.
	Search string
	// Tag limits the result to articles carrying the tag when set.
	Tag string
	// IncludeDrafts lets an authorized editor see unpublished articles.
	IncludeDrafts bool
}

// AllowDrafts lets an authorized editor see unpublished articles.
func (f *Filter) AllowDrafts(allow bool) {
	f.IncludeDrafts = allow
}
