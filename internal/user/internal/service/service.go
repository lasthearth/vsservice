package service

import (
	"context"

	userv1 "github.com/lasthearth/vsservice/gen/user/v1"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/lasthearth/vsservice/internal/verification/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type DbRepository interface {
	GetVerificationStatus(ctx context.Context, userID string) (model.VerificationStatus, error)
	GetVerificationStatusByUserGameName(ctx context.Context, userGameName string) (model.VerificationStatus, error)
	GetVerificationCode(ctx context.Context, userID string) (string, error)
	VerifyCode(ctx context.Context, userGameName string, code string) error
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
