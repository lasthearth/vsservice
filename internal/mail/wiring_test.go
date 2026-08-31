package mail_test

import (
	"context"
	"errors"
	"testing"

	mailv1 "github.com/lasthearth/vsservice/gen/mail/v1"
	"github.com/lasthearth/vsservice/internal/kitdef/kitdefread"
	"github.com/lasthearth/vsservice/internal/mail"
	"github.com/lasthearth/vsservice/internal/mail/mailcompose"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// stubKitReader stands in for kitdef's read port so mail's graph resolves
// without the kitdef module.
type stubKitReader struct{}

func (stubKitReader) GetKit(context.Context, string) (*kitdefread.KitDef, error) {
	return nil, errors.New("stub")
}

// TestWiring pins that mail's fx graph resolves: the generated sermapper is
// provided in the outer fx.go, the repository binds to the service interface,
// the service satisfies the gRPC server, and the outward composer port is
// provided.
func TestWiring(t *testing.T) {
	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}

	err = fx.ValidateApp(
		fx.Supply(fx.Annotate(l, fx.As(new(logger.Logger)))),
		fx.Supply(&mongo.Database{}),
		fx.Supply(fx.Annotate(stubKitReader{}, fx.As(new(kitdefread.Reader)))),
		mail.App,
		fx.Invoke(func(mailv1.MailServiceServer) {}),
		fx.Invoke(func(mailcompose.MailComposer) {}),
	)
	if err != nil {
		t.Fatal(err)
	}
}
