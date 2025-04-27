package service

import (
	"context"

	"github.com/go-faster/errors"
	leaderboardv1 "github.com/lasthearth/vsservice/gen/leaderboard/v1"
	"github.com/lasthearth/vsservice/internal/leaderboard/internal/model"
	"github.com/samber/lo"
)

const entriesLimit = 25

var _ leaderboardv1.LeaderboardServiceServer = (*Service)(nil)

type Repository interface {
	ListEntriesSortByDeath(ctx context.Context, limit int) ([]*model.Entry, error)
	ListEntriesSortByKills(ctx context.Context, limit int) ([]*model.Entry, error)
	ListEntriesSortByOnline(ctx context.Context, limit int) ([]*model.Entry, error)
}

func (s *Service) ListEntries(ctx context.Context, req *leaderboardv1.LeaderboardRequest) (*leaderboardv1.LeaderboardResponse, error) {
	var (
		entries []*model.Entry
		err     error
	)

	if req.Limit <= 0 {
		req.Limit = entriesLimit
	}

	switch req.Filter {
	case leaderboardv1.LeaderboardRequest_LEADERBOARD_FILTER_DEATHS:
		entries, err = s.repo.ListEntriesSortByDeath(ctx, int(req.Limit))
	case leaderboardv1.LeaderboardRequest_LEADERBOARD_FILTER_KILLS:
		entries, err = s.repo.ListEntriesSortByKills(ctx, int(req.Limit))
	case leaderboardv1.LeaderboardRequest_LEADERBOARD_FILTER_ONLINE:
		entries, err = s.repo.ListEntriesSortByOnline(ctx, int(req.Limit))
	default:
		return nil, errors.New("unknown filter")
	}
	if err != nil {
		return nil, err
	}

	response := lo.Map(entries, func(entry *model.Entry, index int) *leaderboardv1.LeaderboardEntry {
		return &leaderboardv1.LeaderboardEntry{
			Name:        entry.Name,
			Deaths:      int32(entry.DeathCount),
			Kills:       int32(entry.KillCount),
			HoursPlayed: entry.HoursPlayed,
		}
	})

	return &leaderboardv1.LeaderboardResponse{
		Entries: response,
	}, nil
}
