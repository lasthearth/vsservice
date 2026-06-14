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

func (r *Repository) CloseSeason(ctx context.Context, id string) error {
	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return ierror.ErrNotFound
	}

	now := time.Now()
	_, err = r.seasonsColl.UpdateOne(ctx,
		bson.M{"_id": oid},
		bson.D{{Key: "$set", Value: bson.D{{Key: "ended_at", Value: now}}}},
		options.UpdateOne(),
	)
	if err != nil {
		r.log.Error("CloseSeason: update failed", zap.Error(err))
	}
	return err
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
	// ObjectID-based cursor used by pagination.Find.
	opts := []pagination.OptionFn{
		pagination.WithLimit(int64(limit)),
	}
	if next != "" {
		opts = append(opts, pagination.WithNext(next))
	}

	resp, err := pagination.Find[seasonDTO](ctx, r.seasonsColl, opts...)
	if err != nil {
		if err.Error() == "no data found" {
			return nil, "", nil
		}
		r.log.Error("ListSeasons: find failed", zap.Error(err))
		return nil, "", err
	}

	seasons := make([]*model.Season, len(resp.Data))
	for i, d := range resp.Data {
		seasons[i] = seasonFromDTO(d)
	}
	return seasons, resp.Next, nil
}

func seasonFromDTO(d seasonDTO) *model.Season {
	return model.ReconstituteSeason(d.Model.Id.Hex(), d.Number, d.StartedAt, d.EndedAt)
}
