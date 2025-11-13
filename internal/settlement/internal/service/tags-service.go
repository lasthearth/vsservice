package service

import (
	"context"

	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
)

// AddTagToSettlement implements settlementv1.SettlementServiceServer.
func (s *Service) AddTagToSettlement(ctx context.Context, req *settlementv1.AddTagToSettlementRequest) (*settlementv1.AddTagToSettlementResponse, error) {
	err := s.dbRepo.AddTag(ctx, req.SettlementId, req.TagId)
	if err != nil {
		return nil, err
	}

	set, err := s.dbRepo.GetSettlement(ctx, req.SettlementId)
	if err != nil {
		return nil, err
	}

	resp := s.mapper.ToSettlementProto(*set)
	return &settlementv1.AddTagToSettlementResponse{
		Settlement: resp,
	}, nil
}

// RemoveTagFromSettlement implements settlementv1.SettlementServiceServer.
func (s *Service) RemoveTagFromSettlement(ctx context.Context, req *settlementv1.RemoveTagFromSettlementRequest) (*settlementv1.RemoveTagFromSettlementResponse, error) {
	err := s.dbRepo.RemoveTag(ctx, req.SettlementId, req.TagId)
	if err != nil {
		return nil, err
	}

	set, err := s.dbRepo.GetSettlement(ctx, req.SettlementId)
	if err != nil {
		return nil, err
	}

	resp := s.mapper.ToSettlementProto(*set)
	return &settlementv1.RemoveTagFromSettlementResponse{
		Settlement: resp,
	}, nil
}
