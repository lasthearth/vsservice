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
	// playersColl belongs to internal/player. This domain only reads the two
	// fields that say how far a referee has got; see progress.go.
	playersColl *mgo.Collection
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
		log:         log,
		codesColl:   codesColl,
		eventsColl:  eventsColl,
		playersColl: opts.Database.Collection(playersCollName),
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
	// Per-player counts over every invited player: this domain's stats
	// aggregation and the cross-domain readers (appearance standings).
	createIndex(eventsColl, mgo.IndexModel{
		Keys: bson.D{{Key: "referrer_player_id", Value: 1}},
	})
	// The payout loop's working set, kept small on purpose: an event leaves the
	// index as soon as its last stage is paid, so the index holds the referrals
	// still owed a reward and nothing else. The $or is a top-level field, which
	// is what a partial filter expression allows.
	createIndex(eventsColl, mgo.IndexModel{
		Keys: bson.D{{Key: "next_check_at", Value: 1}},
		Options: options.Index().SetPartialFilterExpression(bson.M{
			"$or": bson.A{
				bson.M{"verification_paid": false},
				bson.M{"join_paid": false},
				bson.M{"referee_paid": false},
			},
		}),
	})
}

// stageField is the persisted flag of a reward stage. A stage with no field is
// one this repository does not know, and the caller gets an error instead of a
// silent no-op.
func stageField(stage model.RewardStage) (string, error) {
	switch stage {
	case model.StageVerification:
		return "verification_paid", nil
	case model.StageJoin:
		return "join_paid", nil
	case model.StageReferee:
		return "referee_paid", nil
	default:
		return "", errors.Join(model.ErrUnknownStage, errors.New("stage "+string(stage)))
	}
}

func codeFromDTO(d dto.ReferralCode) *model.ReferralCode {
	return model.ReconstituteReferralCode(d.Id.Hex(), d.PlayerID, d.PlayerName, d.Code, d.CreatedAt)
}

func eventFromDTO(d dto.ReferralEvent) *model.ReferralEvent {
	return model.ReconstituteReferralEvent(
		d.Id.Hex(),
		d.ReferrerPlayerID,
		d.RefereePlayerID,
		d.ReferrerReward,
		d.RefereeReward,
		d.CoinsAwarded,
		d.VerificationPaid,
		d.JoinPaid,
		d.RefereePaid,
		d.Checks,
		d.NextCheckAt,
		d.CreatedAt,
	)
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

// UpsertCode inserts code if no referral code exists yet for its player_id,
// and returns the document that ended up persisted. Implemented as a
// find-and-update-with-upsert keyed on player_id so concurrent calls for the
// same player cannot race into a duplicate-key error; if a concurrent call
// already inserted a code for the same player_id, the returned code reflects
// that winning document rather than the input.
func (r *Repository) UpsertCode(ctx context.Context, code *model.ReferralCode) (*model.ReferralCode, error) {
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
			{Key: "version", Value: envelope.Version},
		}},
	}
	opts := options.FindOneAndUpdate().
		SetUpsert(true).
		SetReturnDocument(options.After)

	var d dto.ReferralCode
	err := r.codesColl.FindOneAndUpdate(ctx, filter, update, opts).Decode(&d)
	if err != nil {
		l.Error("failed to upsert referral code", zap.Error(err))
		return nil, err
	}

	return codeFromDTO(d), nil
}

// CreateEvent persists a new referral event. Returns ierror.ErrAlreadyReferred
// if refereePlayerID has already been recorded as a referee in another
// event (enforced by the unique index on referee_player_id), which can
// happen if a concurrent call won a race against a prior HasReferee check.
//
// The event starts with nothing paid: every reward of it is owed to a later
// moment, when the referee is verified and then joins the game.
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
		ReferrerReward:   event.ReferrerReward,
		RefereeReward:    event.RefereeReward,
		CoinsAwarded:     event.CoinsAwarded,
		VerificationPaid: event.VerificationPaid,
		JoinPaid:         event.JoinPaid,
		RefereePaid:      event.RefereePaid,
		Checks:           event.Checks,
		NextCheckAt:      event.NextCheckAt,
	}

	if _, err := r.eventsColl.InsertOne(ctx, d); err != nil {
		if mgo.IsDuplicateKeyError(err) {
			return ierror.ErrAlreadyReferred
		}
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

// ListUnpaidEvents returns up to limit events that still owe a reward and are due
// for a look at now, the ones deferred longest ago first. A document written
// before the two-stage payout carries no referrer_reward and is skipped: its
// coins were paid in full when the code was applied, so it owes nothing.
func (r *Repository) ListUnpaidEvents(
	ctx context.Context,
	now time.Time,
	limit int64,
) ([]*model.ReferralEvent, error) {
	l := r.log.With(zap.String("method", "ListUnpaidEvents"))

	filter := bson.M{
		"referrer_reward": bson.M{"$gt": 0},
		"next_check_at":   bson.M{"$lte": now},
		"$or": bson.A{
			bson.M{"verification_paid": false},
			bson.M{"join_paid": false},
			bson.M{"referee_paid": false},
		},
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "next_check_at", Value: 1}}).
		SetLimit(limit)

	cursor, err := r.eventsColl.Find(ctx, filter, opts)
	if err != nil {
		l.Error("failed to find referral events that owe a reward", zap.Error(err))
		return nil, err
	}
	defer func() {
		if err := cursor.Close(ctx); err != nil {
			l.Error("cursor close failed", zap.Error(err))
		}
	}()

	var dtos []dto.ReferralEvent
	if err := cursor.All(ctx, &dtos); err != nil {
		l.Error("failed to decode referral events", zap.Error(err))
		return nil, err
	}

	events := make([]*model.ReferralEvent, len(dtos))
	for i, d := range dtos {
		events[i] = eventFromDTO(d)
	}

	return events, nil
}

