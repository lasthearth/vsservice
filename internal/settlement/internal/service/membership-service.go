package service

import (
	"context"
	"errors"

	settlementv1 "github.com/lasthearth/vsservice/gen/settlement/v1"
	"github.com/lasthearth/vsservice/internal/notification/notificationuc"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/settlement/internal/ierror"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// maxActiveJoinRequests caps how many settlements a player may apply to at once.
const maxActiveJoinRequests = 3

// CreateJoinRequest implements settlementv1.SettlementServiceServer.
func (s *Service) CreateJoinRequest(ctx context.Context, req *settlementv1.CreateJoinRequestRequest) (*settlementv1.CreateJoinRequestResponse, error) {
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Applicant must not already belong to a settlement.
	if err := s.dbRepo.IsMemberOfAnySettlement(ctx, uid); err != nil {
		return nil, err
	}

	if _, err := s.dbRepo.GetSettlement(ctx, req.GetSettlementId()); err != nil {
		return nil, err
	}

	count, err := s.dbRepo.CountUserJoinRequests(ctx, uid)
	if err != nil {
		return nil, err
	}
	if count >= maxActiveJoinRequests {
		return nil, ierror.ErrJoinRequestLimit
	}

	if err := s.dbRepo.CreateJoinRequest(ctx, req.GetSettlementId(), uid); err != nil {
		return nil, err
	}
	return &settlementv1.CreateJoinRequestResponse{}, nil
}

// CancelJoinRequest implements settlementv1.SettlementServiceServer.
func (s *Service) CancelJoinRequest(ctx context.Context, req *settlementv1.CancelJoinRequestRequest) (*settlementv1.CancelJoinRequestResponse, error) {
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.dbRepo.DeleteJoinRequestForUser(ctx, req.GetJoinRequestId(), uid); err != nil {
		return nil, err
	}
	return &settlementv1.CancelJoinRequestResponse{}, nil
}

// GetMyJoinRequests implements settlementv1.SettlementServiceServer.
func (s *Service) GetMyJoinRequests(ctx context.Context, req *settlementv1.GetMyJoinRequestsRequest) (*settlementv1.GetMyJoinRequestsResponse, error) {
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}
	if uid != req.GetUserId() {
		return nil, status.Error(codes.PermissionDenied, "user id mismatch")
	}
	reqs, err := s.dbRepo.GetUserJoinRequests(ctx, uid)
	if err != nil {
		return nil, err
	}
	return &settlementv1.GetMyJoinRequestsResponse{
		JoinRequests: s.mapper.ToJoinRequestsProto(reqs),
	}, nil
}

// ListJoinRequests implements settlementv1.SettlementServiceServer.
func (s *Service) ListJoinRequests(ctx context.Context, req *settlementv1.ListJoinRequestsRequest) (*settlementv1.ListJoinRequestsResponse, error) {
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.requirePermission(ctx, req.GetSettlementId(), uid, model.PermReviewJoinRequest); err != nil {
		return nil, err
	}
	reqs, err := s.dbRepo.GetJoinRequests(ctx, req.GetSettlementId())
	if err != nil {
		return nil, err
	}
	return &settlementv1.ListJoinRequestsResponse{
		JoinRequests: s.mapper.ToJoinRequestsProto(reqs),
	}, nil
}

// ApproveJoinRequest implements settlementv1.SettlementServiceServer.
func (s *Service) ApproveJoinRequest(ctx context.Context, req *settlementv1.ApproveJoinRequestRequest) (*settlementv1.ApproveJoinRequestResponse, error) {
	l := s.log.WithMethod("ApproveJoinRequest").With(zap.String("settlement_id", req.GetSettlementId()))

	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.requirePermission(ctx, req.GetSettlementId(), uid, model.PermReviewJoinRequest); err != nil {
		return nil, err
	}

	// Guard: the request must belong to this settlement.
	jr, err := s.dbRepo.GetJoinRequest(ctx, req.GetJoinRequestId())
	if err != nil {
		return nil, err
	}
	if jr.SettlementId != req.GetSettlementId() {
		return nil, ierror.ErrNotFound
	}

	applicantID, err := s.dbRepo.ApproveJoinRequest(ctx, req.GetJoinRequestId())
	if err != nil {
		return nil, err
	}

	// Notify the applicant. Failure must not undo the join, so it is logged and
	// swallowed — mirrors the news domain.
	set, gerr := s.dbRepo.GetSettlement(ctx, req.GetSettlementId())
	name := req.GetSettlementId()
	if gerr == nil {
		name = set.Name
	}
	if nerr := s.notifier.CreateNotification(ctx,
		"Заявка одобрена",
		"Вы приняты в поселение "+name,
		notificationuc.WithUserId(applicantID),
	); nerr != nil {
		l.Warn("failed to send join-approved notification", zap.Error(nerr), zap.String("user_id", applicantID))
	}

	return &settlementv1.ApproveJoinRequestResponse{}, nil
}

