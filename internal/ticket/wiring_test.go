package ticket_test

import (
	"net/http"
	"testing"

	ticketv1 "github.com/lasthearth/vsservice/gen/ticket/v1"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/logtoadmin"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/ticket"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// TestWiring pins that ticket's fx graph resolves: repository, notifier and
// service bind to their interfaces, and the service is both the gRPC server
// and a scoper.
func TestWiring(t *testing.T) {
	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}

	err = fx.ValidateApp(
		fx.Supply(fx.Annotate(l, fx.As(new(logger.Logger)))),
		fx.Supply(&mongo.Database{}, &notificationuc.Create{}, &logtoadmin.Directory{}, &http.Client{}, config.Config{}),
		ticket.App,
		fx.Invoke(func(ticketv1.TicketServiceServer) {}),
		fx.Invoke(fx.Annotate(func([]interceptor.Scoper) {}, fx.ParamTags(`group:"scopers"`))),
	)
	if err != nil {
		t.Fatal(err)
	}
}
