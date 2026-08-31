package repository

import (
	"context"
	"time"

	"github.com/lasthearth/vsservice/internal/mail/internal/service"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/bson"
	mgo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const (
	mailCollName  = "mail"
	claimCollName = "mail_claims"
)

var _ service.MailRepository = (*Repository)(nil)

type Repository struct {
	log       logger.Logger
	mailColl  *mgo.Collection
	claimColl *mgo.Collection
}

type Opts struct {
	fx.In

	Log      logger.Logger
	Database *mgo.Database
}

func New(opts Opts) *Repository {
	log := opts.Log.WithComponent("mail-repository")
	mailColl := opts.Database.Collection(mailCollName)
	claimColl := opts.Database.Collection(claimCollName)
	setupIndexes(log, mailColl, claimColl)
	return &Repository{
		log:       log,
		mailColl:  mailColl,
		claimColl: claimColl,
	}
}

func setupIndexes(log logger.Logger, mailColl, claimColl *mgo.Collection) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	createIndex := func(coll *mgo.Collection, model mgo.IndexModel) {
		if _, err := coll.Indexes().CreateOne(ctx, model); err != nil {
			log.Error("failed to create index", zap.String("collection", coll.Name()), zap.Error(err))
		}
	}

	// Recipient lookup (targeted + broadcast), newest first.
	createIndex(mailColl, mgo.IndexModel{
		Keys: bson.D{{Key: "recipient", Value: 1}, {Key: "_id", Value: -1}},
	})
	// Idempotency key: unique among the mails that carry one.
	createIndex(mailColl, mgo.IndexModel{
		Keys: bson.D{{Key: "idempotency_key", Value: 1}},
		Options: options.Index().SetUnique(true).
			SetPartialFilterExpression(bson.M{"idempotency_key": bson.M{"$type": "string"}}),
	})
	// One claim row per (mail_id, player_id).
	createIndex(claimColl, mgo.IndexModel{
		Keys:    bson.D{{Key: "mail_id", Value: 1}, {Key: "player_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	createIndex(claimColl, mgo.IndexModel{
		Keys: bson.D{{Key: "player_id", Value: 1}},
	})
}
