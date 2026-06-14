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

	if len(req.GetPlayers()) < 2 {
		return nil, status.Error(codes.InvalidArgument, "at least 2 players required")
	}

	places := make(map[int32]bool, len(req.GetPlayers()))
	for _, p := range req.GetPlayers() {
		if p.GetPlace() < 1 {
			return nil, status.Error(codes.InvalidArgument, "place must be >= 1")
		}
		if places[p.GetPlace()] {
			return nil, status.Error(codes.InvalidArgument, "duplicate places not allowed")
		}
		places[p.GetPlace()] = true
	}

	season, err := s.repo.GetActiveSeason(ctx)
	if err != nil {
		if isDomainError(err, codes.NotFound) {
			return nil, status.Error(codes.InvalidArgument, "no active season")
		}
		l.Error("failed to get active season", zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to get active season")
	}

	playerIDs := make([]string, len(req.GetPlayers()))
	for i, p := range req.GetPlayers() {
		playerIDs[i] = p.GetPlayerId()
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

	placements := make([]PlayerPlacement, len(req.GetPlayers()))
	for i, p := range req.GetPlayers() {
		st, ok := statsMap[p.GetPlayerId()]
		if !ok {
			st = model.NewPlayerStats(p.GetPlayerId(), p.GetPlayerName(), season.ID)
		}
		placements[i] = PlayerPlacement{
			PlayerID:   p.GetPlayerId(),
			Place:      int(p.GetPlace()),
			CurrentELO: st.Elo,
		}
	}

	eloResults := CalculateELO(placements)
	newELOs := make(map[string]int, len(eloResults))
	for _, r := range eloResults {
		newELOs[r.PlayerID] = r.NewELO
	}

	for _, p := range req.GetPlayers() {
		st, ok := statsMap[p.GetPlayerId()]
		if !ok {
			st = model.NewPlayerStats(p.GetPlayerId(), p.GetPlayerName(), season.ID)
		}
		st.SetELO(newELOs[p.GetPlayerId()])
		st.AddKills(int(p.GetKills()))
		if p.GetPlace() == 1 {
			st.RecordWin()
		}
		if err := s.repo.SavePlayerStats(ctx, st); err != nil {
			l.Error("failed to save player stats", zap.String("player_id", p.GetPlayerId()), zap.Error(err))
			return nil, status.Error(codes.Internal, "failed to save player stats")
		}
	}

	return &hgv1.RecordMatchResponse{}, nil
}
