package repository

import (
	"context"
	"errors"
	"time"

	mongomodel "github.com/lasthearth/vsservice/internal/pkg/mongox"
	attachmentdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/attachment"
	settlementdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/settlement"
	vector2dto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/vector2"
	repoerr "github.com/lasthearth/vsservice/internal/settlement/internal/ierror"
	"github.com/lasthearth/vsservice/internal/settlement/internal/service"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"github.com/samber/lo"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
)

func (r *Repository) Create(ctx context.Context, dto settlementdto.Settlement) error {
	// Kind is derived from the type, never trusted from the caller: a document without it answers
	// to neither of the two unique indexes that keep one membership per kind.
	dto.Kind = string(model.SettlementType(dto.Type).Kind())

	r.log.Debug("creating new settlement",
		zap.String("leader_id", dto.Leader.UserId),
		zap.String("settlement_name", dto.Name),
		zap.String("settlement_type", dto.Type))

	r.log.Debug("inserting settlement into database",
		zap.String("leader_id", dto.Leader.UserId),
		zap.String("model_id", dto.Id.Hex()))

	_, err := r.setColl.InsertOne(ctx, dto)
	if err != nil {
		r.log.Error("failed to insert settlement",
			zap.Error(err),
			zap.String("leader_id", dto.Leader.UserId),
			zap.String("model_id", dto.Id.Hex()))
		return err
	}

	r.log.Debug("successfully created settlement",
		zap.String("leader_id", dto.Leader.UserId),
		zap.String("model_id", dto.Id.Hex()))
	return nil
}

func (r *Repository) CountByLeaderID(ctx context.Context, id string) (int64, error) {
	count, err := r.setColl.CountDocuments(ctx, bson.M{
		"members": bson.M{"$elemMatch": bson.M{
			"user_id":  id,
			"role_ids": model.OwnerRoleId,
		}},
	})
	if err != nil {
		r.log.Error("failed to count settlements",
			zap.Error(err),
			zap.String("leader_id", id))
		return 0, err
	}

	return count, nil
}

