package settlementdto

import (
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	attachmentdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/attachment"
	memberdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/member"
	vector2dto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/vector2"
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
	TagIds       []string                   `bson:"tag_ids"`
}
