package server

import (
	"strings"
	"testing"

	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
)

// Build must refuse to start when a registered method is not classified for
// auth. This runs against the real service registration list rather than a
// synthetic ServiceInfo, so a scope table that drifted from the proto is caught
// here and not in production.
//
// The positive direction — every method currently classified — is asserted by
// startup itself: Build runs in the fx OnStart hook and its error aborts the
// process.
func TestBuildRejectsUnclassifiedMethods(t *testing.T) {
	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}

	// No scopers registered, so every non-public method is unclassified. The
	// service implementations are never invoked, only registered.
	s := &Server{
		authInterceptor: interceptor.NewAuth(interceptor.Opts{Log: l}),
		log:             l,
	}

	err = s.Build()
	if err == nil {
		t.Fatal("want Build to fail with no scopers registered, got nil")
	}
	if !strings.Contains(err.Error(), "unclassified") {
		t.Errorf("want an unclassified-method error, got: %v", err)
	}
	// Naming the offenders is the point: a count alone does not tell the
	// developer what to declare.
	if !strings.Contains(err.Error(), "/settlement.v1.SettlementService/RemoveMember") {
		t.Errorf("error must list the offending methods, got: %v", err)
	}
}
