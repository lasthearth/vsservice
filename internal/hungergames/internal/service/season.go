package service

import (
	"context"
	"fmt"

	hgv1 "github.com/lasthearth/vsservice/gen/hungergames/v1"
	"github.com/lasthearth/vsservice/internal/hungergames/internal/model"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ResetSeason closes the active season, archives the standings, pays the rank
// rewards and clears current-season stats.
//
// Order matters: the season is CLAIMED (closed) before any coins move. Credit
// is an atomic $inc, so it cannot be undone — if two callers both read the same
// active season and both paid before closing it, every reward would be paid
// twice with no way to reverse it. Closing first means the loser of the race
// gets ErrSeasonAlreadyClosed and pays nothing.
func (s *Service) ResetSeason(ctx context.Context, req *hgv1.ResetSeasonRequest) (*hgv1.ResetSeasonResponse, error) {
	l := s.log.With(zap.String("method", "ResetSeason"))

	season, err := s.repo.GetActiveSeason(ctx)
	if err != nil {
		if isDomainError(err, codes.NotFound) {
			return nil, status.Error(codes.NotFound, "no active season")
		}
		l.Error("failed to get active season", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get active season")
	}

	allStats, err := s.repo.ListAllPlayerStatsByELO(ctx)
	if err != nil {
		l.Error("failed to list all player stats", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to list player stats")
	}

	// Claim the season. A concurrent reset that read the same active season
	// stops here without having paid anything.
	if err := s.repo.CloseSeason(ctx, season.ID); err != nil {
		if isDomainError(err, codes.AlreadyExists) {
			l.Warn("season already closed by a concurrent reset, no rewards paid",
				zap.String("season_id", season.ID))
			return nil, status.Error(codes.AlreadyExists, "season is already closed")
		}
		l.Error("failed to close season", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to close season")
	}

	rewardMap := make(map[int]int64, len(req.GetRewards()))
	for _, r := range req.GetRewards() {
		rewardMap[int(r.GetRank())] = r.GetCoins()
	}

	results := make([]*model.SeasonResult, len(allStats))
	for i, st := range allStats {
		rank := i + 1
		rewardCoins := rewardMap[rank]

		results[i] = &model.SeasonResult{
			SeasonID:    season.ID,
			PlayerID:    st.PlayerID,
			PlayerName:  st.PlayerName,
			Elo:         st.Elo,
			Wins:        st.Wins,
			Kills:       st.Kills,
			Rank:        rank,
			RewardCoins: rewardCoins,
		}

		if rewardCoins > 0 {
			reason := fmt.Sprintf("Season %d reward, rank %d", season.Number, rank)
			if err := s.donateUC.Credit(ctx, st.PlayerID, st.PlayerName, rewardCoins, reason); err != nil {
				l.Error("failed to credit season reward", zap.String("player_id", st.PlayerID), zap.Error(err))
				// non-fatal: season reset continues even if a reward fails
			}
		}
	}

	if len(results) > 0 {
		if err := s.repo.CreateSeasonResults(ctx, results); err != nil {
			// The season is already closed and rewards are already paid, so
			// this leaves the standings unarchived. Not compensated: the
			// alternative is reopening a season whose coins are spent.
			l.Error("failed to save season results after rewards were paid", zap.Error(err))
			return nil, status.Error(codes.Internal, "failed to save season results")
		}
	}

	if err := s.repo.DeleteAllPlayerStats(ctx); err != nil {
		l.Error("failed to delete player stats", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to reset player stats")
	}

	return &hgv1.ResetSeasonResponse{}, nil
}

func (s *Service) CreateSeason(ctx context.Context, _ *hgv1.CreateSeasonRequest) (*hgv1.CreateSeasonResponse, error) {
	l := s.log.With(zap.String("method", "CreateSeason"))

	_, err := s.repo.GetActiveSeason(ctx)
	if err == nil {
		return nil, status.Error(codes.AlreadyExists, "active season already exists")
	}
	if !isDomainError(err, codes.NotFound) {
		l.Error("failed to check active season", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to check active season")
	}

	count, err := s.repo.CountSeasons(ctx)
	if err != nil {
		l.Error("failed to count seasons", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to create season")
	}

	season := model.NewSeason(count + 1)
	created, err := s.repo.CreateSeason(ctx, season)
	if err != nil {
		l.Error("failed to create season", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to create season")
	}

	return &hgv1.CreateSeasonResponse{Season: toSeasonProto(created)}, nil
}

func (s *Service) ListSeasons(ctx context.Context, req *hgv1.ListSeasonsRequest) (*hgv1.ListSeasonsResponse, error) {
	l := s.log.With(zap.String("method", "ListSeasons"))

	limit := int(req.GetLimit())
	if limit <= 0 {
		limit = 20
	}

	seasons, next, err := s.repo.ListSeasons(ctx, req.GetNext(), limit)
	if err != nil {
		l.Error("failed to list seasons", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to list seasons")
	}

	return &hgv1.ListSeasonsResponse{
		Seasons: lo.Map(seasons, func(season *model.Season, _ int) *hgv1.SeasonInfo {
			return toSeasonProto(season)
		}),
		Next: next,
	}, nil
}

func (s *Service) GetSeasonLeaderboard(ctx context.Context, req *hgv1.GetSeasonLeaderboardRequest) (*hgv1.GetSeasonLeaderboardResponse, error) {
	l := s.log.With(zap.String("method", "GetSeasonLeaderboard"), zap.String("season_id", req.GetSeasonId()))

	if _, err := s.repo.GetSeasonByID(ctx, req.GetSeasonId()); err != nil {
		if isDomainError(err, codes.NotFound) {
			return nil, status.Error(codes.NotFound, "season not found")
		}
		l.Error("failed to get season", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get season")
	}

	results, err := s.repo.ListSeasonResults(ctx, req.GetSeasonId())
	if err != nil {
		l.Error("failed to list season results", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to list season results")
	}

	return &hgv1.GetSeasonLeaderboardResponse{
		Entries: lo.Map(results, func(r *model.SeasonResult, _ int) *hgv1.SeasonResultEntry {
			return toSeasonResultProto(r)
		}),
	}, nil
}

func (s *Service) GetPlayerStats(ctx context.Context, req *hgv1.GetPlayerStatsRequest) (*hgv1.GetPlayerStatsResponse, error) {
	l := s.log.With(zap.String("method", "GetPlayerStats"),
		zap.String("season_id", req.GetSeasonId()),
		zap.String("player_id", req.GetPlayerId()),
	)

	season, err := s.repo.GetSeasonByID(ctx, req.GetSeasonId())
	if err != nil {
		if isDomainError(err, codes.NotFound) {
			return nil, status.Error(codes.NotFound, "season not found")
		}
		l.Error("failed to get season", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get season")
	}

	// Active season: look in current stats
	if season.IsActive() {
		st, err := s.repo.GetPlayerStats(ctx, req.GetSeasonId(), req.GetPlayerId())
		if err != nil {
			if isDomainError(err, codes.NotFound) {
				return nil, status.Error(codes.NotFound, "player stats not found")
			}
			l.Error("failed to get player stats", zap.Error(err))
			return nil, status.Error(codes.Internal, "failed to get player stats")
		}
		return &hgv1.GetPlayerStatsResponse{
			Stats: &hgv1.SeasonResultEntry{
				PlayerId:   st.PlayerID,
				PlayerName: st.PlayerName,
				Elo:        int32(st.Elo),
				Wins:       int32(st.Wins),
				Kills:      int32(st.Kills),
			},
		}, nil
	}

	// Ended season: look in archived results
	result, err := s.repo.GetPlayerSeasonResult(ctx, req.GetSeasonId(), req.GetPlayerId())
	if err != nil {
		if isDomainError(err, codes.NotFound) {
			return nil, status.Error(codes.NotFound, "player stats not found")
		}
		l.Error("failed to get player season result", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get player stats")
	}

	return &hgv1.GetPlayerStatsResponse{Stats: toSeasonResultProto(result)}, nil
}

func toSeasonProto(s *model.Season) *hgv1.SeasonInfo {
	info := &hgv1.SeasonInfo{
		Id:        s.ID,
		Number:    int32(s.Number),
		StartedAt: timestamppb.New(s.StartedAt),
	}
	if s.EndedAt != nil {
		info.EndedAt = timestamppb.New(*s.EndedAt)
	}
	return info
}

func toSeasonResultProto(r *model.SeasonResult) *hgv1.SeasonResultEntry {
	return &hgv1.SeasonResultEntry{
		PlayerId:    r.PlayerID,
		PlayerName:  r.PlayerName,
		Elo:         int32(r.Elo),
		Wins:        int32(r.Wins),
		Kills:       int32(r.Kills),
		Rank:        int32(r.Rank),
		RewardCoins: r.RewardCoins,
	}
}
