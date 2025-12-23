package serverinfo

import (
	"context"
	"time"

	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"github.com/lasthearth/vsservice/internal/serverinfo/internal/dto"
	"github.com/lasthearth/vsservice/internal/serverinfo/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
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
	si, err := r.GetServerInfo(ctx)
	if err != nil {
		return err
	}

	newSi, err := updateFn(ctx, si)
	if err != nil {
		return err
	}

	oid, err := mongox.ParseObjectID(si.Id)
	if err != nil {
		return err
	}

	newModel := mongox.NewModel()
	newModel.UpdatedAt = time.Now()
	newModel.CreatedAt = si.CreatedAt
	newModel.Id = oid

	dtoSi := dto.ServerInfo{
		Model:       newModel,
		WorldTime:   newSi.WorldTime,
		TotalOnline: newSi.TotalOnline,
		MaxOnline:   75,
	}

	_, err = r.coll.UpdateOne(ctx, bson.M{}, bson.M{"$set": dtoSi})
	if err != nil {
		return err
	}
	return nil
}
