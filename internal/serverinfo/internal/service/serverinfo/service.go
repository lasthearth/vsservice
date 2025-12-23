package serverinfo

import (
	"context"

	serverinfov1 "github.com/lasthearth/vsservice/gen/serverinfo/v1"
	"github.com/lasthearth/vsservice/internal/serverinfo/internal/model"
)

type ServerInfoRepository interface {
	GetServerInfo(ctx context.Context) (*model.ServerInfo, error)
	Update(
		ctx context.Context,
		updateFn func(
			context.Context,
			*model.ServerInfo,
		) (*model.ServerInfo, error),
	) error
}

// TotalOnline implements serverinfov1.ServerInfoServiceServer.
func (s *Service) TotalOnline(ctx context.Context, req *serverinfov1.TotalOnlineRequest) (*serverinfov1.TotalOnlineResponse, error) {
	info, err := s.repo.GetServerInfo(ctx)
	if err != nil {
		return nil, err
	}
	return &serverinfov1.TotalOnlineResponse{
		Online:    int32(info.TotalOnline),
		MaxOnline: int32(info.MaxOnline),
	}, nil
}

// WorldTime implements serverinfov1.ServerInfoServiceServer.
func (s *Service) WorldTime(ctx context.Context, req *serverinfov1.WorldTimeRequest) (*serverinfov1.WorldTimeResponse, error) {
	info, err := s.repo.GetServerInfo(ctx)
	if err != nil {
		return nil, err
	}
	return &serverinfov1.WorldTimeResponse{
		Time: info.WorldTime,
	}, nil
}
