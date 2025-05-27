package service

import (
	"net/http"

	"github.com/eapache/go-resiliency/retrier"
	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
)

var _ settlementv1.SettlementServiceServer = (*Service)(nil)

type Opts struct {
	fx.In
	Client  *http.Client
	Log     logger.Logger
	Cfg     config.Config
	Retrier *retrier.Retrier
	DbRepo  SettlementRepository
}

type Service struct {
	client  *http.Client
	log     logger.Logger
	cfg     config.Config
	dbRepo  SettlementRepository
	retrier *retrier.Retrier
}

func New(opts Opts) *Service {
	return &Service{
		client:  opts.Client,
		log:     opts.Log,
		cfg:     opts.Cfg,
		dbRepo:  opts.DbRepo,
		retrier: opts.Retrier,
	}
}
