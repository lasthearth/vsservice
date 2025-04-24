package service

import (
	"net/http"

	"github.com/eapache/go-resiliency/retrier"
	rulesv1 "github.com/lasthearth/vsservice/gen/rules/v1"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
)

var _ rulesv1.RuleServiceServer = (*Service)(nil)

type Opts struct {
	fx.In
	// Needs to be a client for making HTTP requests to sso
	// So http client must automatically manage tokens
	Client        *http.Client
	Log           logger.Logger
	Cfg           config.Config
	Retrier       *retrier.Retrier
	DbRepo        DbRepository
	SsoRepository SsoRepository
}

type Service struct {
	// Needs to be a client for making HTTP requests to sso
	// So http client must automatically manage tokens
	client  *http.Client
	log     logger.Logger
	cfg     config.Config
	dbRepo  DbRepository
	ssoRepo SsoRepository
	retrier *retrier.Retrier
}

func New(opts Opts) *Service {
	return &Service{
		client:  opts.Client,
		log:     opts.Log,
		cfg:     opts.Cfg,
		dbRepo:  opts.DbRepo,
		ssoRepo: opts.SsoRepository,
		retrier: opts.Retrier,
	}
}
