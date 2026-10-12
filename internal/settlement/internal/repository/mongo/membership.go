package repository

import (
	"context"
	"errors"
	"slices"

	mongomodel "github.com/lasthearth/vsservice/internal/pkg/mongox"
	repoerr "github.com/lasthearth/vsservice/internal/settlement/internal/ierror"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

// membershipDoc is the slice of a settlement document the membership rules read.
type membershipDoc struct {
	Kind    string `bson:"kind"`
	Members []struct {
		UserID  string   `bson:"user_id"`
		RoleIds []string `bson:"role_ids"`
	} `bson:"members"`
}

// Memberships reads what userID holds across both kinds. The two per-kind unique indexes bound the
// answer to one settlement and one guild.
//
// Both kinds are named in one $or so the query runs against those two partial indexes instead of
// scanning the collection: a partial index serves only a query that carries its filter condition,
// and this one runs on every path that adds a member.
func (r *Repository) Memberships(ctx context.Context, userID string) (model.Memberships, error) {
	l := r.log.WithMethod("memberships").With(zap.String("user_id", userID))

	filter := bson.M{"$or": bson.A{
		bson.M{"members.user_id": userID, "kind": string(model.KindSettlement)},
		bson.M{"members.user_id": userID, "kind": string(model.KindGuild)},
	}}
	cursor, err := r.setColl.Find(ctx, filter, options.Find().SetProjection(bson.M{
		"kind":             1,
		"members.user_id":  1,
		"members.role_ids": 1,
	}))
	if err != nil {
		l.Error("failed to read memberships", zap.Error(err))
		return model.Memberships{}, err
	}
	defer func() {
		if err := cursor.Close(ctx); err != nil {
			l.Error("cursor close failed", zap.Error(err))
		}
	}()

	var docs []membershipDoc
	if err := cursor.All(ctx, &docs); err != nil {
		l.Error("failed to decode memberships", zap.Error(err))
		return model.Memberships{}, err
	}

	var inSettlement, inGuild, ownsGuild bool
	for _, d := range docs {
		// The filter returns only the two stored kinds, and SettlementType.Kind derives anything
		// else to a settlement, so this branch agrees with the indexes that bounded the result.
		if model.SettlementKind(d.Kind) == model.KindGuild {
			inGuild = true
			for _, m := range d.Members {
				if m.UserID == userID && slices.Contains(m.RoleIds, model.OwnerRoleId) {
					ownsGuild = true
				}
			}
			continue
		}
		inSettlement = true
	}

	return model.Memberships{
		InSettlement: inSettlement,
		InGuild:      inGuild,
		OwnsGuild:    ownsGuild,
	}, nil
}

// EnsureMayJoin implements service.SettlementDbRepository: it reads what userID holds and applies
// model.Memberships.MayJoin to it. The rule lives on the model; this is the read that feeds it.
//
// Every path that adds a member goes through it, including the ones inside this package, so an
// invitation or a join request written days earlier is checked again at the moment it is accepted.
func (r *Repository) EnsureMayJoin(ctx context.Context, userID string, kind model.SettlementKind) error {
	m, err := r.Memberships(ctx, userID)
	if err != nil {
		return err
	}
	return membershipErr(m.MayJoin(kind))
}

// EnsureMayOwnGuild implements service.SettlementDbRepository: it applies
// model.Memberships.MayOwnGuild to what userID holds. Founding a guild, taking its ownership and
// being granted it all go through here.
func (r *Repository) EnsureMayOwnGuild(ctx context.Context, userID string) error {
	m, err := r.Memberships(ctx, userID)
	if err != nil {
		return err
	}
	return membershipErr(m.MayOwnGuild())
}

// kindOf reads the kind of one group. It projects two fields, because the paths that add a member
// need the kind before they touch the document and never need the rest of it.
func (r *Repository) kindOf(ctx context.Context, settlementID string) (model.SettlementKind, error) {
	oid, err := mongomodel.ParseObjectID(settlementID)
	if err != nil {
		return "", repoerr.ErrNotFound
	}

	var doc struct {
		Kind string `bson:"kind"`
		Type string `bson:"type"`
	}
	if err := r.setColl.FindOne(ctx, bson.M{"_id": oid},
		options.FindOne().SetProjection(bson.M{"kind": 1, "type": 1})).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return "", repoerr.ErrNotFound
		}
		return "", err
	}
	if doc.Kind != "" {
		return model.SettlementKind(doc.Kind), nil
	}
	// A document written before the kind was stored answers from its type, which is what the kind is
	// derived from. Migration 2 backfills the field, so this only covers a document an older build
	// wrote while the new one was starting.
	return model.SettlementType(doc.Type).Kind(), nil
}

// membershipErr turns a model membership rule into the domain error the interceptor maps to a gRPC
// code. A nil rule error stays nil.
func membershipErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, model.ErrAlreadyInSettlement):
		return repoerr.ErrAlreadyMember
	case errors.Is(err, model.ErrAlreadyInGuild):
		return repoerr.ErrAlreadyInGuild
	case errors.Is(err, model.ErrGuildOwnerInSettlement):
		return repoerr.ErrGuildOwnerInSettlement
	default:
		return err
	}
}

// duplicateMemberErr names the conflict one of the two per-kind unique indexes just reported. The
// membership check ran a moment earlier, so this answers only for a player who joined another group
// of the same kind in between.
func duplicateMemberErr(kind model.SettlementKind) error {
	if kind == model.KindGuild {
		return repoerr.ErrAlreadyInGuild
	}
	return repoerr.ErrAlreadyMember
}
