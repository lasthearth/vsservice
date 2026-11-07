package service

import (
	kitv1 "github.com/lasthearth/vsservice/gen/kit/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
)

var _ kitv1.KitServiceServer = (*Service)(nil)

type Service struct {
	kitRepo        KitRepository
	assignmentRepo AssignmentRepository
	log            logger.Logger
	mapper         Mapper
}

type Opts struct {
	fx.In
	Log            logger.Logger
	AssignmentRepo AssignmentRepository
	Mapper         Mapper
}

func NewFx(opts Opts) *Service {
	return New(
		nil,
		opts.AssignmentRepo,
		opts.Log,
		opts.Mapper,
	)
}

func New(
	kitRepo KitRepository,
	assignmentRepo AssignmentRepository,
	log logger.Logger,
	mapper Mapper,
) *Service {
	return &Service{
		kitRepo:        kitRepo,
		assignmentRepo: assignmentRepo,
		log:            log,
		mapper:         mapper,
	}
}
