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

// TransferImperialFavor implements settlementv1.SettlementServiceServer.
func (s *Service) TransferImperialFavor(ctx context.Context, req *settlementv1.TransferImperialFavorRequest) (*settlementv1.TransferImperialFavorResponse, error) {
	l := s.log.WithMethod("TransferImperialFavor").
		With(zap.String("from", req.GetFromSettlementId()),
			zap.String("to", req.GetToSettlementId()),
			zap.Int64("amount", req.GetAmount()))

	if req.GetAmount() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be positive")
	}
	if req.GetFromSettlementId() == req.GetToSettlementId() {
		return nil, status.Error(codes.InvalidArgument, "from and to must differ")
	}

	callerID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.dbRepo.IsLeaderOfSettlement(ctx, req.GetFromSettlementId(), callerID); err != nil {
		return nil, status.Error(codes.PermissionDenied, "caller is not the leader of the source settlement")
	}

	from, err := s.dbRepo.UpdateSettlement(ctx, req.GetFromSettlementId(),
		func(_ context.Context, s *model.Settlement) (*model.Settlement, error) {
			return s, s.DeductFavor(req.GetAmount())
		},
	)
	if err != nil {
		l.Error("failed to deduct favor from source", zap.Error(err))
		return nil, err
	}

	to, err := s.dbRepo.UpdateSettlement(ctx, req.GetToSettlementId(),
		func(_ context.Context, s *model.Settlement) (*model.Settlement, error) {
			s.AddFavor(req.GetAmount())
			return s, nil
		},
	)
	if err != nil {
		l.Error("failed to add favor to target (deduction already applied)", zap.Error(err))
		return nil, err
	}

	_ = s.dbRepo.CreateFavorLog(ctx, model.ImperialFavorLog{
		SettlementId: req.GetFromSettlementId(),
		AdminId:      callerID,
		Amount:       -req.GetAmount(),
		Reason:       "transfer to " + req.GetToSettlementId(),
	})
	_ = s.dbRepo.CreateFavorLog(ctx, model.ImperialFavorLog{
		SettlementId: req.GetToSettlementId(),
		AdminId:      callerID,
		Amount:       req.GetAmount(),
		Reason:       "transfer from " + req.GetFromSettlementId(),
	})

	return &settlementv1.TransferImperialFavorResponse{
		FromSettlement: s.mapper.ToSettlementProto(*from),
		ToSettlement:   s.mapper.ToSettlementProto(*to),
	}, nil
}
