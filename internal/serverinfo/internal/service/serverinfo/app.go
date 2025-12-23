package serverinfo

import (
	serverinfov1 "github.com/lasthearth/vsservice/gen/serverinfo/v1"
	"github.com/lasthearth/vsservice/internal/pkg/logger"
)

var _ = serverinfov1.ServerInfoServiceServer((*Service)(nil))

type Service struct {
	log  logger.Logger
	repo ServerInfoRepository
}

func NewService(log logger.Logger, repo ServerInfoRepository) *Service {
	return &Service{
		log:  log,
		repo: repo,
	}
}
