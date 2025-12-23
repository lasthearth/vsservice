package serverinfo

import (
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"github.com/lasthearth/vsservice/internal/serverinfo/internal/service/serverinfo"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

var _ serverinfo.ServerInfoRepository = (*Repository)(nil)

const (
	CollectionName = "serverinfo"
)

// Repository implements the repository interface for serverinfo operations
type Repository struct {
	coll *mongo.Collection
	log  logger.Logger
}

// New creates a new instance of Repository
func New(
	coll *mongo.Collection,
	log logger.Logger,
) *Repository {
	l := log.WithComponent("serverinfo")
	return &Repository{
		coll: coll,
		log:  l,
	}
}
