package assignment

import (
	"github.com/lasthearth/vsservice/internal/kit/internal/service"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

var _ service.AssignmentRepository = (*Repository)(nil)

const (
	CollectionName = "kit-assignments"
)

// Repository implements the repository interface for assignment operations
type Repository struct {
	coll   *mongo.Collection
	log    logger.Logger
	mapper Mapper
}

// New creates a new instance of Repository
func New(
	coll *mongo.Collection,
	log logger.Logger,
	mapper Mapper,
) *Repository {
	l := log.WithComponent("assignment-repository")
	return &Repository{
		coll:   coll,
		log:    l,
		mapper: mapper,
	}
}
