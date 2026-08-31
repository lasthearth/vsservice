package kitdef_test

import (
	"testing"

	kitdefv1 "github.com/lasthearth/vsservice/gen/kitdef/v1"
	"github.com/lasthearth/vsservice/internal/kitdef"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// TestWiring pins that kitdef's fx graph resolves: the generated sermapper is
// provided in the outer fx.go, the repository binds to the service interface,
// and the service satisfies the gRPC server.
func TestWiring(t *testing.T) {
	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}

	err = fx.ValidateApp(
		fx.Supply(fx.Annotate(l, fx.As(new(logger.Logger)))),
		fx.Supply(&mongo.Database{}),
		kitdef.App,
		fx.Invoke(func(kitdefv1.KitDefServiceServer) {}),
	)
	if err != nil {
		t.Fatal(err)
	}
}
