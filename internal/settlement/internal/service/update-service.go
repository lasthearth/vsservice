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

// UpdateSettlement implements settlementv1.SettlementServiceServer.
func (s *Service) UpdateSettlement(ctx context.Context, req *settlementv1.UpdateSettlementRequest) (*settlementv1.UpdateSettlementResponse, error) {
	l := s.log.WithMethod("UpdateSettlement").With(zap.String("id", req.GetId()))

	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	if len(req.GetAttachments()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "attachments cannot be empty")
	}

	if err := s.dbRepo.IsLeaderOfSettlement(ctx, req.GetId(), uid); err != nil {
		l.Error("permission check failed", zap.Error(err))
		return nil, status.Error(codes.PermissionDenied, "user is not leader")
	}

	attachs := make([]model.Attachment, len(req.GetAttachments()))
	for i, a := range req.GetAttachments() {
		if err := s.mediaUrl.Validate(a.GetUrl()); err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid attachment url")
		}
		attachs[i] = model.Attachment{Url: a.GetUrl(), Desc: a.GetDescription()}
	}

	updated, err := s.dbRepo.UpdateSettlement(ctx, req.GetId(),
		func(_ context.Context, settlement *model.Settlement) (*model.Settlement, error) {
			settlement.SetProfile(req.GetName(), req.GetDescription(), attachs)
			return settlement, nil
		},
	)
	if err != nil {
		l.Error("failed to update settlement", zap.Error(err))
		return nil, err
	}

	return &settlementv1.UpdateSettlementResponse{
		Settlement: s.mapper.ToSettlementProto(*updated),
	}, nil
}