// RejectJoinRequest implements settlementv1.SettlementServiceServer.
func (s *Service) RejectJoinRequest(ctx context.Context, req *settlementv1.RejectJoinRequestRequest) (*settlementv1.RejectJoinRequestResponse, error) {
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.requirePermission(ctx, req.GetSettlementId(), uid, model.PermReviewJoinRequest); err != nil {
		return nil, err
	}
	if err := s.dbRepo.DeleteJoinRequest(ctx, req.GetJoinRequestId(), req.GetSettlementId()); err != nil {
		return nil, err
	}
	return &settlementv1.RejectJoinRequestResponse{}, nil
}

// LeaveSettlement implements settlementv1.SettlementServiceServer.
func (s *Service) LeaveSettlement(ctx context.Context, req *settlementv1.LeaveSettlementRequest) (*settlementv1.LeaveSettlementResponse, error) {
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	_, err = s.dbRepo.UpdateSettlement(ctx, req.GetSettlementId(),
		func(_ context.Context, set *model.Settlement) (*model.Settlement, error) {
			if err := set.RemoveMember(uid); err != nil {
				return nil, mapModelErr(err)
			}
			return set, nil
		},
	)
	if err != nil {
		return nil, err
	}
	return &settlementv1.LeaveSettlementResponse{}, nil
}

// UpdateContactInfo implements settlementv1.SettlementServiceServer.
func (s *Service) UpdateContactInfo(ctx context.Context, req *settlementv1.UpdateContactInfoRequest) (*settlementv1.UpdateContactInfoResponse, error) {
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.requireOwner(ctx, req.GetSettlementId(), uid); err != nil {
		return nil, err
	}

	updated, err := s.dbRepo.UpdateSettlement(ctx, req.GetSettlementId(),
		func(_ context.Context, set *model.Settlement) (*model.Settlement, error) {
			if err := set.SetContactInfo(req.GetContactInfo()); err != nil {
				return nil, status.Error(codes.InvalidArgument, err.Error())
			}
			return set, nil
		},
	)
	if err != nil {
		return nil, err
	}
	return &settlementv1.UpdateContactInfoResponse{Settlement: s.mapper.ToSettlementProto(*updated)}, nil
}

// TransferOwnership implements settlementv1.SettlementServiceServer.
func (s *Service) TransferOwnership(ctx context.Context, req *settlementv1.TransferOwnershipRequest) (*settlementv1.TransferOwnershipResponse, error) {
	uid, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := s.requireOwner(ctx, req.GetSettlementId(), uid); err != nil {
		return nil, err
	}

	updated, err := s.dbRepo.UpdateSettlement(ctx, req.GetSettlementId(),
		func(_ context.Context, set *model.Settlement) (*model.Settlement, error) {
			if err := set.TransferOwnership(uid, req.GetToUserId()); err != nil {
				return nil, mapModelErr(err)
			}
			return set, nil
		},
	)
	if err != nil {
		return nil, err
	}
	return &settlementv1.TransferOwnershipResponse{Settlement: s.mapper.ToSettlementProto(*updated)}, nil
}

// mapModelErr converts a settlement-model business error into a typed domain
// error so the interceptor maps it to the right gRPC code.
func mapModelErr(err error) error {
	switch {
	case errors.Is(err, model.ErrLastOwner):
		return ierror.ErrLastOwner
	case errors.Is(err, model.ErrRolesDisabled):
		return ierror.ErrRolesDisabled
	case errors.Is(err, model.ErrRoleLimit):
		return ierror.ErrRoleLimit
	case errors.Is(err, model.ErrRoleNotFound):
		return ierror.ErrRoleNotFound
	case errors.Is(err, model.ErrRoleNameInvalid), errors.Is(err, model.ErrContactInvalid):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, model.ErrMemberNotFound), errors.Is(err, model.ErrTargetNotMember):
		return ierror.ErrMemberNotFound
	case errors.Is(err, model.ErrAlreadyOwner):
		return ierror.ErrAlreadyOwner
	case errors.Is(err, model.ErrNotOwnerRole):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return err
	}
}
