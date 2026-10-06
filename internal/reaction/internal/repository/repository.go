package repository

import (
	"context"
	"time"

	"github.com/lasthearth/vsservice/internal/reaction/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
)

// reactionDoc is the `reactions` collection document.
type reactionDoc struct {
	Target    string    `bson:"target"`
	UserID    string    `bson:"user_id"`
	Emoji     string    `bson:"emoji"`
	CreatedAt time.Time `bson:"created_at"`
}

// Toggle removes the player's emoji from the target if it is there, adds it
// otherwise, and reports whether it is on afterwards.
func (r *Repository) Toggle(ctx context.Context, target, userID, emoji string) (bool, error) {
	filter := bson.M{"target": target, "user_id": userID, "emoji": emoji}

	res, err := r.coll.DeleteOne(ctx, filter)
	if err != nil {
		r.logger.Error("failed to remove reaction", zap.Error(err))
		return false, err
	}
	if res.DeletedCount > 0 {
		return false, nil
	}

	doc := reactionDoc{Target: target, UserID: userID, Emoji: emoji, CreatedAt: time.Now()}
	if _, err := r.coll.InsertOne(ctx, doc); err != nil {
		// A concurrent toggle already put it on: the end state is "on" too.
		if mongo.IsDuplicateKeyError(err) {
			return true, nil
		}
		r.logger.Error("failed to add reaction", zap.Error(err))
		return false, err
	}

	return true, nil
}

// Counts returns non-zero counts per (target, emoji) for the given targets.
func (r *Repository) Counts(ctx context.Context, targets []string) ([]model.Count, error) {
	pipeline := bson.A{
		bson.D{{Key: "$match", Value: bson.M{"target": bson.M{"$in": targets}}}},
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.D{{Key: "target", Value: "$target"}, {Key: "emoji", Value: "$emoji"}}},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
	}

	cursor, err := r.coll.Aggregate(ctx, pipeline)
	if err != nil {
		r.logger.Error("failed to count reactions", zap.Error(err))
		return nil, err
	}

	var rows []struct {
		ID struct {
			Target string `bson:"target"`
			Emoji  string `bson:"emoji"`
		} `bson:"_id"`
		Count int64 `bson:"count"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		r.logger.Error("failed to decode reaction counts", zap.Error(err))
		return nil, err
	}

	counts := make([]model.Count, 0, len(rows))
	for _, row := range rows {
		counts = append(counts, model.Count{Target: row.ID.Target, Emoji: row.ID.Emoji, Count: row.Count})
	}

	return counts, nil
}

// UserEmojis returns, per target, the emojis the player has put.
func (r *Repository) UserEmojis(ctx context.Context, userID string, targets []string) (map[string][]string, error) {
	filter := bson.M{"user_id": userID, "target": bson.M{"$in": targets}}

	cursor, err := r.coll.Find(ctx, filter)
	if err != nil {
		r.logger.Error("failed to list user reactions", zap.Error(err))
		return nil, err
	}

	var docs []reactionDoc
	if err := cursor.All(ctx, &docs); err != nil {
		r.logger.Error("failed to decode user reactions", zap.Error(err))
		return nil, err
	}

	result := make(map[string][]string, len(docs))
	for _, doc := range docs {
		result[doc.Target] = append(result[doc.Target], doc.Emoji)
	}

	return result, nil
}
