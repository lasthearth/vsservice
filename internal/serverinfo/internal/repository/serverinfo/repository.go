package serverinfo

import (
	"context"

	"github.com/lasthearth/vsservice/internal/serverinfo/internal/dto"
	"github.com/lasthearth/vsservice/internal/serverinfo/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func (r *Repository) GetServerInfo(ctx context.Context) (*model.ServerInfo, error) {
	var si dto.ServerInfo
	err := r.coll.FindOne(ctx, bson.M{}).Decode(&si)
	if err != nil {
		return nil, err
	}
	return &model.ServerInfo{
		WorldTime:   si.WorldTime,
		TotalOnline: si.TotalOnline,
	}, nil
}

func (r *Repository) Update(
	ctx context.Context,
	updateFn func(
		context.Context,
		*model.ServerInfo,
	) (*model.ServerInfo, error),
) error {
	si, err := r.GetServerInfo(ctx)
	if err != nil {
		return err
	}

	newSi, err := updateFn(ctx, si)
	if err != nil {
		return err
	}

	_, err = r.coll.UpdateOne(ctx, bson.M{}, bson.M{"$set": newSi})
	if err != nil {
		return err
	}
	return nil
}
