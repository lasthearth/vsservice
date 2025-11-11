//go:generate goverter gen github.com/lasthearth/vsservice/internal/settlement-tag/internal/repository
package repository

import (
	"context"
	"time"

	"github.com/lasthearth/vsservice/internal/settlement-tag/internal/dto"
	"github.com/lasthearth/vsservice/internal/settlement-tag/internal/model"
	"github.com/lasthearth/vsservice/internal/settlement-tag/internal/service"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/fx"
)

const (
	tagsCollName        = "settlement-tags"
	settlementsCollName = "settlements"
)

var _ service.Repository = (*Repository)(nil)

// goverter:converter
// goverter:output:file repomapper/mapper.go
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:ObjectIdToString
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:StringToObjectID
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:ObjectIdToObjectId
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTime
type Mapper interface {
	// goverter:autoMap Model
	FromTagDto(dto dto.Tag) model.Tag
	FromTagDtos(dtos []dto.Tag) []model.Tag

	// goverter:ignore Model
	ToTagDto(tag model.Tag) dto.Tag
}

// Repository handles settlement and tag-related database operations
type Repository struct {
	tagsColl        *mongo.Collection
	settlementsColl *mongo.Collection
	mapper          Mapper
}

type Opts struct {
	fx.In
	Mapper Mapper
	Db     *mongo.Database
}

func New(opts Opts) *Repository {
	tagsColl := opts.Db.Collection(tagsCollName)
	settlementsColl := opts.Db.Collection(settlementsCollName)

	setupIndexes(tagsColl, settlementsColl)

	return &Repository{
		tagsColl:        tagsColl,
		settlementsColl: settlementsColl,
		mapper:          opts.Mapper,
	}
}

func setupIndexes(tagsColl, settlementsColl *mongo.Collection) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tagNameIdx := mongo.IndexModel{
		Keys: bson.D{
			{Key: "name", Value: 1},
		},
		Options: options.Index().SetUnique(true),
	}
	tagsColl.Indexes().CreateOne(ctx, tagNameIdx)

	tagActiveIdx := mongo.IndexModel{
		Keys: bson.D{
			{Key: "is_active", Value: 1},
		},
	}
	tagsColl.Indexes().CreateOne(ctx, tagActiveIdx)

	settlementTagIdx := mongo.IndexModel{
		Keys: bson.D{
			{Key: "tag_ids", Value: 1},
		},
	}
	settlementsColl.Indexes().CreateOne(ctx, settlementTagIdx)
}
