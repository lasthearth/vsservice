package service

import (
	"context"

	reactionv1 "github.com/lasthearth/vsservice/gen/reaction/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/reaction/internal/model"
	"github.com/lasthearth/vsservice/internal/reaction/internal/repository"
	"go.uber.org/fx"
)

var _ reactionv1.ReactionServiceServer = (*Service)(nil)

var _ Repository = (*repository.Repository)(nil)

// Repository stores reactions.
type Repository interface {
	Toggle(ctx context.Context, target, userID, emoji string) (bool, error)
	Counts(ctx context.Context, targets []string) ([]model.Count, error)
	UserEmojis(ctx context.Context, userID string, targets []string) (map[string][]string, error)
}

type Opts struct {
	fx.In
	Logger logger.Logger
	Repo   Repository
}

type Service struct {
	logger logger.Logger
	repo   Repository
}

func New(opts Opts) *Service {
	return &Service{
		logger: opts.Logger.WithComponent("service"),
		repo:   opts.Repo,
	}
}
