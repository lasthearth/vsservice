package questiondto

import "github.com/lasthearth/vsservice/internal/pkg/mongo"

type Question struct {
	mongo.Model `bson:",inline"`
	Question    string
}
