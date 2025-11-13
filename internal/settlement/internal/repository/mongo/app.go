//go:generate goverter gen github.com/lasthearth/vsservice/internal/settlement/internal/repository/mongo
package repository

import (
	"context"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/config"
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
)

const (
	settlementCollName           = "settlements"
	settlementReqCollName        = "settlement_requests"
	settlementInvitationCollName = "settlement_invitations"
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

	// goverter:ignore Members TagIds
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
	Cfg      config.Config
	Database *mongo.Database
	Client   *mongo.Client
	Mapper   Mapper
}

type Repository struct {
	log logger.Logger
	cfg config.Config
	// Settlements collection
	setColl *mongo.Collection
	// Settlement requests collection
	setReqColl *mongo.Collection
	// Settlement invitations collection
	setInvColl *mongo.Collection
	// MongoDB client used for transactions
	client *mongo.Client
	mapper Mapper
}

func New(opts Opts) *Repository {
	sColl := opts.Database.Collection(settlementCollName)
	srColl := opts.Database.Collection(settlementReqCollName)
	siColl := opts.Database.Collection(settlementInvitationCollName)
	logger := opts.Log.WithComponent("settlement-mongo-repository")
	setupIndexes(sColl, srColl, siColl)
	return &Repository{
		log:        logger,
		cfg:        opts.Cfg,
		setColl:    sColl,
		setReqColl: srColl,
		setInvColl: siColl,
		client:     opts.Client,
		mapper:     opts.Mapper,
	}
}

func setupIndexes(
	setColl *mongo.Collection,
	setReqColl *mongo.Collection,
	setInvColl *mongo.Collection,
) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	scIdx := mongo.IndexModel{
		Keys: bson.D{
			{
				Key:   "leader.user_id",
				Value: 1,
			},
			{
				Key:   "members.user_id",
				Value: 1,
			},
		},
		Options: options.Index().SetUnique(true),
	}
	setColl.Indexes().CreateOne(ctx, scIdx)

	srIdx := mongo.IndexModel{
		Keys: bson.D{
			{Key: "leader.user_id", Value: -1},
		},
		Options: options.Index().SetUnique(true),
	}
	setReqColl.Indexes().CreateOne(ctx, srIdx)

	siIdx := mongo.IndexModel{
		Keys: bson.D{
			{
				Key:   "user_id",
				Value: 1,
			},
			{
				Key:   "settlement_id",
				Value: 1,
			},
		},
		Options: options.Index().SetUnique(true),
	}
	setInvColl.Indexes().CreateOne(ctx, siIdx)
}
