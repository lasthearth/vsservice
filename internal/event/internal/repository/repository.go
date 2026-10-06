package repository

import (
	"context"
	"errors"
	"time"

	"github.com/lasthearth/vsservice/internal/event/internal/dto"
	"github.com/lasthearth/vsservice/internal/event/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

// notDeleted matches events that were not soft-deleted.
var notDeleted = bson.M{"deleted_at": bson.M{"$exists": false}}

// Create stores a new event and returns it with id and timestamps.
func (r *Repository) Create(ctx context.Context, event *model.Event) (*model.Event, error) {
	doc := fromModel(event)
	doc.Model = mongox.NewModel()

	if _, err := r.coll.InsertOne(ctx, doc); err != nil {
		r.logger.Error("failed to insert event", zap.Error(err))
		return nil, err
	}

	created := toModel(doc)
	return &created, nil
}

// Get returns a not-deleted event by id.
func (r *Repository) Get(ctx context.Context, id string) (*model.Event, error) {
	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return nil, model.ErrNotFound
	}

	filter := bson.M{"_id": oid, "deleted_at": bson.M{"$exists": false}}

	var doc dto.Event
	if err := r.coll.FindOne(ctx, filter).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, model.ErrNotFound
		}
		r.logger.Error("failed to find event", zap.String("id", id), zap.Error(err))
		return nil, err
	}

	event := toModel(doc)
	return &event, nil
}

// Update saves the editable fields of an existing, not-deleted event.
func (r *Repository) Update(ctx context.Context, event *model.Event) (*model.Event, error) {
	oid, err := mongox.ParseObjectID(event.Id)
	if err != nil {
		return nil, model.ErrNotFound
	}

	doc := fromModel(event)
	set := bson.M{
		"title":       doc.Title,
		"description": doc.Description,
		"cover":       doc.Cover,
		"location":    doc.Location,
		"starts_at":   doc.StartsAt,
		"until":       doc.Until,
		"updated_at":  time.Now(),
	}
	update := bson.M{"$set": set, "$inc": bson.M{"version": 1}}
	if doc.EndsAt != nil {
		set["ends_at"] = *doc.EndsAt
	} else {
		update["$unset"] = bson.M{"ends_at": ""}
	}

	filter := bson.M{"_id": oid, "deleted_at": bson.M{"$exists": false}}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)

	var updated dto.Event
	if err := r.coll.FindOneAndUpdate(ctx, filter, update, opts).Decode(&updated); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, model.ErrNotFound
		}
		r.logger.Error("failed to update event", zap.String("id", event.Id), zap.Error(err))
		return nil, err
	}

	result := toModel(updated)
	return &result, nil
}

// SoftDelete marks an event deleted.
func (r *Repository) SoftDelete(ctx context.Context, id, deletedBy string) error {
	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return model.ErrNotFound
	}

	filter := bson.M{"_id": oid, "deleted_at": bson.M{"$exists": false}}
	update := bson.M{"$set": bson.M{"deleted_at": time.Now(), "deleted_by": deletedBy}}

	res, err := r.coll.UpdateOne(ctx, filter, update)
	if err != nil {
		r.logger.Error("failed to delete event", zap.String("id", id), zap.Error(err))
		return err
	}
	if res.MatchedCount == 0 {
		return model.ErrNotFound
	}

	return nil
}

// ListUpcoming returns events that have not ended by now, soonest first.
func (r *Repository) ListUpcoming(ctx context.Context, now time.Time, limit int) ([]model.Event, error) {
	filter := bson.M{"until": bson.M{"$gte": now}}
	return r.list(ctx, filter, bson.D{{Key: "starts_at", Value: 1}}, limit)
}

// ListPast returns events that have ended by now, most recent first.
func (r *Repository) ListPast(ctx context.Context, now time.Time, limit int) ([]model.Event, error) {
	filter := bson.M{"until": bson.M{"$lt": now}}
	return r.list(ctx, filter, bson.D{{Key: "until", Value: -1}}, limit)
}

func (r *Repository) list(ctx context.Context, filter bson.M, sort bson.D, limit int) ([]model.Event, error) {
	for k, v := range notDeleted {
		filter[k] = v
	}

	opts := options.Find().SetSort(sort).SetLimit(int64(limit))
	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		r.logger.Error("failed to list events", zap.Error(err))
		return nil, err
	}

	var docs []dto.Event
	if err := cursor.All(ctx, &docs); err != nil {
		r.logger.Error("failed to decode events", zap.Error(err))
		return nil, err
	}

	events := make([]model.Event, 0, len(docs))
	for _, doc := range docs {
		events = append(events, toModel(doc))
	}

	return events, nil
}

func fromModel(e *model.Event) dto.Event {
	return dto.Event{
		Title:       e.Title,
		Description: e.Description,
		Cover:       e.Cover,
		Location:    e.Location,
		StartsAt:    e.StartsAt,
		EndsAt:      e.EndsAt,
		Until:       e.Until(),
		CreatedBy:   e.CreatedBy,
	}
}

func toModel(d dto.Event) model.Event {
	return model.Event{
		Id:          d.Model.Id.Hex(),
		Title:       d.Title,
		Description: d.Description,
		Cover:       d.Cover,
		Location:    d.Location,
		StartsAt:    d.StartsAt,
		EndsAt:      d.EndsAt,
		CreatedBy:   d.CreatedBy,
		CreatedAt:   d.Model.CreatedAt,
		UpdatedAt:   d.Model.UpdatedAt,
	}
}
