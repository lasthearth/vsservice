package questiondto

import (
	"github.com/lasthearth/vsservice/internal/pkg/mongo"
	"github.com/lasthearth/vsservice/internal/rules/model"
)

func (m *Question) ToModel() *model.Question {
	return &model.Question{
		ID:        m.Id.Hex(),
		Question:  m.Question,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

func FromModel(question *model.Question) *Question {
	return &Question{
		Model:    mongo.NewModel(),
		Question: question.Question,
	}
}
