package dto

import (
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type News struct {
	Model     mongox.Model `bson:",inline"`
	Title     string       `bson:"title"`
	Preview   string       `bson:"preview"`
	Content   string       `bson:"content,omitempty"`
	CreatedBy string       `bson:"created_by"`
	DeletedAt *time.Time   `bson:"deleted_at,omitempty"`
	DeletedBy *string      `bson:"deleted_by,omitempty"`
	ViewCount int64        `bson:"view_count"`
	ViewerIDs []string     `bson:"viewer_ids,omitempty"`
}

func (n News) Id() bson.ObjectID {
	return n.Model.Id
}
