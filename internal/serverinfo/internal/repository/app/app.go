package repofx

import (
	"github.com/lasthearth/vsservice/internal/serverinfo/internal/repository/serverinfo"
	serverinfosrv "github.com/lasthearth/vsservice/internal/serverinfo/internal/service/serverinfo"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
)

var App = fx.Options(
	fx.Provide(
		fx.Annotate(
			func(db *mongo.Database) *mongo.Collection {
				return db.Collection(serverinfo.CollectionName)
			},
			fx.ResultTags(`name:"serverinfo_col"`),
		),

		fx.Annotate(
			serverinfo.NewFx,
			fx.As(new(serverinfosrv.ServerInfoRepository)),
		),
	),
)
