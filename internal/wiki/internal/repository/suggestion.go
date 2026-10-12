package repository

import (
	"context"
	"errors"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"github.com/lasthearth/vsservice/internal/pkg/mongox/pagination"
	"github.com/lasthearth/vsservice/internal/wiki/internal/dto"
	"github.com/lasthearth/vsservice/internal/wiki/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

// CreateSuggestion stores a player proposal.
func (r *Repository) CreateSuggestion(
	ctx context.Context,
	suggestion *model.Suggestion,
) (*model.Suggestion, error) {
	l := r.logger.WithMethod("create_suggestion").With(zap.String("author", suggestion.AuthorName))
	l.Info("creating wiki suggestion")

	stored := r.mapper.ToSuggestion(*suggestion)
	stored.Model = mongox.NewModel()

	res, err := r.collSuggestions.InsertOne(ctx, stored)
	if err != nil {
		return nil, err
	}

	return r.getSuggestionByFilter(ctx, map[string]any{"_id": res.InsertedID})
}

// GetSuggestion returns one proposal by id.
func (r *Repository) GetSuggestion(ctx context.Context, id string) (*model.Suggestion, error) {
	objID, err := mongox.ParseObjectID(id)
	if err != nil {
		return nil, model.ErrSuggestionNotFound
	}

	return r.getSuggestionByFilter(ctx, map[string]any{"_id": objID})
}

// ListSuggestions returns proposals, newest first.
func (r *Repository) ListSuggestions(
	ctx context.Context,
	filter model.SuggestionFilter,
	next string,
	limit int64,
) (*model.SuggestionPage, error) {
	l := r.logger.WithMethod("list_suggestions").With(zap.Int64("limit", limit))
	l.Info("listing wiki suggestions")

	query := map[string]any{}

	if filter.AuthorName != "" {
		query["author_name"] = filter.AuthorName
	}

	if filter.Status != "" {
		query["status"] = filter.Status
	}

	suggestions, nextToken, err := pagination.List(
		ctx, r.collSuggestions, next, limit, r.suggestionToModel,
		pagination.WithFilter(query),
		pagination.WithSort(bson.D{{Key: "created_at", Value: -1}}),
	)
	if err != nil {
		l.Error("failed to list wiki suggestions", zap.Error(err))

		return nil, err
	}

	return &model.SuggestionPage{Suggestions: suggestions, Next: nextToken}, nil
}

// CountPendingSuggestions counts what one author still has waiting.
func (r *Repository) CountPendingSuggestions(ctx context.Context, authorName string) (int64, error) {
	return r.collSuggestions.CountDocuments(ctx, map[string]any{
		"author_name": authorName,
		"status":      model.SuggestionPending,
	})
}

// SaveSuggestion stores a reviewed proposal.
//
// The write is pinned to the version it read, so two editors deciding the same
// proposal cannot both win: the second one is told it moved on.
func (r *Repository) SaveSuggestion(
	ctx context.Context,
	suggestion *model.Suggestion,
) (*model.Suggestion, error) {
	l := r.logger.WithMethod("save_suggestion").With(zap.String("id", suggestion.Id))
	l.Info("saving wiki suggestion")

	objID, err := mongox.ParseObjectID(suggestion.Id)
	if err != nil {
		return nil, model.ErrSuggestionNotFound
	}

	saved, err := mongox.UpdateDoc(
		ctx, r.collSuggestions, map[string]any{"_id": objID}, model.ErrSuggestionNotFound,
		r.suggestionToModel,
		func(suggestion *model.Suggestion) dto.Suggestion {
			return r.mapper.ToSuggestion(*suggestion)
		},
		func(_ context.Context, current *model.Suggestion) (*model.Suggestion, error) {
			if !current.IsPending() {
				return nil, model.ErrSuggestionReviewed
			}

			return suggestion, nil
		},
	)
	if err != nil {
		if errors.Is(err, mongox.ErrConflict) {
			return nil, model.ErrSuggestionReviewed
		}

		return nil, err
	}

	return saved, nil
}

// SetAppliedSuggestionArticle records the article an approved proposal produced.
//
// The write is separate from the decision, because the article does not exist
// until the decision is reserved. Only one editor can reserve a proposal, so
// only that editor ever writes an article for it.
func (r *Repository) SetAppliedSuggestionArticle(ctx context.Context, id string, articleId string) error {
	objID, err := mongox.ParseObjectID(id)
	if err != nil {
		return model.ErrSuggestionNotFound
	}

	res, err := r.collSuggestions.UpdateOne(ctx,
		map[string]any{"_id": objID, "status": model.SuggestionApproved},
		map[string]any{"$set": map[string]any{
			"applied_article_id": articleId,
			"updated_at":         time.Now().UTC().Truncate(time.Millisecond),
		}},
	)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return model.ErrSuggestionNotFound
	}

	return nil
}

// ReopenSuggestion returns a reserved proposal to the queue.
//
// The service calls it when writing the article failed, so a proposal is never
// left approved with nothing behind it. The filter carries the approved state,
// so the call cannot undo a decision that already moved on.
func (r *Repository) ReopenSuggestion(ctx context.Context, id string) error {
	objID, err := mongox.ParseObjectID(id)
	if err != nil {
		return model.ErrSuggestionNotFound
	}

	_, err = r.collSuggestions.UpdateOne(ctx,
		map[string]any{"_id": objID, "status": model.SuggestionApproved},
		map[string]any{
			"$set": map[string]any{
				"status":     model.SuggestionPending,
				"updated_at": time.Now().UTC().Truncate(time.Millisecond),
			},
			"$unset": map[string]any{"reviewer_name": "", "reviewed_at": ""},
		},
	)

	return err
}

// ensureSuggestionIndexes creates the indexes the review queue relies on.
func (r *Repository) ensureSuggestionIndexes(ctx context.Context) {
	models := []mongo.IndexModel{
		{
			Keys:    map[string]any{"status": 1, "created_at": -1},
			Options: options.Index().SetName("wiki_suggestion_status_created"),
		},
		{
			Keys:    map[string]any{"author_name": 1, "status": 1},
			Options: options.Index().SetName("wiki_suggestion_author_status"),
		},
	}

	if _, err := r.collSuggestions.Indexes().CreateMany(ctx, models); err != nil {
		r.logger.Warn("failed to create wiki suggestion indexes: " + err.Error())
	}
}

// getSuggestionByFilter loads one proposal.
func (r *Repository) getSuggestionByFilter(
	ctx context.Context,
	filter map[string]any,
) (*model.Suggestion, error) {
	var stored dto.Suggestion

	if err := r.collSuggestions.FindOne(ctx, filter).Decode(&stored); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, model.ErrSuggestionNotFound
		}

		return nil, err
	}

	return r.suggestionToModel(stored), nil
}

// suggestionToModel converts a stored proposal and applies its envelope.
func (r *Repository) suggestionToModel(stored dto.Suggestion) *model.Suggestion {
	suggestion := r.mapper.FromSuggestion(stored)
	suggestion.AdoptDocument(stored.Model.Id.Hex(), stored.Model.CreatedAt)
	suggestion.AdoptEnvelope(model.Envelope{
		Id:        stored.Model.Id.Hex(),
		CreatedAt: stored.Model.CreatedAt,
		UpdatedAt: stored.Model.UpdatedAt,
		Version:   stored.Model.Version,
	})

	return &suggestion
}
