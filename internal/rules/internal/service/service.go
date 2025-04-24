package service

import (
	"context"
	"slices"

	rulesv1 "github.com/lasthearth/vsservice/gen/rules/v1"
	httpdto "github.com/lasthearth/vsservice/internal/rules/internal/dto/http"
	"github.com/lasthearth/vsservice/internal/rules/model"
	"github.com/samber/lo"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type DbRepository interface {
	GetRandomQuestions(ctx context.Context, count int) ([]*model.Question, error)
	CreateQuestion(ctx context.Context, question *model.Question) error
	GetVerificationRequests(ctx context.Context) ([]*model.Verification, error)
}

type SsoRepository interface {
	GetUserRoles(ctx context.Context, userId string) ([]httpdto.Role, error)
	GetRoles(ctx context.Context) ([]httpdto.Role, error)
	UpdateUserRoles(ctx context.Context, userId string, roleIds []string) error
}

// CreateQuestion implements rulesv1.RuleServiceServer
func (s *Service) CreateQuestion(ctx context.Context, req *rulesv1.CreateQuestionRequest) (*rulesv1.CreateQuestionResponse, error) {
	if req.Question == "" {
		return nil, ErrQuestionRequired
	}

	err := s.dbRepo.CreateQuestion(ctx, &model.Question{
		Question: req.Question,
	})
	if err != nil {
		return nil, err
	}

	return &rulesv1.CreateQuestionResponse{}, nil
}

// GetRandomQuestions implements rulesv1.RuleServiceServer
func (s *Service) GetRandomQuestions(ctx context.Context, req *rulesv1.GetRandomQuestionsRequest) (*rulesv1.GetRandomQuestionsResponse, error) {
	questions, err := s.dbRepo.GetRandomQuestions(ctx, int(req.Count))
	if err != nil {
		return nil, err
	}

	resp := lo.Map(questions, func(item *model.Question, index int) *rulesv1.GetRandomQuestionsResponse_Question {
		return &rulesv1.GetRandomQuestionsResponse_Question{
			Id:       item.ID,
			Question: item.Question,
		}
	})

	return &rulesv1.GetRandomQuestionsResponse{
		Questions: resp,
	}, nil
}

// ListVerificationRequests implements rulesv1.RuleServiceServer
func (s *Service) ListVerificationRequests(ctx context.Context, req *rulesv1.ListVerificationRequestsRequest) (*rulesv1.ListVerificationRequestsResponse, error) {
	reqs, err := s.dbRepo.GetVerificationRequests(ctx)
	if err != nil {
		return nil, err
	}

	resp := lo.Map(reqs, func(v *model.Verification, index int) *rulesv1.VerifyRequest {
		answers := lo.Map(v.Answers, func(a model.Answer, _ int) *rulesv1.VerifyRequest_Answer {
			return &rulesv1.VerifyRequest_Answer{
				Question: a.Question,
				Answer:   a.Answer,
			}
		})

		return &rulesv1.VerifyRequest{
			UserId:  v.UserID,
			Answers: answers,
		}
	})

	return &rulesv1.ListVerificationRequestsResponse{
		Requests: resp,
	}, nil
}

// VerifyRequest implements rulesv1.RuleServiceServer.
func (s *Service) VerifyRequest(ctx context.Context, req *rulesv1.VerifyRequestRequest) (*rulesv1.VerifyRequestResponse, error) {
	// err := s.checkPermissions(ctx, req.UserId)
	// if err != nil {
	// 	return nil, err
	// }

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
		return ErrAlreadyVerified
	}

	return nil
}

// checkPermissions checks if the user has the required roles to verify a request.
// Returns an error if the user does not have the required roles.
func (s *Service) checkPermissions(ctx context.Context, userId string) error {
	requiredSsoRoles := []string{"admin", "verifier"}

	roles, err := s.ssoRepo.GetUserRoles(ctx, userId)
	if err != nil {
		return err
	}

	for _, role := range roles {
		if slices.Contains(requiredSsoRoles, role.Name) {
			return nil
		}
	}

	return status.Error(codes.PermissionDenied, ErrPermissionDenied.Error())
}