// ClaimReward marks one stage as paid before the coins move, so a stage is paid
// at most once even when several passes overlap. The update is conditional on
// the flag still being false: the pass that changes the document is the only one
// that pays.
func (r *Repository) ClaimReward(
	ctx context.Context,
	refereePlayerID string,
	stage model.RewardStage,
	coins int64,
) (bool, error) {
	l := r.log.With(
		zap.String("method", "ClaimReward"),
		zap.String("referee_player_id", refereePlayerID),
		zap.String("stage", string(stage)),
	)

	field, err := stageField(stage)
	if err != nil {
		l.Error("refusing to claim an unknown reward stage", zap.Error(err))
		return false, err
	}

	update := bson.D{
		{Key: "$set", Value: bson.D{
			{Key: field, Value: true},
			{Key: "updated_at", Value: time.Now()},
		}},
	}
	if stage != model.StageReferee {
		// coins_awarded is what the referrer has received; the referee's own
		// reward is not part of that total.
		update = append(update, bson.E{Key: "$inc", Value: bson.D{{Key: "coins_awarded", Value: coins}}})
	}

	result, err := r.eventsColl.UpdateOne(
		ctx,
		bson.M{"referee_player_id": refereePlayerID, field: false},
		update,
	)
	if err != nil {
		l.Error("failed to claim referral reward", zap.Error(err))
		return false, err
	}

	return result.ModifiedCount == 1, nil
}

// ReleaseReward clears the claim of a stage whose payout failed, so the next
// pass tries it again.
func (r *Repository) ReleaseReward(
	ctx context.Context,
	refereePlayerID string,
	stage model.RewardStage,
	coins int64,
) error {
	l := r.log.With(
		zap.String("method", "ReleaseReward"),
		zap.String("referee_player_id", refereePlayerID),
		zap.String("stage", string(stage)),
	)

	field, err := stageField(stage)
	if err != nil {
		l.Error("refusing to release an unknown reward stage", zap.Error(err))
		return err
	}

	update := bson.D{
		{Key: "$set", Value: bson.D{
			{Key: field, Value: false},
			{Key: "updated_at", Value: time.Now()},
		}},
	}
	if stage != model.StageReferee {
		update = append(update, bson.E{Key: "$inc", Value: bson.D{{Key: "coins_awarded", Value: -coins}}})
	}

	result, err := r.eventsColl.UpdateOne(
		ctx,
		bson.M{"referee_player_id": refereePlayerID, field: true},
		update,
	)
	if err != nil {
		l.Error("failed to release referral reward", zap.Error(err))
		return err
	}
	if result.ModifiedCount != 1 {
		// Nothing to release: the claim was already gone, which is the state the
		// caller wanted anyway.
		l.Warn("released a reward stage that was not claimed")
	}

	return nil
}

// DeferEvent records when the loop may look at this event again, and how many
// fruitless looks it has had. It is the only thing that keeps an event whose
// player never moves out of every pass.
func (r *Repository) DeferEvent(ctx context.Context, event *model.ReferralEvent) error {
	l := r.log.With(
		zap.String("method", "DeferEvent"),
		zap.String("referee_player_id", event.RefereePlayerID),
	)

	_, err := r.eventsColl.UpdateOne(
		ctx,
		bson.M{"referee_player_id": event.RefereePlayerID},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "next_check_at", Value: event.NextCheckAt},
			{Key: "checks", Value: int64(event.Checks)},
			{Key: "updated_at", Value: time.Now()},
		}}},
	)
	if err != nil {
		l.Error("failed to defer a referral event", zap.Error(err))
		return err
	}

	return nil
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
