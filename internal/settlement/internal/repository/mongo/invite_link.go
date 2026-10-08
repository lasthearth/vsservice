package repository

import (
	"context"
	"errors"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	invitelinkdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/invitelink"
	memberdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/member"
	repoerr "github.com/lasthearth/vsservice/internal/settlement/internal/ierror"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

// inviteLinksListLimit bounds the leader's list of links.
const inviteLinksListLimit = 50

// CreateInviteLink stores a new invite link.
func (r *Repository) CreateInviteLink(ctx context.Context, link *model.InviteLink) (*model.InviteLink, error) {
	l := r.log.WithMethod("CreateInviteLink").With(zap.String("settlement_id", link.SettlementId))

	doc := r.mapper.FromInviteLinkModel(*link)
	doc.Model = mongox.NewModel()

	if _, err := r.inviteLinkColl.InsertOne(ctx, doc); err != nil {
		l.Error("failed to insert invite link", zap.Error(err))
		return nil, err
	}

	created := r.mapper.ToInviteLinkModel(doc)
	return &created, nil
}

// CountActiveInviteLinks counts the settlement's links that can still be used
// at now: not revoked, not expired and with uses left.
func (r *Repository) CountActiveInviteLinks(ctx context.Context, settlementID string, now time.Time) (int64, error) {
	filter := bson.M{
		"settlement_id": settlementID,
		"revoked_at":    bson.M{"$exists": false},
		"$and": bson.A{
			bson.M{"$or": bson.A{
				bson.M{"expires_at": bson.M{"$exists": false}},
				bson.M{"expires_at": bson.M{"$gt": now}},
			}},
			bson.M{"$or": bson.A{
				bson.M{"max_uses": 0},
				bson.M{"$expr": bson.M{"$lt": bson.A{"$uses", "$max_uses"}}},
			}},
		},
	}

	count, err := r.inviteLinkColl.CountDocuments(ctx, filter)
	if err != nil {
		r.log.WithMethod("CountActiveInviteLinks").Error("failed to count invite links", zap.Error(err))
		return 0, err
	}
	return count, nil
}

// ListInviteLinks returns the settlement's links that were not revoked, newest first.
func (r *Repository) ListInviteLinks(ctx context.Context, settlementID string) ([]model.InviteLink, error) {
	l := r.log.WithMethod("ListInviteLinks").With(zap.String("settlement_id", settlementID))

	filter := bson.M{"settlement_id": settlementID, "revoked_at": bson.M{"$exists": false}}
	opts := options.Find().SetSort(bson.D{{Key: "_id", Value: -1}}).SetLimit(inviteLinksListLimit)

	cursor, err := r.inviteLinkColl.Find(ctx, filter, opts)
	if err != nil {
		l.Error("failed to list invite links", zap.Error(err))
		return nil, err
	}

	var docs []invitelinkdto.InviteLink
	if err := cursor.All(ctx, &docs); err != nil {
		l.Error("failed to decode invite links", zap.Error(err))
		return nil, err
	}

	return r.mapper.ToInviteLinkModels(docs), nil
}

// GetInviteLinkByCode returns the link with this code, revoked or not.
func (r *Repository) GetInviteLinkByCode(ctx context.Context, code string) (*model.InviteLink, error) {
	doc, err := r.findInviteLink(ctx, r.inviteLinkColl, bson.M{"code": code})
	if err != nil {
		return nil, err
	}

	link := r.mapper.ToInviteLinkModel(*doc)
	return &link, nil
}

// UpdateInviteLink loads one of the settlement's links, lets updateFn change it
// through its methods and stores it with a version-guarded replace.
func (r *Repository) UpdateInviteLink(
	ctx context.Context,
	settlementID, linkID string,
	updateFn func(ctx context.Context, link *model.InviteLink) (*model.InviteLink, error),
) (*model.InviteLink, error) {
	oid, err := mongox.ParseObjectID(linkID)
	if err != nil {
		return nil, repoerr.ErrInviteLinkNotFound
	}

	updated, err := mongox.UpdateDoc(
		ctx,
		r.inviteLinkColl,
		bson.M{"_id": oid, "settlement_id": settlementID},
		repoerr.ErrInviteLinkNotFound,
		r.inviteLinkToModel,
		r.inviteLinkFromModel,
		updateFn,
	)
	if err != nil && !errors.Is(err, repoerr.ErrInviteLinkNotFound) {
		r.log.WithMethod("UpdateInviteLink").Error("failed to update invite link", zap.Error(err))
	}
	return updated, err
}

// JoinByInviteLink adds userID to the settlement behind the link.
//
// The use is reserved first, with the same version-guarded replace as every
// other update (mongox.UpdateDoc): useFn spends it on the model, and two
// players racing for the last use cannot both get it. Only then is the member
// pushed; if that fails (the player joined elsewhere a moment ago, or the
// settlement is gone) the use is given back. The deployment has no replica set,
// so this order — reserve, act, compensate — replaces a transaction.
func (r *Repository) JoinByInviteLink(
	ctx context.Context,
	code, userID string,
	useFn func(ctx context.Context, link *model.InviteLink) (*model.InviteLink, error),
) (*model.InviteLink, error) {
	l := r.log.WithMethod("JoinByInviteLink").With(zap.String("user_id", userID))

	link, err := mongox.UpdateDoc(
		ctx,
		r.inviteLinkColl,
		bson.M{"code": code},
		repoerr.ErrInviteLinkNotFound,
		r.inviteLinkToModel,
		r.inviteLinkFromModel,
		func(ctx context.Context, link *model.InviteLink) (*model.InviteLink, error) {
			if err := r.IsMemberOfAnySettlement(ctx, userID); err != nil {
				return nil, err
			}
			return useFn(ctx, link)
		},
	)
	if err != nil {
		if !isExpectedJoinErr(err) {
			l.Error("failed to spend invite link use", zap.Error(err))
		}
		return nil, err
	}

	if err := r.pushMember(ctx, link.SettlementId, userID); err != nil {
		// The player did not get in: give the use back.
		//
		// The refund must not run on the caller's context. When pushMember failed
		// because that context was cancelled or hit its deadline, a refund on the
		// same context fails for the same reason and the use is burned
		// permanently — the failure is correlated by construction. Detach it and
		// give it its own short deadline.
		refundCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, rerr := r.inviteLinkColl.UpdateOne(refundCtx,
			bson.M{"code": code, "uses": bson.M{"$gt": 0}},
			bson.M{"$inc": bson.M{"uses": -1, "version": 1}},
		); rerr != nil {
			l.Error("failed to give an invite link use back", zap.Error(rerr))
		}
		if !isExpectedJoinErr(err) {
			l.Error("failed to add member by invite link", zap.Error(err))
		}
		return nil, err
	}

	// Pending applications elsewhere are moot now. Not fatal: the player is in.
	if _, err := r.setJoinReqColl.DeleteMany(ctx, bson.M{"user_id": userID}); err != nil {
		l.Warn("failed to clear join requests", zap.Error(err))
	}

	return link, nil
}

// pushMember adds a plain member. The unique members.user_id index turns a
// concurrent second membership into ErrAlreadyMember.
func (r *Repository) pushMember(ctx context.Context, settlementID, userID string) error {
	sid, err := mongox.ParseObjectID(settlementID)
	if err != nil {
		return repoerr.ErrNotFound
	}

	member := memberdto.Member{UserId: userID, RoleIds: []string{}}
	res, err := r.setColl.UpdateOne(ctx, bson.M{"_id": sid},
		bson.D{
			{Key: "$push", Value: bson.D{{Key: "members", Value: member}}},
			{Key: "$set", Value: bson.D{{Key: "updated_at", Value: time.Now()}}},
			// Bump version. UpdateSettlement replaces the WHOLE document under a
			// {version: N} guard (mongox.UpdateDoc), so a $push that leaves
			// version alone is invisible to it: a concurrent settlement edit
			// would still match the guard and write back its own snapshot of
			// members, silently erasing a player who was already told the join
			// succeeded and whose link use was already spent.
			{Key: "$inc", Value: bson.D{{Key: "version", Value: 1}}},
		},
	)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return repoerr.ErrAlreadyMember
		}
		return err
	}
	if res.MatchedCount == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

func (r *Repository) findInviteLink(ctx context.Context, coll *mongo.Collection, filter bson.M) (*invitelinkdto.InviteLink, error) {
	var doc invitelinkdto.InviteLink
	if err := coll.FindOne(ctx, filter).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, repoerr.ErrInviteLinkNotFound
		}
		return nil, err
	}
	return &doc, nil
}

func (r *Repository) inviteLinkToModel(d invitelinkdto.InviteLink) *model.InviteLink {
	m := r.mapper.ToInviteLinkModel(d)
	return &m
}

func (r *Repository) inviteLinkFromModel(m *model.InviteLink) invitelinkdto.InviteLink {
	return r.mapper.FromInviteLinkModel(*m)
}

// isExpectedJoinErr reports business outcomes that are not worth an error log.
func isExpectedJoinErr(err error) bool {
	return errors.Is(err, repoerr.ErrInviteLinkNotFound) ||
		errors.Is(err, repoerr.ErrAlreadyMember) ||
		errors.Is(err, repoerr.ErrNotFound) ||
		errors.Is(err, repoerr.ErrInviteLinkExpired) ||
		errors.Is(err, repoerr.ErrInviteLinkExhausted) ||
		errors.Is(err, repoerr.ErrInviteLinkRevoked)
}
