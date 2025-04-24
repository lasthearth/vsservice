package rules

import (
	"net/http"

	rulesv1 "github.com/lasthearth/vsservice/gen/rules/v1"
	"github.com/lasthearth/vsservice/internal/pkg/config"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/pkg/tokenmanager"
	mongorepo "github.com/lasthearth/vsservice/internal/rules/internal/repository/mongo"
	ssorepo "github.com/lasthearth/vsservice/internal/rules/internal/repository/sso"
	"github.com/lasthearth/vsservice/internal/rules/internal/service"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"go.uber.org/fx"
)

const module = "rules"

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
				fx.As(new(service.DbRepository)),
			),

			func(client *http.Client, c config.Config) *tokenmanager.Manager {
				return tokenmanager.NewManager(http.DefaultClient, tokenmanager.Config{
					ClientID:     c.ClientID,
					ClientSecret: c.ClientSecret,
					TokenUrl:     c.TokenUrl,
					Resource:     c.Resource,
					Scopes:       c.Scopes,
				})
			},
			fx.Annotate(
				ssorepo.New,
				fx.As(new(service.SsoRepository)),
			),
		),

		fx.Provide(
			fx.Annotate(service.New,
				fx.As(new(rulesv1.RuleServiceServer)),
			),

			fx.Annotate(service.New,
				fx.As(new(interceptor.Scoper)),
				fx.ResultTags(`group:"scopers"`),
			),
		),
	),
)
