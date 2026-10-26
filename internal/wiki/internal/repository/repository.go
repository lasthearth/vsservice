package repository

import (
	"context"
	"errors"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"github.com/lasthearth/vsservice/internal/pkg/mongox/pagination"
	"github.com/lasthearth/vsservice/internal/pkg/regexsafe"
	"github.com/lasthearth/vsservice/internal/wiki/internal/dto"
	"github.com/lasthearth/vsservice/internal/wiki/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
)

// Create stores a new article.
func (r *Repository) Create(ctx context.Context, article *model.Article) (*model.Article, error) {
	l := r.logger.WithMethod("create").With(zap.String("slug", article.Slug))
	l.Info("creating wiki article")

	stored := assignRevisionIds(r.mapper.ToArticle(*article))
	stored.Model = mongox.NewModel()

	res, err := r.coll.InsertOne(ctx, stored)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, model.ErrSlugTaken
		}

		return nil, err
	}

	return r.getByFilter(ctx, map[string]any{"_id": res.InsertedID})
}

// Get returns one article by id.
func (r *Repository) Get(ctx context.Context, id string) (*model.Article, error) {
	filter, err := idFilter(id)
	if err != nil {
		return nil, err
	}

	return r.getByFilter(ctx, filter)
}

// GetBySlug returns one article by its address.
func (r *Repository) GetBySlug(ctx context.Context, slug string) (*model.Article, error) {
	return r.getByFilter(ctx, map[string]any{"slug": slug})
}

// ExistsSlug reports whether another article already uses the slug.
func (r *Repository) ExistsSlug(ctx context.Context, slug string, exceptId string) (bool, error) {
	filter := map[string]any{"slug": slug}

	if exceptId != "" {
		objID, err := mongox.ParseObjectID(exceptId)
		if err != nil {
			return false, model.ErrNotFound
		}

		filter["_id"] = map[string]any{"$ne": objID}
	}

	count, err := r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// List returns a page of articles matching the filter.
//
// Filters compose into one query, so paging and counting always describe the
// same result set.
func (r *Repository) List(ctx context.Context, filter model.Filter, next string, limit int64) (*model.Page, error) {
	l := r.logger.WithMethod("list").With(zap.Int64("limit", limit))
	l.Info("listing wiki articles")

	query := listFilter(filter)

	total, err := r.coll.CountDocuments(ctx, query)
	if err != nil {
		return nil, err
	}

	articles, nextToken, err := pagination.List(
		ctx, r.coll, next, limit, r.toModel,
		pagination.WithFilter(query),
		pagination.WithSort(bson.D{{Key: "updated_at", Value: -1}}),
	)
	if err != nil {
		l.Error("failed to list wiki articles", zap.Error(err))

		return nil, err
	}

	return &model.Page{Articles: articles, Next: nextToken, Total: total}, nil
}

// Save replaces an article together with its revision history.
//
// The shared update helper owns the write: it carries the stored envelope over,
// stamps the modification time, and pins the replace to the version it read, so
// two editors saving the same page end with one winner instead of a silent
// overwrite.
func (r *Repository) Save(
	ctx context.Context,
	article *model.Article,
	expectedVersion int64,
) (*model.Article, error) {
	l := r.logger.WithMethod("save").With(zap.String("id", article.Envelope.Id))
	l.Info("saving wiki article")

	filter, err := idFilter(article.Envelope.Id)
	if err != nil {
		return nil, err
	}

	saved, err := mongox.UpdateDoc(
		ctx, r.coll, filter, model.ErrNotFound,
		r.toModel,
		func(article *model.Article) dto.Article {
			return assignRevisionIds(r.mapper.ToArticle(*article))
		},
		func(_ context.Context, current *model.Article) (*model.Article, error) {
			if expectedVersion != 0 && expectedVersion != current.Envelope.Version {
				return nil, model.ErrVersionConflict
			}

			return article, nil
		},
	)
	if err != nil {
		if errors.Is(err, mongox.ErrConflict) {
			return nil, model.ErrVersionConflict
		}
		if mongo.IsDuplicateKeyError(err) {
			return nil, model.ErrSlugTaken
		}

		return nil, err
	}

	return saved, nil
}

// Delete removes an article and its revision history.
func (r *Repository) Delete(ctx context.Context, id string) error {
	l := r.logger.WithMethod("delete").With(zap.String("id", id))
	l.Info("deleting wiki article")

	filter, err := idFilter(id)
	if err != nil {
		return err
	}

	res, err := r.coll.DeleteOne(ctx, filter)
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return model.ErrNotFound
	}

	return nil
}

