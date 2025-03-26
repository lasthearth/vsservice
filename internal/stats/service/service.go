package service

import (
	"context"
	v1 "github.com/ripls56/vsservice/gen/proto/v1"
	"github.com/ripls56/vsservice/internal/stats/internal/dto/httpdto"
	"github.com/ripls56/vsservice/internal/stats/model"
	"go.uber.org/zap"
	"time"
)

type Repository interface {
	GetByName(ctx context.Context, name string) (*model.Stats, error)
	Exists(ctx context.Context, name string) (bool, error)
	Create(ctx context.Context, httpStats *httpdto.Stats) (*model.Stats, error)
	Update(ctx context.Context, httpStats *httpdto.Stats) (*model.Stats, error)
}

func (s *Service) startFetching(ctx context.Context) error {
	ticker := time.NewTicker(time.Duration(s.cfg.StatsFetchingIntervalSecs) * time.Second)
	ch := make(chan *httpdto.Stats)
	defer close(ch)

	go func() {
		for stats := range ch {
			s.log.Info("fetching stats", zap.String("name", stats.Name))
			exists, err := s.repo.Exists(ctx, stats.Name)
			if err != nil {
				s.log.Error("exists", zap.Error(err))
			}
			if !exists {
				_, err = s.repo.Create(ctx, stats)
				if err != nil {
					s.log.Error("create stats", zap.Error(err))
				}
			} else {
				_, err = s.repo.Update(ctx, stats)
				if err != nil {
					s.log.WithComponent(stats.Name).Error("update stats", zap.Error(err))
				}
			}
		}
	}()

	s.fetcher.Fetch(ctx, ch)

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			s.fetcher.Fetch(ctx, ch)
		}
	}
}

func (s *Service) stopFetching() {
	s.fetcher.Stop()
}

func (s *Service) GetPlayerStats(ctx context.Context, req *v1.PlayerStatsRequest) (*v1.PlayerStatsResponse, error) {
	stats, err := s.repo.GetByName(ctx, req.Name)
	if err != nil {
		return nil, err
	}

	return &v1.PlayerStatsResponse{
		Name:          stats.Name,
		DeathCount:    int32(stats.DeathCount),
		HoursPlayed:   stats.HoursPlayed,
		LastOnline:    stats.LastOnline.UnixMilli(),
		PlayersKilled: int32(stats.PlayersKilled),
	}, nil
}
