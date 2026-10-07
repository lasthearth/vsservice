//go:generate go tool goverter gen github.com/lasthearth/vsservice/internal/event/internal/repository
package repository

import (
	"context"
	"errors"
	"time"

	"github.com/lasthearth/vsservice/internal/event/internal/dto"
	"github.com/lasthearth/vsservice/internal/event/internal/ierror"
	"github.com/lasthearth/vsservice/internal/event/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

// goverter:converter
// goverter:output:file repomapper/mapper.go
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:ObjectIdToString
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTime
type Mapper interface {
	// goverter:ignore Model Until DeletedAt DeletedBy
	FromModel(model.Event) dto.Event

	// goverter:autoMap Model
	// goverter:map Id Id
	ToModel(dto.Event) model.Event
	ToModels([]dto.Event) []model.Event
}

// notDeleted matches events that were not soft-deleted.
func notDeleted(filter bson.M) bson.M {
	filter["deleted_at"] = bson.M{"$exists": false}
	return filter
}

// toDTO maps the model and fills the denormalized Until.
func (r *Repository) toDTO(event *model.Event) dto.Event {
	doc := r.mapper.FromModel(*event)
	doc.Until = event.Until()
	return doc
}

// Create stores a new event and returns it with id and timestamps.
func (r *Repository) Create(ctx context.Context, event *model.Event) (*model.Event, error) {
	l := r.logger.WithMethod("Create")

	doc := r.toDTO(event)
	doc.Model = mongox.NewModel()

	if _, err := r.coll.InsertOne(ctx, doc); err != nil {
		l.Error("failed to insert event", zap.Error(err))
		return nil, err
	}

	created := r.mapper.ToModel(doc)
	return &created, nil
}

// Get returns a not-deleted event by id.
func (r *Repository) Get(ctx context.Context, id string) (*model.Event, error) {
	doc, err := r.find(ctx, id)
	if err != nil {
		return nil, err
	}

	event := r.mapper.ToModel(*doc)
	return &event, nil
}

// UpdateEvent loads a not-deleted event, lets updateFn change it through its
// methods and stores the result in place.
func (r *Repository) UpdateEvent(
	ctx context.Context,
	id string,
	updateFn func(ctx context.Context, e *model.Event) (*model.Event, error),
) (*model.Event, error) {
	l := r.logger.WithMethod("UpdateEvent").With(zap.String("id", id))

	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return nil, ierror.ErrNotFound
	}

	// UpdateDoc owns the read-modify-write: it carries the stored envelope over,
	// stamps updated_at, and pins the replace to the version it read, retrying
	// before giving up. Without the pin two editors saving the same event both
	// succeed and the second silently discards the first. The not-deleted clause
	// stays in the filter, so an event deleted mid-cycle makes the guard miss
	// and the retry reports not found instead of resurrecting it.
	updated, err := mongox.UpdateDoc(
		ctx,
		r.coll,
		notDeleted(bson.M{"_id": oid}),
		ierror.ErrNotFound,
		func(d dto.Event) *model.Event {
			m := r.mapper.ToModel(d)
			return &m
		},
		r.toDTO,
		updateFn,
	)
	if err != nil && !errors.Is(err, ierror.ErrNotFound) {
		l.Error("failed to update event", zap.Error(err))
	}

	return updated, err
}

// SoftDelete marks an event deleted.
func (r *Repository) SoftDelete(ctx context.Context, id, deletedBy string) error {
	l := r.logger.WithMethod("SoftDelete").With(zap.String("id", id))

	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return ierror.ErrNotFound
	}

	update := bson.M{"$set": bson.M{"deleted_at": time.Now(), "deleted_by": deletedBy}}

	res, err := r.coll.UpdateOne(ctx, notDeleted(bson.M{"_id": oid}), update)
	if err != nil {
		l.Error("failed to delete event", zap.Error(err))
		return err
	}
	if res.MatchedCount == 0 {
		return ierror.ErrNotFound
	}

	return nil
}

// ListUpcoming returns events that have not ended by now, soonest first.
// _id breaks ties, so events sharing a starts_at keep a stable order between
// requests instead of swapping places.
func (r *Repository) ListUpcoming(ctx context.Context, now time.Time, limit int) ([]model.Event, error) {
	filter := bson.M{"until": bson.M{"$gte": now}}
	sort := bson.D{{Key: "starts_at", Value: 1}, {Key: "_id", Value: 1}}
	return r.list(ctx, filter, sort, limit)
}

// ListPast returns events that have ended by now, most recent first.
func (r *Repository) ListPast(ctx context.Context, now time.Time, limit int) ([]model.Event, error) {
	filter := bson.M{"until": bson.M{"$lt": now}}
	sort := bson.D{{Key: "until", Value: -1}, {Key: "_id", Value: -1}}
	return r.list(ctx, filter, sort, limit)
}

// find returns the stored not-deleted document.
func (r *Repository) find(ctx context.Context, id string) (*dto.Event, error) {
	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return nil, ierror.ErrNotFound
	}

	var doc dto.Event
	if err := r.coll.FindOne(ctx, notDeleted(bson.M{"_id": oid})).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ierror.ErrNotFound
		}
		r.logger.Error("failed to find event", zap.String("id", id), zap.Error(err))
		return nil, err
	}

	return &doc, nil
}

func (r *Repository) list(ctx context.Context, filter bson.M, sort bson.D, limit int) ([]model.Event, error) {
	l := r.logger.WithMethod("list")

	opts := options.Find().SetSort(sort).SetLimit(int64(limit))
	cursor, err := r.coll.Find(ctx, notDeleted(filter), opts)
	if err != nil {
		l.Error("failed to list events", zap.Error(err))
		return nil, err
	}

	var docs []dto.Event
	if err := cursor.All(ctx, &docs); err != nil {
		l.Error("failed to decode events", zap.Error(err))
		return nil, err
	}

	return r.mapper.ToModels(docs), nil
}