func (r *Repository) Update(ctx context.Context, opts service.UpdateSettlementOpts) error {
	l := r.log.
		With(
			zap.String("leader_id", opts.Leader.UserId),
			zap.String("settlement_name", opts.Name),
			zap.String("settlement_type", string(opts.Type)),
		).
		WithMethod("update")
	l.Debug("updating settlement")

	objectID, err := mongomodel.ParseObjectID(opts.ID)
	if err != nil {
		r.log.Error("invalid settlement ID format", zap.Error(err), zap.String("settlement_id", opts.ID))
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	attachments := lo.Map(opts.Attachments, func(item model.Attachment, index int) attachmentdto.Attachment {
		return *attachmentdto.FromModel(&item)
	})

	result, err := r.setColl.UpdateOne(
		ctx,
		bson.M{"_id": objectID},
		bson.D{
			{
				Key: "$set",
				Value: bson.D{
					{Key: "name", Value: opts.Name},
					{Key: "type", Value: string(opts.Type)},
					// The kind follows the type: an upgrade must not leave the document in the
					// index of the kind it no longer belongs to.
					{Key: "kind", Value: string(opts.Type.Kind())},
					{Key: "coordinates", Value: vector2dto.FromModel(&opts.Coordinates)},
					{Key: "attachments", Value: attachments},
					{Key: "diplomacy", Value: opts.Diplomacy},
					{Key: "description", Value: opts.Description},
					{Key: "updated_at", Value: time.Now()},
				},
			},
		},
	)
	if err != nil {
		l.Error("failed to update settlement")
		return err
	}

	l.Debug(
		"successfully updated settlement",
		zap.Int64("modified_count", result.MatchedCount),
	)
	return nil
}

// UpdateSettlement implements service.SettlementDbRepository.
func (r *Repository) UpdateSettlement(
	ctx context.Context,
	id string,
	updateFn func(ctx context.Context, s *model.Settlement) (*model.Settlement, error),
) (*model.Settlement, error) {
	l := r.log.WithMethod("UpdateSettlement").With(zap.String("id", id))

	oid, err := mongomodel.ParseObjectID(id)
	if err != nil {
		return nil, repoerr.ErrNotFound
	}

	updated, err := mongomodel.UpdateDoc(
		ctx,
		r.setColl,
		bson.M{"_id": oid},
		repoerr.ErrNotFound,
		func(d settlementdto.Settlement) *model.Settlement {
			m := r.mapper.FromSettlementDTO(d)
			return &m
		},
		r.toSettlementDTO,
		updateFn,
	)
	if err != nil && !errors.Is(err, repoerr.ErrNotFound) {
		l.Error("failed to update settlement", zap.Error(err))
	}
	if updated != nil {
		updated.DeriveLeader()
	}
	return updated, err
}

// GetSettlement implements service.SettlementDbRepository.
func (r *Repository) GetSettlement(ctx context.Context, id string) (*model.Settlement, error) {
	r.log.Debug("retrieving settlement by ID", zap.String("settlement_id", id))

	objectID, err := mongomodel.ParseObjectID(id)
	if err != nil {
		r.log.Error("invalid settlement ID format", zap.Error(err), zap.String("settlement_id", id))
		return nil, err
	}

	res := r.setColl.FindOne(ctx, bson.M{"_id": objectID})
	if res.Err() != nil {
		if errors.Is(res.Err(), mongo.ErrNoDocuments) {
			return nil, repoerr.ErrNotFound
		}

		r.log.Error("failed to find settlement", zap.Error(res.Err()), zap.String("settlement_id", id))
		return nil, res.Err()
	}

	var settlement settlementdto.Settlement
	if err := res.Decode(&settlement); err != nil {
		r.log.Error("failed to decode settlement", zap.Error(err), zap.String("settlement_id", id))
		return nil, err
	}

	r.log.Debug("settlement retrieved", zap.String("settlement_id", id))
	set := r.mapper.FromSettlementDTO(settlement)
	set.DeriveLeader()
	return &set, nil
}

// toSettlementDTO maps a model back to its document and stamps the kind the two unique indexes
// filter on. UpdateDoc replaces the whole document, so a mapping that dropped the kind would take
// that document out of both indexes and free its members to take a second membership of the kind.
func (r *Repository) toSettlementDTO(m *model.Settlement) settlementdto.Settlement {
	dto := r.mapper.ToSettlementDTO(*m)
	dto.Kind = string(m.Type.Kind())
	return dto
}

// GetSettlementByUserId implements service.SettlementDbRepository: the settlement the user belongs
// to. A guild is served by GetGuildByUserId, because a player may hold one of each.
func (r *Repository) GetSettlementByUserId(ctx context.Context, userId string) (*model.Settlement, error) {
	return r.getByUserID(ctx, userId, model.KindSettlement)
}

// GetGuildByUserId implements service.SettlementDbRepository.
func (r *Repository) GetGuildByUserId(ctx context.Context, userId string) (*model.Settlement, error) {
	return r.getByUserID(ctx, userId, model.KindGuild)
}

// getByUserID finds the one group of this kind the user belongs to. Naming the kind in the filter
// is what lets the query use that kind's unique index, and that index is what bounds the answer to
// a single document.
func (r *Repository) getByUserID(ctx context.Context, userId string, kind model.SettlementKind) (*model.Settlement, error) {
	l := r.log.WithMethod("get_by_user_id").
		With(zap.String("user_id", userId), zap.String("kind", string(kind)))

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	filter := bson.M{"members.user_id": userId, "kind": string(kind)}
	found := r.setColl.FindOne(ctx, filter)
	err := found.Err()
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, repoerr.ErrNotFound
		}
		l.Error("find error", zap.Error(err))
		return nil, err
	}

	var settlement settlementdto.Settlement
	if err := found.Decode(&settlement); err != nil {
		l.Error("decode error", zap.Error(err))
		return nil, err
	}

	set := r.mapper.FromSettlementDTO(settlement)
	set.DeriveLeader()
	l.Debug("group retrieved", zap.String("settlement_id", set.Id))
	return &set, nil
}

// GetAllByKind implements service.SettlementDbRepository: every approved group of one kind.
// model.KindAny lists both, the meaning it carries everywhere else in this package.
func (r *Repository) GetAllByKind(ctx context.Context, kind model.SettlementKind) ([]model.Settlement, error) {
	l := r.log.WithMethod("get_all_by_kind").With(zap.String("kind", string(kind)))

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	filter := bson.M{}
	if kind != model.KindAny {
		filter["kind"] = string(kind)
	}

	found, err := r.setColl.Find(ctx, filter)
	if err != nil {
		l.Error("find error", zap.Error(err))
		return nil, err
	}
	defer func() {
		if err := found.Close(ctx); err != nil {
			l.Error("cursor close failed", zap.Error(err))
		}
	}()

	var settlements []settlementdto.Settlement
	if err := found.All(ctx, &settlements); err != nil {
		l.Error("cursor error", zap.Error(err))
		return nil, err
	}

	res := r.mapper.FromSettlementsDTO(settlements)
	for i := range res {
		res[i].DeriveLeader()
	}
	l.Debug("groups retrieved", zap.Int("count", len(res)))
	return res, nil
}

