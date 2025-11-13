package repository

import (
	"context"
	"errors"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	settlementdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/settlement"
	"github.com/lasthearth/vsservice/internal/settlement/internal/ierror"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func (r *Repository) AddTag(ctx context.Context, settlementId, tagId string) error {
	oid, err := mongox.ParseObjectID(settlementId)
	if err != nil {
		return errors.Join(ierror.ErrInvalidArgument, err)
	}

	var settlement settlementdto.Settlement
	err = r.setColl.FindOne(
		ctx,
		bson.M{"_id": oid},
	).Decode(&settlement)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return ierror.ErrNotFound
		}
		return err
	}

	isTagLimitReached := len(settlement.TagIds) >= 20
	if isTagLimitReached {
		return ierror.ErrTooManyTagsLimit
	}

	for _, existingTagId := range settlement.TagIds {
		isExistingTag := existingTagId == tagId
		if isExistingTag {
			return ierror.ErrSettlementAlreadyHasTag
		}
	}

	filter := bson.M{
		"_id": oid,
	}
	update := bson.M{
		"$addToSet": bson.M{"tag_ids": tagId},
		"$set":      bson.M{"updated_at": time.Now()},
	}

	result, err := r.setColl.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return ierror.ErrNotFound
	}

	return nil
}

// RemoveTagFromSettlement removes a tag ID from a settlement
func (r *Repository) RemoveTag(ctx context.Context, settlementId, tagId string) error {
	oid, err := mongox.ParseObjectID(settlementId)
	if err != nil {
		return errors.Join(ierror.ErrInvalidArgument, err)
	}

	filter := bson.M{"_id": oid}
	update := bson.M{
		"$pull": bson.M{"tag_ids": tagId},
		"$set":  bson.M{"updated_at": time.Now()},
	}

	result, err := r.setColl.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return ierror.ErrNotFound
	}

	return nil
}
