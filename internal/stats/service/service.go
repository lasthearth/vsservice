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

func (s *Service) statsLoop(ctx context.Context, errCh chan error, ch chan *httpdto.Stats) {
	for stats := range ch {
		go func(stats *httpdto.Stats) {
			s.log.Info("fetching stats", zap.String("name", stats.Name))

			err := s.retrier.Run(func() error {
				ctxTimeout, cancelTimeout := context.WithTimeout(ctx, time.Second*5)
				defer cancelTimeout()
				exists, err := s.repo.Exists(ctxTimeout, stats.Name)

				if err != nil {
					s.log.Error("exists", zap.Error(err))
				}

				if !exists {
					ctx, cancel := context.WithTimeout(ctx, time.Second*5)
					defer cancel()
					_, err = s.repo.Create(ctx, stats)
					if err != nil {
						s.log.Error("create stats", zap.Error(err))
						return err
					}
				} else {
					ctx, cancel := context.WithTimeout(ctx, time.Second*5)
					defer cancel()
					_, err = s.repo.Update(ctx, stats)
					if err != nil {
						s.log.WithComponent(stats.Name).Error("update stats", zap.Error(err))
						return err
					}
				}

				return nil
			})

			if err != nil {
				errCh <- err
			}
		}(stats)

		err := <-errCh
		s.log.Error("failed to add person skip to next", zap.Error(err))
		continue
	}
}

func (s *Service) startFetching(ctx context.Context) error {
	ticker := time.NewTicker(time.Duration(s.cfg.StatsFetchingIntervalSecs) * time.Second)

	statsCh := make(chan *httpdto.Stats)
	defer close(statsCh)

	errCh := make(chan error)
	defer close(errCh)

	go s.statsLoop(ctx, errCh, statsCh)

	s.fetcher.Fetch(ctx, statsCh)

	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return nil
		case err := <-errCh:
			return err
		case <-ticker.C:
			s.fetcher.Fetch(ctx, statsCh)
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
