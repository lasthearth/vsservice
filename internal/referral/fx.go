package referral

import (
	"context"

	referralv1 "github.com/lasthearth/vsservice/gen/referral/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	mongorepo "github.com/lasthearth/vsservice/internal/referral/internal/repository/referral"
	"github.com/lasthearth/vsservice/internal/referral/internal/service"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/fx"
)

const module = "referral"

var App = fx.Options(
	fx.Module(
		module,
		fx.Decorate(
			func(l logger.Logger) logger.Logger {
				return l.WithScope(module)
			},
		),

		fx.Provide(
			fx.Private,
			fx.Annotate(
				mongorepo.New,
				fx.As(new(service.Repository)),
			),
		),

		fx.Provide(
			fx.Annotate(service.New,
				fx.As(new(referralv1.ReferralServiceServer)),
			),
			fx.Annotate(service.New,
				fx.As(new(interceptor.Scoper)),
				fx.ResultTags(`group:"scopers"`),
			),
		),

		// Pays the referral rewards whose moment has come: a share on the
		// referee's verification, the rest on their first join. It reads the
		// referee's player document, which the site and the game server both
		// write, so there is no event to hang the payout on.
		fx.Provide(service.NewPayoutWorker),
		fx.Invoke(func(lc fx.Lifecycle, w *service.PayoutWorker) {
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			lc.Append(fx.Hook{
				OnStart: func(context.Context) error {
					go func() {
						defer close(done)
						w.Run(ctx)
					}()
					return nil
				},
				OnStop: func(stopCtx context.Context) error {
					cancel()
					select {
					case <-done:
					case <-stopCtx.Done():
					}
					return nil
				},
			})
		}),
	),
)
