package service

import (
	referralv1 "github.com/lasthearth/vsservice/gen/referral/v1"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"go.uber.org/fx"
)

var _ referralv1.ReferralServiceServer = (*Service)(nil)

// Opts are the service's dependencies. Neither the donate wallet nor the logger
// is one of them: the service only records referrals, and PayoutWorker is the
// one thing that pays them and the one thing that keeps the loop's records.
type Opts struct {
	fx.In
	Cfg    config.Config
	DbRepo Repository
}

type Service struct {
	cfg    config.Config
	dbRepo Repository
}

func New(opts Opts) *Service {
	return &Service{
		cfg:    opts.Cfg,
		dbRepo: opts.DbRepo,
	}
}
