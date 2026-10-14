//go:generate go tool goverter gen github.com/lasthearth/vsservice/internal/mod/internal/repository
package repository

import (
	"context"
	"errors"

	dto "github.com/lasthearth/vsservice/internal/mod/internal/dto/mongo"
	"github.com/lasthearth/vsservice/internal/mod/internal/ierror"
	"github.com/lasthearth/vsservice/internal/mod/internal/model"
	"github.com/lasthearth/vsservice/internal/pkg/mongox"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.uber.org/zap"
)

// goverter:converter
// goverter:output:file repomapper/mapper.go
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:ObjectIdToString
// goverter:extend github.com/lasthearth/vsservice/internal/pkg/goverter:TimeToTime
// goverter:extend AudienceToString
// goverter:extend AudienceFromString
// goverter:extend SideToString
// goverter:extend SideFromString
type Mapper interface {
	// goverter:ignore Model
	FromModel(model.Mod) dto.Mod

	// goverter:autoMap Model
	ToModel(dto.Mod) model.Mod
}

// listSort is the display order. The id breaks ties, so a half-applied reorder
// cannot make two reads of the same list disagree.
var listSort = bson.D{{Key: "position", Value: 1}, {Key: "_id", Value: 1}}

// List returns every mod in display order, HIDDEN ones included.
func (r *Repository) List(ctx context.Context) ([]model.Mod, error) {
	return r.list(ctx, bson.M{})
}

// ListRequired returns the mods the "download everything" archive packs, in
// display order.
func (r *Repository) ListRequired(ctx context.Context) ([]model.Mod, error) {
	return r.list(ctx, bson.M{"audience": AudienceToString(model.AudienceRequired)})
}

func (r *Repository) list(ctx context.Context, filter bson.M) ([]model.Mod, error) {
	l := r.logger.WithMethod("List")

	cursor, err := r.coll.Find(ctx, filter, options.Find().SetSort(listSort))
	if err != nil {
		l.Error("failed to list mods", zap.Error(err))
		return nil, err
	}

	var docs []dto.Mod
	if err := cursor.All(ctx, &docs); err != nil {
		l.Error("failed to decode mods", zap.Error(err))
		return nil, err
	}

	mods := make([]model.Mod, 0, len(docs))
	for _, doc := range docs {
		mods = append(mods, *r.toModel(doc))
	}

	return mods, nil
}

// Get returns one mod by its ModDB id.
func (r *Repository) Get(ctx context.Context, modId string) (*model.Mod, error) {
	var doc dto.Mod
	if err := r.coll.FindOne(ctx, bson.M{"id": modId}).Decode(&doc); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ierror.ErrNotInList
		}

		r.logger.WithMethod("Get").Error("failed to find mod", zap.String("id", modId), zap.Error(err))
		return nil, err
	}

	return r.toModel(doc), nil
}

// Create stores a new mod at the top of the list.
//
// The shift is a single UpdateMany, so every stored mod moves down exactly once.
// A mod that is already there is refused by the unique index rather than by a
// read-then-write that two admins could both pass.
func (r *Repository) Create(ctx context.Context, mod *model.Mod) error {
	doc := r.mapper.FromModel(*mod)
	doc.Model = mongox.NewModel()
	doc.Position = 0

	if _, err := r.coll.UpdateMany(ctx, bson.M{}, bson.M{"$inc": bson.M{"position": 1}}); err != nil {
		r.logger.WithMethod("Create").Error("failed to shift positions", zap.Error(err))
		return err
	}

	if _, err := r.coll.InsertOne(ctx, doc); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return ierror.ErrAlreadyExists
		}

		r.logger.WithMethod("Create").Error("failed to insert mod", zap.Error(err))
		return err
	}

	return nil
}

// Save loads a mod, lets fn change it through its methods and stores it with a
// version-guarded replace (see mongox.UpdateDoc).
func (r *Repository) Save(
	ctx context.Context,
	modId string,
	fn func(ctx context.Context, mod *model.Mod) (*model.Mod, error),
) (*model.Mod, error) {
	updated, err := mongox.UpdateDoc(
		ctx,
		r.coll,
		bson.M{"id": modId},
		ierror.ErrNotInList,
		r.toModel,
		r.toDTO,
		fn,
	)
	if err != nil && !errors.Is(err, ierror.ErrNotInList) {
		r.logger.WithMethod("Save").Error("failed to update mod", zap.String("id", modId), zap.Error(err))
	}

	return updated, err
}

// Delete removes a mod from the list.
func (r *Repository) Delete(ctx context.Context, modId string) error {
	res, err := r.coll.DeleteOne(ctx, bson.M{"id": modId})
	if err != nil {
		r.logger.WithMethod("Delete").Error("failed to delete mod", zap.String("id", modId), zap.Error(err))
		return err
	}

	if res.DeletedCount == 0 {
		return ierror.ErrNotInList
	}

	return nil
}

// Reorder writes the display order. The caller has already checked that ids is
// a permutation of the stored mods, so this only applies it.
func (r *Repository) Reorder(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	writes := make([]mongo.WriteModel, 0, len(ids))
	for position, id := range ids {
		writes = append(writes, mongo.NewUpdateOneModel().
			SetFilter(bson.M{"id": id}).
			SetUpdate(bson.M{"$set": bson.M{"position": position}}))
	}

	if _, err := r.coll.BulkWrite(ctx, writes); err != nil {
		r.logger.WithMethod("Reorder").Error("failed to reorder mods", zap.Error(err))
		return err
	}

	return nil
}

// toModel maps a document into the domain model. The mapper carries the write
// time out of the Mongo envelope on its own.
func (r *Repository) toModel(doc dto.Mod) *model.Mod {
	mod := r.mapper.ToModel(doc)

	return &mod
}

// toDTO maps a model back to its document.
func (r *Repository) toDTO(mod *model.Mod) dto.Mod {
	return r.mapper.FromModel(*mod)
}
