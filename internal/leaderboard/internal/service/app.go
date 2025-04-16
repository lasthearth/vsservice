package service

import "go.uber.org/fx"

type Opts struct {
	fx.In
	Repo Repository
}

type Service struct {
	repo Repository
}

func New(opts Opts) *Service {
	return &Service{
		repo: opts.Repo,
	}
}
