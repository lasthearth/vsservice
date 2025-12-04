package repository

import (
	"context"
	"time"

	"github.com/go-faster/errors"
	mongomodel "github.com/lasthearth/vsservice/internal/pkg/mongox"
	attachmentdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/attachment"
	memberdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/member"
	vector2dto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/vector2"
	verificationdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/verification"
	repoerr "github.com/lasthearth/vsservice/internal/settlement/internal/ierror"

	"github.com/lasthearth/vsservice/internal/settlement/internal/service"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"github.com/samber/lo"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
)

func (r *Repository) CreateRequest(ctx context.Context, opts service.SettlementOpts) error {
	r.log.Info("creating new settlement request",
		zap.String("leader_id", opts.Leader.UserId),
		zap.String("settlement_name", opts.Name),
		zap.String("settlement_type", string(opts.Type)))

	attachments := lo.Map(opts.Attachments, func(item model.Attachment, _ int) attachmentdto.Attachment {
		return *attachmentdto.FromModel(&item)
	})

	dto := verificationdto.SettlementVerification{
		Model:           mongomodel.NewModel(),
		Name:            opts.Name,
		Type:            string(opts.Type),
		Description:     opts.Description,
		Coordinates:     *vector2dto.FromModel(&opts.Coordinates),
		Leader:          memberdto.Member(opts.Leader),
		Attachments:     attachments,
		Diplomacy:       opts.Diplomacy,
		Status:          string(model.SettlementStatusPending),
		RejectionReason: "",
	}

	r.log.Debug("inserting settlement request into database",
		zap.String("leader_id", opts.Leader.UserId),
		zap.String("model_id", dto.Id.Hex()))

	_, err := r.setReqColl.InsertOne(ctx, dto)
	if err != nil {
		r.log.Error("failed to insert settlement request",
			zap.Error(err),
			zap.String("leader_id", opts.Leader.UserId),
			zap.String("model_id", dto.Id.Hex()))
		return err
	}

	r.log.Info("successfully created settlement request",
		zap.String("leader_id", opts.Leader.UserId),
		zap.String("model_id", dto.Id.Hex()))
	return nil
}

func (r *Repository) UpdateRequest(ctx context.Context, opts service.SettlementOpts) error {
	r.log.Info("updating settlement request",
		zap.String("leader_id", opts.Leader.UserId),
		zap.String("settlement_name", opts.Name),
		zap.String("settlement_type", string(opts.Type)))

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	attachments := lo.Map(opts.Attachments, func(item model.Attachment, _ int) attachmentdto.Attachment {
		return *attachmentdto.FromModel(&item)
	})

	updateFields := bson.D{
		{Key: "name", Value: opts.Name},
		{Key: "type", Value: string(opts.Type)},
		{Key: "leader", Value: memberdto.FromModel(&opts.Leader)},
		{Key: "coordinates", Value: vector2dto.FromModel(&opts.Coordinates)},
		{Key: "attachments", Value: attachments},
		{Key: "status", Value: string(model.SettlementStatusPending)},
		{Key: "rejection_reason", Value: ""},
		{Key: "updated_at", Value: time.Now()},
	}

	r.log.Debug("updating verification request in database",
		zap.String("leader_id", opts.Leader.UserId))

	result, err := r.setReqColl.UpdateOne(
		ctx,
		bson.M{"leader.user_id": opts.Leader.UserId},
		bson.D{
			{Key: "$set", Value: updateFields},
		},
	)
	if err != nil {
		r.log.Error("failed to update verification request",
			zap.Error(err),
			zap.String("user_id", opts.Leader.UserId))
		return err
	}

	r.log.Info("successfully updated verification request",
		zap.String("user_id", opts.Leader.UserId),
		zap.Int64("modified_count", result.ModifiedCount))
	return nil
}

// Approve implements service.SettlementDbRepository.
func (r *Repository) Approve(ctx context.Context, id string) error {
	l := r.log.
		With(zap.String("settlement_id", id)).
		WithMethod("approve")
	l.Info("approving settlement request")

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	objectID, err := mongomodel.ParseObjectID(id)
	if err != nil {
		l.Error("invalid settlement ID format", zap.Error(err))
		return err
	}

	l.Debug("executing update query")

	session, err := r.client.StartSession()
	if err != nil {
		l.Error("failed to start session", zap.Error(err))
		return err
	}

	defer session.EndSession(ctx)

	return mongo.WithSession(ctx, session, func(context.Context) error {
		found := r.setReqColl.FindOneAndUpdate(
			ctx,
			bson.M{"_id": objectID},
			bson.D{
				{
					Key: "$set",
					Value: bson.D{
						{Key: "status", Value: model.SettlementStatusApproved},
						{Key: "updated_at", Value: time.Now()},
					},
				},
			},
		)

		err = found.Err()
		if err != nil {
			if errors.Is(err, mongo.ErrNoDocuments) {
				l.Warn("settlement not found", zap.Error(err))
				return repoerr.ErrNotFound
			}

			l.Error("update error", zap.Error(err))
			return err
		}

		var dto verificationdto.SettlementVerification

		err = found.Decode(&dto)
		if err != nil {
			l.Error("decode error", zap.Error(err))
			return err
		}

		l.Info("successfully approved settlement request")
		l.Debug("leader here", zap.String("leader_id", dto.Leader.UserId))
		// check existence if exists update instead of create
		_, err := r.GetSettlement(ctx, dto.Id.Hex())
		if err != nil {
			if errors.Is(err, repoerr.ErrNotFound) {
				model := mongomodel.NewModel()
				model.Id = dto.Id

				cdto := r.mapper.FromVerification(dto)
				cdto.Members = make([]memberdto.Member, 0)
				return r.Create(ctx, cdto)
			}

			return err
		}

		setModel := dto.ToModel()
		return r.Update(ctx, service.UpdateSettlementOpts{
			ID:          id,
			Name:        dto.Name,
			Type:        setModel.Type,
			Coordinates: setModel.Coordinates,
			Attachments: setModel.Attachments,
			Leader:      *dto.Leader.ToModel(),
			Diplomacy:   dto.Diplomacy,
			Description: dto.Description,
		})
	})
}

