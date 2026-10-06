package event_test

import (
	"testing"

	eventv1 "github.com/lasthearth/vsservice/gen/event/v1"
	"github.com/lasthearth/vsservice/internal/event"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/mediaurl"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// TestWiring pins that event's fx graph resolves. Resolving the gRPC server
// proves the two things a compile cannot catch: the repository binds to the
// service's Repository interface, and the generated sermapper is provided in
// the outer fx.go (generating it inside the interface's own package would make
// go generate fail instead).
func TestWiring(t *testing.T) {
	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}

	err = fx.ValidateApp(
		fx.Supply(fx.Annotate(l, fx.As(new(logger.Logger)))),
		fx.Supply(&mongo.Database{}, mediaurl.New(config.Config{}), &notificationuc.Create{}),
		event.App,
		fx.Invoke(func(eventv1.EventServiceServer) {}),
	)
	if err != nil {
		t.Fatal(err)
	}
}
