package service

import (
	"testing"

	appearancev1 "github.com/lasthearth/vsservice/gen/appearance/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// Every AppearanceService method must be classified: public in matcher.go or
// in this Scoper's table. Startup enforces the same rule for the whole server.
func TestEveryMethodClassified(t *testing.T) {
	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}

	s := &Service{}
	srv := grpc.NewServer()
	appearancev1.RegisterAppearanceServiceServer(srv, s)

	auth := interceptor.NewAuth(interceptor.Opts{Log: l, Scopers: []interceptor.Scoper{s}})
	if err := auth.VerifyCoverage(srv); err != nil {
		t.Fatal(err)
	}
}
