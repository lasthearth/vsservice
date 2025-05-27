package service

import (
	"context"

	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Submit implements settlementv1.SettlementServiceServer
func (s *Service) Submit(ctx context.Context, req *settlementv1.SubmitRequest) (*settlementv1.SubmitResponse, error) {
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	if req.Attachments == nil {
		return nil, status.Error(codes.InvalidArgument, "attachments cannot be empty")
	}

	s.log.Info("submitting settlement request",
		zap.String("leader_id", userID),
		zap.String("settlement_name", req.Name))

	attachments := lo.Map(req.Attachments, func(item *settlementv1.Attachment, index int) model.Attachment {
		return model.Attachment{
			Url:      item.Url,
			Desc:     item.Desc,
			MimeType: item.MimeType,
		}
	})

	opts := SettlementOpts{
		Name: req.Name,
		Type: model.SettlementTypeVillage,
		Leader: model.Member{
			UserID: userID,
		},
		Coordinates: model.Vector2{
			X: int(req.Coordinates.X),
			Y: int(req.Coordinates.Y),
		},
		Attachments: attachments,
	}

	if err := s.dbRepo.Submit(ctx, opts); err != nil {
		s.log.Error("failed to create settlement", zap.Error(err))
		return nil, err
	}

	return &settlementv1.SubmitResponse{}, nil
}

// Get implements settlementv1.SettlementServiceServer
func (s *Service) Get(ctx context.Context, req *settlementv1.GetRequest) (*settlementv1.GetResponse, error) {
	s.log.Info("getting settlement details", zap.String("settlement_id", req.Id))

	settlement, err := s.dbRepo.GetSettlement(ctx, req.Id)
	if err != nil {
		s.log.Error("failed to get settlement", zap.Error(err))
		return nil, err
	}

	if settlement == nil {
		return nil, ErrSettlementNotFound
	}

	return &settlementv1.GetResponse{
		Settlement: convertSettlementToProto(settlement),
	}, nil
}

// List implements settlementv1.SettlementServiceServer
func (s *Service) List(ctx context.Context, req *settlementv1.ListRequest) (*settlementv1.ListResponse, error) {
	s.log.Info("listing all settlements")

	settlements, err := s.dbRepo.GetAllSettlements(ctx)
	if err != nil {
		s.log.Error("failed to list settlements", zap.Error(err))
		return nil, err
	}

	protoSettlements := lo.Map(settlements, func(s *model.Settlement, _ int) *settlementv1.Settlement {
		return convertSettlementToProto(s)
	})

	return &settlementv1.ListResponse{
		Settlements: protoSettlements,
	}, nil
}

// ListPending implements settlementv1.SettlementServiceServer
func (s *Service) ListPending(ctx context.Context, req *settlementv1.ListPendingRequest) (*settlementv1.ListPendingResponse, error) {
	s.log.Info("listing pending settlement requests")

	settlements, err := s.dbRepo.GetPendingSettlements(ctx)
	if err != nil {
		s.log.Error("failed to list pending settlements", zap.Error(err))
		return nil, err
	}

	protoSettlements := lo.Map(settlements, func(s *model.SettlementVerification, _ int) *settlementv1.Settlement {
		return convertVerificationToProto(s)
	})

	return &settlementv1.ListPendingResponse{
		Settlements: protoSettlements,
	}, nil
}

// Approve implements settlementv1.SettlementServiceServer
func (s *Service) Approve(ctx context.Context, req *settlementv1.ApproveRequest) (*settlementv1.ApproveResponse, error) {
	s.log.Info("approving settlement request", zap.String("settlement_id", req.Id))

	settlement, err := s.dbRepo.GetSettlementRequest(ctx, req.Id)
	if err != nil {
		s.log.Error("failed to get settlement", zap.Error(err))
		return nil, err
	}

	if settlement == nil {
		return nil, ErrSettlementNotFound
	}

	if settlement.Status == model.SettlementStatusApproved {
		return nil, ErrAlreadyApproved
	}

	if err := s.dbRepo.Approve(ctx, req.Id); err != nil {
		s.log.Error("failed to approve settlement", zap.Error(err))
		return nil, err
	}

	return &settlementv1.ApproveResponse{}, nil
}

// Reject implements settlementv1.SettlementServiceServer
func (s *Service) Reject(ctx context.Context, req *settlementv1.RejectRequest) (*settlementv1.RejectResponse, error) {
	s.log.Info("rejecting settlement request",
		zap.String("settlement_id", req.Id),
		zap.String("rejection_reason", req.RejectionReason))

	if err := s.dbRepo.Reject(ctx, req.Id, req.RejectionReason); err != nil {
		s.log.Error("failed to reject settlement", zap.Error(err))
		return nil, err
	}

	return &settlementv1.RejectResponse{}, nil
}

// RemoveMember implements settlementv1.SettlementServiceServer
func (s *Service) RemoveMember(ctx context.Context, req *settlementv1.RemoveMemberRequest) (*settlementv1.RemoveMemberResponse, error) {
	s.log.Info("removing member from settlement",
		zap.String("settlement_id", req.SettlementId),
		zap.String("user_id", req.UserId))

	if err := s.dbRepo.RemoveMember(ctx, req.SettlementId, req.UserId); err != nil {
		s.log.Error("failed to remove member", zap.Error(err))
		return nil, err
	}

	return &settlementv1.RemoveMemberResponse{}, nil
}

// InviteMember implements settlementv1.SettlementServiceServer
func (s *Service) InviteMember(ctx context.Context, req *settlementv1.InviteMemberRequest) (*settlementv1.InviteMemberResponse, error) {
	s.log.Info("inviting member to settlement",
		zap.String("settlement_id", req.SettlementId),
		zap.String("user_id", req.UserId))

	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Check if requester is the leader of the settlement
	settlement, err := s.dbRepo.GetSettlement(ctx, req.SettlementId)
	if err != nil {
		s.log.Error("failed to get settlement", zap.Error(err))
		return nil, err
	}

	if settlement == nil {
		return nil, ErrSettlementNotFound
	}

	if settlement.Leader.UserID != userID {
		return nil, status.Error(codes.Aborted, ErrPermissionDenied.Error())
	}

	member := model.Member{
		UserID: req.UserId,
	}

	if err := s.dbRepo.AddMember(ctx, req.SettlementId, member); err != nil {
		s.log.Error("failed to add member", zap.Error(err))
		return nil, err
	}

	return &settlementv1.InviteMemberResponse{}, nil
}

func convertVerificationToProto(s *model.SettlementVerification) *settlementv1.Settlement {
	var stype settlementv1.SettlementType

	switch s.Type {
	case model.SettlementTypeProvince:
		stype = settlementv1.SettlementType_SETTLEMENT_TYPE_PROVINCE
	case model.SettlementTypeCity:
		stype = settlementv1.SettlementType_SETTLEMENT_TYPE_CITY
	case model.SettlementTypeVillage:
		stype = settlementv1.SettlementType_SETTLEMENT_TYPE_VILLAGE
	default:
		stype = settlementv1.SettlementType_SETTLEMENT_TYPE_UNSPECIFIED
	}

	return &settlementv1.Settlement{
		Id:   s.Id,
		Name: s.Name,
		Type: stype,
		Leader: &settlementv1.Member{
			UserId: s.Leader.UserID,
		},
		Members: nil,
		Coordinates: &settlementv1.Vector2{
			X: int32(s.Coordinates.X),
			Y: int32(s.Coordinates.Y),
		},
		CreatedAt: s.CreatedAt.Unix(),
		UpdatedAt: s.UpdatedAt.Unix(),
	}
}

func convertSettlementToProto(s *model.Settlement) *settlementv1.Settlement {
	members := lo.Map(s.Members, func(m model.Member, _ int) *settlementv1.Member {
		return &settlementv1.Member{
			UserId: m.UserID,
		}
	})

	var stype settlementv1.SettlementType

	switch s.Type {
	case model.SettlementTypeProvince:
		stype = settlementv1.SettlementType_SETTLEMENT_TYPE_PROVINCE
	case model.SettlementTypeCity:
		stype = settlementv1.SettlementType_SETTLEMENT_TYPE_CITY
	case model.SettlementTypeVillage:
		stype = settlementv1.SettlementType_SETTLEMENT_TYPE_VILLAGE
	default:
		stype = settlementv1.SettlementType_SETTLEMENT_TYPE_UNSPECIFIED
	}

	return &settlementv1.Settlement{
		Id:   s.ID,
		Name: s.Name,
		Type: stype,
		Leader: &settlementv1.Member{
			UserId: s.Leader.UserID,
		},
		Members: members,
		Coordinates: &settlementv1.Vector2{
			X: int32(s.Coordinates.X),
			Y: int32(s.Coordinates.Y),
		},
		CreatedAt: s.CreatedAt.Unix(),
		UpdatedAt: s.UpdatedAt.Unix(),
	}
}
