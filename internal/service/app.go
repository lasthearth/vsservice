package service

import (
	"context"
	v1 "github.com/ripls56/vsservice/gen/proto/v1"
	"github.com/ripls56/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
)

type VsApiV1Opts struct {
	fx.In
	Log     logger.Logger
	Service StatsService
}

type StatsService interface {
	GetPlayerStats(ctx context.Context, req *v1.PlayerStatsRequest) (*v1.PlayerStatsResponse, error)
}

type VsApiV1 struct {
	service StatsService
	log     logger.Logger
}

func (v *VsApiV1) GetPlayerStats(ctx context.Context, request *v1.PlayerStatsRequest) (*v1.PlayerStatsResponse, error) {
	return v.service.GetPlayerStats(ctx, request)
}

func NewVsApiV1(opts VsApiV1Opts) *VsApiV1 {
	return &VsApiV1{
		log:     opts.Log,
		service: opts.Service,
	}
}
