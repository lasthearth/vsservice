package serverinfo

import (
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
)

type Opts struct {
	fx.In

	Collection *mongo.Collection `name:"serverinfo_col"`
	Logger     logger.Logger
}

func NewFx(opts Opts) *Repository {
	return New(opts.Collection, opts.Logger)
}
