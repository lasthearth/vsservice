package service

import (
	"context"

	rulesv1 "github.com/lasthearth/vsservice/gen/rules/v1"
	userv1 "github.com/lasthearth/vsservice/gen/user/v1"
	"github.com/lasthearth/vsservice/internal/rules/model"
	"github.com/samber/lo"
)

type Repository interface {
	VerificationRequest(ctx context.Context, userID string, answers []model.Answer) error
}

// Verify implements userv1.UserServiceServer
func (s *Service) Verify(ctx context.Context, req *rulesv1.VerifyRequest) (*userv1.VerifyResponse, error) {
	answers := lo.Map(req.Answers, func(v *rulesv1.VerifyRequest_Answer, _ int) model.Answer {
		return model.Answer{
			Question: v.Question,
			Answer:   v.Answer,
		}
	})
	if err := s.repo.VerificationRequest(ctx, req.UserId, answers); err != nil {
		return nil, err
	}
	return &userv1.VerifyResponse{}, nil
}
