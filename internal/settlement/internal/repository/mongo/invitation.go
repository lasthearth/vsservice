package repository

import (
	"context"
	"errors"
	"time"

	mongomodel "github.com/lasthearth/vsservice/internal/pkg/mongox"
	invitationdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/invitation"
	memberdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/member"
	"github.com/lasthearth/vsservice/internal/settlement/internal/repository/mongo/repoerr"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
)

func (r *Repository) GetUserInvitations(ctx context.Context, userID string) ([]model.Invitation, error) {
	l := r.log.
		With(
			zap.String("user_id", userID),
		).
		WithMethod("get_user_invitations")

	l.Info("getting user invitations")

	filter := bson.M{"user_id": userID}
	cursor, err := r.setInvColl.Find(ctx, filter)
	if err != nil {
		l.Error("failed to get user invitations", zap.Error(err))
		return nil, err
	}
	defer cursor.Close(ctx)

	var invitations []invitationdto.Invitation
	if err := cursor.All(ctx, &invitations); err != nil {
		l.Error("failed to get user invitations", zap.Error(err))
		return nil, err
	}

	l.Info("user invitations retrieved successfully")

	return r.mapper.ToInvModels(invitations), nil
}

func (r *Repository) AcceptInvitation(ctx context.Context, invitationID, userID string) error {
	l := r.log.
		With(
			zap.String("invitation_id", invitationID),
		).
		WithMethod("accept_invitation")

	l.Info("accepting invitation")

	session, err := r.client.StartSession()
	if err != nil {
		l.Error("failed to start session", zap.Error(err))
		return err
	}

	defer session.EndSession(ctx)

	err = mongo.WithSession(ctx, session, func(ctx context.Context) error {
		inv, err := r.getInvitation(ctx, invitationID)
		if err != nil {
			l.Error("failed to get invitation", zap.Error(err))
			return err
		}

		if userID != inv.UserId {
			return repoerr.ErrPermissionDenied
		}

		member := memberdto.Member{
			UserId: inv.UserId,
		}

		sid, err := mongomodel.ParseObjectID(inv.SettlementId)
		if err != nil {
			return err
		}

		upd, err := r.setColl.UpdateOne(
			ctx,
			bson.M{"_id": sid},
			bson.D{
				{
					Key: "$push",
					Value: bson.D{
						{Key: "members", Value: member},
					},
				},
				{
					Key: "$set",
					Value: bson.D{
						{Key: "updated_at", Value: time.Now()},
					},
				},
			},
		)
		if err != nil {
			l.Error("failed to update settlement", zap.Error(err))
			return err
		}

		err = r.DeleteInvitation(ctx, invitationID)
		if err != nil {
			l.Error("failed to delete invitation", zap.Error(err))
			return err
		}

		l.Info(
			"invitation accepted successfully",
			zap.Int64("modified_settlement", upd.ModifiedCount),
		)

		return nil
	})

	return err
}

func (r *Repository) GetInvitations(ctx context.Context, settlementID string) ([]model.Invitation, error) {
	l := r.log.
		With(
			zap.String("settlement_id", settlementID),
		).
		WithMethod("get_invitations")

	l.Info("getting invitations")

	filter := bson.M{"settlement_id": settlementID}
	cursor, err := r.setInvColl.Find(ctx, filter)
	if err != nil {
		l.Error("failed to get invitations", zap.Error(err))
		return nil, err
	}
	defer cursor.Close(ctx)

	var invitations []invitationdto.Invitation
	if err := cursor.All(ctx, &invitations); err != nil {
		l.Error("failed to get invitations", zap.Error(err))
		return nil, err
	}

	l.Info("invitations retrieved successfully")

	return r.mapper.ToInvModels(invitations), nil
}

// CreateInvitation implements service.SettlementDbRepository.
func (r *Repository) CreateInvitation(ctx context.Context, settlementID, userID string) error {
	l := r.log.
		With(
			zap.String("settlement_id", settlementID),
			zap.String("user_id", userID),
		).
		WithMethod("create_invitation")

	l.Info("creating invitation")
	dto := invitationdto.Invitation{
		Id:           bson.NewObjectIDFromTimestamp(time.Now()),
		UserId:       userID,
		SettlementId: settlementID,
	}

	_, err := r.setInvColl.InsertOne(ctx, dto)
	if err != nil {
		l.Error("failed to create invitation", zap.Error(err))
		return err
	}

	l.Info("invitation created successfully")
	return nil
}

