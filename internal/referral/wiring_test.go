package referral_test

import (
	"testing"

	referralv1 "github.com/lasthearth/vsservice/gen/referral/v1"
	"github.com/lasthearth/vsservice/internal/donate/donateuc"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/referral"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// TestWiring pins that referral's fx graph resolves: the repository binds to
// its interface, the service is both the gRPC server and a scoper, and the
// payout worker gets the repository and the donate wallet it pays through.
func TestWiring(t *testing.T) {
	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}

	err = fx.ValidateApp(
		fx.Supply(fx.Annotate(l, fx.As(new(logger.Logger)))),
		fx.Supply(&mongo.Database{}, &mongo.Client{}, &donateuc.AddCoinsUseCase{}, config.Config{}),
		referral.App,
		fx.Invoke(func(referralv1.ReferralServiceServer) {}),
		fx.Invoke(fx.Annotate(func([]interceptor.Scoper) {}, fx.ParamTags(`group:"scopers"`))),
	)
	if err != nil {
		t.Fatal(err)
	}
}
