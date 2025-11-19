package assignment

import (
	"github.com/lasthearth/vsservice/internal/kit/internal/repository/assignment/repomapper"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
)

type Opts struct {
	fx.In

	Collection *mongo.Collection `name:"kit_assignment_col"`
	Logger     logger.Logger
}

func NewFx(opts Opts) *Repository {
	mapper := &repomapper.MapperImpl{}
	return New(opts.Collection, opts.Logger, mapper)
}
