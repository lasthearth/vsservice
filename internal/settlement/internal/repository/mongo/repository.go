package repository

import (
	"context"
	"errors"
	"time"

	mongomodel "github.com/lasthearth/vsservice/internal/pkg/mongo"
	attachmentdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/attachment"
	memberdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/member"
	settlementdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/settlement"
	vector2dto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/vector2"
	"github.com/lasthearth/vsservice/internal/settlement/internal/repository/mongo/repoerr"
	"github.com/lasthearth/vsservice/internal/settlement/internal/service"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"github.com/samber/lo"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
)

func (r *Repository) Create(ctx context.Context, dto settlementdto.Settlement) error {
	r.log.Info("creating new settlement",
		zap.String("leader_id", dto.Leader.UserId),
		zap.String("settlement_name", dto.Name),
		zap.String("settlement_type", string(dto.Type)))

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

	r.log.Info("successfully created settlement",
		zap.String("leader_id", dto.Leader.UserId),
		zap.String("model_id", dto.Id.Hex()))
	return nil
}

func (r *Repository) CountByLeaderID(ctx context.Context, id string) (int64, error) {
	count, err := r.setColl.CountDocuments(ctx, bson.D{{
		Key:   "leader.user_id",
		Value: id,
	}})
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
	l.Info("updating settlement")

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
					{Key: "leader", Value: memberdto.FromModel(&opts.Leader)},
					{Key: "coordinates", Value: vector2dto.FromModel(&opts.Coordinates)},
					{Key: "attachments", Value: attachments},
					{Key: "updated_at", Value: time.Now()},
				},
			},
		},
	)
	if err != nil {
		l.Error("failed to update settlement")
		return err
	}

	l.Info(
		"successfully updated settlement",
		zap.Int64("modified_count", result.MatchedCount),
	)
	return nil
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
		if res.Err() == mongo.ErrNoDocuments {
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
	return settlement.ToModel(), nil
}

// GetSettlementsByLeader implements service.SettlementDbRepository.
func (r *Repository) GetSettlementByLeader(ctx context.Context, leaderID string) (*model.Settlement, error) {
	r.log.Info("retrieving settlements by leader", zap.String("leader_id", leaderID))

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	r.log.Debug("executing find query on settlement collection")
	found := r.setColl.FindOne(ctx, bson.M{"leader_id": leaderID})
	err := found.Err()
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, repoerr.ErrNotFound
		}
		r.log.Error("find error", zap.Error(err))
		return nil, err
	}

	var settlement settlementdto.Settlement
	if err := found.Decode(&settlement); err != nil {
		r.log.Error("cursor error", zap.Error(err))
		return nil, err
	}

	res := settlement.ToModel()
	r.log.Info("successfully retrieved settlements", zap.String("settlement_id", res.Id))
	return res, nil
}

// GetAllSettlements implements service.SettlementDbRepository.
func (r *Repository) GetAllSettlements(ctx context.Context) ([]model.Settlement, error) {
	r.log.Info("retrieving all settlements")

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	r.log.Debug("executing find query on settlement collection")
	found, err := r.setColl.Find(ctx, bson.M{})
	if err != nil {
		r.log.Error("find error", zap.Error(err))
		return nil, err
	}
	defer found.Close(ctx)

	var settlements []settlementdto.Settlement
	if err := found.All(ctx, &settlements); err != nil {
		r.log.Error("cursor error", zap.Error(err))
		return nil, err
	}

	result := lo.Map(settlements, func(item settlementdto.Settlement, index int) model.Settlement {
		return *item.ToModel()
	})

	r.log.Info("successfully retrieved all settlements", zap.Int("count", len(result)))
	return result, nil
}

// IsMemberOrLeader checks if a user is already a member or leader of any settlement. Returns ErrAlreadyMember if the user is already a member or leader.
func (r *Repository) IsMemberOrLeader(ctx context.Context, settlementID, memberID string) error {
	l := r.log.
		With(
			zap.String("settlement_id", settlementID),
			zap.String("user_id", memberID),
		).
		WithMethod("is_member_or_leader")
	l.Info("checking if user is member of settlement")

	filterAny := bson.M{
		"$or": bson.A{
			bson.M{"members.user_id": memberID},
			bson.M{"leader.user_id": memberID},
		},
	}
	count, err := r.setColl.CountDocuments(ctx, filterAny)
	if err != nil {
		l.Error("failed to check existing membership", zap.Error(err))
		return err
	}
	if count > 0 {
		l.Warn("user already in a settlement, cannot invite")
		return repoerr.ErrAlreadyMember
	}

	return nil
}

// IsLeaderOfSettlement checks if a user is already a leader of any settlement. Returns ErrNotLeader if the user is not a leader.
func (r *Repository) IsLeaderOfSettlement(ctx context.Context, settlementID, userID string) error {
	l := r.log.
		With(
			zap.String("settlement_id", settlementID),
			zap.String("user_id", userID),
		).
		WithMethod("is_leader_of_settlement")
	l.Info("checking if user is leader of settlement")

	setId, err := mongomodel.ParseObjectID(settlementID)
	if err != nil {
		l.Error("failed to parse settlement ID", zap.Error(err))
		return err
	}

	filter := bson.M{"_id": setId, "leader.user_id": userID}
	count, err := r.setColl.CountDocuments(ctx, filter)
	if err != nil {
		l.Error("failed to check existing leadership", zap.Error(err))
		return err
	}
	if count <= 0 {
		l.Warn("user already leader of settlement")
		return repoerr.ErrNotLeader
	}

	return nil
}
