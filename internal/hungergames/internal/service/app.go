package service

import (
	"errors"

	hgv1 "github.com/lasthearth/vsservice/gen/hungergames/v1"
	"github.com/lasthearth/vsservice/internal/donate/donateuc"
	pkgerr "github.com/lasthearth/vsservice/internal/pkg/ierror"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.uber.org/fx"
	"google.golang.org/grpc/codes"
)

var _ hgv1.HungerGamesServiceServer = (*Service)(nil)

// Service implements HungerGamesServiceServer.
type Service struct {
	repo     Repository
	donateUC *donateuc.AddCoinsUseCase
	log      logger.Logger
}

// Opts are the fx-injected dependencies for the service.
type Opts struct {
	fx.In

	Repo     Repository
	DonateUC *donateuc.AddCoinsUseCase
	Logger   logger.Logger
}

func New(opts Opts) *Service {
	return &Service{
		repo:     opts.Repo,
		donateUC: opts.DonateUC,
		log:      opts.Logger,
	}
}

// isDomainError returns true when err is a *pkgerr.DomainError with the given code.
func isDomainError(err error, code codes.Code) bool {
	var de *pkgerr.DomainError
	return errors.As(err, &de) && de.Code == code
}
