package mod_test

import (
	"testing"

	modv1 "github.com/lasthearth/vsservice/gen/mod/v1"
	"github.com/lasthearth/vsservice/internal/mod"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// TestWiring pins that mod's fx graph resolves: the repository binds to the
// service's Repository and the archive's RequiredMods, and both generated
// mappers are provided in the outer fx.go.
func TestWiring(t *testing.T) {
	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}

	err = fx.ValidateApp(
		fx.Supply(fx.Annotate(l, fx.As(new(logger.Logger)))),
		fx.Supply(&mongo.Database{}),
		fx.Supply(config.Config{ModsArchiveDir: t.TempDir()}),
		mod.App,
		fx.Invoke(func(modv1.ModServiceServer) {}),
		fx.Invoke(func(mod.ArchiveHandler) {}),
	)
	if err != nil {
		t.Fatal(err)
	}
}
