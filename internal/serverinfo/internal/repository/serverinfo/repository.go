package serverinfo

import (
	"context"

	"github.com/lasthearth/vsservice/internal/serverinfo/internal/dto"
	"github.com/lasthearth/vsservice/internal/serverinfo/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// GetServerInfo reads the singleton document the game server writes. The
// service is a pure reader: total_online, max_online, world_time and heartbeat_at
// all belong to the game, and the only write this domain makes is the sweeper
// zeroing a total_online whose heartbeat went stale.
func (r *Repository) GetServerInfo(ctx context.Context) (*model.ServerInfo, error) {
	var si dto.ServerInfo
	finded := r.coll.FindOne(ctx, bson.M{})
	err := finded.Err()
	if err != nil {
		return nil, err
	}

	err = finded.Decode(&si)
	if err != nil {
		return nil, err
	}

	return &model.ServerInfo{
		Id:          si.Id.Hex(),
		WorldTime:   si.WorldTime,
		TotalOnline: si.TotalOnline,
		MaxOnline:   si.MaxOnline,
		CreatedAt:   si.CreatedAt,
		UpdatedAt:   si.UpdatedAt,
	}, nil
}
