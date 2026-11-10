//go:generate go tool goverter gen github.com/lasthearth/vsservice/internal/player/internal/repository/player/repository
package repository

import (
	"context"
	"errors"
	"time"
	"unicode/utf8"

	pkgierror "github.com/lasthearth/vsservice/internal/pkg/ierror"
	"github.com/lasthearth/vsservice/internal/pkg/regexsafe"
	dto "github.com/lasthearth/vsservice/internal/player/internal/dto/mongo"
	verificationdto "github.com/lasthearth/vsservice/internal/player/internal/dto/mongo/verification"
	"github.com/lasthearth/vsservice/internal/player/internal/ierror"
	"github.com/lasthearth/vsservice/internal/player/internal/model"
	"github.com/lasthearth/vsservice/internal/player/internal/model/verification"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// goverter:converter
// goverter:output:file repomapper/mapper.go
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:ObjectIdToString
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTime
// goverter:extend AvatarWithPrefix
type Mapper interface {
	// goverter:ignore Model
	FromPlayer(p model.Player) dto.Player
	// goverter:ignore Model
	FromVerification(verification verification.Verification) verificationdto.Verification
	FromAnswer(answer verification.Answer) verificationdto.Answer

	// goverter:context avatarPrefix
	ToPlayers(dtos []dto.Player, avatarPrefix string) []model.Player
	// goverter:autoMap Model
	// goverter:context avatarPrefix
	ToPlayer(dto dto.Player, avatarPrefix string) model.Player
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

	p := r.mapper.ToPlayer(dto, r.getAvatarPrefix())
	return &p, nil
}

// GetUsersByIds returns the players matching the given user IDs.
// IDs without a matching document are silently omitted (partial success).
func (r *Repository) GetUsersByIds(ctx context.Context, ids []string) ([]model.Player, error) {
	filter := bson.M{
		"user_id": bson.M{"$in": ids},
	}

	cursor, err := r.coll.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	var dtos []dto.Player
	if err := cursor.All(ctx, &dtos); err != nil {
		return nil, err
	}

	users := r.mapper.ToPlayers(dtos, r.getAvatarPrefix())
	return users, nil
}

// maxSearchQueryRunes mirrors the roster's maxRosterQueryRunes: SearchUsers is a
// name-fragment lookup, and the query is escaped into a $regex, which expands
// every metacharacter into two runes.
const maxSearchQueryRunes = 32

// errQueryTooLong is the answer to a query over maxSearchQueryRunes.
var errQueryTooLong = pkgierror.InvalidArgument("query is longer than 32 characters")

// boundedSearchQuery refuses an oversized query.
//
// The byte length goes first, so a body over the limit is answered before the
// rune count is computed over it. The rune count is the bound itself, because
// that is what the escaped pattern grows from.
func boundedSearchQuery(query string) error {
	if len(query) > maxSearchQueryRunes*utf8.UTFMax {
		return errQueryTooLong
	}

	if utf8.RuneCountInString(query) > maxSearchQueryRunes {
		return errQueryTooLong
	}

	return nil
}

// SearchUsers searches for players based on a query string, looking for matches
// in the user_game_name and user_name fields. The query reaches a $regex
// escaped: an unescaped one lets a caller build a pattern, and a pattern such as
// (a*)*b makes MongoDB's regex engine backtrack until the worker threads are
// gone.
//
// The query is bounded before it is escaped, so the expansion is bounded too: a
// caller may send a body up to the gRPC receive limit, and nothing between that
// limit and the escape stops it.
func (r *Repository) SearchUsers(
	ctx context.Context,
	query string,
) ([]model.Player, error) {
	if err := boundedSearchQuery(query); err != nil {
		return nil, err
	}

	escaped := regexsafe.Escape(query)
	filter := bson.M{
		"$or": []bson.M{
			{"user_game_name": bson.M{"$regex": escaped, "$options": "i"}},
			{"user_name": bson.M{"$regex": escaped, "$options": "i"}},
		},
	}

	proj := options.Find().
		SetProjection(bson.D{
			{Key: "user_game_name", Value: 1},
			{Key: "user_name", Value: 1},
			{Key: "user_id", Value: 1},
			{Key: "previous_nickname", Value: 1},
			{Key: "avatar", Value: 1},
			{Key: "about", Value: 1},
			{Key: "about_updated_at", Value: 1},
		}).
		SetSort(bson.D{
			{Key: "_id", Value: 1},
		}).
		SetLimit(int64(limit))

	cursor, err := r.coll.Find(ctx, filter, proj)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	var dtos []dto.Player
	if err := cursor.All(ctx, &dtos); err != nil {
		return nil, err
	}

	users := r.mapper.ToPlayers(dtos, r.getAvatarPrefix())
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

func (r *Repository) UpdateById(ctx context.Context, uid string, p model.PlayerUpdate) error {
	update := r.buildUpdateSet(p)
	if len(update) == 0 {
		// MongoDB rejects an update document with no operators, and the caller
		// asked for no change, so there is nothing to write.
		return nil
	}

	_, err := r.coll.UpdateOne(
		ctx,
		bson.M{"user_id": uid},
		update,
	)
	return err
}

// buildUpdateSet turns an update into the operator document the write applies.
// Fields the caller did not set are left alone; an empty About is an unset
// rather than an empty string, so a hidden description leaves no trace in the
// document and reads back as "no description".
func (r *Repository) buildUpdateSet(p model.PlayerUpdate) bson.M {
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
	// A grant and a recorded change are mutually exclusive: writing the field in
	// $set and $unset in one update makes MongoDB reject the whole document.
	// ClearNicknameCooldown wins, because a caller that clears it is asking for
	// the cooldown to be gone.
	if p.LastNicknameChangedAt != nil && !p.ClearNicknameCooldown {
		update["last_nickname_changed_at"] = *p.LastNicknameChangedAt
	}
	if p.IsOnline != nil {
		update["is_online"] = *p.IsOnline
	}
	if p.Avatar != nil {
		if p.Avatar.Original != "" {
			update["avatar.original"] = p.Avatar.Original
		}
		if p.Avatar.X96 != "" {
			update["avatar.x96"] = p.Avatar.X96
		}
		if p.Avatar.X48 != "" {
			update["avatar.x48"] = p.Avatar.X48
		}
	}

	unset := bson.M{}
	if p.ClearNicknameCooldown {
		unset["last_nickname_changed_at"] = ""
	}

	// The description and its write time are one decision, so they are made in
	// one branch. Splitting them lets a caller that removes the text and passes a
	// timestamp put about_updated_at in both operators, and MongoDB then rejects
	// the whole update as a path conflict.
	switch {
	case p.About == nil:
		if p.AboutUpdatedAt != nil {
			update["about_updated_at"] = *p.AboutUpdatedAt
		}
	case *p.About == "":
		unset["about"] = ""
		unset["about_updated_at"] = ""
	default:
		update["about"] = *p.About
		if p.AboutUpdatedAt != nil {
			update["about_updated_at"] = *p.AboutUpdatedAt
		}
	}

	// An operator with no fields makes MongoDB reject the whole write, so only
	// the operators that carry something are sent.
	doc := bson.M{}
	if len(update) > 0 {
		doc["$set"] = update
	}
	if len(unset) > 0 {
		doc["$unset"] = unset
	}

	return doc
}

func (r *Repository) getAvatarPrefix() string {
	return r.cfg.CdnUrl + "/avatars"
}
