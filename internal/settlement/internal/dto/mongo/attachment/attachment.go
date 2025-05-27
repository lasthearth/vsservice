package attachmentdto

import (
	"github.com/lasthearth/vsservice/internal/settlement/model"
)

type Attachment struct {
	Url      string `bson:"url"`
	Desc     string `bson:"desc"`
	MimeType string `bson:"mime_type"`
}

func (a *Attachment) ToModel() *model.Attachment {
	return &model.Attachment{
		Url:      a.Url,
		Desc:     a.Desc,
		MimeType: a.MimeType,
	}
}

func FromModel(m *model.Attachment) *Attachment {
	return &Attachment{
		Url:      m.Url,
		Desc:     m.Desc,
		MimeType: m.MimeType,
	}
}
