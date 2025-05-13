package service

import (
	"context"

	rulesv1 "github.com/lasthearth/vsservice/gen/rules/v1"
	"github.com/lasthearth/vsservice/internal/rules/model"
	"github.com/samber/lo"
)

type DbRepository interface {
	GetRandomQuestions(ctx context.Context, count int) ([]*model.Question, error)
	CreateQuestion(ctx context.Context, question *model.Question) error
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
