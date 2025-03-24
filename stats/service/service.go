package service

import (
	"context"
	v1 "github.com/ripls56/vsservice/gen/proto/v1"
	"github.com/ripls56/vsservice/stats/model"
	"github.com/samber/lo"
	"strings"
)

type Repository interface {
	Get(name string) (*model.Stats, error)
}

func (s *Service) GetPlayerStats(ctx context.Context, req *v1.PlayerStatsRequest) (*v1.PlayerStatsResponse, error) {
	stats, err := s.repo.Get(req.Name)
	if err != nil {
		return nil, err
	}

	deaths := lo.Map(stats.Deaths, func(item model.Death, index int) *v1.PlayerStatsResponse_Death {
		return &v1.PlayerStatsResponse_Death{
			Cause:      strings.ToLower(item.Cause),
			EntityName: item.EntityName,
		}
	})

	return &v1.PlayerStatsResponse{
		Id:            stats.ID,
		Name:          stats.Name,
		DeathCount:    int32(stats.DeathCount),
		Deaths:        deaths,
		HoursPlayed:   stats.HoursPlayed,
		LastOnline:    stats.LastOnline,
		PlayersKilled: int32(stats.PlayersKilled),
	}, nil

}
