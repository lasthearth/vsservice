package service

import (
	"context"
	"errors"
	"slices"

	verificationv1 "github.com/lasthearth/vsservice/gen/verification/v1"
	httpdto "github.com/lasthearth/vsservice/internal/player/internal/dto/http"
	"github.com/lasthearth/vsservice/internal/player/internal/model"
	"github.com/lasthearth/vsservice/internal/player/internal/model/verification"
	"github.com/lasthearth/vsservice/internal/player/internal/repository/verification/repository/repoerr"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/samber/lo"
)

type DbRepository interface {
	GetVerification(ctx context.Context, userId string) (*verification.Verification, error)
	GetVerificationRequests(ctx context.Context) ([]verification.Verification, error)
	GetVerificationStatusByUserGameName(ctx context.Context, userGameName string) (verification.VerificationStatus, error)
	Create(ctx context.Context, userId string, v verification.Verification) error
	Update(ctx context.Context, userId string, v verification.Verification) error
}

type PlayerRepository interface {
	GetPlayerByUserId(ctx context.Context, userId string) (*model.Player, error)
}

type SsoRepository interface {
	GetUserRoles(ctx context.Context, userId string) ([]httpdto.Role, error)
	GetRoles(ctx context.Context) ([]httpdto.Role, error)
	UpdateUserRoles(ctx context.Context, userId string, roleIds []string) error
	UpdateUserProfileNick(ctx context.Context, userId, nickname string) error
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
			playerRoleId = roles[i].Id
		}
	}

	v, err := s.dbRepo.GetVerification(ctx, req.UserId)
	if err != nil {
		return nil, err
	}

	v.Approve()

	err = s.dbRepo.Update(ctx, req.UserId, *v)
	if err != nil {
		return nil, err
	}

	err = s.ssoRepo.UpdateUserRoles(ctx, req.UserId, []string{playerRoleId})
	if err != nil {
		return nil, err
	}

	return &verificationv1.ApproveResponse{}, nil
}

// Reject implements verificationv1.VerificationServiceServer.
func (s *Service) Reject(ctx context.Context, req *verificationv1.RejectRequest) (*verificationv1.RejectResponse, error) {
	v, err := s.dbRepo.GetVerification(ctx, req.UserId)
	if err != nil {
		return nil, err
	}

	v.Reject(req.RejectionReason.RejectionReason)

	err = s.dbRepo.Update(ctx, req.UserId, *v)
	if err != nil {
		return nil, err
	}

	return &verificationv1.RejectResponse{}, nil
}

// Submit implements verificationv1.VerificationServiceServer.
func (s *Service) Submit(ctx context.Context, req *verificationv1.SubmitRequest) (*verificationv1.SubmitResponse, error) {
	// TODO: переделать на маппер
	answers := lo.Map(req.Answers, func(v *verificationv1.Answer, _ int) verification.Answer {
		return verification.Answer{
			Question: v.Question,
			Answer:   v.Answer,
		}
	})

	userId, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.ssoRepo.UpdateUserProfileNick(ctx, userId, req.UserGameName); err != nil {
		return nil, err
	}

	v := verification.Verification{
		Contacts: req.Contacts,
		Answers:  answers,
	}

	existVerification, err := s.dbRepo.GetVerification(ctx, userId)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			if err := s.dbRepo.Create(ctx, userId, v); err != nil {
				return nil, err
			}
		}

		return nil, err
	}

	err = existVerification.CanSubmit()
	if err != nil {
		return nil, err
	}

	if err := s.dbRepo.Update(ctx, userId, v); err != nil {
		return nil, err
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
		Id:              verif.Id,
		Status:          string(verif.Status),
		RejectionReason: verif.RejectionReason,
	}, nil
}

// List implements verificationv1.VerificationServiceServer.
func (s *Service) List(ctx context.Context, req *verificationv1.ListRequest) (*verificationv1.ListResponse, error) {
	reqs, err := s.dbRepo.GetVerificationRequests(ctx)
	if err != nil {
		return nil, err
	}

	resp := make([]*verificationv1.ListResponse_VerifyUserRequest, len(reqs))
	for i, req := range reqs {
		p, err := s.playerRepo.GetPlayerByUserId(ctx, req.UserId)
		if err != nil {
			return nil, err
		}

		answers := lo.Map(
			req.Answers,
			func(a verification.Answer, _ int) *verificationv1.Answer {
				return &verificationv1.Answer{
					Question: a.Question,
					Answer:   a.Answer,
				}
			},
		)

		resp[i] = &verificationv1.ListResponse_VerifyUserRequest{
			UserId:       p.UserId,
			UserName:     p.UserName,
			UserGameName: p.UserGameName,
			Contacts:     req.Contacts,
			Answers:      answers,
		}
	}

	return &verificationv1.ListResponse{
		Requests: resp,
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
		return verification.ErrAlreadyVerified
	}

	return nil
}

// VerificationStatus implements verificationv1.VerificationServiceServer.
func (s *Service) VerificationStatus(
	ctx context.Context,
	req *verificationv1.VerifyStatusRequest,
) (*verificationv1.VerifyStatusResponse, error) {
	v, err := s.dbRepo.GetVerification(ctx, req.UserId)
	if err != nil {
		return nil, err
	}

	return &verificationv1.VerifyStatusResponse{
		Status: string(v.Status),
	}, nil
}

// VerifyStatusByName implements verificationv1.VerificationServiceServer.
func (s *Service) VerifyStatusByName(ctx context.Context, req *verificationv1.VerifyStatusByNameRequest) (*verificationv1.VerifyStatusResponse, error) {
	status, err := s.dbRepo.GetVerificationStatusByUserGameName(ctx, req.UserGameName)
	if err != nil {
		return nil, err
	}

	return &verificationv1.VerifyStatusResponse{
		Status: string(status),
	}, nil
}
