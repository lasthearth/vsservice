package service

import (
	userv1 "github.com/lasthearth/vsservice/gen/user/v1"
	"go.uber.org/fx"
)

var _ userv1.UserServiceServer = (*Service)(nil)

type Opts struct {
	fx.In
	DbRepo  DbRepository
	SsoRepo SsoRepository
}

type Service struct {
	dbRepo  DbRepository
	ssoRepo SsoRepository
}

func New(opts Opts) *Service {
	return &Service{
		dbRepo:  opts.DbRepo,
		ssoRepo: opts.SsoRepo,
	}
}
