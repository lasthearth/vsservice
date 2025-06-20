package repository

import (
	"context"
	"time"

	mongomodel "github.com/lasthearth/vsservice/internal/pkg/mongo"
	invitationdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/invitation"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/zap"
)

// func (r *Repository) InviteMember(ctx context.Context, settlementID string, member model.Member) error { l := r.log.
// 		With(zap.String("settlement_id", settlementID), zap.String("user_id", member.UserID)).
// 		WithMethod("invite_member")

// 	l.Info("inviting member to settlement")

// 	objectID, err := mongomodel.ParseObjectID(settlementID)
// 	if err != nil {
// 		return err
// 	}

// 	l.Debug("checking existing membership")
// 	err = r.IsMemberOrLeader(ctx, settlementID, member.UserID)
// 	if err != nil {
// 		return err
// 	}

// 	dtoMember := memberdto.FromModel(&member)
// 	update := bson.D{
// 		{Key: "$push", Value: bson.D{{Key: "members", Value: dtoMember}}},
// 		{Key: "$set", Value: bson.D{{Key: "updated_at", Value: time.Now()}}},
// 	}
// 	res, err := r.setInvColl.UpdateOne(ctx,
// 		bson.M{"_id": objectID},
// 		update,
// 	)
// 	if err != nil {
// 		l.Error("failed to invite member", zap.Error(err))
// 		return err
// 	}
// 	if res.MatchedCount == 0 {
// 		return ErrNotFound
// 	}

//		l.Info("member invited successfully")
//		return nil
//	}
//

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

// InviteMember implements service.SettlementDbRepository.
func (r *Repository) InviteMember(ctx context.Context, settlementID, userID string) error {
	l := r.log.
		With(
			zap.String("settlement_id", settlementID),
			zap.String("user_id", userID),
		).
		WithMethod("invite_member")

	l.Info("inviting member to settlement")

	err := r.IsMemberOrLeader(ctx, settlementID, userID)
	if err != nil {
		return err
	}

	dto := invitationdto.Invitation{
		Id:           bson.NewObjectIDFromTimestamp(time.Now()),
		UserId:       userID,
		SettlementId: settlementID,
	}

	_, err = r.setInvColl.InsertOne(ctx, dto)
	if err != nil {
		l.Error("failed to invite member", zap.Error(err))
		return err
	}

	l.Info("member invited successfully")
	return nil
}

// InviteMember implements service.SettlementDbRepository.
func (r *Repository) RevokeInvitation(ctx context.Context, invID string) error {
	l := r.log.
		With(
			zap.String("invitation_id", invID),
		).
		WithMethod("revoke_invitation")

	l.Info("revoking invitation")

	filter := bson.M{"_id": invID}
	_, err := r.setInvColl.DeleteOne(ctx, filter)
	if err != nil {
		l.Error("failed to revoke invitation", zap.Error(err))
		return err
	}

	l.Info("invitation revoked successfully")
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

	l.Info("successfully removed member from settlement",
		zap.Int64("modified_count", result.ModifiedCount))
	return nil
}
