package donate_test

import (
	"testing"

	donatev1 "github.com/lasthearth/vsservice/gen/donate/v1"
	"github.com/lasthearth/vsservice/internal/donate"
	"github.com/lasthearth/vsservice/internal/donate/donateuc"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/mediaurl"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// TestWiring pins that donate's graph still resolves after the purchase rules
// moved into internal/usecase behind fx.Private — the failure mode a compile
// cannot catch.
func TestWiring(t *testing.T) {
	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}

	err = fx.ValidateApp(
		fx.Supply(fx.Annotate(l, fx.As(new(logger.Logger)))),
		fx.Supply(&mongo.Database{}, &mongo.Client{}, mediaurl.New(config.Config{})),
		donate.App,
		fx.Invoke(func(donatev1.DonateServiceServer, *donateuc.AddCoinsUseCase) {}),
	)
	if err != nil {
		t.Fatal(err)
	}
}
