package service

import (
	"context"

	hgv1 "github.com/lasthearth/vsservice/gen/hungergames/v1"
	"github.com/lasthearth/vsservice/internal/hungergames/internal/model"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const defaultLeaderboardLimit = 25

func (s *Service) RecordMatch(ctx context.Context, req *hgv1.RecordMatchRequest) (*hgv1.RecordMatchResponse, error) {
	l := s.log.With(zap.String("method", "RecordMatch"))

	if len(req.Players) < 2 {
		return nil, status.Error(codes.InvalidArgument, "at least 2 players required")
	}

	places := make(map[int32]bool, len(req.Players))
	for _, p := range req.Players {
		if p.Place < 1 {
			return nil, status.Error(codes.InvalidArgument, "place must be >= 1")
		}
		if places[p.Place] {
			return nil, status.Error(codes.InvalidArgument, "duplicate places not allowed")
		}
		places[p.Place] = true
	}

	season, err := s.repo.GetActiveSeason(ctx)
	if err != nil {
		if isDomainError(err, codes.NotFound) {
			return nil, status.Error(codes.InvalidArgument, "no active season")
		}
		l.Error("failed to get active season", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get active season")
	}

	playerIDs := make([]string, len(req.Players))
	for i, p := range req.Players {
		playerIDs[i] = p.PlayerId
	}

	existingStats, err := s.repo.GetPlayerStatsByIDs(ctx, season.ID, playerIDs)
	if err != nil {
		l.Error("failed to fetch player stats", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to fetch player stats")
	}

	statsMap := make(map[string]*model.PlayerStats, len(existingStats))
	for _, st := range existingStats {
		statsMap[st.PlayerID] = st
	}

	placements := make([]PlayerPlacement, len(req.Players))
	for i, p := range req.Players {
		st, ok := statsMap[p.PlayerId]
		if !ok {
			st = model.NewPlayerStats(p.PlayerId, p.PlayerName, season.ID)
		}
		placements[i] = PlayerPlacement{
			PlayerID:   p.PlayerId,
			Place:      int(p.Place),
			CurrentELO: st.Elo,
		}
	}

	eloResults := CalculateELO(placements)
	newELOs := make(map[string]int, len(eloResults))
	for _, r := range eloResults {
		newELOs[r.PlayerID] = r.NewELO
	}

	for _, p := range req.Players {
		st, ok := statsMap[p.PlayerId]
		if !ok {
			st = model.NewPlayerStats(p.PlayerId, p.PlayerName, season.ID)
		}
		st.SetELO(newELOs[p.PlayerId])
		st.AddKills(int(p.Kills))
		if p.Place == 1 {
			st.RecordWin()
		}
		if err := s.repo.SavePlayerStats(ctx, st); err != nil {
			l.Error("failed to save player stats", zap.String("player_id", p.PlayerId), zap.Error(err))
			return nil, status.Error(codes.Internal, "failed to save player stats")
		}
	}

	return &hgv1.RecordMatchResponse{}, nil
}
