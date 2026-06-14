package service

import (
	"context"

	hgv1 "github.com/lasthearth/vsservice/gen/hungergames/v1"
	"github.com/lasthearth/vsservice/internal/hungergames/internal/model"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Service) ListLeaderboard(ctx context.Context, req *hgv1.ListLeaderboardRequest) (*hgv1.ListLeaderboardResponse, error) {
	l := s.log.With(zap.String("method", "ListLeaderboard"))

	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = defaultLeaderboardLimit
	}

	entries, err := s.repo.ListPlayerStatsByELO(ctx, limit)
	if err != nil {
		l.Error("failed to list leaderboard", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to list leaderboard")
	}

	return &hgv1.ListLeaderboardResponse{
		Entries: lo.Map(entries, func(e *model.PlayerStats, i int) *hgv1.LeaderboardEntry {
			return &hgv1.LeaderboardEntry{
				PlayerId:   e.PlayerID,
				PlayerName: e.PlayerName,
				Elo:        int32(e.Elo),
				Wins:       int32(e.Wins),
				Kills:      int32(e.Kills),
				Rank:       int32(i + 1),
			}
		}),
	}, nil
}
