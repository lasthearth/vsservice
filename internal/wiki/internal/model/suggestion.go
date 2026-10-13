package model

import "time"

// MaxPendingSuggestions bounds how many proposals one author may leave waiting
// for review. Without it a single account can bury the queue.
const MaxPendingSuggestions = 5

// Review states of a proposal. Plain strings keep the values readable in the
// database and let a new state be added without a migration.
const (
	// SuggestionPending waits for an editor.
	SuggestionPending = "pending"
	// SuggestionApproved was applied to the wiki.
	SuggestionApproved = "approved"
	// SuggestionRejected was refused, with a reason.
	SuggestionRejected = "rejected"
)

// IsKnownSuggestionStatus reports whether the value is a state the domain has.
func IsKnownSuggestionStatus(status string) bool {
	switch status {
	case SuggestionPending, SuggestionApproved, SuggestionRejected:
		return true
	default:
		return false
	}
}

// Kinds of a proposal. Plain strings for the same reason as the states above.
const (
	// SuggestionNewArticle asks for a page that does not exist yet.
	SuggestionNewArticle = "new_article"
	// SuggestionArticleEdit asks for a change to an existing page.
	SuggestionArticleEdit = "article_edit"
)

// IsKnownSuggestionKind reports whether the value is a kind the domain has.
func IsKnownSuggestionKind(kind string) bool {
	return kind == SuggestionNewArticle || kind == SuggestionArticleEdit
}

// Suggestion is an article or an edit proposed by a player.
//
// It is deliberately not a draft article: a draft is an editor's own unfinished
// page, while a suggestion is someone else's text waiting for a decision, with
// an author, a reviewer and an outcome of its own. Publishing a suggestion goes
// through the article domain, so a proposal can never produce a state an editor
// could not have written directly.
type Suggestion struct {
	Id string
	// Kind and ArticleId say what the proposal targets.
	Kind      string
	ArticleId string
	// Proposal is the article as the author wants it to be.
	Proposal Article
	// BaseVersion is the article version the proposal was written against; 0 for
	// a new article and for proposals made before the field existed.
	BaseVersion int64
	// Base is the article as the author saw it, without its history. Nil for a
	// new article.
	Base *Article
	// Status, Note and AuthorName describe the proposal itself.
	Status     string
	Note       string
	AuthorName string
	// ReviewerName and RejectReason are filled when it is reviewed.
	ReviewerName string
	RejectReason string
	// AppliedArticleId is the article the proposal produced, once approved.
	AppliedArticleId string
	CreatedAt        time.Time
	ReviewedAt       *time.Time
	// Envelope carries the stored document's identifier and version, which the
	// repository applies when a proposal is read.
	Envelope Envelope
}

// NewSuggestion builds a pending proposal out of player input.
//
// The kind is decided by whether the author pointed at an existing article: an
// edit needs a target, everything else proposes a new page. The proposal is
// validated by the article rules before it is accepted, so a proposal that
// could not be published is refused at the door rather than in review.
func NewSuggestion(input SuggestionInput, authorName string, now time.Time) (*Suggestion, error) {
	kind := SuggestionNewArticle
	if input.ArticleId != "" {
		kind = SuggestionArticleEdit
	}

	proposal, err := NewArticle(input.Proposal, authorName, now)
	if err != nil {
		return nil, err
	}

	suggestion := &Suggestion{
		Kind:       kind,
		ArticleId:  input.ArticleId,
		Proposal:   *proposal,
		Status:     SuggestionPending,
		Note:       input.Note,
		AuthorName: authorName,
		CreatedAt:  now,
	}

	if kind != SuggestionArticleEdit {
		return suggestion, nil
	}

	// The service reads the target article before this call, so a proposal
	// without it would name a page nobody can apply the change to.
	if input.Base == nil {
		return nil, ErrNotFound
	}

	if input.BaseVersion != 0 && input.BaseVersion != input.Base.Envelope.Version {
		return nil, ErrSuggestionOutdated
	}

	suggestion.BaseVersion = input.Base.Envelope.Version

	// The base is kept for the reviewer to compare against, not to be edited.
	// Its history is dropped: carrying it here would multiply the stored
	// proposal by every revision the article holds.
	base := input.Base.withoutHistory()
	suggestion.Base = &base

	return suggestion, nil
}

