package service

import (
	"testing"

	mailv1 "github.com/lasthearth/vsservice/gen/mail/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/zap"
	"google.golang.org/grpc"
)

// Every MailService method must be classified: public in matcher.go or in this
// Scoper's table. Startup enforces the same rule for the whole server, and
// AdminListMail is admin-only, so a missing entry would either deny every
// caller (empty scope) or ship as an unguarded read.
func TestEveryMethodClassified(t *testing.T) {
	zc := zap.NewProductionConfig()
	l, err := logger.New(&zc)
	if err != nil {
		t.Fatal(err)
	}

	s := &Service{}
	srv := grpc.NewServer()
	mailv1.RegisterMailServiceServer(srv, s)

	auth := interceptor.NewAuth(interceptor.Opts{Log: l, Scopers: []interceptor.Scoper{s}})
	if err := auth.VerifyCoverage(srv); err != nil {
		t.Fatal(err)
	}
}

// The admin list is a read, so it must not ride on the compose or revoke scope.
func TestAdminListRequiresReadScope(t *testing.T) {
	s := &Service{}

	got, ok := s.Scope()["/mail.v1.MailService/AdminListMail"]
	if !ok {
		t.Fatal("AdminListMail is not in the scope table")
	}
	if got != interceptor.Scope("mail:read") {
		t.Fatalf("scope = %q, want mail:read", got)
	}
}
