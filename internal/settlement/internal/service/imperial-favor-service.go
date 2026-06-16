package service

import (
	"context"

	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AddImperialFavor implements settlementv1.SettlementServiceServer.
func (s *Service) AddImperialFavor(ctx context.Context, req *settlementv1.AddImperialFavorRequest) (*settlementv1.AddImperialFavorResponse, error) {
	l := s.log.WithMethod("AddImperialFavor").With(zap.String("settlement_id", req.GetSettlementId()), zap.Int64("amount", req.GetAmount()))

	if req.GetAmount() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be positive")
	}

	adminID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	updated, err := s.dbRepo.UpdateSettlement(ctx, req.GetSettlementId(),
		func(_ context.Context, settlement *model.Settlement) (*model.Settlement, error) {
			settlement.AddFavor(req.GetAmount())
			return settlement, nil
		},
	)
	if err != nil {
		l.Error("failed to add imperial favor", zap.Error(err))
		return nil, err
	}

	if err := s.dbRepo.CreateFavorLog(ctx, model.ImperialFavorLog{
		SettlementId: req.GetSettlementId(),
		AdminId:      adminID,
		Amount:       req.GetAmount(),
		Reason:       req.GetReason(),
	}); err != nil {
		l.Error("failed to create favor log", zap.Error(err))
	}

	return &settlementv1.AddImperialFavorResponse{
		Settlement: s.mapper.ToSettlementProto(*updated),
	}, nil
}

// DeductImperialFavor implements settlementv1.SettlementServiceServer.
func (s *Service) DeductImperialFavor(ctx context.Context, req *settlementv1.DeductImperialFavorRequest) (*settlementv1.DeductImperialFavorResponse, error) {
	l := s.log.WithMethod("DeductImperialFavor").With(zap.String("settlement_id", req.GetSettlementId()), zap.Int64("amount", req.GetAmount()))

	if req.GetAmount() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be positive")
	}

	adminID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	updated, err := s.dbRepo.UpdateSettlement(ctx, req.GetSettlementId(),
		func(_ context.Context, settlement *model.Settlement) (*model.Settlement, error) {
			if err := settlement.DeductFavor(req.GetAmount()); err != nil {
				return nil, status.Error(codes.InvalidArgument, err.Error())
			}
			return settlement, nil
		},
	)
	if err != nil {
		l.Error("failed to deduct imperial favor", zap.Error(err))
		return nil, err
	}

	if err := s.dbRepo.CreateFavorLog(ctx, model.ImperialFavorLog{
		SettlementId: req.GetSettlementId(),
		AdminId:      adminID,
		Amount:       -req.GetAmount(),
		Reason:       req.GetReason(),
	}); err != nil {
		l.Error("failed to create favor log", zap.Error(err))
	}

	return &settlementv1.DeductImperialFavorResponse{
		Settlement: s.mapper.ToSettlementProto(*updated),
	}, nil
}

// ListImperialFavorLogs implements settlementv1.SettlementServiceServer.
func (s *Service) ListImperialFavorLogs(ctx context.Context, req *settlementv1.ListImperialFavorLogsRequest) (*settlementv1.ListImperialFavorLogsResponse, error) {
	l := s.log.WithMethod("ListImperialFavorLogs").With(zap.String("settlement_id", req.GetSettlementId()))

	logs, nextToken, err := s.dbRepo.ListFavorLogs(ctx, req.GetSettlementId(), req.GetAdminId(), req.GetOrderBy(), req.GetNextToken())
	if err != nil {
		l.Error("failed to list favor logs", zap.Error(err))
		return nil, err
	}

	return &settlementv1.ListImperialFavorLogsResponse{
		Logs:      s.mapper.ToImperialFavorLogsProto(logs),
		NextToken: nextToken,
	}, nil
}
