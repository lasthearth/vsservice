package reaction_test

import (
	"testing"

	reactionv1 "github.com/lasthearth/vsservice/gen/reaction/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/reaction"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// TestWiring pins that reaction's fx graph resolves: the repository binds to
// the service's Repository interface and the service satisfies the gRPC server.
func TestWiring(t *testing.T) {
	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}

	err = fx.ValidateApp(
		fx.Supply(fx.Annotate(l, fx.As(new(logger.Logger)))),
		fx.Supply(&mongo.Database{}),
		reaction.App,
		fx.Invoke(func(reactionv1.ReactionServiceServer) {}),
	)
	if err != nil {
		t.Fatal(err)
	}
}
