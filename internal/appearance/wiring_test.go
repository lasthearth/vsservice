package appearance_test

import (
	"testing"

	appearancev1 "github.com/lasthearth/vsservice/gen/appearance/v1"
	"github.com/lasthearth/vsservice/internal/appearance"
	"github.com/lasthearth/vsservice/internal/donate/donateuc"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// TestWiring pins that appearance's fx graph resolves: both repositories bind
// to the service's interfaces and both generated mappers are provided in the
// outer fx.go.
func TestWiring(t *testing.T) {
	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}

	err = fx.ValidateApp(
		fx.Supply(fx.Annotate(l, fx.As(new(logger.Logger)))),
		fx.Supply(&mongo.Database{}),
		fx.Supply(&donateuc.AddCoinsUseCase{}),
		appearance.App,
		fx.Invoke(func(appearancev1.AppearanceServiceServer) {}),
	)
	if err != nil {
		t.Fatal(err)
	}
}
