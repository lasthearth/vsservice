package repository

import (
	"context"

	"github.com/lasthearth/vsservice/internal/imperial-point/internal/dto"
	"github.com/lasthearth/vsservice/internal/imperial-point/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func (r *Repository) CreatePoint(ctx context.Context, point model.ImperialPoint) (*model.ImperialPoint, error) {
	treeOid := bson.NilObjectID
	if point.TreeId != "" {
		var err error
		treeOid, err = mongox.ParseObjectID(point.TreeId)
		if err != nil {
			return nil, err
		}
	}
	d := dto.ImperialPoint{
		Model:         mongox.NewModel(),
		Name:          point.Name,
		Description:   point.Description,
		BiRatePerHour: point.BiRatePerHour,
		TreeId:        treeOid,
	}
	if _, err := r.coll.InsertOne(ctx, d); err != nil {
		return nil, err
	}
	point.SetId(d.Id.Hex())
	return &point, nil
}

func (r *Repository) UpdatePoint(ctx context.Context, point model.ImperialPoint) (*model.ImperialPoint, error) {
	oid, err := mongox.ParseObjectID(point.Id)
	if err != nil {
		return nil, err
	}
	update := bson.M{"$set": bson.M{
		"name":             point.Name,
		"description":      point.Description,
		"bi_rate_per_hour": point.BiRatePerHour,
		"tree_id":          point.TreeId,
	}}
	res, err := r.coll.UpdateByID(ctx, oid, update)
	if err != nil {
		return nil, err
	}
	if res.MatchedCount == 0 {
		return nil, mongo.ErrNoDocuments
	}
	return &point, nil
}

func (r *Repository) GetPoint(ctx context.Context, id string) (*model.ImperialPoint, error) {
	oid, err := mongox.ParseObjectID(id)
	if err != nil {
		return nil, err
	}
	var d dto.ImperialPoint
	if err := r.coll.FindOne(ctx, bson.M{"_id": oid}).Decode(&d); err != nil {
		return nil, err
	}
	return fromDTO(d), nil
}

func (r *Repository) ListPoints(ctx context.Context) ([]model.ImperialPoint, error) {
	cur, err := r.coll.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	var docs []dto.ImperialPoint
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	out := make([]model.ImperialPoint, len(docs))
	for i, d := range docs {
		out[i] = *fromDTO(d)
	}
	return out, nil
}

func (r *Repository) SaveControl(ctx context.Context, pointId string, control *model.PointControl) error {
	oid, err := mongox.ParseObjectID(pointId)
	if err != nil {
		return err
	}
	var update bson.M
	if control == nil {
		update = bson.M{"$unset": bson.M{"control": ""}}
	} else {
		soid, err := mongox.ParseObjectID(control.SettlementId)
		if err != nil {
			return err
		}
		update = bson.M{"$set": bson.M{"control": dto.PointControl{
			Side:            control.Side,
			SettlementId:    soid,
			ControlledSince: control.ControlledSince,
		}}}
	}
	_, err = r.coll.UpdateByID(ctx, oid, update)
	return err
}

func fromDTO(d dto.ImperialPoint) *model.ImperialPoint {
	p := &model.ImperialPoint{
		Id:            d.Id.Hex(),
		Name:          d.Name,
		Description:   d.Description,
		BiRatePerHour: d.BiRatePerHour,
		TreeId:        d.TreeId.Hex(),
	}
	if d.Control != nil {
		p.RestoreControl(&model.PointControl{
			Side:            d.Control.Side,
			SettlementId:    d.Control.SettlementId.Hex(),
			ControlledSince: d.Control.ControlledSince,
		})
	}
	return p
}