// IsLeaderOfSettlement checks that a user holds the owner role in the given
// settlement. Returns ErrNotLeader otherwise.
func (r *Repository) IsLeaderOfSettlement(ctx context.Context, settlementID, userID string) error {
	l := r.log.
		With(
			zap.String("settlement_id", settlementID),
			zap.String("user_id", userID),
		).
		WithMethod("is_leader_of_settlement")
	l.Debug("checking if user is owner of settlement")

	setId, err := mongomodel.ParseObjectID(settlementID)
	if err != nil {
		l.Error("failed to parse settlement ID", zap.Error(err))
		return err
	}

	filter := bson.M{
		"_id": setId,
		"members": bson.M{"$elemMatch": bson.M{
			"user_id":  userID,
			"role_ids": model.OwnerRoleId,
		}},
	}
	count, err := r.setColl.CountDocuments(ctx, filter)
	if err != nil {
		l.Error("failed to check existing leadership", zap.Error(err))
		return err
	}
	if count <= 0 {
		l.Warn("user is not owner of settlement")
		return repoerr.ErrNotLeader
	}

	return nil
}

// DeleteSettlement hard-deletes a settlement and its invitations and join
// requests. Imperial favor logs are retained for audit.
func (r *Repository) DeleteSettlement(ctx context.Context, settlementID string) error {
	l := r.log.With(zap.String("settlement_id", settlementID)).WithMethod("delete_settlement")

	oid, err := mongomodel.ParseObjectID(settlementID)
	if err != nil {
		return repoerr.ErrNotFound
	}

	res, err := r.setColl.DeleteOne(ctx, bson.M{"_id": oid})
	if err != nil {
		l.Error("failed to delete settlement", zap.Error(err))
		return err
	}
	if res.DeletedCount == 0 {
		return repoerr.ErrNotFound
	}

	r.deleteGroupRelations(ctx, settlementID)
	return nil
}

// DeleteSoloGuild removes a guild while one member is all it has, and its invitations and its
// applications with it. The count is part of the delete filter, so a player who joins between the
// service's read and this write makes the delete match nothing instead of going with the guild.
//
// It answers ErrDisbandConflict when the guild is no longer solo and ErrNotFound when it is gone.
func (r *Repository) DeleteSoloGuild(ctx context.Context, guildID, userID string) error {
	l := r.log.With(zap.String("guild_id", guildID), zap.String("user_id", userID)).
		WithMethod("delete_solo_guild")

	oid, err := mongomodel.ParseObjectID(guildID)
	if err != nil {
		return repoerr.ErrNotFound
	}

	res, err := r.setColl.DeleteOne(ctx, bson.M{
		"_id":             oid,
		"kind":            string(model.KindGuild),
		"members.user_id": userID,
		"members":         bson.M{"$size": 1},
	})
	if err != nil {
		l.Error("failed to disband the guild", zap.Error(err))
		return err
	}
	if res.DeletedCount == 0 {
		// The guild is either gone or no longer solo, and the two answer differently. The count
		// settles it, and it only runs on this race path.
		count, cerr := r.setColl.CountDocuments(ctx, bson.M{"_id": oid})
		if cerr != nil {
			l.Error("failed to re-read the guild after a delete that matched nothing", zap.Error(cerr))
			return cerr
		}
		if count == 0 {
			return repoerr.ErrNotFound
		}
		l.Warn("the guild gained a member before the disband landed",
			zap.String("event", "settlement.guild.disband_conflict"),
		)
		return repoerr.ErrDisbandConflict
	}

	l.Debug("guild disbanded by its last owner")
	r.deleteGroupRelations(ctx, guildID)
	return nil
}

// deleteGroupRelations drops the invitations and the applications that point at a group. Both are
// best effort: the group is already gone, and what is left answers NOT_FOUND when a player opens it.
func (r *Repository) deleteGroupRelations(ctx context.Context, groupID string) {
	l := r.log.With(zap.String("settlement_id", groupID))
	if _, err := r.setInvColl.DeleteMany(ctx, bson.M{"settlement_id": groupID}); err != nil {
		l.Error("failed to delete invitations", zap.Error(err))
	}
	if _, err := r.setJoinReqColl.DeleteMany(ctx, bson.M{"settlement_id": groupID}); err != nil {
		l.Error("failed to delete join requests", zap.Error(err))
	}
}
