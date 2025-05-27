package service

import (
	"context"
	"slices"

	verificationv1 "github.com/lasthearth/vsservice/gen/verification/v1"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	httpdto "github.com/lasthearth/vsservice/internal/verification/internal/dto/http"
	"github.com/lasthearth/vsservice/internal/verification/model"
	"github.com/samber/lo"
)

type VerificationDbRepository interface {
	GetVerification(ctx context.Context, userID string) (*model.Verification, error)
	GetVerificationRequests(ctx context.Context) ([]*model.Verification, error)
	Approve(ctx context.Context, userId string) error
	Reject(ctx context.Context, userId, rejectReason string) error
	Create(ctx context.Context, opts VerifyOpts) error
	Update(ctx context.Context, opts VerifyOpts) error
}

type SsoRepository interface {
	GetUserRoles(ctx context.Context, userId string) ([]httpdto.Role, error)
	GetRoles(ctx context.Context) ([]httpdto.Role, error)
	UpdateUserRoles(ctx context.Context, userId string, roleIds []string) error
	UpdateUserProfileNick(ctx context.Context, userID, nickname string) error
}

// List implements verificationv1.VerificationService
func (s *Service) List(ctx context.Context, req *verificationv1.ListRequest) (*verificationv1.ListResponse, error) {
	reqs, err := s.dbRepo.GetVerificationRequests(ctx)
	if err != nil {
		return nil, err
	}

	resp := lo.Map(reqs, func(v *model.Verification, index int) *verificationv1.ListResponse_VerifyUserRequest {
		answers := lo.Map(v.Answers, func(a model.Answer, _ int) *verificationv1.Answer {
			return &verificationv1.Answer{
				Question: a.Question,
				Answer:   a.Answer,
			}
		})

		return &verificationv1.ListResponse_VerifyUserRequest{
			UserId:       v.UserID,
			UserName:     v.UserName,
			UserGameName: v.UserGameName,
			Contacts:     v.Contacts,
			Answers:      answers,
		}
	})

	return &verificationv1.ListResponse{
		Requests: resp,
	}, nil
}

// Approve implements verificationv1.VerificationServiceServer.
func (s *Service) Approve(ctx context.Context, req *verificationv1.ApproveRequest) (*verificationv1.ApproveResponse, error) {
	err := s.checkUserRoles(ctx, req.UserId)
	if err != nil {
		return nil, err
	}

	roles, err := s.ssoRepo.GetRoles(ctx)
	if err != nil {
		return nil, err
	}

	playerRoleId := ""
	for i := range roles {
		if roles[i].Name == "player" {
			playerRoleId = roles[i].ID
		}
	}

	err = s.ssoRepo.UpdateUserRoles(ctx, req.UserId, []string{playerRoleId})
	if err != nil {
		return nil, err
	}

	err = s.dbRepo.Approve(ctx, req.UserId)
	if err != nil {
		return nil, err
	}

	return &verificationv1.ApproveResponse{}, nil
}

// Reject implements verificationv1.VerificationServiceServer.
func (s *Service) Reject(ctx context.Context, req *verificationv1.RejectRequest) (*verificationv1.RejectResponse, error) {
	err := s.checkUserRoles(ctx, req.UserId)
	if err != nil {
		return nil, err
	}

	err = s.dbRepo.Reject(ctx, req.UserId, req.RejectionReason)
	if err != nil {
		return nil, err
	}

	return &verificationv1.RejectResponse{}, nil
}

// Submit implements verificationv1.VerificationServiceServer.
func (s *Service) Submit(ctx context.Context, req *verificationv1.SubmitRequest) (*verificationv1.SubmitResponse, error) {
	answers := lo.Map(req.Answers, func(v *verificationv1.Answer, _ int) model.Answer {
		return model.Answer{
			Question: v.Question,
			Answer:   v.Answer,
		}
	})

	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.ssoRepo.UpdateUserProfileNick(ctx, userID, req.UserGameName); err != nil {
		return nil, err
	}

	// Check if verification already exists
	existingVerification, err := s.dbRepo.GetVerification(ctx, userID)
	if err != nil {
		return nil, err
	}

	verifyOpts := VerifyOpts{
		UserID:       userID,
		UserName:     req.UserName,
		UserGameName: req.UserGameName,
		Contacts:     req.Contacts,
		Answers:      answers,
	}

	// If no verification exists, create a new one
	if existingVerification == nil {
		if err := s.dbRepo.Create(ctx, verifyOpts); err != nil {
			return nil, err
		}
		return &verificationv1.SubmitResponse{}, nil
	}

	// Handle based on existing verification status
	switch existingVerification.Status {
	case model.VerificationStatusPending:
		// If pending, return error
		return nil, ErrVerificationPending
	case model.VerificationStatusRejected:
		// If rejected, update existing verification
		if err := s.dbRepo.Update(ctx, verifyOpts); err != nil {
			return nil, err
		}
	default:
		// For other statuses (approved, verified), create new verification
		if err := s.dbRepo.Create(ctx, verifyOpts); err != nil {
			return nil, err
		}
	}

	return &verificationv1.SubmitResponse{}, nil
}

// Details implements verificationv1.VerificationServiceServer.
func (s *Service) Details(ctx context.Context, req *verificationv1.DetailsRequest) (*verificationv1.DetailsResponse, error) {
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	verif, err := s.dbRepo.GetVerification(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &verificationv1.DetailsResponse{
		Id:              verif.ID,
		Status:          string(verif.Status),
		RejectionReason: verif.RejectionReason,
	}, nil
}

func (s *Service) checkUserRoles(ctx context.Context, userId string) error {
	ssoRoleName := "player"

	roles, err := s.ssoRepo.GetUserRoles(ctx, userId)
	if err != nil {
		return err
	}

	isVerified := slices.ContainsFunc(roles, func(role httpdto.Role) bool {
		return role.Name == ssoRoleName
	})
	if isVerified {
		return ErrAlreadyVerified
	}

	return nil
}
