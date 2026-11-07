package kit_test

import (
	"testing"

	kitv1 "github.com/lasthearth/vsservice/gen/kit/v1"
	"github.com/lasthearth/vsservice/internal/kit"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// TestWiring pins that kit's fx graph still resolves now that the event bus is
// gone: the service is built from its repositories, its mapper and a logger, and
// nothing in the graph asks for a message connection.
//
// This domain is not composed into main.go, so no running process would report a
// broken graph here. The test is the only thing that does.
func TestWiring(t *testing.T) {
	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}

	err = fx.ValidateApp(
		fx.NopLogger,
		fx.Supply(fx.Annotate(l, fx.As(new(logger.Logger)))),
		fx.Supply(&mongo.Database{}),
		fx.Supply(config.Config{}),
		kit.App,
		fx.Invoke(func(kitv1.KitServiceServer) {}),
	)
	if err != nil {
		t.Fatal(err)
	}
}
