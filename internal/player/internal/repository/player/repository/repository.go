//go:generate goverter gen github.com/lasthearth/vsservice/internal/player/internal/repository/player/repository
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	dto "github.com/lasthearth/vsservice/internal/player/internal/dto/mongo"
	verificationdto "github.com/lasthearth/vsservice/internal/player/internal/dto/mongo/verification"
	"github.com/lasthearth/vsservice/internal/player/internal/ierror"
	"github.com/lasthearth/vsservice/internal/player/internal/model"
	"github.com/lasthearth/vsservice/internal/player/internal/model/verification"
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

const limit = 7

func (r *Repository) GetUserById(ctx context.Context, id string) (*model.Player, error) {
	filter := bson.M{
		"user_id": id,
	}

	finded := r.coll.FindOne(ctx, filter)
	err := finded.Err()
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ierror.ErrNotFound
		}

		return nil, err
	}

	var dto dto.Player
	err = finded.Decode(&dto)
	if err != nil {
		return nil, err
	}

	p := r.mapper.ToPlayer(dto)
	return &p, nil
}

// SearchUsers searches for players based on a query string, looking for matches in user_game_name and user_name fields.
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
			{Key: "previous_nickname", Value: 1},
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

// UpdatePlayerNickname updates the player's nickname and related fields
func (r *Repository) UpdatePlayerNickname(
	ctx context.Context,
	userId,
	newNickname,
	previousNickname string,
	lastChangedAt time.Time,
) error {
	update := bson.M{
		"user_game_name":           newNickname,
		"previous_nickname":        previousNickname,
		"last_nickname_changed_at": lastChangedAt,
	}

	_, err := r.coll.UpdateOne(ctx, bson.M{"user_id": userId}, bson.M{"$set": update})
	return err
}

func (r *Repository) UpdateById(ctx context.Context, id string, p model.Player) error {
	dto := r.mapper.FromPlayer(p)
	updset, _ := mongox.ComputeUpdateBson(
		dto,
		mongox.WithoutFields("_id"),
	)

	fmt.Printf("%+v", updset)

	return nil
}

func (r *Repository) UpdateByUserGameName(ctx context.Context, userGameName string, p model.PlayerUpdate) error {
	update := bson.M{}
	if p.UserId != nil && *p.UserId != "" {
		update["user_id"] = *p.UserId
	}
	if p.UserName != nil && *p.UserName != "" {
		update["user_name"] = *p.UserName
	}
	if p.UserGameName != nil && *p.UserGameName != "" {
		update["user_game_name"] = *p.UserGameName
	}
	if p.PreviousNickname != nil && *p.PreviousNickname != "" {
		update["previous_nickname"] = *p.PreviousNickname
	}
	if p.LastNicknameChangedAt != nil {
		update["last_nickname_changed_at"] = *p.LastNicknameChangedAt
	}
	if p.IsOnline != nil {
		update["is_online"] = *p.IsOnline
	}

	_, err := r.coll.UpdateOne(
		ctx,
		bson.M{"user_game_name": userGameName},
		bson.M{"$set": update},
	)
	return err
}

// GetByUserGameName implements event.PlayerRepository.
func (r *Repository) GetByUserGameName(ctx context.Context, userGameName string) (*model.Player, error) {
	l := r.log.WithMethod("get_by_user_game_name").With(zap.String("user_game_name", userGameName))

	l.Info("get player by user_game_name")

	var dto dto.Player
	finded := r.coll.FindOne(ctx, bson.M{"user_game_name": userGameName})
	err := finded.Err()
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ierror.ErrNotFound
		}
		return nil, err
	}

	err = finded.Decode(&dto)
	if err != nil {
		return nil, err
	}

	res := r.mapper.ToPlayer(dto)
	return &res, nil
}
