package repository

import (
	"context"
	"errors"
	"time"

	"github.com/lasthearth/vsservice/internal/hungergames/internal/ierror"
	"github.com/lasthearth/vsservice/internal/hungergames/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"github.com/lasthearth/vsservice/internal/pkg/mongox/pagination"
	"go.mongodb.org/mongo-driver/v2/bson"
	mgo "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

type seasonDTO struct {
	mongox.Model `bson:",inline"`
	Number       int        `bson:"number"`
	StartedAt    time.Time  `bson:"started_at"`
	EndedAt      *time.Time `bson:"ended_at,omitempty"`
}

func (d seasonDTO) Id() bson.ObjectID { return d.Model.Id }

func (r *Repository) GetActiveSeason(ctx context.Context) (*model.Season, error) {
	var d seasonDTO
	err := r.seasonsColl.FindOne(ctx, bson.M{"ended_at": bson.M{"$exists": false}}).Decode(&d)
	if err != nil {
		if errors.Is(err, mgo.ErrNoDocuments) {
			return nil, ierror.ErrNoActiveSeason
		}
		r.log.Error("GetActiveSeason: find failed", zap.Error(err))
		return nil, err
	}
	return seasonFromDTO(d), nil
}

func (r *Repository) GetSeasonByID(ctx context.Context, id string) (*model.Season, error) {
	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return nil, ierror.ErrNotFound
	}

	var d seasonDTO
	if err := r.seasonsColl.FindOne(ctx, bson.M{"_id": oid}).Decode(&d); err != nil {
		if errors.Is(err, mgo.ErrNoDocuments) {
			return nil, ierror.ErrNotFound
		}
		r.log.Error("GetSeasonByID: find failed", zap.Error(err))
		return nil, err
	}
	return seasonFromDTO(d), nil
}

func (r *Repository) CreateSeason(ctx context.Context, season *model.Season) (*model.Season, error) {
	m := newModel()
	d := seasonDTO{
		Model:     m,
		Number:    season.Number,
		StartedAt: season.StartedAt,
	}

	if _, err := r.seasonsColl.InsertOne(ctx, d); err != nil {
		r.log.Error("CreateSeason: insert failed", zap.Error(err))
		return nil, err
	}

	season.AssignID(m.Id.Hex())
	return season, nil
}

// CloseSeason stamps ended_at, but only while the season is still open. The
// "still open" predicate makes this a claim: two concurrent ResetSeason calls
// both read the same active season, and whoever loses the UpdateOne race gets
// ErrSeasonAlreadyClosed instead of paying out a second set of rewards.
func (r *Repository) CloseSeason(ctx context.Context, id string) error {
	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return ierror.ErrNotFound
	}

	now := time.Now()
	res, err := r.seasonsColl.UpdateOne(ctx,
		bson.M{"_id": oid, "ended_at": bson.M{"$exists": false}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "ended_at", Value: now}}}},
		options.UpdateOne(),
	)
	if err != nil {
		r.log.Error("CloseSeason: update failed", zap.Error(err))
		return err
	}
	if res.MatchedCount == 0 {
		// Either the id does not exist or the season already has ended_at. The
		// caller only ever passes an id it just read as active, so treat it as
		// a lost race rather than a missing document.
		return ierror.ErrSeasonAlreadyClosed
	}
	return nil
}

func (r *Repository) CountSeasons(ctx context.Context) (int, error) {
	n, err := r.seasonsColl.CountDocuments(ctx, bson.M{})
	if err != nil {
		r.log.Error("CountSeasons: count failed", zap.Error(err))
		return 0, err
	}
	return int(n), nil
}

func (r *Repository) ListSeasons(ctx context.Context, next string, limit int) ([]*model.Season, string, error) {
	// Default sort is _id descending (newest season first), compatible with the
	// ObjectID-based cursor used by pagination.List.
	seasons, nextToken, err := pagination.List(ctx, r.seasonsColl, next, int64(limit), seasonFromDTO)
	if err != nil {
		r.log.Error("ListSeasons: find failed", zap.Error(err))
		return nil, "", err
	}

	return seasons, nextToken, nil
}

func seasonFromDTO(d seasonDTO) *model.Season {
	return model.ReconstituteSeason(d.Model.Id.Hex(), d.Number, d.StartedAt, d.EndedAt)
}
