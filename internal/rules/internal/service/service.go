package service

import (
	"context"

	rulesv1 "github.com/lasthearth/vsservice/gen/rules/v1"
	"github.com/lasthearth/vsservice/internal/rules/model"
	"github.com/lasthearth/vsservice/internal/server/interceptor"
	"github.com/samber/lo"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type DbRepository interface {
	GetRandomQuestions(ctx context.Context, count int) ([]*model.Question, error)
	CreateQuestion(ctx context.Context, question *model.Question) error
	ListQuestions(ctx context.Context) ([]*model.Question, error)
	DeleteQuestion(ctx context.Context, id string) error
}

// CreateQuestion implements rulesv1.RuleServiceServer
func (s *Service) CreateQuestion(ctx context.Context, req *rulesv1.CreateQuestionRequest) (*rulesv1.CreateQuestionResponse, error) {
	if req.Question == "" {
		return nil, ErrQuestionRequired
	}

	userID, err := interceptor.GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	err = s.dbRepo.CreateQuestion(ctx, &model.Question{
		Question:  req.Question,
		CreatedBy: userID,
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

// ListQuestions implements rulesv1.RuleServiceServer
func (s *Service) ListQuestions(ctx context.Context, _ *rulesv1.ListQuestionsRequest) (*rulesv1.ListQuestionsResponse, error) {
	questions, err := s.dbRepo.ListQuestions(ctx)
	if err != nil {
		return nil, err
	}

	resp := lo.Map(questions, func(item *model.Question, index int) *rulesv1.ListQuestionsResponse_Question {
		return &rulesv1.ListQuestionsResponse_Question{
			Id:        item.ID,
			Question:  item.Question,
			CreatedBy: item.CreatedBy,
			CreatedAt: timestamppb.New(item.CreatedAt),
		}
	})

	return &rulesv1.ListQuestionsResponse{
		Questions: resp,
	}, nil
}

// DeleteQuestion implements rulesv1.RuleServiceServer
func (s *Service) DeleteQuestion(ctx context.Context, req *rulesv1.DeleteQuestionRequest) (*rulesv1.DeleteQuestionResponse, error) {
	err := s.dbRepo.DeleteQuestion(ctx, req.Id)
	if err != nil {
		return nil, err
	}

	return &rulesv1.DeleteQuestionResponse{}, nil
}
