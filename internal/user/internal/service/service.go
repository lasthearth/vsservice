package service

import (
	"context"

	rulesv1 "github.com/lasthearth/vsservice/gen/rules/v1"
	userv1 "github.com/lasthearth/vsservice/gen/user/v1"
	"github.com/lasthearth/vsservice/internal/rules/model"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type DbRepository interface {
	CreateVerificationRequest(ctx context.Context, opts VerifyOpts) error
	GetVerificationStatus(ctx context.Context, userID string) (model.VerificationStatus, error)
	GetVerificationStatusByUserGameName(ctx context.Context, userGameName string) (model.VerificationStatus, error)
	GetVerificationCode(ctx context.Context, userID string) (string, error)
	VerifyCode(ctx context.Context, userGameName string, code string) error
}

type SsoRepository interface {
	UpdateUserProfileNick(ctx context.Context, userID, nickname string) error
}

// Verify implements userv1.UserServiceServer
func (s *Service) Verify(ctx context.Context, req *rulesv1.VerifyRequest) (*userv1.VerifyResponse, error) {
	answers := lo.Map(req.Answers, func(v *rulesv1.Answer, _ int) model.Answer {
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

	if err := s.dbRepo.CreateVerificationRequest(ctx, VerifyOpts{
		UserID:       userID,
		UserName:     req.UserName,
		UserGameName: req.UserGameName,
		Contacts:     req.Contacts,
		Answers:      answers,
	}); err != nil {
		return nil, err
	}

	return &userv1.VerifyResponse{}, nil
}

// VerifyStatus implements userv1.UserServiceServer
func (s *Service) VerifyStatusByName(ctx context.Context, req *userv1.VerifyStatusByNameRequest) (*userv1.VerifyStatusResponse, error) {
	if req.UserGameName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "user name is required")
	}

	status, err := s.dbRepo.GetVerificationStatusByUserGameName(ctx, req.UserGameName)
	if err != nil {
		return nil, err
	}

	return &userv1.VerifyStatusResponse{
		Status: string(status),
	}, nil
}

// VerifyStatus implements userv1.UserServiceServer
func (s *Service) VerifyStatus(ctx context.Context, req *userv1.VerifyStatusRequest) (*userv1.VerifyStatusResponse, error) {
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	status, err := s.dbRepo.GetVerificationStatus(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &userv1.VerifyStatusResponse{
		Status: string(status),
	}, nil
}

// GetVerifyCode implements userv1.UserServiceServer.
func (s *Service) GetVerifyCode(ctx context.Context, req *userv1.GetVerifyCodeRequest) (*userv1.GetVerifyCodeResponse, error) {
	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	code, err := s.dbRepo.GetVerificationCode(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &userv1.GetVerifyCodeResponse{
		Code: code,
	}, nil
}

// VerifyCode implements userv1.UserServiceServer.
func (s *Service) VerifyCode(ctx context.Context, req *userv1.VerifyCodeRequest) (*userv1.VerifyCodeResponse, error) {
	err := s.dbRepo.VerifyCode(ctx, req.UserGameName, req.Code)
	if err != nil {
		return nil, err
	}

	return &userv1.VerifyCodeResponse{}, nil
}
