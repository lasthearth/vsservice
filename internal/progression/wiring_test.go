package progression_test

import (
	"testing"

	imperialpointv1 "github.com/lasthearth/vsservice/gen/imperialpoint/v1"
	progressionv1 "github.com/lasthearth/vsservice/gen/progression/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/progression"
	"github.com/lasthearth/vsservice/internal/settlement/settlementuc"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

func TestWiring(t *testing.T) {
	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}
	err = fx.ValidateApp(
		fx.Supply(fx.Annotate(l, fx.As(new(logger.Logger)))),
		fx.Supply(&mongo.Database{}, &settlementuc.FavorOps{}),
		progression.App,
		fx.Invoke(func(progressionv1.ProgressionServiceServer, imperialpointv1.ImperialPointServiceServer) {}),
	)
	if err != nil {
		t.Fatal(err)
	}
}
