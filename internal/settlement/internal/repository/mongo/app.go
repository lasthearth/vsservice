//go:generate go tool goverter gen github.com/lasthearth/vsservice/internal/settlement/internal/repository/mongo
package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/logger"
	invitationdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/invitation"
	invitelinkdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/invitelink"
	notifierdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/notifier"
	settlementdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/settlement"
	verificationdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/verification"
	"github.com/lasthearth/vsservice/internal/settlement/internal/service"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const (
	settlementCollName            = "settlements"
	settlementReqCollName         = "settlement_requests"
	settlementInvitationCollName  = "settlement_invitations"
	settlementJoinRequestCollName = "settlement_join_requests"
	imperialFavorLogCollName      = "imperial_favor_logs"
	settlementInviteLinkCollName  = "settlement_invite_links"
	// settlementNotifierCollName is written by the game server; vsservice only
	// reads it and creates its unique index.
	settlementNotifierCollName = "settlement_notifiers"
)

var _ service.SettlementRepository = (*Repository)(nil)

// goverter:converter
// goverter:output:file repomapper/mapper.go
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:ObjectIdToString
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:ObjectIdToObjectId
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTime
type Mapper interface {
	FromInvModels([]model.Invitation) []invitationdto.Invitation
	// goverter:ignore Id
	FromInvModel(model.Invitation) invitationdto.Invitation
	ToInvModels(dto []invitationdto.Invitation) []model.Invitation
	ToInvModel(dto invitationdto.Invitation) model.Invitation

	// NotifierReissues is mapped: the request keeps a copy of the counter so a
	// settlement re-created under the same id continues where the deleted one
	// left off.
	// Kind comes across on its own: both documents carry it, and it is the same
	// value because approval copies the request's type onto the settlement.
	// goverter:ignore Members TagIds ImperialFavor Roles RolesEnabled ContactInfo
	FromVerification(dto verificationdto.SettlementVerification) settlementdto.Settlement

	FromSettlementsDTO([]settlementdto.Settlement) []model.Settlement

	ToNotifierPlacement(dto notifierdto.Notifier) model.NotifierPlacement

	// goverter:autoMap Model
	FromSettlementDTO(dto settlementdto.Settlement) model.Settlement

	// Kind is stamped by the repository after the mapping, from the model's type: see
	// Repository.toSettlementDTO. The model carries no copy of it, so there is nothing to map.
	// goverter:ignore Model Kind
	ToSettlementDTO(model.Settlement) settlementdto.Settlement

	// goverter:autoMap Model
	ToInviteLinkModel(invitelinkdto.InviteLink) model.InviteLink
	ToInviteLinkModels([]invitelinkdto.InviteLink) []model.InviteLink

	// goverter:ignore Model
	FromInviteLinkModel(model.InviteLink) invitelinkdto.InviteLink
}

type Opts struct {
	fx.In
	Log      logger.Logger
	Database *mongo.Database
	Client   *mongo.Client
	Mapper   Mapper
}

type Repository struct {
	log logger.Logger
	// Settlements collection
	setColl *mongo.Collection
	// Settlement requests collection
	setReqColl *mongo.Collection
	// Settlement invitations collection
	setInvColl *mongo.Collection
	// Settlement join requests collection
	setJoinReqColl *mongo.Collection
	// Imperial favor log collection
	favorLogColl *mongo.Collection
	// Settlement invite links collection
	inviteLinkColl *mongo.Collection
	// Settlement notifier placements collection (read-only here)
	notifierColl *mongo.Collection
	// MongoDB client used for transactions
	client *mongo.Client
	mapper Mapper
}

func New(opts Opts) (*Repository, error) {
	sColl := opts.Database.Collection(settlementCollName)
	srColl := opts.Database.Collection(settlementReqCollName)
	siColl := opts.Database.Collection(settlementInvitationCollName)
	sjrColl := opts.Database.Collection(settlementJoinRequestCollName)
	flColl := opts.Database.Collection(imperialFavorLogCollName)
	ilColl := opts.Database.Collection(settlementInviteLinkCollName)
	snColl := opts.Database.Collection(settlementNotifierCollName)
	// The component names the layer, not the module: the plan's enum is
	// grpc|gateway|repo|mongo|nats|http|job|webhook, and the module already
	// rides along in scope, which fx sets per domain.
	logger := opts.Log.WithComponent("repo")
	if err := setupIndexes(logger, sColl, srColl, siColl, sjrColl, flColl, ilColl, snColl); err != nil {
		return nil, err
	}
	return &Repository{
		log:            logger,
		setColl:        sColl,
		setReqColl:     srColl,
		setInvColl:     siColl,
		setJoinReqColl: sjrColl,
		favorLogColl:   flColl,
		inviteLinkColl: ilColl,
		notifierColl:   snColl,
		client:         opts.Client,
		mapper:         opts.Mapper,
	}, nil
}

