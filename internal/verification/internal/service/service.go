package service

import (
	"context"
	"slices"

	rulesv1 "github.com/lasthearth/vsservice/gen/rules/v1"
	verificationv1 "github.com/lasthearth/vsservice/gen/verification/v1"
	"github.com/lasthearth/vsservice/internal/rules/model"
	httpdto "github.com/lasthearth/vsservice/internal/verification/internal/dto/http"
	"github.com/samber/lo"
)

type DbRepository interface {
	GetVerificationRequests(ctx context.Context) ([]*model.Verification, error)
	ApproveVerificationRequest(ctx context.Context, userId string) error
}

type SsoRepository interface {
	GetUserRoles(ctx context.Context, userId string) ([]httpdto.Role, error)
	GetRoles(ctx context.Context) ([]httpdto.Role, error)
	UpdateUserRoles(ctx context.Context, userId string, roleIds []string) error
}

// ListVerificationRequests implements rulesv1.RuleServiceServer
func (s *Service) ListVerificationRequests(ctx context.Context, req *verificationv1.ListRequest) (*verificationv1.ListResponse, error) {
	reqs, err := s.dbRepo.GetVerificationRequests(ctx)
	if err != nil {
		return nil, err
	}

	resp := lo.Map(reqs, func(v *model.Verification, index int) *verificationv1.Answer {
		answers := lo.Map(v.Answers, func(a model.Answer, _ int) *verificationv1.Answer {
			return &verificationv1.Answer{
				Question: a.Question,
				Answer:   a.Answer,
			}
		})

		return &rulesv1.ListVerificationRequestsResponse_VerifyUserRequest{
			UserId:       v.UserID,
			UserName:     v.UserName,
			UserGameName: v.UserGameName,
			Contacts:     v.Contacts,
			Answers:      answers,
		}
	})

	return &rulesv1.ListVerificationRequestsResponse{
		Requests: resp,
	}, nil
}

// VerifyRequest implements rulesv1.RuleServiceServer.
func (s *Service) VerifyRequest(ctx context.Context, req *rulesv1.VerifyRequestRequest) (*rulesv1.VerifyRequestResponse, error) {
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

	err = s.dbRepo.ApproveVerificationRequest(ctx, req.UserId)
	if err != nil {
		return nil, err
	}

	return &rulesv1.VerifyRequestResponse{}, nil
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
		err := s.dbRepo.ApproveVerificationRequest(ctx, userId)
		if err != nil {
			return err
		}
		return ErrAlreadyVerified
	}

	return nil
}

// Approve implements verificationv1.VerificationServiceServer.
func (s *Service) Approve(context.Context, *verificationv1.ApproveRequest) (*verificationv1.ApproveResponse, error) {
	panic("unimplemented")
}

// List implements verificationv1.VerificationServiceServer.
func (s *Service) List(context.Context, *verificationv1.ListRequest) (*verificationv1.ListResponse, error) {
	panic("unimplemented")
}

// Reject implements verificationv1.VerificationServiceServer.
func (s *Service) Reject(context.Context, *verificationv1.RejectRequest) (*verificationv1.RejectResponse, error) {
	panic("unimplemented")
}

// Submit implements verificationv1.VerificationServiceServer.
func (s *Service) Submit(context.Context, *verificationv1.SubmitRequest) (*verificationv1.SubmitResponse, error) {
	panic("unimplemented")
}
