package serverinfo

import (
	"context"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"github.com/lasthearth/vsservice/internal/serverinfo/internal/dto"
	"github.com/lasthearth/vsservice/internal/serverinfo/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

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

func (r *Repository) Update(
	ctx context.Context,
	updateFn func(
		context.Context,
		*model.ServerInfo,
	) (*model.ServerInfo, error),
) error {
	_, err := mongox.UpdateDoc(
		ctx,
		r.coll,
		bson.M{},
		mongo.ErrNoDocuments,
		func(si dto.ServerInfo) *model.ServerInfo {
			return &model.ServerInfo{
				Id:          si.Id.Hex(),
				WorldTime:   si.WorldTime,
				TotalOnline: si.TotalOnline,
				MaxOnline:   si.MaxOnline,
				CreatedAt:   si.CreatedAt,
				UpdatedAt:   si.UpdatedAt,
			}
		},
		func(si *model.ServerInfo) dto.ServerInfo {
			return dto.ServerInfo{
				WorldTime:   si.WorldTime,
				TotalOnline: si.TotalOnline,
				// MaxOnline is not player-driven; kept pinned as before this
				// method was folded into mongox.UpdateDoc.
				MaxOnline: 75,
			}
		},
		updateFn,
	)
	return err
}
