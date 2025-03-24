package service

import (
	v1 "github.com/ripls56/vsservice/gen/proto/v1"
	"github.com/ripls56/vsservice/logger"
	"go.uber.org/fx"
)

type VsApiV1Opts struct {
	fx.In
	Log logger.Logger
}

type VsApiV1 struct {
	v1.UnimplementedVintageServiceServer
	log logger.Logger
}

func NewVsApiV1(opts VsApiV1Opts) *VsApiV1 {
	return &VsApiV1{
		log: opts.Log,
	}
}
