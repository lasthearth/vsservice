package donate_test

import (
	"context"
	"testing"

	donatev1 "github.com/lasthearth/vsservice/gen/donate/v1"
	"github.com/lasthearth/vsservice/internal/donate"
	"github.com/lasthearth/vsservice/internal/donate/donateuc"
	"github.com/lasthearth/vsservice/internal/mail/mailcompose"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/mediaurl"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// stubComposer stands in for mail's outward composer port so donate's graph
// resolves without the mail module.
type stubComposer struct{}

func (stubComposer) ComposeItemMail(context.Context, string, string, string, string, []mailcompose.ItemSpec) error {
	return nil
}
func (stubComposer) ComposeKitMail(context.Context, string, string, string, string, string) error {
	return nil
}

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
		fx.Supply(fx.Annotate(stubComposer{}, fx.As(new(mailcompose.MailComposer)))),
		donate.App,
		fx.Invoke(func(donatev1.DonateServiceServer, *donateuc.AddCoinsUseCase) {}),
	)
	if err != nil {
		t.Fatal(err)
	}
}
