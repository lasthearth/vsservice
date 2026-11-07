package player_test

import (
	"net/http"
	"testing"

	"github.com/eapache/go-resiliency/retrier"
	userv1 "github.com/lasthearth/vsservice/gen/user/v1"
	verificationv1 "github.com/lasthearth/vsservice/gen/verification/v1"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/tokenmanager"
	"github.com/lasthearth/vsservice/internal/player"
	"github.com/minio/minio-go/v7"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// TestWiring pins that player's fx graph resolves: one repository instance
// behind both service.DbRepository and presence.Repository, and the presence
// sweeper available to the lifecycle hook that starts it.
//
// Nothing else catches this. A provider the sweeper's hook cannot resolve, or a
// second fx.As that silently drops one of the two interfaces, compiles cleanly
// and only fails when the process starts.
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
		fx.Supply(&minio.Client{}),
		fx.Supply(&http.Client{}),
		fx.Supply(&retrier.Retrier{}),
		fx.Supply(&tokenmanager.Manager{}),
		fx.Supply(&notificationuc.Create{}),
		fx.Supply(config.Config{}),
		player.App,
		fx.Invoke(func(userv1.UserServiceServer) {}),
		fx.Invoke(func(verificationv1.VerificationServiceServer) {}),
	)
	if err != nil {
		t.Fatal(err)
	}
}
