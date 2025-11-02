package verificationdto

import (
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	attachmentdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/attachment"
	memberdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/member"
	vector2dto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/vector2"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"github.com/samber/lo"
)

type SettlementVerification struct {
	mongox.Model `bson:",inline"`
	Name         string                     `bson:"name"`
	Type         string                     `bson:"type"`
	Leader       memberdto.Member           `bson:"leader"`
	Coordinates  vector2dto.Vector2         `bson:"coordinates"`
	Attachments  []attachmentdto.Attachment `bson:"attachments"`
	Diplomacy    string                     `bson:"diplomacy"`
	Description  string                     `bson:"description"`

	Status          string `bson:"status"`
	RejectionReason string `bson:"rejection_reason"`
}

func (s *SettlementVerification) ToModel() *model.SettlementVerification {
	attachments := lo.Map(s.Attachments, func(attachment attachmentdto.Attachment, _ int) model.Attachment {
		return *attachment.ToModel()
	})

	return &model.SettlementVerification{
		Id:              s.Id.Hex(),
		Name:            s.Name,
		Type:            model.SettlementType(s.Type),
		Leader:          *s.Leader.ToModel(),
		Coordinates:     *s.Coordinates.ToModel(),
		Description:     s.Description,
		Diplomacy:       s.Diplomacy,
		Attachments:     attachments,
		UpdatedAt:       s.UpdatedAt,
		CreatedAt:       s.CreatedAt,
		Status:          model.SettlementStatus(s.Status),
		RejectionReason: s.RejectionReason,
	}
}
