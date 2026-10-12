package repository

import (
	"context"

	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/wiki/internal/dto"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/fx"
)

// Opts is what the wiki repository needs from the container.
type Opts struct {
	fx.In
	Logger logger.Logger
	Db     *mongo.Database
	Mapper Mapper
}

// Repository stores wiki articles in MongoDB.
type Repository struct {
	logger          logger.Logger
	coll            *mongo.Collection
	collSuggestions *mongo.Collection
	mapper          Mapper
}

func New(opts Opts) *Repository {
	l := opts.Logger.WithComponent("repository")
	coll := opts.Db.Collection(dto.Collection)
	collSuggestions := opts.Db.Collection(dto.SuggestionCollection)
	r := &Repository{
		logger:          l,
		coll:            coll,
		collSuggestions: collSuggestions,
		mapper:          opts.Mapper,
	}
	r.ensureIndexes(context.Background())
	r.ensureSuggestionIndexes(context.Background())

	return r
}

// ensureIndexes creates the indexes the wiki queries rely on.
//
// The slug index is unique, which is what makes two editors racing for the same
// address end with one article and one ALREADY_EXISTS instead of two pages with
// the same URL. Creation is best effort: a failure is logged and the service
// still starts, so a missing index never takes the wiki down.
func (r *Repository) ensureIndexes(ctx context.Context) {
	models := []mongo.IndexModel{
		{
			Keys:    map[string]any{"slug": 1},
			Options: options.Index().SetUnique(true).SetName("wiki_slug_unique"),
		},
		{
			Keys:    map[string]any{"status": 1, "updated_at": -1},
			Options: options.Index().SetName("wiki_status_updated"),
		},
		{
			Keys:    map[string]any{"section": 1, "status": 1},
			Options: options.Index().SetName("wiki_section_status"),
		},
	}

	if _, err := r.coll.Indexes().CreateMany(ctx, models); err != nil {
		r.logger.Warn("failed to create wiki indexes: " + err.Error())
	}
}
