package repository

import (
	"context"
	"errors"
	"time"

	mongomodel "github.com/lasthearth/vsservice/internal/pkg/mongox"
	joinrequestdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/joinrequest"
	memberdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/member"
	repoerr "github.com/lasthearth/vsservice/internal/settlement/internal/ierror"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
)

// CountUserJoinRequests counts the open applications of one kind. The kind is stored on the
// request, so the count needs no join with the groups.
func (r *Repository) CountUserJoinRequests(ctx context.Context, userID string, kind model.SettlementKind) (int64, error) {
	count, err := r.setJoinReqColl.CountDocuments(ctx, bson.M{"user_id": userID, "kind": string(kind)})
	if err != nil {
		r.log.Error("failed to count user join requests", zap.Error(err), zap.String("user_id", userID))
		return 0, err
	}
	return count, nil
}

func (r *Repository) CreateJoinRequest(ctx context.Context, settlementID, userID string) error {
	l := r.log.
		With(zap.String("settlement_id", settlementID), zap.String("user_id", userID)).
		WithMethod("create_join_request")

	// The kind is copied onto the request so a later join can clear the applications of this kind
	// only. A group never changes kind, so the copy cannot go stale.
	kind, err := r.kindOf(ctx, settlementID)
	if err != nil {
		return err
	}

	dto := joinrequestdto.JoinRequest{
		Id:           bson.NewObjectIDFromTimestamp(time.Now()),
		UserId:       userID,
		SettlementId: settlementID,
		Kind:         string(kind),
	}
	if _, err := r.setJoinReqColl.InsertOne(ctx, dto); err != nil {
		// The unique (user_id, settlement_id) index fires when the same player applies to the same
		// group twice. It says nothing about membership, so it must not answer as a membership error.
		if mongo.IsDuplicateKeyError(err) {
			l.Info("an application to this group already stands")
			return repoerr.ErrJoinRequestExists
		}
		l.Error("failed to create join request", zap.Error(err))
		return err
	}
	return nil
}

func (r *Repository) getJoinRequest(ctx context.Context, joinRequestID string) (*joinrequestdto.JoinRequest, error) {
	oid, err := mongomodel.ParseObjectID(joinRequestID)
	if err != nil {
		return nil, repoerr.ErrNotFound
	}
	res := r.setJoinReqColl.FindOne(ctx, bson.M{"_id": oid})
	if err := res.Err(); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, repoerr.ErrNotFound
		}
		return nil, err
	}
	var dto joinrequestdto.JoinRequest
	if err := res.Decode(&dto); err != nil {
		return nil, err
	}
	return &dto, nil
}

func (r *Repository) GetJoinRequest(ctx context.Context, joinRequestID string) (*model.JoinRequest, error) {
	dto, err := r.getJoinRequest(ctx, joinRequestID)
	if err != nil {
		return nil, err
	}
	return dto.ToModel(), nil
}

