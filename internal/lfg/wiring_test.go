package lfg_test

import (
	"testing"

	lfgv1 "github.com/lasthearth/vsservice/gen/lfg/v1"
	"github.com/lasthearth/vsservice/internal/lfg"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// TestWiring pins that lfg's fx graph resolves: the repository binds to the
// service's Repository interface and both generated mappers are provided in
// the outer fx.go.
func TestWiring(t *testing.T) {
	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}

	err = fx.ValidateApp(
		fx.Supply(fx.Annotate(l, fx.As(new(logger.Logger)))),
		fx.Supply(&mongo.Database{}, &notificationuc.Create{}),
		lfg.App,
		fx.Invoke(func(lfgv1.LfgServiceServer) {}),
	)
	if err != nil {
		t.Fatal(err)
	}
}
