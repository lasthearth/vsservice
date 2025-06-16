package service

import (
	"bytes"
	"context"
	"fmt"
	"mime"

	"github.com/google/uuid"
	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Submit implements settlementv1.SettlementServiceServer
func (s *Service) Submit(ctx context.Context, req *settlementv1.SubmitRequest) (*settlementv1.SubmitResponse, error) {
	bucketName := "settlementsreq"
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

	attahs := make([]model.Attachment, len(req.Attachments))

	for _, attachment := range req.Attachments {
		uid, err := uuid.NewV7()
		if err != nil {
			return nil, err
		}

		rd := bytes.NewReader(attachment.Data)
		mimeType := mime.TypeByExtension(attachment.Ext)

		filename := fmt.Sprintf("%s%s", uid.String(), attachment.Ext)
		_, err = s.storage.UploadObject(
			ctx,
			bucketName,
			filename,
			rd,
			int64(len(attachment.Data)),
			mimeType,
		)
		if err != nil {
			return nil, err
		}

		url := fmt.Sprintf("%s/%s/%s", s.cfg.CdnUrl, bucketName, filename)

		attahs = append(attahs, model.Attachment{
			Url:      url,
			Desc:     attachment.Description,
			MimeType: mimeType,
		})
	}

	stype, err := TypeFromReqProto(req.Type)
	if err != nil {
		return nil, err
	}

	opts := SettlementOpts{
		Name: req.Name,
		Type: *stype,
		Leader: model.Member{
			UserId: userID,
		},
		Coordinates: model.Vector2{
			X: int(req.Coordinates.X),
			Y: int(req.Coordinates.Y),
		},
		Attachments: attahs,
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
		Settlement: s.mapper.ToSettlementProto(*settlement),
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

	return &settlementv1.ListResponse{
		Settlements: s.mapper.ToSettlementProtos(settlements),
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

	return &settlementv1.ListPendingResponse{
		Settlements: s.mapper.VerifsToSettlementProtos(settlements),
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

	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		s.log.Error("failed to get user id", zap.Error(err))
		return nil, err
	}

	if err := s.dbRepo.IsLeaderOfSettlement(ctx, req.SettlementId, uid); err != nil {
		s.log.Error("failed to check if user is leader", zap.Error(err))
		return nil, err
	}

	if err := s.dbRepo.InviteMember(ctx, req.SettlementId, req.UserId); err != nil {
		s.log.Error("failed to add member", zap.Error(err))
		return nil, err
	}

	return &settlementv1.InviteMemberResponse{}, nil
}

// GetInvitations implements settlementv1.SettlementServiceServer.
func (s *Service) GetInvitations(ctx context.Context, req *settlementv1.GetInvitationsRequest) (*settlementv1.GetInvitationsResponse, error) {
	l := s.log.
		With(zap.String("settlement_id", req.SettlementId)).
		WithMethod("get_invitations")

	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		l.Error("failed to get user id", zap.Error(err))
		return nil, err
	}

	if err := s.dbRepo.IsLeaderOfSettlement(ctx, req.SettlementId, uid); err != nil {
		l.Error("failed to check member or leader", zap.Error(err))
		return nil, err
	}

	invitations, err := s.dbRepo.GetInvitations(ctx, req.SettlementId)
	if err != nil {
		l.Error("failed to get invitations", zap.Error(err))
		return nil, err
	}

	return &settlementv1.GetInvitationsResponse{
		Invitations: s.mapper.ToInvProtos(invitations),
	}, nil
}

// RevokeInvitation implements settlementv1.SettlementServiceServer.
func (s *Service) RevokeInvitation(context.Context, *settlementv1.RevokeInvitationRequest) (*settlementv1.RevokeInvitationResponse, error) {
	panic("unimplemented")
}
