package repository

import (
	"context"
	"time"

	"github.com/lasthearth/vsservice/internal/hungergames/internal/ierror"
	"github.com/lasthearth/vsservice/internal/hungergames/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
	mgo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

type playerStatsDTO struct {
	mongox.Model `bson:",inline"`
	PlayerID     string `bson:"player_id"`
	PlayerName   string `bson:"player_name"`
	Elo          int    `bson:"elo"`
	Wins         int    `bson:"wins"`
	Kills        int    `bson:"kills"`
	SeasonID     string `bson:"season_id"`
}

func (r *Repository) GetPlayerStats(ctx context.Context, seasonID, playerID string) (*model.PlayerStats, error) {
	var d playerStatsDTO
	err := r.playerStatsColl.FindOne(ctx, bson.M{"player_id": playerID, "season_id": seasonID}).Decode(&d)
	if err != nil {
		if err == mgo.ErrNoDocuments {
			return nil, ierror.ErrNotFound
		}
		r.log.Error("GetPlayerStats: find failed", zap.Error(err))
		return nil, err
	}
	return playerStatsFromDTO(d), nil
}

func (r *Repository) GetPlayerStatsByIDs(ctx context.Context, seasonID string, playerIDs []string) ([]*model.PlayerStats, error) {
	filter := bson.M{
		"season_id": seasonID,
		"player_id": bson.M{"$in": playerIDs},
	}
	cursor, err := r.playerStatsColl.Find(ctx, filter)
	if err != nil {
		r.log.Error("GetPlayerStatsByIDs: find failed", zap.Error(err))
		return nil, err
	}
	defer func() {
		if err := cursor.Close(ctx); err != nil {
			r.log.Error("cursor close failed", zap.Error(err))
		}
	}()

	var dtos []playerStatsDTO
	if err := cursor.All(ctx, &dtos); err != nil {
		return nil, err
	}

	stats := make([]*model.PlayerStats, len(dtos))
	for i, d := range dtos {
		stats[i] = playerStatsFromDTO(d)
	}
	return stats, nil
}

func (r *Repository) SavePlayerStats(ctx context.Context, stats *model.PlayerStats) error {
	now := time.Now()

	filter := bson.M{"player_id": stats.PlayerID, "season_id": stats.SeasonID}
	update := bson.D{
		{Key: "$set", Value: bson.D{
			{Key: "player_name", Value: stats.PlayerName},
			{Key: "elo", Value: stats.Elo},
			{Key: "wins", Value: stats.Wins},
			{Key: "kills", Value: stats.Kills},
			{Key: "updated_at", Value: now},
		}},
		{Key: "$setOnInsert", Value: bson.D{
			{Key: "_id", Value: newModel().Id},
			{Key: "created_at", Value: now},
			{Key: "player_id", Value: stats.PlayerID},
			{Key: "season_id", Value: stats.SeasonID},
		}},
	}

	_, err := r.playerStatsColl.UpdateOne(ctx, filter, update, options.UpdateOne().SetUpsert(true))
	if err != nil {
		r.log.Error("SavePlayerStats: upsert failed", zap.Error(err))
	}
	return err
}

func (r *Repository) ListPlayerStatsByELO(ctx context.Context, limit int) ([]*model.PlayerStats, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "elo", Value: -1}}).
		SetLimit(int64(limit))

	cursor, err := r.playerStatsColl.Find(ctx, bson.M{}, opts)
	if err != nil {
		r.log.Error("ListPlayerStatsByELO: find failed", zap.Error(err))
		return nil, err
	}
	defer func() {
		if err := cursor.Close(ctx); err != nil {
			r.log.Error("cursor close failed", zap.Error(err))
		}
	}()

	var dtos []playerStatsDTO
	if err := cursor.All(ctx, &dtos); err != nil {
		return nil, err
	}

	return playerStatsSliceFromDTOs(dtos), nil
}

func (r *Repository) ListAllPlayerStatsByELO(ctx context.Context) ([]*model.PlayerStats, error) {
	opts := options.Find().SetSort(bson.D{{Key: "elo", Value: -1}})

	cursor, err := r.playerStatsColl.Find(ctx, bson.M{}, opts)
	if err != nil {
		r.log.Error("ListAllPlayerStatsByELO: find failed", zap.Error(err))
		return nil, err
	}
	defer func() {
		if err := cursor.Close(ctx); err != nil {
			r.log.Error("cursor close failed", zap.Error(err))
		}
	}()

	var dtos []playerStatsDTO
	if err := cursor.All(ctx, &dtos); err != nil {
		return nil, err
	}

	return playerStatsSliceFromDTOs(dtos), nil
}

func (r *Repository) DeleteAllPlayerStats(ctx context.Context) error {
	_, err := r.playerStatsColl.DeleteMany(ctx, bson.M{})
	if err != nil {
		r.log.Error("DeleteAllPlayerStats: delete failed", zap.Error(err))
	}
	return err
}

func playerStatsFromDTO(d playerStatsDTO) *model.PlayerStats {
	return model.ReconstitutePlayerStats(
		d.Id.Hex(), d.PlayerID, d.PlayerName, d.Elo, d.Wins, d.Kills, d.SeasonID, d.CreatedAt, d.UpdatedAt,
	)
}

func playerStatsSliceFromDTOs(dtos []playerStatsDTO) []*model.PlayerStats {
	out := make([]*model.PlayerStats, len(dtos))
	for i, d := range dtos {
		out[i] = playerStatsFromDTO(d)
	}
	return out
}
