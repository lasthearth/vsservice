package questiondto

import "github.com/lasthearth/vsservice/internal/pkg/mongox"

type Question struct {
	mongox.Model `bson:",inline"`
	Question     string
}
