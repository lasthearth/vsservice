package settlementdto

import (
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	attachmentdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/attachment"
	memberdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/member"
	vector2dto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/vector2"
	"github.com/lasthearth/vsservice/internal/settlement/model"
	"github.com/samber/lo"
)

type Settlement struct {
	mongox.Model `bson:",inline"`
	Name         string                     `bson:"name"`
	Type         string                     `bson:"type"`
	Leader       memberdto.Member           `bson:"leader"`
	Members      []memberdto.Member         `bson:"members"`
	Coordinates  vector2dto.Vector2         `bson:"coordinates"`
	Attachments  []attachmentdto.Attachment `bson:"attachments"`
	Diplomacy    string                     `bson:"diplomacy"`
	Description  string                     `bson:"description"`
}

func (s *Settlement) ToModel() *model.Settlement {
	members := lo.Map(s.Members, func(member memberdto.Member, _ int) model.Member {
		return *member.ToModel()
	})

	attachments := lo.Map(s.Attachments, func(attachment attachmentdto.Attachment, _ int) model.Attachment {
		return *attachment.ToModel()
	})

	return &model.Settlement{
		Id:          s.Id.Hex(),
		Name:        s.Name,
		Type:        model.SettlementType(s.Type),
		Leader:      *s.Leader.ToModel(),
		Members:     members,
		Coordinates: *s.Coordinates.ToModel(),
		Attachments: attachments,
		Diplomacy:   s.Diplomacy,
		Description: s.Description,
		UpdatedAt:   s.UpdatedAt,
		CreatedAt:   s.CreatedAt,
	}
}
