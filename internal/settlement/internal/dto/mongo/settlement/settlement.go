package settlementdto

import (
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	attachmentdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/attachment"
	memberdto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/member"
	roledto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/role"
	vector2dto "github.com/lasthearth/vsservice/internal/settlement/internal/dto/mongo/vector2"
)

type Settlement struct {
	mongox.Model `bson:",inline"`
	Name         string `bson:"name"`
	Type         string `bson:"type"`
	// Kind is derived from Type on every write: "settlement" or "guild". It is stored because the
	// two unique indexes that keep one membership of each kind per player are partial indexes, and
	// a partial index can only filter on the equality of a stored field. Never set it by hand.
	Kind          string                     `bson:"kind"`
	Leader        memberdto.Member           `bson:"leader"`
	Members       []memberdto.Member         `bson:"members"`
	Coordinates   vector2dto.Vector2         `bson:"coordinates"`
	Attachments   []attachmentdto.Attachment `bson:"attachments"`
	Diplomacy     string                     `bson:"diplomacy"`
	Description   string                     `bson:"description"`
	TagIds        []string                   `bson:"tag_ids"`
	ImperialFavor int64                      `bson:"imperial_favor"`
	Roles         []roledto.Role             `bson:"roles"`
	RolesEnabled  bool                       `bson:"roles_enabled"`
	ContactInfo   string                     `bson:"contact_info"`
	// NotifierReissues counts the notifier blocks an admin has re-sent.
	NotifierReissues int `bson:"notifier_reissues"`
}
