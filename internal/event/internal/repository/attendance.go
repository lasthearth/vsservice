package repository

import (
	"context"
	"time"

	"github.com/lasthearth/vsservice/internal/event/internal/dto"
	"github.com/lasthearth/vsservice/internal/event/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

// maxMyEvents bounds how many sign-ups of one player are looked at.
const maxMyEvents = 500

// SetAttendance signs userID up for eventID or withdraws them. Idempotent: an
// upsert keyed by the unique (event_id, user_id) pair keeps the first sign-up
// time, and withdrawing twice is a no-op.
func (r *Repository) SetAttendance(ctx context.Context, eventID, userID string, attending bool, now time.Time) error {
	l := r.logger.WithMethod("SetAttendance").With(zap.String("event_id", eventID))
	key := bson.M{"event_id": eventID, "user_id": userID}

	if !attending {
		if _, err := r.attendees.DeleteOne(ctx, key); err != nil {
			l.Error("failed to withdraw", zap.Error(err))
			return err
		}
		return nil
	}

	_, err := r.attendees.UpdateOne(
		ctx,
		key,
		bson.M{"$setOnInsert": bson.M{"created_at": now}},
		options.UpdateOne().SetUpsert(true),
	)
	// Two concurrent upserts of the same pair: the loser hits the unique index,
	// and the player is signed up either way.
	if err != nil && !mongo.IsDuplicateKeyError(err) {
		l.Error("failed to sign up", zap.Error(err))
		return err
	}
	return nil
}

// Attendees summarizes sign-ups for each of eventIDs. Events nobody signed up
// for are absent from the map.
func (r *Repository) Attendees(ctx context.Context, eventIDs []string) (map[string]model.Attendees, error) {
	out := make(map[string]model.Attendees, len(eventIDs))
	if len(eventIDs) == 0 {
		return out, nil
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"event_id": bson.M{"$in": eventIDs}}}},
		{{Key: "$sort", Value: bson.D{{Key: "created_at", Value: 1}, {Key: "_id", Value: 1}}}},
		{{Key: "$group", Value: bson.M{
			"_id":   "$event_id",
			"count": bson.M{"$sum": 1},
			"users": bson.M{"$push": "$user_id"},
		}}},
		{{Key: "$project", Value: bson.M{
			"count":   1,
			"preview": bson.M{"$slice": bson.A{"$users", model.AttendeePreviewSize}},
		}}},
	}

	cursor, err := r.attendees.Aggregate(ctx, pipeline)
	if err != nil {
		r.logger.WithMethod("Attendees").Error("failed to count attendees", zap.Error(err))
		return nil, err
	}

	var rows []struct {
		EventID string   `bson:"_id"`
		Count   int32    `bson:"count"`
		Preview []string `bson:"preview"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.EventID] = model.Attendees{Count: row.Count, Preview: row.Preview}
	}
	return out, nil
}

// ListAttendees returns up to limit attendees of eventID in sign-up order, and
// how many there are in total.
func (r *Repository) ListAttendees(ctx context.Context, eventID string, limit int) ([]string, int64, error) {
	l := r.logger.WithMethod("ListAttendees").With(zap.String("event_id", eventID))
	filter := bson.M{"event_id": eventID}

	total, err := r.attendees.CountDocuments(ctx, filter)
	if err != nil {
		l.Error("failed to count attendees", zap.Error(err))
		return nil, 0, err
	}

	ids, err := r.userIDs(ctx, filter, int64(limit))
	if err != nil {
		l.Error("failed to list attendees", zap.Error(err))
		return nil, 0, err
	}
	return ids, total, nil
}

// AllAttendees returns every attendee of eventID, for notifications.
func (r *Repository) AllAttendees(ctx context.Context, eventID string) ([]string, error) {
	return r.userIDs(ctx, bson.M{"event_id": eventID}, 0)
}

// ListMine returns events userID signed up for: not over yet, soonest first,
// or with past, the finished ones, most recent first.
func (r *Repository) ListMine(ctx context.Context, userID string, now time.Time, past bool, limit int) ([]model.Event, error) {
	l := r.logger.WithMethod("ListMine")

	cursor, err := r.attendees.Find(
		ctx,
		bson.M{"user_id": userID},
		options.Find().
			SetProjection(bson.M{"event_id": 1}).
			SetSort(bson.D{{Key: "created_at", Value: -1}}).
			SetLimit(maxMyEvents),
	)
	if err != nil {
		l.Error("failed to list sign-ups", zap.Error(err))
		return nil, err
	}
	var rows []dto.Attendance
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []model.Event{}, nil
	}

	oids := make([]bson.ObjectID, 0, len(rows))
	for _, row := range rows {
		// A malformed id cannot match any event; skip it rather than fail.
		if oid, err := mongox.ParseObjectID(row.EventId); err == nil {
			oids = append(oids, oid)
		}
	}

	filter := bson.M{"_id": bson.M{"$in": oids}, "until": bson.M{"$gte": now}}
	sort := bson.D{{Key: "starts_at", Value: 1}, {Key: "_id", Value: 1}}
	if past {
		filter["until"] = bson.M{"$lt": now}
		sort = bson.D{{Key: "until", Value: -1}, {Key: "_id", Value: -1}}
	}
	return r.list(ctx, filter, sort, limit)
}

// ListStartingWithin returns not-deleted events that start in (from, to].
func (r *Repository) ListStartingWithin(ctx context.Context, from, to time.Time) ([]model.Event, error) {
	filter := bson.M{"starts_at": bson.M{"$gt": from, "$lte": to}}
	sort := bson.D{{Key: "starts_at", Value: 1}, {Key: "_id", Value: 1}}
	return r.list(ctx, filter, sort, 100)
}

// ClaimReminders marks the attendees of eventID who were not yet reminded
// about startsAt as reminded and returns them. Each row is claimed by a
// conditional update, so two service instances never remind the same player
// twice.
func (r *Repository) ClaimReminders(ctx context.Context, eventID string, startsAt time.Time) ([]string, error) {
	l := r.logger.WithMethod("ClaimReminders").With(zap.String("event_id", eventID))
	pending := bson.M{"event_id": eventID, "reminded_for": bson.M{"$ne": startsAt}}

	cursor, err := r.attendees.Find(ctx, pending, options.Find().SetProjection(bson.M{"user_id": 1}))
	if err != nil {
		l.Error("failed to find attendees to remind", zap.Error(err))
		return nil, err
	}
	var rows []dto.Attendance
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}

	claimed := make([]string, 0, len(rows))
	for _, row := range rows {
		res, err := r.attendees.UpdateOne(
			ctx,
			bson.M{"_id": row.Id, "reminded_for": bson.M{"$ne": startsAt}},
			bson.M{"$set": bson.M{"reminded_for": startsAt}},
		)
		if err != nil {
			l.Error("failed to claim reminder", zap.Error(err))
			continue
		}
		if res.ModifiedCount == 1 {
			claimed = append(claimed, row.UserId)
		}
	}
	return claimed, nil
}

func (r *Repository) userIDs(ctx context.Context, filter bson.M, limit int64) ([]string, error) {
	opts := options.Find().
		SetProjection(bson.M{"user_id": 1}).
		SetSort(bson.D{{Key: "created_at", Value: 1}, {Key: "_id", Value: 1}})
	if limit > 0 {
		opts.SetLimit(limit)
	}

	cursor, err := r.attendees.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	var rows []dto.Attendance
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.UserId)
	}
	return ids, nil
}
