//go:generate go tool goverter gen github.com/lasthearth/vsservice/internal/appearance/internal/repository
package repository

import (
	"context"

	"github.com/lasthearth/vsservice/internal/appearance/internal/dto"
	"github.com/lasthearth/vsservice/internal/appearance/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

// goverter:converter
// goverter:output:file repomapper/mapper.go
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTime
type Mapper interface {
	FromModel(model.Appearance) dto.Appearance
	ToModel(dto.Appearance) model.Appearance
	ToModels([]dto.Appearance) []model.Appearance
}

// List returns every saved appearance.
func (r *Repository) List(ctx context.Context) ([]model.Appearance, error) {
	cursor, err := r.coll.Find(ctx, bson.M{})
	if err != nil {
		r.logger.WithMethod("List").Error("failed to find appearances", zap.Error(err))
		return nil, err
	}

	var docs []dto.Appearance
	if err := cursor.All(ctx, &docs); err != nil {
		r.logger.WithMethod("List").Error("failed to decode appearances", zap.Error(err))
		return nil, err
	}

	return r.mapper.ToModels(docs), nil
}

// Save stores the player's appearance, replacing the previous one.
func (r *Repository) Save(ctx context.Context, a *model.Appearance) error {
	doc := r.mapper.FromModel(*a)
	filter := bson.M{"user_id": doc.UserId}
	replace := func() error {
		_, err := r.coll.ReplaceOne(ctx, filter, doc, options.Replace().SetUpsert(true))
		return err
	}
	err := replace()
	// Two first-time saves racing on the unique {user_id} index: this insert
	// lost, so a 500 would reach one of two identical requests. The document
	// exists now, so one retry lands on the replace path and this caller's
	// choice wins — the same outcome as two saves arriving in order.
	if mongo.IsDuplicateKeyError(err) {
		err = replace()
	}
	if err != nil {
		r.logger.WithMethod("Save").Error("failed to save appearance", zap.Error(err))
	}
	return err
}

// Delete removes the player's appearance; nothing saved is not an error.
func (r *Repository) Delete(ctx context.Context, userID string) error {
	if _, err := r.coll.DeleteOne(ctx, bson.M{"user_id": userID}); err != nil {
		r.logger.WithMethod("Delete").Error("failed to delete appearance", zap.Error(err))
		return err
	}
	return nil
}
