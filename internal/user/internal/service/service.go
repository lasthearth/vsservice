package service

import (
	"context"

	rulesv1 "github.com/lasthearth/vsservice/gen/rules/v1"
	userv1 "github.com/lasthearth/vsservice/gen/user/v1"
	"github.com/lasthearth/vsservice/internal/rules/model"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/samber/lo"
)

type DbRepository interface {
	VerificationRequest(ctx context.Context, userID, userGameName, contacts string, answers []model.Answer) error
}

type SsoRepository interface {
	UpdateUserProfileNick(ctx context.Context, userID, nickname string) error
}

// Verify implements userv1.UserServiceServer userid in this request not provided, userid only for response
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

	if err := s.dbRepo.VerificationRequest(ctx, userID, req.UserGameName, req.Contacts, answers); err != nil {
		return nil, err
	}

	return &userv1.VerifyResponse{}, nil
}
