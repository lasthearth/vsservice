//go:generate go tool goverter gen github.com/lasthearth/vsservice/internal/lfg/internal/repository
package repository

import (
	"context"
	"errors"
	"time"

	"github.com/lasthearth/vsservice/internal/lfg/internal/dto"
	"github.com/lasthearth/vsservice/internal/lfg/internal/ierror"
	"github.com/lasthearth/vsservice/internal/lfg/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

// keepAfterExpiry is how long a post outlives the board before the TTL index
// removes it.
const keepAfterExpiry = 24 * time.Hour

// goverter:converter
// goverter:output:file repomapper/mapper.go
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:ObjectIdToString
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTime
type Mapper interface {
	// goverter:ignore Model DeleteAt
	FromModel(model.Post) dto.Post

	// goverter:autoMap Model
	ToModel(dto.Post) model.Post
}

// open matches posts still on the board at now.
func open(filter bson.M, now time.Time) bson.M {
	filter["expires_at"] = bson.M{"$gt": now}
	filter["closed_at"] = bson.M{"$exists": false}
	return filter
}

// The board sort orders. Each must be served by a matching index in lfgIndexes;
// index_covers_sort_test.go pins the agreement, because a mismatch is silent.
var (
	sortSessions  = bson.D{{Key: "starts_at", Value: 1}, {Key: "_id", Value: 1}}
	sortTeammates = bson.D{{Key: "bumped_at", Value: -1}, {Key: "_id", Value: -1}}
)

// toDTO maps the model and fills the TTL field.
func (r *Repository) toDTO(post *model.Post) dto.Post {
	doc := r.mapper.FromModel(*post)
	doc.DeleteAt = doc.ExpiresAt.Add(keepAfterExpiry)
	return doc
}

// toModel maps a document and normalizes absent lists to empty ones.

func (r *Repository) toModel(doc dto.Post) *model.Post {
	if doc.PlayDays == nil {
		doc.PlayDays = []int32{}
	}
	if doc.PlayTimes == nil {
		doc.PlayTimes = []string{}
	}
	m := r.mapper.ToModel(doc)
	return &m
}

// Create stores a new post.
func (r *Repository) Create(ctx context.Context, post *model.Post) (*model.Post, error) {
	doc := r.toDTO(post)
	doc.Model = mongox.NewModel()

	if _, err := r.coll.InsertOne(ctx, doc); err != nil {
		r.logger.WithMethod("Create").Error("failed to insert post", zap.Error(err))
		return nil, err
	}

	return r.toModel(doc), nil
}

// Get returns a post by id, open or not.
func (r *Repository) Get(ctx context.Context, id string) (*model.Post, error) {
	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return nil, ierror.ErrNotFound
	}

	var doc dto.Post
	if err := r.coll.FindOne(ctx, bson.M{"_id": oid}).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ierror.ErrNotFound
		}
		r.logger.WithMethod("Get").Error("failed to find post", zap.String("id", id), zap.Error(err))
		return nil, err
	}

	return r.toModel(doc), nil
}

// ListOpen returns posts of one kind still on the board at now: sessions
// soonest first, teammate posts freshest first. An empty activity means all
// of them.
func (r *Repository) ListOpen(
	ctx context.Context,
	now time.Time,
	kind model.Kind,
	activity model.Activity,
	limit int,
) ([]model.Post, error) {
	l := r.logger.WithMethod("ListOpen")

	filter := open(bson.M{"kind": string(kind)}, now)
	if activity != "" {
		filter["activities"] = string(activity)
	}
	sort := sortSessions
	if kind == model.KindTeammate {
		sort = sortTeammates
	}
	opts := options.Find().SetSort(sort).SetLimit(int64(limit))

	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		l.Error("failed to list posts", zap.Error(err))
		return nil, err
	}

	var docs []dto.Post
	if err := cursor.All(ctx, &docs); err != nil {
		l.Error("failed to decode posts", zap.Error(err))
		return nil, err
	}

	posts := make([]model.Post, 0, len(docs))
	for _, doc := range docs {
		posts = append(posts, *r.toModel(doc))
	}
	return posts, nil
}

// CountOpenByAuthor counts the author's posts of one kind still on the board
// at now.
func (r *Repository) CountOpenByAuthor(ctx context.Context, authorID string, kind model.Kind, now time.Time) (int64, error) {
	count, err := r.coll.CountDocuments(ctx, open(bson.M{"author_id": authorID, "kind": string(kind)}, now))
	if err != nil {
		r.logger.WithMethod("CountOpenByAuthor").Error("failed to count posts", zap.Error(err))
		return 0, err
	}
	return count, nil
}

// UpdatePost loads a post, lets updateFn change it through its methods and
// stores it with a version-guarded replace (see mongox.UpdateDoc).
func (r *Repository) UpdatePost(
	ctx context.Context,
	id string,
	updateFn func(ctx context.Context, p *model.Post) (*model.Post, error),
) (*model.Post, error) {
	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return nil, ierror.ErrNotFound
	}

	updated, err := mongox.UpdateDoc(
		ctx,
		r.coll,
		bson.M{"_id": oid},
		ierror.ErrNotFound,
		r.toModel,
		r.toDTO,
		updateFn,
	)
	if err != nil && !errors.Is(err, ierror.ErrNotFound) && !isBusinessErr(err) {
		r.logger.WithMethod("UpdatePost").Error("failed to update post", zap.String("id", id), zap.Error(err))
	}
	return updated, err
}

// isBusinessErr reports outcomes the caller is told about and that are not worth
// an error log: model refusals, and losing the version guard under contention,
// which the proto documents as a normal FAILED_PRECONDITION.
func isBusinessErr(err error) bool {
	return errors.Is(err, ierror.ErrPostClosed) ||
		errors.Is(err, ierror.ErrOwnPost) ||
		errors.Is(err, ierror.ErrPostFull) ||
		errors.Is(err, ierror.ErrNotAuthor) ||
		errors.Is(err, ierror.ErrNotRenewable) ||
		errors.Is(err, ierror.ErrRenewTooSoon) ||
		errors.Is(err, mongox.ErrConflict)
}
