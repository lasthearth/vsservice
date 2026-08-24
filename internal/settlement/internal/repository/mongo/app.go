//go:generate go tool goverter gen github.com/lasthearth/vsservice/internal/settlement/internal/repository/mongo
package repository

import (
	"context"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/logger"
	invitationdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/invitation"
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

	// goverter:ignore Members TagIds ImperialFavor Roles RolesEnabled ContactInfo
	FromVerification(dto verificationdto.SettlementVerification) settlementdto.Settlement

	FromSettlementsDTO([]settlementdto.Settlement) []model.Settlement

	// goverter:autoMap Model
	FromSettlementDTO(dto settlementdto.Settlement) model.Settlement

	// goverter:ignore Model
	ToSettlementDTO(model.Settlement) settlementdto.Settlement
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
	// MongoDB client used for transactions
	client *mongo.Client
	mapper Mapper
}

func New(opts Opts) *Repository {
	sColl := opts.Database.Collection(settlementCollName)
	srColl := opts.Database.Collection(settlementReqCollName)
	siColl := opts.Database.Collection(settlementInvitationCollName)
	sjrColl := opts.Database.Collection(settlementJoinRequestCollName)
	flColl := opts.Database.Collection(imperialFavorLogCollName)
	logger := opts.Log.WithComponent("settlement-mongo-repository")
	setupIndexes(logger, sColl, srColl, siColl, sjrColl, flColl)
	return &Repository{
		log:            logger,
		setColl:        sColl,
		setReqColl:     srColl,
		setInvColl:     siColl,
		setJoinReqColl: sjrColl,
		favorLogColl:   flColl,
		client:         opts.Client,
		mapper:         opts.Mapper,
	}
}

func setupIndexes(
	log logger.Logger,
	setColl *mongo.Collection,
	setReqColl *mongo.Collection,
	setInvColl *mongo.Collection,
	setJoinReqColl *mongo.Collection,
	favorLogColl *mongo.Collection,
) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	createIndex := func(coll *mongo.Collection, model mongo.IndexModel) {
		if _, err := coll.Indexes().CreateOne(ctx, model); err != nil {
			log.Error("failed to create index", zap.String("collection", coll.Name()), zap.Error(err))
		}
	}

	// One settlement per player: a user_id may appear in members at most once
	// across all settlement documents. Owners are members too, so this single
	// array index enforces the whole invariant at the DB level.
	createIndex(setColl, mongo.IndexModel{
		Keys:    bson.D{{Key: "members.user_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})

	// A settlement has at most one verification request. Kept partial so
	// documents predating the settlement (settlement_id unset) don't collide.
	createIndex(setReqColl, mongo.IndexModel{
		Keys:    bson.D{{Key: "settlement_id", Value: 1}},
		Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"settlement_id": bson.M{"$type": "string"}}),
	})
	// A user without a settlement yet has at most one open first-time request.
	createIndex(setReqColl, mongo.IndexModel{
		Keys:    bson.D{{Key: "leader.user_id", Value: 1}},
		Options: options.Index().SetUnique(true),
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
}
