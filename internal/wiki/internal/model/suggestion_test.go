package model

import (
	"errors"
	"testing"
)

// TestNewSuggestionKeepsTheBaseItWasWrittenAgainst covers what an edit proposal
// remembers: the version it started from and the text the author was looking
// at, so a reviewer can see what the approval would replace.
func TestNewSuggestionKeepsTheBaseItWasWrittenAgainst(t *testing.T) {
	article := created(t)
	article.Envelope.Version = 3
	article.Revisions = []Revision{{Id: "old"}}

	suggestion, err := NewSuggestion(SuggestionInput{
		ArticleId:   article.Envelope.Id,
		Proposal:    draft(),
		Base:        article,
		BaseVersion: article.Envelope.Version,
	}, "author-1", now)
	if err != nil {
		t.Fatalf("NewSuggestion: %v", err)
	}

	if suggestion.Kind != SuggestionArticleEdit {
		t.Fatalf("kind = %q, want an edit", suggestion.Kind)
	}
	if suggestion.BaseVersion != 3 {
		t.Errorf("base version = %d, want 3", suggestion.BaseVersion)
	}
	if suggestion.Base == nil {
		t.Fatal("the proposal kept no base to compare against")
	}
	if suggestion.Base == article {
		t.Error("the base shares the stored article, so editing it would write through")
	}
	if len(suggestion.Base.Revisions) != 0 {
		t.Errorf("the base carries %d revisions, want none", len(suggestion.Base.Revisions))
	}
	if len(article.Revisions) != 1 {
		t.Error("the stored article lost its history")
	}
}

// TestNewSuggestionTakesTheStoredVersionOnAnUnstatedBase covers a client that
// does not say which version it read: the proposal is pinned to what storage
// holds now, which is the best answer available.
func TestNewSuggestionTakesTheStoredVersionOnAnUnstatedBase(t *testing.T) {
	article := created(t)
	article.Envelope.Version = 4

	suggestion, err := NewSuggestion(SuggestionInput{
		ArticleId: article.Envelope.Id,
		Proposal:  draft(),
		Base:      article,
	}, "author-1", now)
	if err != nil {
		t.Fatalf("NewSuggestion: %v", err)
	}

	if suggestion.BaseVersion != 4 {
		t.Errorf("base version = %d, want the stored 4", suggestion.BaseVersion)
	}
}

func TestNewSuggestionRefusesAnOutdatedBaseVersion(t *testing.T) {
	article := created(t)
	article.Envelope.Version = 4

	_, err := NewSuggestion(SuggestionInput{
		ArticleId:   article.Envelope.Id,
		Proposal:    draft(),
		Base:        article,
		BaseVersion: 3,
	}, "author-1", now)
	if !errors.Is(err, ErrSuggestionOutdated) {
		t.Fatalf("err = %v, want the outdated base reported", err)
	}
}

// TestNewSuggestionRefusesAnEditWithoutItsBase covers the invariant the service
// keeps by reading the article first: a proposal that names a page nobody
// loaded is refused rather than queued.
func TestNewSuggestionRefusesAnEditWithoutItsBase(t *testing.T) {
	_, err := NewSuggestion(SuggestionInput{
		ArticleId: "article-1",
		Proposal:  draft(),
	}, "author-1", now)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestIsStaleAgainst(t *testing.T) {
	at := func(version int64) *Article {
		article := created(t)
		article.Envelope.Version = version

		return article
	}

	edit := &Suggestion{Kind: SuggestionArticleEdit, BaseVersion: 3}
	if edit.IsStaleAgainst(at(3)) {
		t.Error("a proposal written against the version on record is not stale")
	}
	if !edit.IsStaleAgainst(at(4)) {
		t.Error("a proposal the article moved past is stale")
	}

	// A proposal from before base versions were kept cannot be checked at all,
	// so it needs the reviewer to confirm it explicitly.
	legacy := &Suggestion{Kind: SuggestionArticleEdit}
	if !legacy.IsStaleAgainst(at(1)) {
		t.Error("a proposal without a base version must count as stale")
	}

	// A new page has nothing to fall behind.
	fresh := &Suggestion{Kind: SuggestionNewArticle}
	if fresh.IsStaleAgainst(at(9)) {
		t.Error("a proposal for a new article is never stale")
	}
}
