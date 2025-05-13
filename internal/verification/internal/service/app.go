package service

import (
	"net/http"

	"github.com/eapache/go-resiliency/retrier"
	verificationv1 "github.com/lasthearth/vsservice/gen/verification/v1"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
)

var _ verificationv1.VerificationServiceServer = (*Service)(nil)

type Opts struct {
	fx.In
	// Needs to be a client for making HTTP requests to sso
	// So http client must automatically manage tokens
	Client        *http.Client
	Log           logger.Logger
	Cfg           config.Config
	Retrier       *retrier.Retrier
	DbRepo        VerificationDbRepository
	SsoRepository SsoRepository
}

type Service struct {
	// Needs to be a client for making HTTP requests to sso
	// So http client must automatically manage tokens
	client  *http.Client
	log     logger.Logger
	cfg     config.Config
	dbRepo  VerificationDbRepository
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
