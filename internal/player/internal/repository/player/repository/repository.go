//go:generate goverter gen github.com/lasthearth/vsservice/internal/player/internal/repository/player/repository
package repository

import (
	"context"

	dto "github.com/lasthearth/vsservice/internal/player/internal/dto/mongo"
	verificationdto "github.com/lasthearth/vsservice/internal/player/internal/dto/mongo/verification"
	"github.com/lasthearth/vsservice/internal/player/internal/model"
	"github.com/lasthearth/vsservice/internal/player/internal/model/verification"
	"github.com/lasthearth/vsservice/internal/player/internal/repository/player/repository/repomapper"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

var _ Mapper = (*repomapper.MapperImpl)(nil)

// goverter:converter
// goverter:output:file repomapper/mapper.go
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:ObjectIdToString
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTime
type Mapper interface {
	// goverter:ignore Model
	FromPlayer(p model.Player) dto.Player
	// goverter:ignore Model
	FromVerification(verification verification.Verification) verificationdto.Verification
	FromAnswer(answer verification.Answer) verificationdto.Answer

	ToPlayers(dtos []dto.Player) []model.Player
	// goverter:autoMap Model
	ToPlayer(dto dto.Player) model.Player
	// goverter:autoMap Model
	ToVerification(dto verificationdto.Verification) verification.Verification
	ToAnswer(dto verificationdto.Answer) verification.Answer
}

func (r *Repository) GetPlayerByUserId(
	ctx context.Context,
	userId string,
) (*model.Player, error) {
	player := &model.Player{}
	err := r.coll.FindOne(ctx, bson.M{"user_id": userId}).Decode(player)
	if err != nil {
		return nil, err
	}
	return player, nil
}

const limit = 7

// SearchUsers implements service.DbRepository.
func (r *Repository) SearchUsers(
	ctx context.Context,
	query string,
) ([]model.Player, error) {
	filter := bson.M{
		"$or": []bson.M{
			{"user_game_name": bson.M{"$regex": query, "$options": "i"}},
			{"user_name": bson.M{"$regex": query, "$options": "i"}},
		},
	}

	proj := options.Find().
		SetProjection(bson.D{
			{Key: "user_game_name", Value: 1},
			{Key: "user_name", Value: 1},
			{Key: "user_id", Value: 1},
		}).
		SetSort(bson.D{
			{Key: "_id", Value: 1},
		}).
		SetLimit(int64(limit))

	cursor, err := r.coll.Find(ctx, filter, proj)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var dtos []dto.Player
	if err := cursor.All(ctx, &dtos); err != nil {
		return nil, err
	}

	users := r.mapper.ToPlayers(dtos)
	return users, nil
}
