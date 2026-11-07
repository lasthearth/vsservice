package serverinfo_test

import (
	"testing"

	serverinfov1 "github.com/lasthearth/vsservice/gen/serverinfo/v1"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/serverinfo"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// TestWiring pins that serverinfo's fx graph resolves: the read-only repository
// binds to the service, and the sweeper is available to the lifecycle hook that
// starts it.
//
// Nothing else catches this. The sweeper is provided privately and consumed only
// by an fx.Invoke, so a graph that cannot resolve it compiles cleanly and fails
// when the process starts.
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
		serverinfo.App,
		fx.Invoke(func(serverinfov1.ServerInfoServiceServer) {}),
	)
	if err != nil {
		t.Fatal(err)
	}
}
