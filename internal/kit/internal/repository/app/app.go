package repofx

import (
	"github.com/lasthearth/vsservice/internal/kit/internal/repository/assignment"
	"github.com/lasthearth/vsservice/internal/kit/internal/repository/kit"
	"github.com/lasthearth/vsservice/internal/kit/internal/service"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
)

var App = fx.Options(
	fx.Provide(
		fx.Annotate(
			func(db *mongo.Database) *mongo.Collection {
				return db.Collection(assignment.CollectionName)
			},
			fx.ResultTags(`name:"kit_assignment_col"`),
		),

		fx.Annotate(
			assignment.NewFx,
			fx.As(new(service.AssignmentRepository)),
		),

		fx.Annotate(
			kit.NewRepository,
			fx.As(new(service.KitRepository)),
		),
	),
)
