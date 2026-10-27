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
		s.opFor(ctx).Event(opFavorTransferred).
			Entity(entitySettlement, req.GetFromSettlementId()).
			Field(
				zap.String("to_settlement_id", req.GetToSettlementId()),
				zap.Int64("amount", req.GetAmount()),
				zap.String("currency", favor),
			).
			Error("failed to deduct favor from source", err)
		return nil, err
	}

	to, err := s.dbRepo.UpdateSettlement(ctx, req.GetToSettlementId(),
		func(_ context.Context, s *model.Settlement) (*model.Settlement, error) {
			s.AddFavor(req.GetAmount())
			return s, nil
		},
	)
	if err != nil {
		// The source is already deducted and the two writes are not atomic, so
		// this record is the only trace of favor that left one settlement and
		// never reached the other: it says so in the reason.
		s.opFor(ctx).Event(opFavorTransferred).
			Entity(entitySettlement, req.GetFromSettlementId()).
			Field(
				zap.String("to_settlement_id", req.GetToSettlementId()),
				zap.Int64("amount", req.GetAmount()),
				zap.Int64("from_balance", from.ImperialFavor),
				zap.String("currency", favor),
			).
			Reason("the source was deducted and crediting the target failed").
			Error("failed to add favor to target", err)
		return nil, err
	}

	// Both balances moved, so the transfer is the fact to keep: one record, with
	// what each side ended on.
	s.opFor(ctx).Event(opFavorTransferred).
		Entity(entitySettlement, req.GetFromSettlementId()).
		Field(
			zap.String("to_settlement_id", req.GetToSettlementId()),
			zap.Int64("amount", req.GetAmount()),
			zap.Int64("from_balance", from.ImperialFavor),
			zap.Int64("to_balance", to.ImperialFavor),
			zap.String("currency", favor),
		).
		Info("imperial favor transferred")

	// The ledger rows report the transfer; a missing one costs a report line,
	// not the favor, so the failure is logged and the transfer still answers.
	if err := s.dbRepo.CreateFavorLog(ctx, model.ImperialFavorLog{
		SettlementId: req.GetFromSettlementId(),
		AdminId:      callerID,
		Amount:       -req.GetAmount(),
		Reason:       "transfer to " + req.GetToSettlementId(),
	}); err != nil {
		l.Warn("failed to write the source favor ledger row", zap.Error(err))
	}
	if err := s.dbRepo.CreateFavorLog(ctx, model.ImperialFavorLog{
		SettlementId: req.GetToSettlementId(),
		AdminId:      callerID,
		Amount:       req.GetAmount(),
		Reason:       "transfer from " + req.GetFromSettlementId(),
	}); err != nil {
		l.Warn("failed to write the target favor ledger row", zap.Error(err))
	}

	fromProto, err := s.settlementProto(ctx, *from)
	if err != nil {
		return nil, err
	}
	toProto, err := s.settlementProto(ctx, *to)
	if err != nil {
		return nil, err
	}
	return &settlementv1.TransferImperialFavorResponse{
		FromSettlement: fromProto,
		ToSettlement:   toProto,
	}, nil
}
