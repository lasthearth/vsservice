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

	rewardMap := make(map[int]int64, len(req.Rewards))
	for _, r := range req.Rewards {
		rewardMap[int(r.Rank)] = r.Coins
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
			if err := s.repo.AddCoinsToWallet(ctx, st.PlayerID, st.PlayerName, rewardCoins); err != nil {
				l.Error("failed to add reward coins", zap.String("player_id", st.PlayerID), zap.Error(err))
				// non-fatal: season reset continues even if a reward fails
			} else if err := s.repo.CreateCreditTransaction(ctx, st.PlayerID, rewardCoins, reason); err != nil {
				l.Error("failed to record reward transaction", zap.String("player_id", st.PlayerID), zap.Error(err))
			}
		}
	}

	if len(results) > 0 {
		if err := s.repo.CreateSeasonResults(ctx, results); err != nil {
			l.Error("failed to save season results", zap.Error(err))
			return nil, status.Error(codes.Internal, "failed to save season results")
		}
	}

	if err := s.repo.CloseSeason(ctx, season.ID); err != nil {
		l.Error("failed to close season", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to close season")
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

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}

	seasons, next, err := s.repo.ListSeasons(ctx, req.Next, limit)
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
	l := s.log.With(zap.String("method", "GetSeasonLeaderboard"), zap.String("season_id", req.SeasonId))

	if _, err := s.repo.GetSeasonByID(ctx, req.SeasonId); err != nil {
		if isDomainError(err, codes.NotFound) {
			return nil, status.Error(codes.NotFound, "season not found")
		}
		l.Error("failed to get season", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get season")
	}

	results, err := s.repo.ListSeasonResults(ctx, req.SeasonId)
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
		zap.String("season_id", req.SeasonId),
		zap.String("player_id", req.PlayerId),
	)

	season, err := s.repo.GetSeasonByID(ctx, req.SeasonId)
	if err != nil {
		if isDomainError(err, codes.NotFound) {
			return nil, status.Error(codes.NotFound, "season not found")
		}
		l.Error("failed to get season", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get season")
	}

	// Active season: look in current stats
	if season.IsActive() {
		st, err := s.repo.GetPlayerStats(ctx, req.SeasonId, req.PlayerId)
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
	result, err := s.repo.GetPlayerSeasonResult(ctx, req.SeasonId, req.PlayerId)
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
