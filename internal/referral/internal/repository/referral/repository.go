package repository

import (
	"context"
	"errors"
	"time"

	dto "github.com/lasthearth/vsservice/internal/referral/internal/dto/mongo"
	"github.com/lasthearth/vsservice/internal/referral/internal/ierror"
	"github.com/lasthearth/vsservice/internal/referral/internal/model"
	"github.com/lasthearth/vsservice/internal/referral/internal/service"

	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
	mgo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const (
	referralCodesCollName  = "referral_codes"
	referralEventsCollName = "referral_events"
)

var _ service.Repository = (*Repository)(nil)

type Repository struct {
	log        logger.Logger
	codesColl  *mgo.Collection
	eventsColl *mgo.Collection
}

type Opts struct {
	fx.In

	Log      logger.Logger
	Database *mgo.Database
	Client   *mgo.Client
}

func New(opts Opts) *Repository {
	log := opts.Log.WithComponent("referral-repository")
	codesColl := opts.Database.Collection(referralCodesCollName)
	eventsColl := opts.Database.Collection(referralEventsCollName)
	setupIndexes(log, codesColl, eventsColl)
	return &Repository{
		log:        log,
		codesColl:  codesColl,
		eventsColl: eventsColl,
	}
}

func setupIndexes(log logger.Logger, codesColl, eventsColl *mgo.Collection) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	createIndex := func(coll *mgo.Collection, model mgo.IndexModel) {
		if _, err := coll.Indexes().CreateOne(ctx, model); err != nil {
			log.Error("failed to create index", zap.String("collection", coll.Name()), zap.Error(err))
		}
	}

	createIndex(codesColl, mgo.IndexModel{
		Keys:    bson.D{{Key: "code", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	createIndex(codesColl, mgo.IndexModel{
		Keys:    bson.D{{Key: "player_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	createIndex(eventsColl, mgo.IndexModel{
		Keys:    bson.D{{Key: "referee_player_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
}

func codeFromDTO(d dto.ReferralCode) *model.ReferralCode {
	return model.ReconstituteReferralCode(d.Id.Hex(), d.PlayerID, d.PlayerName, d.Code, d.CreatedAt)
}

// GetCodeByPlayerID returns the referral code owned by playerID, or
// ierror.ErrNotFound if no code exists for that player.
func (r *Repository) GetCodeByPlayerID(ctx context.Context, playerID string) (*model.ReferralCode, error) {
	l := r.log.With(zap.String("method", "GetCodeByPlayerID"), zap.String("player_id", playerID))

	var d dto.ReferralCode
	err := r.codesColl.FindOne(ctx, bson.M{"player_id": playerID}).Decode(&d)
	if err != nil {
		if errors.Is(err, mgo.ErrNoDocuments) {
			return nil, ierror.ErrNotFound
		}
		l.Error("failed to find referral code", zap.Error(err))
		return nil, err
	}

	return codeFromDTO(d), nil
}

// GetCodeByCode returns the referral code matching code, or
// ierror.ErrNotFound if no such code exists.
func (r *Repository) GetCodeByCode(ctx context.Context, code string) (*model.ReferralCode, error) {
	l := r.log.With(zap.String("method", "GetCodeByCode"), zap.String("code", code))

	var d dto.ReferralCode
	err := r.codesColl.FindOne(ctx, bson.M{"code": code}).Decode(&d)
	if err != nil {
		if errors.Is(err, mgo.ErrNoDocuments) {
			return nil, ierror.ErrNotFound
		}
		l.Error("failed to find referral code", zap.Error(err))
		return nil, err
	}

	return codeFromDTO(d), nil
}

// UpsertCode inserts code if no referral code exists yet for its player_id.
// Implemented as an update-with-upsert keyed on player_id so concurrent
// calls for the same player cannot race into a duplicate-key error.
func (r *Repository) UpsertCode(ctx context.Context, code *model.ReferralCode) error {
	l := r.log.With(zap.String("method", "UpsertCode"), zap.String("player_id", code.PlayerID))

	envelope := mongox.NewModel()
	filter := bson.M{"player_id": code.PlayerID}
	update := bson.D{
		{Key: "$setOnInsert", Value: bson.D{
			{Key: "_id", Value: envelope.Id},
			{Key: "player_id", Value: code.PlayerID},
			{Key: "player_name", Value: code.PlayerName},
			{Key: "code", Value: code.Code},
			{Key: "created_at", Value: envelope.CreatedAt},
			{Key: "updated_at", Value: envelope.UpdatedAt},
		}},
	}
	opts := options.UpdateOne().SetUpsert(true)

	_, err := r.codesColl.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		l.Error("failed to upsert referral code", zap.Error(err))
		return err
	}

	return nil
}

// CreateEvent persists a new referral event.
func (r *Repository) CreateEvent(ctx context.Context, event *model.ReferralEvent) error {
	l := r.log.With(
		zap.String("method", "CreateEvent"),
		zap.String("referrer_player_id", event.ReferrerPlayerID),
		zap.String("referee_player_id", event.RefereePlayerID),
	)

	d := dto.ReferralEvent{
		Model:            mongox.NewModel(),
		ReferrerPlayerID: event.ReferrerPlayerID,
		RefereePlayerID:  event.RefereePlayerID,
		CoinsAwarded:     event.CoinsAwarded,
	}

	if _, err := r.eventsColl.InsertOne(ctx, d); err != nil {
		l.Error("failed to create referral event", zap.Error(err))
		return err
	}

	return nil
}

// HasReferee reports whether refereePlayerID has already been recorded as a
// referee in any referral event.
func (r *Repository) HasReferee(ctx context.Context, refereePlayerID string) (bool, error) {
	l := r.log.With(zap.String("method", "HasReferee"), zap.String("referee_player_id", refereePlayerID))

	count, err := r.eventsColl.CountDocuments(ctx, bson.M{"referee_player_id": refereePlayerID})
	if err != nil {
		l.Error("failed to count referral events", zap.Error(err))
		return false, err
	}

	return count > 0, nil
}

// GetStatsByPlayerID returns the total number of referrals made by playerID
// and the total coins awarded for them. Returns zero values if playerID has
// no referral events.
func (r *Repository) GetStatsByPlayerID(ctx context.Context, playerID string) (int64, int64, error) {
	l := r.log.With(zap.String("method", "GetStatsByPlayerID"), zap.String("player_id", playerID))

	pipeline := mgo.Pipeline{
		bson.D{{Key: "$match", Value: bson.D{{Key: "referrer_player_id", Value: playerID}}}},
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: nil},
			{Key: "total_referrals", Value: bson.D{{Key: "$sum", Value: 1}}},
			{Key: "total_coins", Value: bson.D{{Key: "$sum", Value: "$coins_awarded"}}},
		}}},
	}

	cursor, err := r.eventsColl.Aggregate(ctx, pipeline)
	if err != nil {
		l.Error("failed to aggregate referral stats", zap.Error(err))
		return 0, 0, err
	}
	defer func() {
		if err := cursor.Close(ctx); err != nil {
			l.Error("cursor close failed", zap.Error(err))
		}
	}()

	if !cursor.Next(ctx) {
		if err := cursor.Err(); err != nil {
			l.Error("failed to iterate referral stats cursor", zap.Error(err))
			return 0, 0, err
		}
		return 0, 0, nil
	}

	var result struct {
		TotalReferrals int64 `bson:"total_referrals"`
		TotalCoins     int64 `bson:"total_coins"`
	}
	if err := cursor.Decode(&result); err != nil {
		l.Error("failed to decode referral stats", zap.Error(err))
		return 0, 0, err
	}

	return result.TotalReferrals, result.TotalCoins, nil
}