// getByFilter loads one article and converts it, mapping a missing document onto
// the domain error.
func (r *Repository) getByFilter(ctx context.Context, filter map[string]any) (*model.Article, error) {
	var stored dto.Article

	if err := r.coll.FindOne(ctx, filter).Decode(&stored); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, model.ErrNotFound
		}

		return nil, err
	}

	return r.toModel(stored), nil
}

// toModel converts a stored document into a domain article and applies the
// envelope the document owns.
func (r *Repository) toModel(stored dto.Article) *model.Article {
	article := r.mapper.FromArticle(stored)
	article.AdoptDocument(model.Envelope{
		Id:        stored.Model.Id.Hex(),
		CreatedAt: stored.CreatedAt,
		UpdatedAt: stored.UpdatedAt,
		Version:   stored.Version,
	})

	revisions := make([]model.Revision, 0, len(stored.Revisions))

	for _, revision := range stored.Revisions {
		converted := r.mapper.FromRevision(revision)
		converted.Article.AdoptDocument(model.Envelope{
			Id:        revision.Article.Model.Id.Hex(),
			CreatedAt: revision.Article.CreatedAt,
			UpdatedAt: revision.Article.UpdatedAt,
			Version:   revision.Article.Version,
		})

		revisions = append(revisions, converted)
	}

	article.HoldRevisions(revisions)

	return &article
}

// assignRevisionIds gives every stored revision that has none an identifier of
// its own.
//
// Revisions live inside the article document, and MongoDB assigns _id only to a
// top-level document, never to an element of an array field. Without this the
// history is readable but no revision can be addressed, so a restore has
// nothing to name.
func assignRevisionIds(stored dto.Article) dto.Article {
	for i := range stored.Revisions {
		if stored.Revisions[i].Id == "" {
			stored.Revisions[i].Id = bson.NewObjectID().Hex()
		}
	}

	return stored
}

// idFilter builds the filter matching one live article by id.
func idFilter(id string) (map[string]any, error) {
	objID, err := mongox.ParseObjectID(id)
	if err != nil {
		return nil, model.ErrNotFound
	}

	return map[string]any{"_id": objID}, nil
}

// listFilter turns a domain filter into a Mongo query.
func listFilter(filter model.Filter) map[string]any {
	query := map[string]any{}

	if filter.Section != "" {
		query["section"] = filter.Section
	}

	// A status filter narrows the result; it never widens it. Drafts leave the
	// repository only for a caller who may see them, so a reader who asks for
	// drafts still gets the published pages and nothing else.
	switch {
	case filter.IncludeDrafts && filter.Status != "":
		query["status"] = filter.Status
	case filter.IncludeDrafts:
		// Every status: the caller may see drafts and published pages.
	default:
		query["status"] = model.StatusPublished
	}

	if filter.Tag != "" {
		query["tags"] = filter.Tag
	}

	if filter.Search != "" {
		pattern := bson.M{"$regex": regexsafe.Escape(filter.Search), "$options": "i"}
		query["$or"] = []bson.M{
			{"title": pattern},
			{"slug": pattern},
			{"lead": pattern},
			{"tags": pattern},
		}
	}

	return query
}
