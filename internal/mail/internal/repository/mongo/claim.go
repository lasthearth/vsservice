package repository

import (
	"context"
	"errors"

	dto "github.com/lasthearth/vsservice/internal/mail/internal/dto/mongo"
	"github.com/lasthearth/vsservice/internal/mail/internal/ierror"
	"github.com/lasthearth/vsservice/internal/mail/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
	mgo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/zap"
)

// InsertClaim writes the claim row. The unique (mail_id, player_id) index makes
// this the claimed-before-grant anchor: a duplicate means a row already exists,
// reported as created=false rather than an error.
func (r *Repository) InsertClaim(ctx context.Context, claim *model.MailClaim) (bool, error) {
	d := claimToDTO(claim)
	d.Model = mongox.NewModel()

	result, err := r.claimColl.InsertOne(ctx, d)
	if err != nil {
		if mgo.IsDuplicateKeyError(err) {
			return false, nil
		}
		return false, err
	}

	oid, err := mongox.ParseAnyObjectID(result.InsertedID)
	if err != nil {
		return false, err
	}
	claim.MarkCreated(oid.Hex())
	return true, nil
}

func (r *Repository) GetClaim(ctx context.Context, mailID, playerID string) (*model.MailClaim, error) {
	var d dto.MailClaim
	err := r.claimColl.FindOne(ctx, bson.M{"mail_id": mailID, "player_id": playerID}).Decode(&d)
	if err != nil {
		if errors.Is(err, mgo.ErrNoDocuments) {
			return nil, ierror.ErrNotFound
		}
		return nil, err
	}
	return claimFromDTO(d), nil
}

func (r *Repository) UpdateClaim(
	ctx context.Context,
	mailID, playerID string,
	updateFn func(ctx context.Context, c *model.MailClaim) (*model.MailClaim, error),
) (*model.MailClaim, error) {
	return mongox.UpdateDoc(
		ctx,
		r.claimColl,
		bson.M{"mail_id": mailID, "player_id": playerID},
		ierror.ErrNotFound,
		claimFromDTO,
		claimToDTOPtr,
		updateFn,
	)
}

func (r *Repository) ListClaimsForPlayer(ctx context.Context, playerID string) ([]*model.MailClaim, error) {
	l := r.log.With(zap.String("method", "ListClaimsForPlayer"), zap.String("player_id", playerID))

	cursor, err := r.claimColl.Find(ctx, bson.M{"player_id": playerID})
	if err != nil {
		l.Error("failed to find claims", zap.Error(err))
		return nil, err
	}
	defer func() {
		if err := cursor.Close(ctx); err != nil {
			l.Error("cursor close failed", zap.Error(err))
		}
	}()

	var dtos []dto.MailClaim
	if err := cursor.All(ctx, &dtos); err != nil {
		l.Error("failed to decode claims", zap.Error(err))
		return nil, err
	}

	result := make([]*model.MailClaim, len(dtos))
	for i, d := range dtos {
		result[i] = claimFromDTO(d)
	}
	return result, nil
}

func claimFromDTO(d dto.MailClaim) *model.MailClaim {
	return model.ReconstituteMailClaim(
		d.Model.Id.Hex(), d.MailID, d.PlayerID, model.MailState(d.State), d.ReadAt, d.ClaimedAt,
	)
}

// claimToDTO builds a BSON-ready MailClaim DTO. The mongox.Model envelope is
// owned by the caller, not by this conversion.
func claimToDTO(c *model.MailClaim) dto.MailClaim {
	return dto.MailClaim{
		MailID:    c.MailID,
		PlayerID:  c.PlayerID,
		State:     string(c.State),
		ReadAt:    c.ReadAt,
		ClaimedAt: c.ClaimedAt,
	}
}

func claimToDTOPtr(c *model.MailClaim) dto.MailClaim { return claimToDTO(c) }
