package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime"

	"github.com/google/uuid"
	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	"github.com/lasthearth/vsservice/internal/pkg/image"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/settlement/internal/ierror"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Submit implements settlementv1.SettlementServiceServer
func (s *Service) Submit(ctx context.Context, req *settlementv1.SubmitRequest) (*settlementv1.SubmitResponse, error) {
	bucketName := "settlementsreq"
	fileExt := ".webp"
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	if req.Attachments == nil {
		return nil, status.Error(codes.InvalidArgument, "attachments cannot be empty")
	}

	s.log.Info("submitting settlement request",
		zap.String("leader_id", userID),
		zap.String("settlement_name", req.Name),
		zap.Int("attachments", len(req.Attachments)))

	stype, err := TypeFromReqProto(req.Type)
	if err != nil {
		return nil, err
	}

	if err := s.dbRepo.IsMemberOrLeader(ctx, "", userID); err != nil {
		s.log.Error("user validation failed", zap.Error(err), zap.String("user_id", userID))
		if err != ierror.ErrAlreadyMember {
			return nil, err
		}
	}

	attachs := make([]model.Attachment, len(req.Attachments))

	s.log.Debug("attach len", zap.Int("attachments", len(attachs)))

	for i, attachment := range req.Attachments {
		uid, err := uuid.NewV7()
		if err != nil {
			return nil, err
		}

		webp, err := image.ConvertToWebp(attachment.Data)
		if err != nil {
			return nil, err
		}

		mimeType := mime.TypeByExtension(fileExt)
		rd := bytes.NewReader(webp)

		filename := fmt.Sprintf("%s%s", uid.String(), fileExt)
		_, err = s.storage.UploadObject(
			ctx,
			bucketName,
			filename,
			rd,
			int64(len(webp)),
			mimeType,
		)
		if err != nil {
			return nil, err
		}

		url := fmt.Sprintf("%s/%s/%s", s.cfg.CdnUrl, bucketName, filename)

		attachs[i] = model.Attachment{
			Url:      url,
			Desc:     attachment.Description,
			MimeType: mimeType,
		}
	}

	opts := SettlementOpts{
		Name: req.Name,
		Type: *stype,
		Leader: model.Member{
			UserId: userID,
		},
		Description: req.Description,
		Diplomacy:   req.Diplomacy,
		Coordinates: model.Vector2{
			X: int(req.Coordinates.X),
			Y: int(req.Coordinates.Y),
		},
		Attachments: attachs,
	}

	found, err := s.dbRepo.GetSettlementRequestByLeader(ctx, userID)
	if err != nil {
		// If no existing request found, create a new one
		if errors.Is(err, ierror.ErrNotFound) {
			if err := s.dbRepo.CreateRequest(ctx, opts); err != nil {
				s.log.Error("failed to create new settlement request", zap.Error(err))
				return nil, err
			}

			return &settlementv1.SubmitResponse{}, nil
		}

		s.log.Error("error checking for existing settlement request", zap.Error(err))
		return nil, err
	}

	// User already has a request, handle level up or update
	if found.Status == model.SettlementStatusPending {
		s.log.Info("request already submitted", zap.String("user_id", userID))
		return nil, status.Error(codes.AlreadyExists, "settlement request already pending")
	}

	if found.Status == model.SettlementStatusApproved {
		found.LvlUp()
		s.log.Debug("settlement level up",
			zap.String("before", string(opts.Type)),
			zap.String("after", string(found.Type)),
		)
	}
	
	opts.Type = found.Type
	if err := s.dbRepo.UpdateRequest(ctx, opts); err != nil {
		s.log.Error("failed to update settlement request", zap.Error(err))
		return nil, err
	}

	s.log.Info("settlement req created", zap.Int("attachment", len(attachs)))

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
		return nil, status.Error(codes.NotFound, ierror.ErrNotFound.Error())
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
		return nil, ierror.ErrNotFound
	}

	if settlement.Status == model.SettlementStatusApproved {
		return nil, ierror.ErrAlreadyApproved
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
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err := s.dbRepo.IsLeaderOfSettlement(ctx, req.SettlementId, uid); err != nil {
		s.log.Error("failed to check if user is leader", zap.Error(err))
		return nil, status.Error(codes.PermissionDenied, "user is not leader")
	}

	if err := s.dbRepo.IsMemberOrLeader(ctx, req.SettlementId, req.UserId); err != nil {
		s.log.Error("user validation failed", zap.Error(err), zap.String("user_id", req.UserId))
		if err != ierror.ErrAlreadyMember {
			return nil, err
		}
	}

	if err := s.dbRepo.CreateInvitation(ctx, req.SettlementId, req.UserId); err != nil {
		s.log.Error("failed to add member", zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
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
	l = l.With(zap.String("user_id", uid))

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
func (s *Service) RevokeInvitation(ctx context.Context, req *settlementv1.RevokeInvitationRequest) (*settlementv1.RevokeInvitationResponse, error) {
	l := s.log.With(zap.String("method", "RevokeInvitation"), zap.String("settlement_id", req.SettlementId), zap.String("invitation_id", req.InvitationId))
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		l.Error("failed to get user id", zap.Error(err))
		return nil, err
	}
	l = l.With(zap.String("user_id", uid))

	if err := s.dbRepo.IsLeaderOfSettlement(ctx, req.SettlementId, uid); err != nil {
		l.Error("failed to check member or leader", zap.Error(err))
		return nil, err
	}

	if err := s.dbRepo.DeleteInvitationForLeader(ctx, req.InvitationId, req.SettlementId); err != nil {
		s.log.Error("failed to revoke invitation", zap.Error(err))
		return nil, err
	}

	return &settlementv1.RevokeInvitationResponse{}, nil
}

// AcceptInvitation implements settlementv1.SettlementServiceServer.
func (s *Service) AcceptInvitation(ctx context.Context, req *settlementv1.AcceptInvitationRequest) (*settlementv1.AcceptInvitationResponse, error) {
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		s.log.Error("failed to get user id", zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}

	// TODO: утекла бизнесуха в репу, придумать как вынести транкзакции и отрефакторить репу
	if err := s.dbRepo.AcceptInvitation(ctx, req.InvitationId, uid); err != nil {
		s.log.Error("failed to accept invitation", zap.Error(err))
		return nil, err
	}

	return &settlementv1.AcceptInvitationResponse{}, nil
}

// RejectInvitation implements settlementv1.SettlementServiceServer.
func (s *Service) RejectInvitation(ctx context.Context, req *settlementv1.RejectInvitationRequest) (*settlementv1.RejectInvitationResponse, error) {
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		s.log.Error("failed to get user id", zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}

	invs, err := s.dbRepo.GetUserInvitations(ctx, uid)
	if err != nil {
		s.log.Error("failed to reject invitation", zap.Error(err))
		return nil, err
	}

	ids := lo.Map(invs, func(item model.Invitation, _ int) string {
		return item.Id
	})
	if !lo.Contains(ids, req.InvitationId) {
		return nil, status.Errorf(codes.NotFound, "invitation for user %s not found", uid)
	}

	if err := s.dbRepo.DeleteInvitationForUser(ctx, req.InvitationId, uid); err != nil {
		s.log.Error("failed to delete invitation", zap.Error(err))
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &settlementv1.RejectInvitationResponse{}, nil
}

// GetUserInvitations implements settlementv1.SettlementServiceServer.
func (s *Service) GetUserInvitations(ctx context.Context, req *settlementv1.GetUserInvitationsRequest) (*settlementv1.GetUserInvitationsResponse, error) {
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		s.log.Error("failed to get user id", zap.Error(err))
		return nil, err
	}

	if uid != req.UserId {
		return nil, status.Errorf(codes.PermissionDenied, "user id mismatch")
	}

	invitations, err := s.dbRepo.GetUserInvitations(ctx, req.UserId)
	if err != nil {
		s.log.Error("failed to get user invitations", zap.Error(err))
		return nil, err
	}

	return &settlementv1.GetUserInvitationsResponse{
		Invitations: s.mapper.ToInvProtos(invitations),
	}, nil
}

// GetByUserId implements settlementv1.SettlementServiceServer.
func (s *Service) GetByUserId(ctx context.Context, req *settlementv1.GetByUserIdRequest) (*settlementv1.GetByUserIdResponse, error) {
	settlement, err := s.dbRepo.GetSettlementByUserId(ctx, req.UserId)
	if err != nil {
		s.log.Error("failed to get settlements", zap.Error(err))
		return nil, err
	}

	return &settlementv1.GetByUserIdResponse{
		Settlement: s.mapper.ToSettlementProto(*settlement),
	}, nil
}

// VerificationStatus implements settlementv1.SettlementServiceServer.
func (s *Service) VerificationStatus(ctx context.Context, req *settlementv1.VerificationStatusRequest) (*settlementv1.VerificationStatusResponse, error) {
	sreq, err := s.dbRepo.GetSettlementRequestByLeader(ctx, req.UserId)
	if err != nil {
		s.log.Error("failed to get request status", zap.Error(err))
		return nil, err
	}

	return &settlementv1.VerificationStatusResponse{
		Status:          string(sreq.Status),
		RejectionReason: sreq.RejectionReason,
	}, nil
}