func setupIndexes(
	log logger.Logger,
	setColl *mongo.Collection,
	setReqColl *mongo.Collection,
	setInvColl *mongo.Collection,
	setJoinReqColl *mongo.Collection,
	favorLogColl *mongo.Collection,
	inviteLinkColl *mongo.Collection,
	notifierColl *mongo.Collection,
) error {
	// An optional index only speeds a read up, so a failure to create one is logged and carried on.
	createIndex := func(coll *mongo.Collection, spec mongo.IndexModel) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		if _, err := coll.Indexes().CreateOne(ctx, spec); err != nil {
			log.Error("failed to create index", zap.String("collection", coll.Name()), zap.Error(err))
		}
	}
	// A required index carries an invariant. Without it the rule survives only as a read-then-write
	// check in the service, which a concurrent writer can beat, so a failure stops the service
	// instead of being logged: serving anyway would accept writes the domain promises to refuse.
	requireIndex := func(coll *mongo.Collection, spec mongo.IndexModel) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		name, err := coll.Indexes().CreateOne(ctx, spec)
		if err != nil {
			log.Error("failed to create a required index",
				zap.String("collection", coll.Name()), zap.Error(err))
			return fmt.Errorf("create required index on %s: %w", coll.Name(), err)
		}
		log.Debug("required index created", zap.String("index", name))
		return nil
	}

	// One membership per kind: a user_id may appear in the members of at most one settlement and
	// at most one guild, so a player can hold both and never two of the same. Owners are members
	// too, so these two indexes enforce the whole invariant at the DB level.
	//
	// They are partial on the stored `kind` rather than on `type`: a partial index filters on the
	// equality of a stored field, and one equality per kind covers every tier without listing them.
	// Migration 2 backfills the field, and it runs before these indexes are created.
	//
	// The rule that crosses the two kinds — the owner of a guild belongs to no settlement — cannot
	// be expressed by an index over one document, so the service checks it. See
	// model.Memberships.
	required := []struct {
		coll *mongo.Collection
		spec mongo.IndexModel
	}{
		{setColl, uniqueByKind("uniq_members_user_id_settlement", "members.user_id", model.KindSettlement)},
		{setColl, uniqueByKind("uniq_members_user_id_guild", "members.user_id", model.KindGuild)},
		// One request document per leader per kind, so a pending guild request never blocks the
		// settlement one and the other way round. Submit reuses that one document for the life of
		// the group, which is what makes the count per kind and not per open request.
		{setReqColl, uniqueByKind("uniq_leader_user_id_settlement", "leader.user_id", model.KindSettlement)},
		{setReqColl, uniqueByKind("uniq_leader_user_id_guild", "leader.user_id", model.KindGuild)},
	}
	for _, idx := range required {
		if err := requireIndex(idx.coll, idx.spec); err != nil {
			return err
		}
	}

	// A settlement has at most one verification request. Kept partial so
	// documents predating the settlement (settlement_id unset) don't collide.
	createIndex(setReqColl, mongo.IndexModel{
		Keys:    bson.D{{Key: "settlement_id", Value: 1}},
		Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"settlement_id": bson.M{"$type": "string"}}),
	})

	createIndex(setInvColl, mongo.IndexModel{
		Keys: bson.D{
			{Key: "user_id", Value: 1},
			{Key: "settlement_id", Value: 1},
		},
		Options: options.Index().SetUnique(true),
	})

	createIndex(setJoinReqColl, mongo.IndexModel{
		Keys: bson.D{
			{Key: "user_id", Value: 1},
			{Key: "settlement_id", Value: 1},
		},
		Options: options.Index().SetUnique(true),
	})
	createIndex(setJoinReqColl, mongo.IndexModel{
		Keys: bson.D{{Key: "settlement_id", Value: 1}},
	})

	createIndex(favorLogColl, mongo.IndexModel{
		Keys: bson.D{{Key: "settlement_id", Value: -1}},
	})

	// The code is what a link is opened by; unique so a collision fails loudly.
	createIndex(inviteLinkColl, mongo.IndexModel{
		Keys:    bson.D{{Key: "code", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	createIndex(inviteLinkColl, mongo.IndexModel{
		Keys: bson.D{{Key: "settlement_id", Value: 1}, {Key: "_id", Value: -1}},
	})

	// One notifier block stands per settlement. The game server inserts here
	// when a block is placed, so this index is what stops a second one.
	createIndex(notifierColl, mongo.IndexModel{
		Keys:    bson.D{{Key: "settlement_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})

	return nil
}

// uniqueByKind builds a unique index over one field, restricted to the documents of one kind. The
// name is explicit: two indexes over the same field would otherwise both take the derived name and
// the second would be refused as a conflicting index.
func uniqueByKind(name, field string, kind model.SettlementKind) mongo.IndexModel {
	return mongo.IndexModel{
		Keys: bson.D{{Key: field, Value: 1}},
		Options: options.Index().
			SetUnique(true).
			SetName(name).
			SetPartialFilterExpression(bson.M{"kind": string(kind)}),
	}
}
