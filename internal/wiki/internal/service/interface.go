package service

import (
	"context"

	"github.com/lasthearth/vsservice/internal/wiki/internal/model"
	"github.com/lasthearth/vsservice/internal/wiki/internal/repository"
)

// Repository is the storage contract the wiki service depends on. It lives
// beside the service that consumes it, not in the repository package.
type Repository interface {
	// Create stores a new article and returns it as it was written.
	Create(ctx context.Context, article *model.Article) (*model.Article, error)
	// Get returns one article with its revision history.
	Get(ctx context.Context, id string) (*model.Article, error)
	// GetBySlug returns one article with its revision history.
	GetBySlug(ctx context.Context, slug string) (*model.Article, error)
	// ExistsSlug reports whether another article already uses the slug.
	ExistsSlug(ctx context.Context, slug string, exceptId string) (bool, error)
	// List returns a page of articles matching the filter.
	List(ctx context.Context, filter model.Filter, next string, limit int64) (*model.Page, error)
	// Save replaces an article together with its revision history.
	Save(ctx context.Context, article *model.Article, expectedVersion int64) (*model.Article, error)
	// Delete removes an article and its history.
	Delete(ctx context.Context, id string) error

	// CreateSuggestion stores a player proposal.
	CreateSuggestion(ctx context.Context, suggestion *model.Suggestion) (*model.Suggestion, error)
	// GetSuggestion returns one proposal by id.
	GetSuggestion(ctx context.Context, id string) (*model.Suggestion, error)
	// ListSuggestions returns proposals, newest first.
	ListSuggestions(
		ctx context.Context,
		filter model.SuggestionFilter,
		next string,
		limit int64,
	) (*model.SuggestionPage, error)
	// CountPendingSuggestions counts what one author still has waiting.
	CountPendingSuggestions(ctx context.Context, authorName string) (int64, error)
	// SaveSuggestion stores a reviewed proposal.
	SaveSuggestion(ctx context.Context, suggestion *model.Suggestion) (*model.Suggestion, error)
}

var _ Repository = (*repository.Repository)(nil)