// RemoveMember implements service.SettlementDbRepository.
func (r *Repository) RemoveMember(ctx context.Context, settlementID, userID string) error {
	l := r.log.
		With(
			zap.String("settlement_id", settlementID),
			zap.String("user_id", userID)).
		WithMethod("remove_member")

	l.Info("removing member from settlement")

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	objectID, err := mongomodel.ParseObjectID(settlementID)
	if err != nil {
		l.Error("invalid settlement ID format", zap.Error(err))
		return err
	}

	l.Debug("executing update query to remove member")

	result, err := r.setColl.UpdateOne(
		ctx,
		bson.M{"_id": objectID},
		bson.D{
			{
				Key: "$pull",
				Value: bson.D{
					{Key: "members", Value: bson.M{"user_id": userID}},
				},
			},
			{
				Key: "$set",
				Value: bson.D{
					{Key: "updated_at", Value: time.Now()},
				},
			},
		},
	)
	if err != nil {
		l.Error("update error", zap.Error(err))
		return err
	}

	l.Info(
		"successfully removed member from settlement",
		zap.Int64(
			"modified_count",
			result.ModifiedCount,
		),
	)
	return nil
}

func (r *Repository) getInvitation(ctx context.Context, invitationID string) (*model.Invitation, error) {
	l := r.log.
		With(
			zap.String("invitation_id", invitationID),
		).
		WithMethod("get_invitation")

	l.Info("getting invitation")

	oid, err := mongomodel.ParseObjectID(invitationID)
	if err != nil {
		return nil, err
	}

	filter := bson.M{"_id": oid}
	finded := r.setInvColl.FindOne(ctx, filter)
	if err := finded.Err(); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			l.Error("invitation not found", zap.Error(err))
			return nil, repoerr.ErrInvitationNotFound
		}
		l.Error("failed to find invitation", zap.Error(err))
		return nil, err
	}

	var dto invitationdto.Invitation
	if err := finded.Decode(&dto); err != nil {
		l.Error("failed to decode invitation", zap.Error(err))
		return nil, err
	}

	res := r.mapper.ToInvModel(dto)

	l.Info("invitation retrieved successfully")

	return &res, nil
}

func (r *Repository) DeleteInvitationForUser(ctx context.Context, invitationID, userID string) error {
	l := r.log.
		With(
			zap.String("invitation_id", invitationID),
		).
		WithMethod("delete_invitation")

	l.Info("deleting invitation")

	oid, err := mongomodel.ParseObjectID(invitationID)
	if err != nil {
		return err
	}

	filter := bson.M{"_id": oid, "user_id": userID}
	if _, err := r.setInvColl.DeleteOne(ctx, filter); err != nil {
		l.Error("failed to delete invitation", zap.Error(err))
		return err
	}

	l.Info("invitation deleted successfully")

	return nil
}

func (r *Repository) DeleteInvitationForLeader(ctx context.Context, invitationID, settlementID string) error {
	l := r.log.
		With(
			zap.String("invitation_id", invitationID),
		).
		WithMethod("delete_invitation")

	l.Info("deleting invitation")

	oid, err := mongomodel.ParseObjectID(invitationID)
	if err != nil {
		return err
	}

	filter := bson.M{"_id": oid, "settlement_id": settlementID}
	if _, err := r.setInvColl.DeleteOne(ctx, filter); err != nil {
		l.Error("failed to delete invitation", zap.Error(err))
		return err
	}

	l.Info("invitation deleted successfully")

	return nil
}

func (r *Repository) DeleteInvitation(ctx context.Context, invitationID string) error {
	l := r.log.
		With(
			zap.String("invitation_id", invitationID),
		).
		WithMethod("delete_invitation")

	l.Info("deleting invitation")

	oid, err := mongomodel.ParseObjectID(invitationID)
	if err != nil {
		return err
	}

	filter := bson.M{"_id": oid}
	if _, err := r.setInvColl.DeleteOne(ctx, filter); err != nil {
		l.Error("failed to delete invitation", zap.Error(err))
		return err
	}

	l.Info("invitation deleted successfully")

	return nil
}