// SuggestionInput is what a player submits.
type SuggestionInput struct {
	// ArticleId is set when the proposal changes an existing article.
	ArticleId string
	Proposal  Article
	Note      string
	// BaseVersion is the article version the client last read; 0 accepts the
	// stored article as it is.
	BaseVersion int64
	// Base is the stored article the proposal changes. The service fills it in
	// from storage; a client never does.
	Base *Article
}

// AdoptDocument takes the identifier and timestamps of the stored document.
//
// The repository calls it when it reads a proposal, so the domain carries what
// a reader needs without knowing how a document is addressed.
func (s *Suggestion) AdoptDocument(id string, createdAt time.Time) {
	s.Id = id
	s.CreatedAt = createdAt
}

// AdoptDocument takes the part of a stored document that is not the proposal
// itself: where it lives, when it was written and which revision it is.
func (s *Suggestion) AdoptEnvelope(envelope Envelope) {
	s.Envelope = envelope
}

// IsPending reports whether the proposal still waits for a decision.
func (s *Suggestion) IsPending() bool {
	return s.Status == SuggestionPending
}

// Reviewable reports whether the reviewer may decide this proposal now.
//
// A proposal that was already decided is refused, and so is one written by the
// reviewer. Proposing and deciding are two roles: without the second check a
// holder of the review scope could submit a proposal and approve it, which is a
// write the review scope does not carry.
func (s *Suggestion) Reviewable(reviewerName string) error {
	if !s.IsPending() {
		return ErrSuggestionReviewed
	}

	if reviewerName == s.AuthorName {
		return ErrSelfReview
	}

	return nil
}

// IsStaleAgainst reports whether the article changed after the proposal was
// written. A proposal from before base versions were kept cannot be checked
// and counts as stale: the reviewer confirms it explicitly.
func (s *Suggestion) IsStaleAgainst(current *Article) bool {
	if s.Kind != SuggestionArticleEdit {
		return false
	}

	return s.BaseVersion == 0 || current.Envelope.Version != s.BaseVersion
}

// Approve records the decision.
//
// The article the proposal produced is recorded afterwards by SetAppliedArticle,
// because the decision is reserved before the article is written: that order is
// what keeps two editors from applying one proposal twice.
func (s *Suggestion) Approve(reviewerName string, now time.Time) error {
	if err := s.Reviewable(reviewerName); err != nil {
		return err
	}

	s.Status = SuggestionApproved
	s.ReviewerName = reviewerName
	s.ReviewedAt = &now

	return nil
}

// SetAppliedArticle records the article the approved proposal produced.
func (s *Suggestion) SetAppliedArticle(articleId string) {
	s.AppliedArticleId = articleId
}

// Reopen returns a reserved proposal to the queue.
//
// The service calls it when writing the article failed, so a proposal is never
// left approved with nothing behind it and can be decided again.
func (s *Suggestion) Reopen() {
	s.Status = SuggestionPending
	s.ReviewerName = ""
	s.ReviewedAt = nil
}

// Reject records the decision and the reason the author will read.
func (s *Suggestion) Reject(reviewerName string, reason string, now time.Time) error {
	if err := s.Reviewable(reviewerName); err != nil {
		return err
	}

	s.Status = SuggestionRejected
	s.ReviewerName = reviewerName
	s.RejectReason = reason
	s.ReviewedAt = &now

	return nil
}

// ArticleFor stores what a proposal targets.
//
// An edit proposes what the page should look like, so the proposal is applied
// onto the stored article; a new page brings its own identity.
func (s *Suggestion) TargetArticleId() string {
	if s.Kind == SuggestionArticleEdit {
		return s.ArticleId
	}

	return ""
}

// SuggestionFilter narrows the proposal list.
type SuggestionFilter struct {
	// AuthorName limits the list to one author's proposals when set.
	AuthorName string
	// Status limits the list to one review state when set.
	Status string
}

// SuggestionPage is a page of proposals plus the cursor of the following one.
type SuggestionPage struct {
	Suggestions []*Suggestion
	Next        string
}
