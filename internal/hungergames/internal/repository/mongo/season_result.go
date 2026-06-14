package repository

import (
	"context"
	"errors"
	"time"

	"github.com/lasthearth/vsservice/internal/hungergames/internal/ierror"
	"github.com/lasthearth/vsservice/internal/hungergames/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
	mgo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

type seasonResultDTO struct {
	mongox.Model `bson:",inline"`
	SeasonID     string    `bson:"season_id"`
	PlayerID     string    `bson:"player_id"`
	PlayerName   string    `bson:"player_name"`
	Elo          int       `bson:"elo"`
	Wins         int       `bson:"wins"`
	Kills        int       `bson:"kills"`
	Rank         int       `bson:"rank"`
	RewardCoins  int64     `bson:"reward_coins"`
	CreatedAt    time.Time `bson:"created_at"`
}

func (r *Repository) CreateSeasonResults(ctx context.Context, results []*model.SeasonResult) error {
	docs := make([]any, len(results))
	for i, res := range results {
		m := newModel()
		docs[i] = seasonResultDTO{
			Model:       m,
			SeasonID:    res.SeasonID,
			PlayerID:    res.PlayerID,
			PlayerName:  res.PlayerName,
			Elo:         res.Elo,
			Wins:        res.Wins,
			Kills:       res.Kills,
			Rank:        res.Rank,
			RewardCoins: res.RewardCoins,
			CreatedAt:   m.CreatedAt,
		}
	}

	if _, err := r.seasonResultColl.InsertMany(ctx, docs); err != nil {
		r.log.Error("CreateSeasonResults: insert failed", zap.Error(err))
		return err
	}
	return nil
}

func (r *Repository) ListSeasonResults(ctx context.Context, seasonID string) ([]*model.SeasonResult, error) {
	opts := options.Find().SetSort(bson.D{{Key: "rank", Value: 1}}).SetLimit(10)

	cursor, err := r.seasonResultColl.Find(ctx, bson.M{"season_id": seasonID}, opts)
	if err != nil {
		r.log.Error("ListSeasonResults: find failed", zap.Error(err))
		return nil, err
	}
	defer func() {
		if err := cursor.Close(ctx); err != nil {
			r.log.Error("cursor close failed", zap.Error(err))
		}
	}()

	var dtos []seasonResultDTO
	if err := cursor.All(ctx, &dtos); err != nil {
		return nil, err
	}

	out := make([]*model.SeasonResult, len(dtos))
	for i, d := range dtos {
		out[i] = seasonResultFromDTO(d)
	}
	return out, nil
}

func (r *Repository) GetPlayerSeasonResult(ctx context.Context, seasonID, playerID string) (*model.SeasonResult, error) {
	var d seasonResultDTO
	err := r.seasonResultColl.FindOne(ctx, bson.M{"season_id": seasonID, "player_id": playerID}).Decode(&d)
	if err != nil {
		if errors.Is(err, mgo.ErrNoDocuments) {
			return nil, ierror.ErrNotFound
		}
		r.log.Error("GetPlayerSeasonResult: find failed", zap.Error(err))
		return nil, err
	}
	return seasonResultFromDTO(d), nil
}

func seasonResultFromDTO(d seasonResultDTO) *model.SeasonResult {
	return &model.SeasonResult{
		ID:          d.Id.Hex(),
		SeasonID:    d.SeasonID,
		PlayerID:    d.PlayerID,
		PlayerName:  d.PlayerName,
		Elo:         d.Elo,
		Wins:        d.Wins,
		Kills:       d.Kills,
		Rank:        d.Rank,
		RewardCoins: d.RewardCoins,
		CreatedAt:   d.CreatedAt,
	}
}
