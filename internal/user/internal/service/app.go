package service

import (
	userv1 "github.com/lasthearth/vsservice/gen/user/v1"
	"go.uber.org/fx"
)

var _ userv1.UserServiceServer = (*Service)(nil)

type Opts struct {
	fx.In
	Repo Repository
}

type Service struct {
	repo Repository
}

func New(repo Repository) *Service {
	return &Service{
		repo: repo,
	}
}