// Reject implements service.SettlementDbRepository.
func (r *Repository) Reject(ctx context.Context, id string, rejectionReason string) error {
	l := r.log.
		With(zap.String("settlement_id", id)).
		WithMethod("reject")
	l.Info("rejecting settlement request")

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	objectID, err := mongomodel.ParseObjectID(id)
	if err != nil {
		l.Error("invalid settlement ID format", zap.Error(err))
		return err
	}

	l.Debug("executing found query")
	found := r.setReqColl.FindOne(ctx, bson.M{"_id": objectID})
	err = found.Err()
	if err != nil {
		l.Error("find error", zap.Error(err))
		return err
	}

	var dto verificationdto.SettlementVerification

	l.Debug("decoding founded")
	err = found.Decode(&dto)
	if err != nil {
		l.Error("decode error", zap.Error(err))
		return err
	}

	var status model.SettlementStatus
	if dto.Type != string(model.SettlementTypeVillage) {
		status = model.SettlementStatusUpdateRejected
	} else {
		status = model.SettlementStatusRejected
	}

	l.Debug("executing update query")
	updated, err := r.setReqColl.UpdateOne(ctx, bson.M{"_id": objectID},
		bson.D{
			{
				Key: "$set",
				Value: bson.D{
					{Key: "status", Value: status},
					{Key: "rejection_reason", Value: rejectionReason},
					{Key: "updated_at", Value: time.Now()},
				},
			},
		})
	if err != nil {
		l.Error("update error", zap.Error(err))
		return err
	}

	l.Info(
		"successfully rejected settlement request",
		zap.Int64("updated_count", updated.ModifiedCount),
	)
	return nil
}

// GetSettlementRequest implements service.SettlementRepository.
func (r *Repository) GetSettlementRequest(ctx context.Context, id string) (*model.SettlementVerification, error) {
	r.log.Info("retrieving settlement request by ID", zap.String("req_id", id))

	objectID, err := mongomodel.ParseObjectID(id)
	if err != nil {
		r.log.Error("invalid settlement req ID format", zap.Error(err), zap.String("settlement_id", id))
		return nil, err
	}

	res := r.setReqColl.FindOne(ctx, bson.M{"_id": objectID})
	if res.Err() != nil {
		if errors.Is(res.Err(), mongo.ErrNoDocuments) {
			return nil, repoerr.ErrNotFound
		}

		r.log.Error("failed to find settlement request", zap.Error(res.Err()), zap.String("req_id", id))
		return nil, res.Err()
	}

	var settlement verificationdto.SettlementVerification
	if err := res.Decode(&settlement); err != nil {
		r.log.Error("failed to decode settlement request", zap.Error(err), zap.String("settlement_id", id))
		return nil, err
	}

	r.log.Debug("settlement request retrieved", zap.String("req_id", id))
	return settlement.ToModel(), nil
}

// GetSettlementRequestByLeader implements service.SettlementRepository.
func (r *Repository) GetSettlementRequestByLeader(ctx context.Context, leaderID string) (*model.SettlementVerification, error) {
	r.log.Info("retrieving settlement request by leaderID", zap.String("leader_id", leaderID))

	res := r.setReqColl.FindOne(ctx, bson.M{"leader.user_id": leaderID})
	if res.Err() != nil {
		if errors.Is(res.Err(), mongo.ErrNoDocuments) {
			return nil, repoerr.ErrNotFound
		}

		r.log.Error("failed to find settlement request", zap.Error(res.Err()), zap.String("leader_id", leaderID))
		return nil, res.Err()
	}

	var settlement verificationdto.SettlementVerification
	if err := res.Decode(&settlement); err != nil {
		r.log.Error("failed to decode settlement request", zap.Error(err), zap.String("leader_id", leaderID))
		return nil, err
	}

	r.log.Debug("settlement request retrieved", zap.String("leader_id", leaderID))
	return settlement.ToModel(), nil
}

// GetPendingSettlements implements service.SettlementDbRepository.
func (r *Repository) GetPendingSettlements(ctx context.Context) ([]model.SettlementVerification, error) {
	r.log.Info("retrieving pending settlement requests")

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	r.log.Debug("executing find query on settlement collection for pending status")
	found, err := r.setReqColl.Find(ctx, bson.M{"status": model.SettlementStatusPending})
	if err != nil {
		r.log.Error("find error", zap.Error(err))
		return nil, err
	}
	defer found.Close(ctx)

	var settlements []verificationdto.SettlementVerification
	if err := found.All(ctx, &settlements); err != nil {
		r.log.Error("cursor error", zap.Error(err))
		return nil, err
	}

	res := lo.Map(settlements, func(item verificationdto.SettlementVerification, index int) model.SettlementVerification {
		return *item.ToModel()
	})

	r.log.Info("successfully retrieved pending settlements", zap.Int("count", len(res)))
	return res, nil
}