func (r *Repository) GetJoinRequests(ctx context.Context, settlementID string) ([]model.JoinRequest, error) {
	cursor, err := r.setJoinReqColl.Find(ctx, bson.M{"settlement_id": settlementID})
	if err != nil {
		r.log.Error("failed to list join requests", zap.Error(err))
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	var dtos []joinrequestdto.JoinRequest
	if err := cursor.All(ctx, &dtos); err != nil {
		return nil, err
	}
	out := make([]model.JoinRequest, len(dtos))
	for i := range dtos {
		out[i] = *dtos[i].ToModel()
	}
	return out, nil
}

func (r *Repository) GetUserJoinRequests(ctx context.Context, userID string) ([]model.JoinRequest, error) {
	cursor, err := r.setJoinReqColl.Find(ctx, bson.M{"user_id": userID})
	if err != nil {
		r.log.Error("failed to list user join requests", zap.Error(err))
		return nil, err
	}
	defer func() { _ = cursor.Close(ctx) }()

	var dtos []joinrequestdto.JoinRequest
	if err := cursor.All(ctx, &dtos); err != nil {
		return nil, err
	}
	out := make([]model.JoinRequest, len(dtos))
	for i := range dtos {
		out[i] = *dtos[i].ToModel()
	}
	return out, nil
}

func (r *Repository) DeleteJoinRequestForUser(ctx context.Context, joinRequestID, userID string) error {
	oid, err := mongomodel.ParseObjectID(joinRequestID)
	if err != nil {
		return repoerr.ErrNotFound
	}
	res, err := r.setJoinReqColl.DeleteOne(ctx, bson.M{"_id": oid, "user_id": userID})
	if err != nil {
		r.log.Error("failed to delete join request", zap.Error(err))
		return err
	}
	if res.DeletedCount == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

// DeleteJoinRequest removes a join request scoped to a settlement (reviewer action).
func (r *Repository) DeleteJoinRequest(ctx context.Context, joinRequestID, settlementID string) error {
	oid, err := mongomodel.ParseObjectID(joinRequestID)
	if err != nil {
		return repoerr.ErrNotFound
	}
	res, err := r.setJoinReqColl.DeleteOne(ctx, bson.M{"_id": oid, "settlement_id": settlementID})
	if err != nil {
		r.log.Error("failed to delete join request", zap.Error(err))
		return err
	}
	if res.DeletedCount == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

// ApproveJoinRequest adds the applicant as a member and deletes the applicant's pending requests of
// that kind, in one transaction. It re-checks the membership rules and answers ErrAlreadyMember or
// ErrGuildOwnerInSettlement when the applicant joined elsewhere, or founded a guild, in between.
func (r *Repository) ApproveJoinRequest(ctx context.Context, joinRequestID string) (string, error) {
	l := r.log.With(zap.String("join_request_id", joinRequestID)).WithMethod("approve_join_request")

	session, err := r.client.StartSession()
	if err != nil {
		l.Error("failed to start session", zap.Error(err))
		return "", err
	}
	defer session.EndSession(ctx)

	var userID string
	err = mongo.WithSession(ctx, session, func(ctx context.Context) error {
		jr, err := r.getJoinRequest(ctx, joinRequestID)
		if err != nil {
			return err
		}
		userID = jr.UserId

		// Re-check the invariant: the applicant must not have joined another group of this kind, and
		// a guild owner must not land in a settlement, between application and approval.
		kind, err := r.kindOf(ctx, jr.SettlementId)
		if err != nil {
			return err
		}
		if err := r.EnsureMayJoin(ctx, jr.UserId, kind); err != nil {
			return err
		}

		sid, err := mongomodel.ParseObjectID(jr.SettlementId)
		if err != nil {
			return repoerr.ErrNotFound
		}
		member := memberdto.Member{UserId: jr.UserId, RoleIds: []string{}}
		res, err := r.setColl.UpdateOne(ctx, bson.M{"_id": sid},
			bson.D{
				{Key: "$push", Value: bson.D{{Key: "members", Value: member}}},
				{Key: "$set", Value: bson.D{{Key: "updated_at", Value: time.Now()}}},
				// Bump the version, the way pushMember does, so a concurrent whole-document
				// UpdateSettlement cannot write back a snapshot without this member.
				{Key: "$inc", Value: bson.D{{Key: "version", Value: 1}}},
			},
		)
		if err != nil {
			if mongo.IsDuplicateKeyError(err) {
				// The server's answer names the index, which is what tells a same-kind conflict from
				// a legacy index that outlived its migration.
				l.Warn("unique index refused the member", zap.String("kind", string(kind)), zap.Error(err))
				return duplicateMemberErr(kind)
			}
			l.Error("failed to add member", zap.Error(err))
			return err
		}
		if res.MatchedCount == 0 {
			return repoerr.ErrNotFound
		}

		// Drop this kind of pending request of this user (this one included).
		if err := r.clearJoinRequests(ctx, jr.UserId, kind); err != nil {
			l.Error("failed to clear user join requests", zap.Error(err))
			return err
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return userID, nil
}

// clearJoinRequests withdraws every application of one kind. The applications to the other kind
// stay: a player may hold a settlement and a guild at the same time, so joining one says nothing
// about the other.
func (r *Repository) clearJoinRequests(ctx context.Context, userID string, kind model.SettlementKind) error {
	_, err := r.setJoinReqColl.DeleteMany(ctx, bson.M{"user_id": userID, "kind": string(kind)})
	return err
}
