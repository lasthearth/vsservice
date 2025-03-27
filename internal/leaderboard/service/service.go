package service

import (
	"context"
	"github.com/go-faster/errors"
	v1 "github.com/ripls56/vsservice/gen/proto/v1"
	"github.com/ripls56/vsservice/internal/leaderboard/model"
	"github.com/samber/lo"
)

var _ v1.LeaderboardServiceServer = (*Service)(nil)

type Repository interface {
	ListEntriesSortByDeath(ctx context.Context, limit int) ([]*model.Entry, error)
	ListEntriesSortByKills(ctx context.Context, limit int) ([]*model.Entry, error)
	ListEntriesSortByOnline(ctx context.Context, limit int) ([]*model.Entry, error)
}

func (s *Service) ListEntries(ctx context.Context, req *v1.LeaderboardRequest) (*v1.LeaderboardResponse, error) {
	var (
		entries []*model.Entry
		err     error
	)

	switch req.Filter {
	case v1.LeaderboardRequest_LEADERBOARD_FILTER_DEATHS:
		entries, err = s.repo.ListEntriesSortByDeath(ctx, int(req.Limit))
	case v1.LeaderboardRequest_LEADERBOARD_FILTER_KILLS:
		entries, err = s.repo.ListEntriesSortByKills(ctx, int(req.Limit))
	case v1.LeaderboardRequest_LEADERBOARD_FILTER_ONLINE:
		entries, err = s.repo.ListEntriesSortByOnline(ctx, int(req.Limit))
	default:
		return nil, errors.New("unknown filter")
	}
	if err != nil {
		return nil, err
	}

	response := lo.Map(entries, func(entry *model.Entry, index int) *v1.LeaderboardEntry {
		return &v1.LeaderboardEntry{
			Name:        entry.Name,
			Deaths:      int32(entry.DeathCount),
			Kills:       int32(entry.KillCount),
			HoursPlayed: entry.HoursPlayed,
		}
	})

	return &v1.LeaderboardResponse{
		Entries: response,
	}, nil
}
